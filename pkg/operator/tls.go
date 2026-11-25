package operator

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const apiServerGetTimeout = 10 * time.Second

const defaultTLSProfileType = configv1.TLSProfileModernType

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
	default:
		return true
	}
}

func resolveTLSProfile(ctx context.Context, reader client.Reader) *tlsProfile {
	profile := &tlsProfile{adherence: configv1.TLSAdherencePolicyStrictAllComponents, watch: true}
	getCtx, cancel := context.WithTimeout(ctx, apiServerGetTimeout)
	defer cancel()
	var apiServer configv1.APIServer
	if err := reader.Get(getCtx, types.NamespacedName{Name: "cluster"}, &apiServer); err != nil {
		ctrl.Log.WithName("tls").Error(nil, "unable to fetch APIServer config, using default TLS profile", "reason", tlsProfileReadErrorReason(err))
		return profile
	}
	profile.adherence = apiServer.Spec.TLSAdherence
	if profile.adherence != configv1.TLSAdherencePolicyNoOpinion && profile.adherence != configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly && profile.adherence != configv1.TLSAdherencePolicyStrictAllComponents {
		ctrl.Log.WithName("tls").Info("unknown TLS adherence policy, honoring cluster profile", "adherence", profile.adherence)
	}
	if honorClusterTLSProfile(profile.adherence) {
		profile.spec = apiServer.Spec.TLSSecurityProfile.DeepCopy()
	}
	return profile
}

func tlsProfileReadErrorReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case apierrors.IsForbidden(err):
		return "forbidden"
	case apierrors.IsUnauthorized(err):
		return "unauthorized"
	case apierrors.IsNotFound(err):
		return "not-found"
	}
	var transportError *url.Error
	if errors.As(err, &transportError) {
		return "transport"
	}
	return "other"
}

func loadTLSProfile(ctx context.Context, config *rest.Config, isOpenShift bool) (*tlsProfile, error) {
	if !isOpenShift {
		return &tlsProfile{}, nil
	}
	boundedConfig := rest.CopyConfig(config)
	boundedConfig.Timeout = apiServerGetTimeout
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{configv1.GroupVersion})
	mapper.Add(configv1.GroupVersion.WithKind("APIServer"), meta.RESTScopeRoot)
	reader, err := client.New(boundedConfig, client.Options{Scheme: GetOperatorScheme(), Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("create TLS config client: %w", err)
	}
	return resolveTLSProfile(ctx, reader), nil
}

func tlsConfigFromProfile(profile *configv1.TLSSecurityProfile) (*tls.Config, error) {
	profileSpec := configv1.TLSProfiles[defaultTLSProfileType]
	if profile != nil {
		if profile.Type == configv1.TLSProfileCustomType && profile.Custom != nil {
			profileSpec = &profile.Custom.TLSProfileSpec
		} else if named, exists := configv1.TLSProfiles[profile.Type]; exists {
			profileSpec = named
		}
	}
	config := &tls.Config{}
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
	} else {
		config.MinVersion = tls.VersionTLS12
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

func shouldRestart(observed *configv1.APIServer, baseline *tlsProfile) bool {
	honor := honorClusterTLSProfile(observed.Spec.TLSAdherence)
	return honor != honorClusterTLSProfile(baseline.adherence) ||
		(honor && !reflect.DeepEqual(observed.Spec.TLSSecurityProfile, baseline.spec))
}

func setupTLSProfileWatch(ctx context.Context, sharedCache cache.Cache, cancel context.CancelFunc, initial *tlsProfile) error {
	if !initial.watch {
		return nil
	}
	informer, err := sharedCache.GetInformer(ctx, &configv1.APIServer{})
	if err != nil {
		return fmt.Errorf("get APIServer informer: %w", err)
	}
	handle := func(object interface{}) {
		apiServer, ok := object.(*configv1.APIServer)
		if !ok || apiServer == nil || apiServer.Name != "cluster" {
			return
		}
		if shouldRestart(apiServer, initial) {
			ctrl.Log.WithName("tls").Info("cluster TLS configuration changed, shutting down to re-apply it",
				"oldAdherence", initial.adherence, "newAdherence", apiServer.Spec.TLSAdherence)
			cancel()
		}
	}
	if _, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: handle,
		UpdateFunc: func(_, newObject interface{}) {
			handle(newObject)
		},
	}); err != nil {
		return fmt.Errorf("register APIServer handler: %w", err)
	}
	return nil
}
