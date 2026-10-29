package operator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestTLSFallbackDiagnostics(test *testing.T) {
	testCases := []struct {
		name   string
		err    error
		reason string
	}{
		{name: "deadline", err: context.DeadlineExceeded, reason: "timeout"},
		{name: "cancelled", err: fmt.Errorf("SYNTHETIC_BODY: %w", context.Canceled), reason: "cancelled"},
		{name: "api timeout", err: apierrors.NewTimeoutError("SYNTHETIC_BODY", 1), reason: "timeout"},
		{name: "server timeout", err: &apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonServerTimeout, Message: "SYNTHETIC_BODY"}}, reason: "timeout"},
		{name: "forbidden", err: &apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonForbidden, Message: "SYNTHETIC_BODY"}}, reason: "forbidden"},
		{name: "unauthorized", err: apierrors.NewUnauthorized("SYNTHETIC_BODY"), reason: "unauthorized"},
		{name: "not found", err: &apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound, Message: "SYNTHETIC_BODY"}}, reason: "not-found"},
		{name: "transport", err: &url.Error{Op: "Get", URL: "https://SYNTHETIC_HOST.invalid/apis?token=SYNTHETIC_QUERY", Err: errors.New("SYNTHETIC_BODY")}, reason: "transport"},
		{name: "wrapped transport timeout", err: &url.Error{Op: "Get", URL: "https://SYNTHETIC_HOST.invalid/apis?token=SYNTHETIC_QUERY", Err: fmt.Errorf("SYNTHETIC_BODY: %w", context.DeadlineExceeded)}, reason: "timeout"},
		{name: "arbitrary API reason", err: &apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReason("SYNTHETIC_REASON"), Message: "https://SYNTHETIC_HOST.invalid/apis?token=SYNTHETIC_QUERY SYNTHETIC_BODY"}}, reason: "other"},
		{name: "other", err: errors.New("SYNTHETIC_BODY"), reason: "other"},
	}
	if os.Getenv("NE2740_LOG_FIXTURE") == "1" {
		ctrl.SetLogger(zap.New(zap.WriteTo(os.Stdout)))
		for _, testCase := range testCases {
			test.Run(testCase.name, func(test *testing.T) {
				if reason := tlsProfileReadErrorReason(testCase.err); reason != testCase.reason {
					test.Fatalf("reason = %q, want %q", reason, testCase.reason)
				}
				profile, err := resolveTLSProfile(context.Background(), stubDiscovery{resources: []metav1.APIResource{{Kind: "APIServer"}}}, errorReader{err: testCase.err})
				if err != nil || profile == nil || !profile.watch || profile.spec != nil || profile.adherence != configv1.TLSAdherencePolicyStrictAllComponents {
					test.Fatal("GET fallback did not preserve default/log/watch state")
				}
			})
		}
		if _, err := tlsConfigFromProfile(customTLSProfile("unsupported-fixture")); err != nil {
			test.Fatal(err)
		}
		honorClusterTLSProfile("future-fixture")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTLSFallbackDiagnostics$")
	command.Env = append(os.Environ(), "NE2740_LOG_FIXTURE=1")
	output, err := command.CombinedOutput()
	if err != nil {
		test.Fatalf("isolated logging fixture failed: %v", err)
	}
	for _, message := range []string{"unable to fetch APIServer config, using default TLS profile", "skipping unsupported TLS group", "unknown TLS adherence policy, honoring cluster profile"} {
		if !strings.Contains(string(output), message) {
			test.Fatalf("missing diagnostic %q", message)
		}
	}
	for _, testCase := range testCases {
		if !regexp.MustCompile(fmt.Sprintf(`"reason":\s*%q`, testCase.reason)).Match(output) {
			test.Errorf("missing bounded reason %q", testCase.reason)
		}
	}
	for _, sensitive := range []string{"https://", "SYNTHETIC_HOST", "token=", "SYNTHETIC_QUERY", "SYNTHETIC_BODY", "SYNTHETIC_REASON"} {
		if strings.Contains(string(output), sensitive) {
			test.Errorf("fallback diagnostics leaked %q", sensitive)
		}
	}
}
