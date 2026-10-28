package operator

import (
	"context"
	"crypto/tls"
	"fmt"
	"reflect"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const apiServerGetTimeout = 10 * time.Second

var tlsGroupToCurveID = map[configv1.TLSGroup]tls.CurveID{
	configv1.TLSGroupX25519:         tls.X25519,
	configv1.TLSGroupSecP256r1:      tls.CurveP256,
	configv1.TLSGroupSecP384r1:      tls.CurveP384,
	configv1.TLSGroupSecP521r1:      tls.CurveP521,
	configv1.TLSGroupX25519MLKEM768: tls.X25519MLKEM768,
}

type tlsProfile struct {
	spec      *configv1.TLSSecurityProfile
	adherence configv1.TLSAdherencePolicy
	watch     bool
}

func honorClusterTLSProfile(adherence configv1.TLSAdherencePolicy) bool {
	switch adherence {
	case configv1.TLSAdherencePolicyNoOpinion, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly:
		return false
	case configv1.TLSAdherencePolicyStrictAllComponents:
		return true
	default:
		ctrl.Log.WithName("tls").Info("unknown TLS adherence policy, honoring cluster profile", "adherence", adherence)
		return true
	}
}

func discoverAPIServer(discoveryClient discovery.ServerResourcesInterface) (bool, error) {
	resources, err := discoveryClient.ServerResourcesForGroupVersion(configv1.GroupVersion.String())
	if err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return false, nil
		}
		return false, fmt.Errorf("discover APIServer config API: %w", err)
	}
	for _, resource := range resources.APIResources {
		if resource.Kind == "APIServer" {
			return true, nil
		}
	}
	return false, nil
}

func resolveTLSProfile(ctx context.Context, discoveryClient discovery.ServerResourcesInterface, reader client.Reader) (*tlsProfile, error) {
	served, err := discoverAPIServer(discoveryClient)
	if err != nil {
		return nil, err
	}
	if !served {
		return &tlsProfile{}, nil
	}
	profile := &tlsProfile{adherence: configv1.TLSAdherencePolicyStrictAllComponents, watch: true}
	getCtx, cancel := context.WithTimeout(ctx, apiServerGetTimeout)
	defer cancel()
	var apiServer configv1.APIServer
	if err := reader.Get(getCtx, types.NamespacedName{Name: "cluster"}, &apiServer); err != nil {
		ctrl.Log.WithName("tls").Error(err, "unable to fetch APIServer config, using default TLS profile")
		return profile, nil
	}
	profile.adherence = apiServer.Spec.TLSAdherence
	if honorClusterTLSProfile(profile.adherence) {
		profile.spec = apiServer.Spec.TLSSecurityProfile.DeepCopy()
	}
	return profile, nil
}

func loadTLSProfile(ctx context.Context, config *rest.Config) (*tlsProfile, error) {
	boundedConfig := rest.CopyConfig(config)
	boundedConfig.Timeout = apiServerGetTimeout
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(boundedConfig)
	if err != nil {
		return nil, fmt.Errorf("create TLS discovery client: %w", err)
	}
	reader, err := client.New(boundedConfig, client.Options{Scheme: GetOperatorScheme()})
	if err != nil {
		return nil, fmt.Errorf("create TLS config client: %w", err)
	}
	return resolveTLSProfile(ctx, discoveryClient, reader)
}

func tlsConfigFromProfile(profile *configv1.TLSSecurityProfile) (*tls.Config, error) {
	profileSpec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	if profile != nil {
		if profile.Type == configv1.TLSProfileCustomType && profile.Custom != nil {
			profileSpec = &profile.Custom.TLSProfileSpec
		} else if named, exists := configv1.TLSProfiles[profile.Type]; exists {
			profileSpec = named
		}
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	for _, name := range libgocrypto.OpenSSLToIANACipherSuites(profileSpec.Ciphers) {
		suite, err := libgocrypto.CipherSuite(name)
		if err != nil {
			ctrl.Log.WithName("tls").Info("skipping unsupported TLS cipher", "cipher", name, "error", err)
			continue
		}
		config.CipherSuites = append(config.CipherSuites, suite)
	}
	if profileSpec.MinTLSVersion != "" {
		version, err := libgocrypto.TLSVersion(string(profileSpec.MinTLSVersion))
		if err != nil {
			return nil, fmt.Errorf("invalid TLS version %q: %w", profileSpec.MinTLSVersion, err)
		}
		config.MinVersion = version
	}
	for _, group := range profileSpec.Groups {
		if curve, exists := tlsGroupToCurveID[group]; exists {
			config.CurvePreferences = append(config.CurvePreferences, curve)
		} else {
			ctrl.Log.WithName("tls").Info("skipping unsupported TLS group", "group", group)
		}
	}
	return libgocrypto.SecureTLSConfig(config), nil
}

func applyTLSPolicy(policy *tls.Config) func(*tls.Config) {
	return func(config *tls.Config) {
		config.MinVersion = policy.MinVersion
		config.CipherSuites = append([]uint16(nil), policy.CipherSuites...)
		config.CurvePreferences = append([]tls.CurveID(nil), policy.CurvePreferences...)
	}
}

type tlsProfileWatcher struct {
	reader  client.Reader
	cancel  context.CancelFunc
	initial *tlsProfile
}

func (watcher *tlsProfileWatcher) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	if request.Name != "cluster" {
		return ctrl.Result{}, nil
	}
	var apiServer configv1.APIServer
	if err := watcher.reader.Get(ctx, types.NamespacedName{Name: "cluster"}, &apiServer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	honor := honorClusterTLSProfile(apiServer.Spec.TLSAdherence)
	if honor != honorClusterTLSProfile(watcher.initial.adherence) ||
		(honor && !reflect.DeepEqual(apiServer.Spec.TLSSecurityProfile, watcher.initial.spec)) {
		ctrl.Log.WithName("tls").Info("cluster TLS configuration changed, shutting down to re-apply it",
			"oldAdherence", watcher.initial.adherence, "newAdherence", apiServer.Spec.TLSAdherence)
		watcher.cancel()
	}
	return ctrl.Result{}, nil
}

func setupTLSProfileWatch(manager ctrl.Manager, cancel context.CancelFunc, initial *tlsProfile) error {
	if !initial.watch {
		return nil
	}
	return ctrl.NewControllerManagedBy(manager).
		Named("tlsprofilewatcher").
		WithOptions(controller.Options{NeedLeaderElection: ptr.To(false)}).
		For(&configv1.APIServer{}).
		Complete(&tlsProfileWatcher{reader: manager.GetClient(), cancel: cancel, initial: initial})
}
