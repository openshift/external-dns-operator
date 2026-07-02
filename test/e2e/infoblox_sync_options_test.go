//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openshift/external-dns-operator/test/common"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	testInfobloxSyncExtDNSName = "test-extdns-sync-options"
	testInfobloxMaxResults     = 2000
)

func TestExternalDNSInfobloxSyncOptions(t *testing.T) {
	if helper.platform() != infobloxDNSProvider {
		t.Skip("test only runs against Infoblox provider")
	}

	infobloxHelper, ok := helper.(*infobloxTestHelper)
	if !ok {
		t.Fatalf("expected infoblox test helper, got %T", helper)
	}

	syncInterval := metav1.Duration{Duration: 2 * time.Minute}

	t.Log("Creating credentials secret")
	credSecret := infobloxHelper.makeCredentialsSecret(common.OperatorNamespace)
	if err := common.KubeClient.Create(context.TODO(), credSecret); err != nil {
		t.Fatalf("Failed to create credentials secret %s/%s: %v", credSecret.Namespace, credSecret.Name, err)
	}

	t.Log("Creating external dns instance with interval and maxResults")
	extDNS := infobloxHelper.buildOpenShiftExternalDNSWithSyncOptions(
		testInfobloxSyncExtDNSName,
		hostedZoneID,
		hostedZoneDomain,
		"",
		credSecret,
		syncInterval,
		testInfobloxMaxResults,
	)
	if err := common.KubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testInfobloxSyncExtDNSName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), &extDNS)
	}()

	deploymentName := fmt.Sprintf("external-dns-%s", testInfobloxSyncExtDNSName)
	deploymentKey := types.NamespacedName{
		Namespace: "external-dns",
		Name:      deploymentName,
	}

	t.Logf("Waiting for operand deployment %s", deploymentKey)
	if err := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := common.KubeClient.Get(ctx, deploymentKey, deployment); err != nil {
			t.Logf("operand deployment not ready yet: %v", err)
			return false, nil
		}
		if deployment.Status.AvailableReplicas < 1 {
			t.Log("operand deployment is not available yet")
			return false, nil
		}

		intervalArg := fmt.Sprintf("--interval=%s", syncInterval.Duration.String())
		maxResultsArg := fmt.Sprintf("--infoblox-max-results=%d", testInfobloxMaxResults)
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if !strings.HasPrefix(container.Name, "external-dns") {
				continue
			}
			if !containsArg(container.Args, intervalArg) {
				t.Logf("container %s is missing %q in args: %v", container.Name, intervalArg, container.Args)
				return false, nil
			}
			if !containsArg(container.Args, maxResultsArg) {
				t.Logf("container %s is missing %q in args: %v", container.Name, maxResultsArg, container.Args)
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		t.Fatalf("operand deployment did not get expected sync options: %v", err)
	}
}

func containsArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}
