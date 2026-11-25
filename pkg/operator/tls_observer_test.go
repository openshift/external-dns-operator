package operator

import (
	"context"
	"errors"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type tlsInformerCache struct {
	cache.Cache
	informer  cache.Informer
	err       error
	requested bool
}

func (shared *tlsInformerCache) GetInformer(_ context.Context, object client.Object, _ ...cache.InformerGetOption) (cache.Informer, error) {
	_, shared.requested = object.(*configv1.APIServer)
	return shared.informer, shared.err
}

type tlsEventInformer struct {
	cache.Informer
	handler toolscache.ResourceEventHandler
	err     error
}

func (informer *tlsEventInformer) AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	if informer.err != nil {
		return nil, informer.err
	}
	informer.handler = handler
	return nil, nil
}

func TestTLSObserverRegistrationErrors(test *testing.T) {
	failure := errors.New("registration fixture")
	for _, testCase := range []struct {
		name     string
		getError error
		addError error
		message  string
	}{
		{name: "get informer", getError: failure, message: "get APIServer informer:"},
		{name: "add handler", addError: failure, message: "register APIServer handler:"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			informer := &tlsEventInformer{err: testCase.addError}
			shared := &tlsInformerCache{informer: informer, err: testCase.getError}
			err := setupTLSProfileWatch(context.Background(), shared, func() { test.Error("registration failure cancelled manager") }, &tlsProfile{watch: true})
			if !errors.Is(err, failure) || !strings.HasPrefix(err.Error(), testCase.message) || !shared.requested || informer.handler != nil {
				test.Fatalf("registration error=%v requested=%v handler=%v", err, shared.requested, informer.handler)
			}
		})
	}
	shared := &tlsInformerCache{}
	if err := setupTLSProfileWatch(context.Background(), shared, nil, &tlsProfile{}); err != nil || shared.requested {
		test.Fatalf("disabled observer registered informer: %v", err)
	}
}

func TestTLSObserverEventsUseStartupBaseline(test *testing.T) {
	baseline := &tlsProfile{spec: customTLSProfile(configv1.TLSGroupSecP256r1), adherence: configv1.TLSAdherencePolicyStrictAllComponents, watch: true}
	unchanged := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	unchanged.Spec.TLSAdherence = baseline.adherence
	unchanged.Spec.TLSSecurityProfile = baseline.spec.DeepCopy()
	changed := unchanged.DeepCopy()
	changed.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP384r1)
	other := changed.DeepCopy()
	other.Name = "other"
	for _, testCase := range []struct {
		name  string
		event func(toolscache.ResourceEventHandler)
		want  int
	}{
		{name: "initial Add closes read watch gap", event: func(handler toolscache.ResourceEventHandler) { handler.OnAdd(changed, true) }, want: 1},
		{name: "coalesced update", event: func(handler toolscache.ResourceEventHandler) { handler.OnUpdate(changed, changed) }, want: 1},
		{name: "repeated resync", event: func(handler toolscache.ResourceEventHandler) {
			handler.OnUpdate(changed, changed)
			handler.OnUpdate(changed, changed)
		}, want: 2},
		{name: "baseline resync", event: func(handler toolscache.ResourceEventHandler) { handler.OnUpdate(unchanged, unchanged) }},
		{name: "return to baseline", event: func(handler toolscache.ResourceEventHandler) { handler.OnUpdate(changed, unchanged) }},
		{name: "wrong Add type", event: func(handler toolscache.ResourceEventHandler) { handler.OnAdd(&metav1.Status{}, true) }},
		{name: "wrong Update type", event: func(handler toolscache.ResourceEventHandler) { handler.OnUpdate(changed, &metav1.Status{}) }},
		{name: "typed nil", event: func(handler toolscache.ResourceEventHandler) { handler.OnAdd((*configv1.APIServer)(nil), true) }},
		{name: "other Add", event: func(handler toolscache.ResourceEventHandler) { handler.OnAdd(other, true) }},
		{name: "other Update", event: func(handler toolscache.ResourceEventHandler) { handler.OnUpdate(unchanged, other) }},
		{name: "delete waits for later Add", event: func(handler toolscache.ResourceEventHandler) { handler.OnDelete(changed) }},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			informer := &tlsEventInformer{}
			shared := &tlsInformerCache{informer: informer}
			cancellations := 0
			if err := setupTLSProfileWatch(context.Background(), shared, func() { cancellations++ }, baseline); err != nil {
				test.Fatal(err)
			}
			testCase.event(informer.handler)
			if cancellations != testCase.want {
				test.Fatalf("cancellations=%d want=%d", cancellations, testCase.want)
			}
			if baseline.spec.Custom.Groups[0] != configv1.TLSGroupSecP256r1 {
				test.Fatal("observer mutated startup baseline")
			}
		})
	}
}
