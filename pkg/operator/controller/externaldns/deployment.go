/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package externaldnscontroller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"fmt"
	"sort"

	"github.com/google/go-cmp/cmp"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1 "github.com/openshift/api/config/v1"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

const (
	externalDNSProviderTypeAWS          = "aws"
	externalDNSProviderTypeGCP          = "google"
	externalDNSProviderTypeAzure        = "azure"
	externalDNSProviderTypeAzurePrivate = "azure-private-dns"
	externalDNSProviderTypeBlueCat      = "bluecat"
	externalDNSProviderTypeInfoblox     = "infoblox"
	appNameLabel                        = "app.kubernetes.io/name"
	appInstanceLabel                    = "app.kubernetes.io/instance"
	masterNodeRoleLabel                 = "node-role.kubernetes.io/master"
	osLabel                             = "kubernetes.io/os"
	linuxOS                             = "linux"
	azurePrivateDNSZonesResourceSubStr  = "privatednszones"
	credentialsAnnotation               = "externaldns.olm.openshift.io/credentials-secret-hash"
)

// providerStringTable maps ExternalDNSProviderType values from the
// ExternalDNS operator API to the provider string argument expected by ExternalDNS.
var providerStringTable = map[operatorv1alpha1.ExternalDNSProviderType]string{
	operatorv1alpha1.ProviderTypeAWS:      externalDNSProviderTypeAWS,
	operatorv1alpha1.ProviderTypeGCP:      externalDNSProviderTypeGCP,
	operatorv1alpha1.ProviderTypeAzure:    externalDNSProviderTypeAzure,
	operatorv1alpha1.ProviderTypeBlueCat:  externalDNSProviderTypeBlueCat,
	operatorv1alpha1.ProviderTypeInfoblox: externalDNSProviderTypeInfoblox,
}

// sourceStringTable maps ExternalDNSSourceType values from the
// ExternalDNS operator API to the source string argument expected by ExternalDNS.
var sourceStringTable = map[operatorv1alpha1.ExternalDNSSourceType]string{
	operatorv1alpha1.SourceTypeRoute:   "openshift-route",
	operatorv1alpha1.SourceTypeService: "service",
}

// ensureExternalDNSDeployment ensures that the externalDNS deployment exists.
// Returns a Boolean value indicating whether the deployment exists, a pointer to the deployment, and an error when relevant.
func (r *reconciler) ensureExternalDNSDeployment(ctx context.Context, namespace, image string, serviceAccount *corev1.ServiceAccount, externalDNS *operatorv1alpha1.ExternalDNS) (bool, *appsv1.Deployment, error) {
	nsName := types.NamespacedName{Namespace: namespace, Name: controller.ExternalDNSResourceName(externalDNS)}
	configMapName := ""
	if r.config.InjectTrustedCA {
		configMapName = controller.ExternalDNSDestTrustedCAConfigMapName("").Name
	}

	secretExists, secret, err := r.currentExternalDNSSecret(ctx, controller.ExternalDNSDestCredentialsSecretName(namespace, externalDNS.Name))
	if err != nil {
		return false, nil, fmt.Errorf("failed to get the target secret: %w", err)
	}
	if !secretExists {
		return false, nil, fmt.Errorf("target secret not found: %w", err)
	}

	secretHash, err := buildSecretHash(secret.Data)
	if err != nil {
		return false, nil, fmt.Errorf("failed to build the secret's hash: %w", err)
	}

	desired, err := desiredExternalDNSDeployment(namespace, image, secret.Name, secretHash, serviceAccount, externalDNS, r.config.IsOpenShift, r.config.PlatformStatus, configMapName)
	if err != nil {
		return false, nil, fmt.Errorf("failed to build externalDNS deployment: %w", err)
	}

	if err := controllerutil.SetControllerReference(externalDNS, desired, r.scheme); err != nil {
		return false, nil, fmt.Errorf("failed to set the controller reference for deployment: %w", err)
	}

	exist, current, err := r.currentExternalDNSDeployment(ctx, nsName)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get externalDNS deployment: %w", err)
	}

	// create the deployment
	if !exist {
		if err := r.createExternalDNSDeployment(ctx, desired); err != nil {
			return false, nil, err
		}
		// get the deployment from API to catch up the fields added/updated by API and webhooks
		return r.currentExternalDNSDeployment(ctx, nsName)
	}

	// update the deployment
	if updated, err := r.updateExternalDNSDeployment(ctx, current, desired); err != nil {
		return true, current, err
	} else if updated {
		// get the deployment from API to catch up the fields added/updated by API and webhooks
		return r.currentExternalDNSDeployment(ctx, nsName)
	}

	return true, current, nil
}

// currentExternalDNSSecret gets the current externalDNS secret resource.
func (r *reconciler) currentExternalDNSSecret(ctx context.Context, nsName types.NamespacedName) (bool, *corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := r.client.Get(ctx, nsName, secret); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, secret, nil
}

// currentExternalDNSDeployment gets the current externalDNS deployment resource.
func (r *reconciler) currentExternalDNSDeployment(ctx context.Context, nsName types.NamespacedName) (bool, *appsv1.Deployment, error) {
	depl := &appsv1.Deployment{}
	if err := r.client.Get(ctx, nsName, depl); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, depl, nil
}

// desiredExternalDNSDeployment returns the desired deployment resource.
func desiredExternalDNSDeployment(namespace, image, secretName string, secretHash string,
	serviceAccount *corev1.ServiceAccount,
	externalDNS *operatorv1alpha1.ExternalDNS,
	isOpenShift bool,
	platformStatus *configv1.PlatformStatus,
	trustedCAConfigMapName string) (*appsv1.Deployment, error) {

	replicas := int32(1)

	matchLbl := map[string]string{
		appNameLabel:     controller.ExternalDNSBaseName,
		appInstanceLabel: externalDNS.Name,
	}

	nodeSelectorLbl := map[string]string{
		osLabel:             linuxOS,
		masterNodeRoleLabel: "",
	}

	tolerations := []corev1.Toleration{
		{
			Key:      masterNodeRoleLabel,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}

	depl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.ExternalDNSResourceName(externalDNS),
			Namespace: namespace,
			Annotations: map[string]string{
				credentialsAnnotation: secretHash,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLbl,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: matchLbl,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccount.Name,
					NodeSelector:       nodeSelectorLbl,
					Tolerations:        tolerations,
				},
			},
		},
	}

	provider, ok := providerStringTable[externalDNS.Spec.Provider.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %q", externalDNS.Spec.Provider.Type)
	}
	source, ok := sourceStringTable[externalDNS.Spec.Source.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported source type: %q", externalDNS.Spec.Source.Type)
	}

	vbld := newExternalDNSVolumeBuilder(provider, secretName, trustedCAConfigMapName)
	volumes := vbld.build()
	depl.Spec.Template.Spec.Volumes = append(depl.Spec.Template.Spec.Volumes, volumes...)

	cbld := &externalDNSContainerBuilder{
		image:          image,
		provider:       provider,
		source:         source,
		secretName:     secretName,
		volumes:        volumes,
		externalDNS:    externalDNS,
		isOpenShift:    isOpenShift,
		platformStatus: platformStatus,
	}

	if len(externalDNS.Spec.Zones) == 0 {
		// an empty list means publish to all zones
		// this is a special case for Azure
		// both public and private zones will need to be published to
		providerList := []string{provider}
		if cbld.provider == externalDNSProviderTypeAzure {
			providerList = append(providerList, externalDNSProviderTypeAzurePrivate)
		}
		for _, p := range providerList {
			cbld.provider = p
			container, err := cbld.build("")
			if err != nil {
				return nil, fmt.Errorf("failed to build container: %w", err)
			}
			depl.Spec.Template.Spec.Containers = append(depl.Spec.Template.Spec.Containers, *container)
		}
	} else {
		for _, zone := range externalDNS.Spec.Zones {
			container, err := cbld.build(zone)
			if err != nil {
				return nil, fmt.Errorf("failed to build container for zone %s: %w", zone, err)
			}
			depl.Spec.Template.Spec.Containers = append(depl.Spec.Template.Spec.Containers, *container)
		}
	}
	return depl, nil
}

// createExternalDNSDeployment creates the given deployment using the reconciler's client.
func (r *reconciler) createExternalDNSDeployment(ctx context.Context, depl *appsv1.Deployment) error {
	if err := r.client.Create(ctx, depl); err != nil {
		return fmt.Errorf("failed to create externalDNS deployment %s/%s: %w", depl.Namespace, depl.Name, err)
	}
	r.log.Info("created externalDNS deployment", "namespace", depl.Namespace, "name", depl.Name)
	return nil
}

// updateExternalDNSDeployment updates the in-cluster externalDNS deployment.
// Returns a boolean if an update was made, and an error when relevant.
func (r *reconciler) updateExternalDNSDeployment(ctx context.Context, current, desired *appsv1.Deployment) (bool, error) {
	// don't always update (or do simple DeepEqual with) the operand's deployment
	// as this may result into a "fight" between API/admission webhooks and this controller
	// example:
	//  - this controller creates a deployment with the desired fields A, B, C
	//  - API adds some default fields D, E, F (e.g. metadata, imagePullPullPolicy, dnsPolicy)
	//  - mutating webhooks add default values to fields E, F
	//  - this controller gets into reconcile loop and starts all from step 1
	//  - checking that fields A, B, C are the same as desired would save us the before mentioned round trips:
	changed, updated := externalDNSDeploymentChanged(current, desired)
	if !changed {
		return false, nil
	}

	if err := r.client.Update(ctx, updated); err != nil {
		return false, fmt.Errorf("failed to update externalDNS deployment %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	r.log.Info("updated externalDNS deployment", "namespace", desired.Namespace, "name", desired.Name)
	return true, nil
}

// externalDNSDeploymentChanged evaluates whether or not a deployment update is necessary.
// Returns a boolean if an update is necessary, and the deployment resource to update to.
func externalDNSDeploymentChanged(current, expected *appsv1.Deployment) (bool, *appsv1.Deployment) {
	updated := current.DeepCopy()
	return externalDNSAnnotationsChanged(current, expected, updated) || externalDNSContainersChanged(current, expected, updated), updated
}

// externalDNSAnnotationsChanged returns true if the current secret annotation differ from the expected
func externalDNSAnnotationsChanged(current, expected, updated *appsv1.Deployment) bool {
	changed := false
	if current.Annotations == nil {
		updated.Annotations = expected.Annotations
		return true
	}
	for expectedKey, expectedValue := range expected.Annotations {
		currentVal, currentExists := current.Annotations[expectedKey]
		if !currentExists || currentVal != expectedValue {
			updated.Annotations[expectedKey] = expectedValue
			changed = true
		}
	}
	return changed
}

// externalDNSContainersChanged returns true if the current containers differ from the expected
func externalDNSContainersChanged(current, expected, updated *appsv1.Deployment) bool {
	changed := false

	// number of container is different: let's reset them all
	if len(current.Spec.Template.Spec.Containers) != len(expected.Spec.Template.Spec.Containers) {
		updated.Spec.Template.Spec.Containers = expected.Spec.Template.Spec.Containers
		return true
	}

	currentContMap := buildIndexedContainerMap(current.Spec.Template.Spec.Containers)
	expectedContMap := buildIndexedContainerMap(expected.Spec.Template.Spec.Containers)

	// let's check that all the current containers have the desired values set
	for currName, currCont := range currentContMap {
		// if the current container is expected: check its fields
		if expCont, found := expectedContMap[currName]; found {
			if currCont.Image != expCont.Image {
				updated.Spec.Template.Spec.Containers[currCont.Index].Image = expCont.Image
				changed = true
			}
			if !equalStringSliceContent(expCont.Args, currCont.Args) {
				updated.Spec.Template.Spec.Containers[currCont.Index].Args = expCont.Args
				changed = true
			}
		} else {
			// if the current container is not expected: let's not dig deeper - reset all
			updated.Spec.Template.Spec.Containers = expected.Spec.Template.Spec.Containers
			return true
		}
	}

	return changed
}

// indexedContainer is the standard core POD's container with additional index field
type indexedContainer struct {
	corev1.Container
	Index int
}

// buildIndexedContainerMap builds a map from the given list of containers
// key is the container name
// value is the indexed container with index being the sequence number of the given list
func buildIndexedContainerMap(containers []corev1.Container) map[string]indexedContainer {
	m := map[string]indexedContainer{}
	for i, cont := range containers {
		m[cont.Name] = indexedContainer{
			Container: cont,
			Index:     i,
		}
	}
	return m
}

// equalStringSliceContent returns true if 2 string slices have the same content (order doesn't matter)
func equalStringSliceContent(sl1, sl2 []string) bool {
	copy1 := append([]string{}, sl1...)
	copy2 := append([]string{}, sl2...)
	sort.Strings(copy1)
	sort.Strings(copy2)
	return cmp.Equal(copy1, copy2)
}

// buildSecretHash is a utility function to get a checksum of the resource data
func buildSecretHash(data map[string][]byte) (string, error) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, k := range keys {
		_, err := hash.Write([]byte(k))
		if err != nil {
			return "", err
		}
		_, err = hash.Write(data[k])
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
