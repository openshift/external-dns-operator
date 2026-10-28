package operator

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestTLSSessionAcrossServerReplacement(test *testing.T) {
	test.Parallel()
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		test.Run(fmt.Sprintf("TLS%d", version), func(test *testing.T) {
			test.Parallel()
			profile := customTLSProfile(configv1.TLSGroupSecP256r1)
			if version == tls.VersionTLS13 {
				profile.Custom.MinTLSVersion = configv1.VersionTLS13
			}
			first := startPolicyServer(test, profile)
			clientConfig := trustedClientConfig(first)
			clientConfig.MinVersion = version
			clientConfig.MaxVersion = version
			clientConfig.CurvePreferences = []tls.CurveID{tls.CurveP256}
			clientConfig.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
			sessions := &trackingSessionCache{ClientSessionCache: tls.NewLRUClientSessionCache(2)}
			clientConfig.ClientSessionCache = sessions
			firstState := handshakePolicyServer(test, first, clientConfig, true)
			secondState := handshakePolicyServer(test, first, clientConfig, true)
			if firstState.DidResume || !secondState.DidResume {
				test.Fatal("session-resumption control failed")
			}
			oldSession := sessions.saved
			oldKey := sessions.key
			first.Close()
			profile.Custom.Groups = []configv1.TLSGroup{configv1.TLSGroupSecP384r1}
			replacement := startPolicyServer(test, profile)
			sessions.Put(oldKey, oldSession)
			sessions.supplied = false
			clientConfig.CurvePreferences = []tls.CurveID{tls.CurveP384}
			if state := handshakePolicyServer(test, replacement, clientConfig, true); state.DidResume || !sessions.supplied {
				test.Fatalf("replacement resumed=%t old ticket supplied=%t", state.DidResume, sessions.supplied)
			}
			clientConfig.ClientSessionCache = nil
			clientConfig.CurvePreferences = []tls.CurveID{tls.CurveP256}
			handshakePolicyServer(test, replacement, clientConfig, false)
		})
	}
}

type trackingSessionCache struct {
	tls.ClientSessionCache
	saved    *tls.ClientSessionState
	key      string
	supplied bool
}

func (cache *trackingSessionCache) Get(key string) (*tls.ClientSessionState, bool) {
	state, found := cache.ClientSessionCache.Get(key)
	cache.supplied = found && state != nil
	return state, found
}

func (cache *trackingSessionCache) Put(key string, state *tls.ClientSessionState) {
	if state != nil {
		cache.key, cache.saved = key, state
	}
	cache.ClientSessionCache.Put(key, state)
}

func TestHybridOnlyTLSHandshake(test *testing.T) {
	test.Parallel()
	profile := customTLSProfile(configv1.TLSGroupX25519MLKEM768)
	profile.Custom.MinTLSVersion = configv1.VersionTLS13
	server := startPolicyServer(test, profile)
	config := trustedClientConfig(server)
	config.MinVersion = tls.VersionTLS13
	config.CurvePreferences = []tls.CurveID{tls.CurveP256}
	handshakePolicyServer(test, server, config, false)
	config.CurvePreferences = []tls.CurveID{tls.X25519MLKEM768}
	handshakePolicyServer(test, server, config, true)
}

func startPolicyServer(test *testing.T, profile *configv1.TLSSecurityProfile) *httptest.Server {
	test.Helper()
	policy, err := tlsConfigFromProfile(profile)
	if err != nil {
		test.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{NextProtos: []string{"http/1.1"}}
	applyTLSPolicy(policy)(server.TLS)
	server.StartTLS()
	test.Cleanup(server.Close)
	return server
}

func trustedClientConfig(server *httptest.Server) *tls.Config {
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	return &tls.Config{RootCAs: roots, ServerName: "example.com", NextProtos: []string{"h2", "http/1.1"}}
}

func handshakePolicyServer(test *testing.T, server *httptest.Server, config *tls.Config, succeeds bool) tls.ConnectionState {
	test.Helper()
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", server.Listener.Addr().String(), config)
	if err != nil {
		if succeeds {
			test.Fatal(err)
		}
		return tls.ConnectionState{}
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		test.Fatal(err)
	}
	if !succeeds {
		test.Fatal("prohibited handshake succeeded")
	}
	if _, err := io.WriteString(connection, "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"); err != nil {
		test.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, connection); err != nil {
		test.Fatal(err)
	}
	return connection.ConnectionState()
}

type policyAPI struct {
	lock            sync.Mutex
	object          configv1.APIServer
	watchers        []chan configv1.APIServer
	bookmarked      chan bool
	rejectWatchList bool
	watchLists      atomic.Int32
	initialEnds     atomic.Int32
	initialStarted  chan struct{}
	releaseInitial  chan struct{}
	disconnect      chan struct{}
}

func (api *policyAPI) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(response)
	if request.URL.Query().Get("sendInitialEvents") == "true" {
		api.watchLists.Add(1)
		if api.rejectWatchList {
			response.WriteHeader(http.StatusBadRequest)
			if err := encoder.Encode(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonBadRequest, Code: http.StatusBadRequest, Message: "watch list unavailable"}); err != nil {
				return
			}
			return
		}
	}
	if request.URL.Query().Get("watch") == "true" {
		updates := make(chan configv1.APIServer, 4)
		api.lock.Lock()
		api.watchers = append(api.watchers, updates)
		current := *api.object.DeepCopy()
		api.lock.Unlock()
		bookmarks := request.URL.Query().Get("allowWatchBookmarks") == "true"
		if request.URL.Query().Get("sendInitialEvents") == "true" {
			if api.initialStarted != nil {
				api.initialStarted <- struct{}{}
			}
			if err := encoder.Encode(map[string]any{"type": "ADDED", "object": current}); err != nil {
				return
			}
			if api.releaseInitial != nil {
				response.(http.Flusher).Flush()
				select {
				case <-api.releaseInitial:
				case <-api.disconnect:
					return
				case <-request.Context().Done():
					return
				}
			}
			if bookmarks {
				if err := encoder.Encode(map[string]any{"type": "BOOKMARK", "object": map[string]any{
					"apiVersion": configv1.GroupVersion.String(), "kind": "APIServer", "metadata": map[string]any{
						"resourceVersion": current.ResourceVersion, "annotations": map[string]string{"k8s.io/initial-events-end": "true"},
					},
				}}); err != nil {
					return
				}
				api.initialEnds.Add(1)
			}
		}
		response.(http.Flusher).Flush()
		api.bookmarked <- bookmarks
		for {
			select {
			case object := <-updates:
				if err := encoder.Encode(map[string]any{"type": "MODIFIED", "object": object}); err != nil {
					return
				}
				response.(http.Flusher).Flush()
			case <-request.Context().Done():
				return
			}
		}
	}
	api.lock.Lock()
	defer api.lock.Unlock()
	var result any = api.object
	if !strings.HasSuffix(request.URL.Path, "/cluster") {
		result = &configv1.APIServerList{TypeMeta: metav1.TypeMeta{APIVersion: configv1.GroupVersion.String(), Kind: "APIServerList"}, ListMeta: metav1.ListMeta{ResourceVersion: api.object.ResourceVersion}, Items: []configv1.APIServer{api.object}}
	}
	if err := encoder.Encode(result); err != nil {
		return
	}
}

func TestTLSObserverStopsEveryManagerWithoutLeadership(test *testing.T) {
	testTLSObserverManagers(test, false, false, false)
}

func TestTLSObserverInitialEventsRecoverFallback(test *testing.T) {
	testTLSObserverManagers(test, true, false, false)
}

func TestTLSObserverListWatchFallback(test *testing.T) {
	testTLSObserverManagers(test, false, true, false)
}

func TestTLSObserverBookmarkStallAndReconnect(test *testing.T) {
	testTLSObserverManagers(test, false, false, true)
}

func testTLSObserverManagers(test *testing.T, initialReadFailed, rejectWatchList, delayBookmark bool) {
	test.Helper()
	api := &policyAPI{rejectWatchList: rejectWatchList, bookmarked: make(chan bool, 8), object: configv1.APIServer{
		TypeMeta:   metav1.TypeMeta{APIVersion: configv1.GroupVersion.String(), Kind: "APIServer"},
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", ResourceVersion: "1"},
	}}
	api.object.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
	api.object.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP256r1)
	if delayBookmark {
		api.initialStarted = make(chan struct{}, 8)
		api.releaseInitial = make(chan struct{})
		api.disconnect = make(chan struct{}, 1)
	}
	server := httptest.NewServer(api)
	test.Cleanup(server.Close)
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{configv1.GroupVersion})
	mapper.Add(configv1.GroupVersion.WithKind("APIServer"), meta.RESTScopeRoot)
	finished := make(chan error, 2)
	leaderLock := &heldLeaderLock{}
	var managers []ctrl.Manager
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		test.Cleanup(cancel)
		manager, err := ctrl.NewManager(&rest.Config{Host: server.URL}, ctrl.Options{
			Scheme: GetOperatorScheme(), Metrics: metrics.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
			Controller:     config.Controller{SkipNameValidation: ptr.To(true)},
			LeaderElection: true, LeaderElectionResourceLockInterface: leaderLock,
			MapperProvider: func(*rest.Config, *http.Client) (meta.RESTMapper, error) { return mapper, nil },
			Cache:          cache.Options{DefaultEnableWatchBookmarks: ptr.To(true)},
			NewClient: func(config *rest.Config, options client.Options) (client.Client, error) {
				options.Cache = nil
				return client.New(config, options)
			},
		})
		if err != nil {
			test.Fatal(err)
		}
		managers = append(managers, manager)
		initial := &tlsProfile{adherence: api.object.Spec.TLSAdherence, spec: api.object.Spec.TLSSecurityProfile.DeepCopy(), watch: true}
		if initialReadFailed {
			initial.spec = nil
		}
		if err := setupTLSProfileWatch(manager, cancel, initial); err != nil {
			test.Fatal(err)
		}
		go func() { finished <- (&Operator{manager: manager}).Start(ctx) }()
	}
	if delayBookmark {
		for range 2 {
			select {
			case <-api.initialStarted:
			case <-time.After(5 * time.Second):
				test.Fatal("initial WatchList did not start")
			}
		}
		syncCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if managers[0].GetCache().WaitForCacheSync(syncCtx) || api.initialEnds.Load() != 0 {
			test.Fatal("cache synced without initial-events-end")
		}
		api.disconnect <- struct{}{}
		select {
		case <-api.initialStarted:
		case <-time.After(5 * time.Second):
			test.Fatal("disconnected WatchList did not reconnect")
		}
		close(api.releaseInitial)
	}
	for range 2 {
		select {
		case bookmarks := <-api.bookmarked:
			if !bookmarks {
				test.Fatal("cache did not enable watch bookmarks")
			}
		case <-time.After(10 * time.Second):
			test.Fatal("cache did not start")
		}
	}
	if !initialReadFailed {
		select {
		case err := <-finished:
			test.Fatalf("manager stopped before policy change: %v", err)
		default:
		}
		api.lock.Lock()
		api.object.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP384r1)
		api.object.ResourceVersion = "2"
		for _, watcher := range api.watchers {
			watcher <- *api.object.DeepCopy()
		}
		api.lock.Unlock()
	}
	for replica := range 2 {
		select {
		case err := <-finished:
			if err != nil {
				test.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			test.Fatalf("replica %d did not exit after group-only change", replica)
		}
	}
	if leaderLock.acquired.Load() {
		test.Fatal("observer test unexpectedly acquired leadership")
	}
	if api.watchLists.Load() < 2 || (!rejectWatchList && api.initialEnds.Load() < 2) {
		test.Fatalf("WatchList/bookmark path not exercised: requests=%d ends=%d", api.watchLists.Load(), api.initialEnds.Load())
	}
}

type heldLeaderLock struct {
	resourcelock.Interface
	acquired atomic.Bool
}

func (lock *heldLeaderLock) Get(context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	record := &resourcelock.LeaderElectionRecord{HolderIdentity: "another-replica", LeaseDurationSeconds: 3600, RenewTime: metav1.Now()}
	data, err := json.Marshal(record)
	return record, data, err
}

func (lock *heldLeaderLock) Identity() string   { return "observer-replica" }
func (lock *heldLeaderLock) Describe() string   { return "isolated-held-lease" }
func (lock *heldLeaderLock) RecordEvent(string) {}

func (lock *heldLeaderLock) Update(context.Context, resourcelock.LeaderElectionRecord) error {
	lock.acquired.Store(true)
	return fmt.Errorf("unexpected attempt to acquire held lease")
}
