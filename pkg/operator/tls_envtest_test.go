package operator

import (
	"context"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestTLSObserverEnvtest(test *testing.T) {
	environment := &envtest.Environment{CRDs: []*apiextensionsv1.CustomResourceDefinition{{
		ObjectMeta: metav1.ObjectMeta{Name: "apiservers.config.openshift.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: configv1.GroupName, Scope: apiextensionsv1.ClusterScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "apiservers", Singular: "apiserver", Kind: "APIServer", ListKind: "APIServerList"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1", Served: true, Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: ptr.To(true)}}}},
		},
	}}}
	config, err := environment.Start()
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			test.Error(err)
		}
	})
	apiClient, err := client.New(config, client.Options{Scheme: GetOperatorScheme()})
	if err != nil {
		test.Fatal(err)
	}
	apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
	apiServer.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP256r1)
	if err := apiClient.Create(context.Background(), apiServer); err != nil {
		test.Fatal(err)
	}
	for _, initialGap := range []bool{true, false} {
		name := "Update"
		if initialGap {
			name = "initial Add"
		}
		test.Run(name, func(test *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			initial, err := loadTLSProfile(ctx, config, true)
			if err != nil {
				test.Fatal(err)
			}
			if initialGap {
				initial.spec = nil
			}
			shared, err := cache.New(config, cache.Options{Scheme: GetOperatorScheme(), DefaultEnableWatchBookmarks: ptr.To(true)})
			if err != nil {
				test.Fatal(err)
			}
			if err := setupTLSProfileWatch(ctx, shared, cancel, initial); err != nil {
				test.Fatal(err)
			}
			if ctx.Err() != nil {
				test.Fatal("registration before cache Start blocked or cancelled")
			}
			finished := make(chan error, 1)
			go func() { finished <- shared.Start(ctx) }()
			if !initialGap {
				if !shared.WaitForCacheSync(ctx) {
					test.Fatal("unchanged initial Add did not sync")
				}
				apiServer.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP384r1)
				if err := apiClient.Update(ctx, apiServer); err != nil {
					test.Fatal(err)
				}
			}
			if err := <-finished; err != nil || ctx.Err() != context.Canceled {
				test.Fatalf("cache shutdown=%v context=%v", err, ctx.Err())
			}
		})
	}
}
