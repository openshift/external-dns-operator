package operator

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestTLSFallbackDiagnostics(test *testing.T) {
	if os.Getenv("NE2740_LOG_FIXTURE") == "1" {
		ctrl.SetLogger(zap.New(zap.WriteTo(os.Stdout)))
		profile, err := resolveTLSProfile(context.Background(), stubDiscovery{resources: []metav1.APIResource{{Kind: "APIServer"}}}, errorReader{err: context.DeadlineExceeded})
		if err != nil || !profile.watch {
			test.Fatal("GET fallback failed")
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
}
