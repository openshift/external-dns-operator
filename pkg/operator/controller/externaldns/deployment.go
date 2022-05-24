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
	"reflect"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1 "github.com/openshift/api/config/v1"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
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
	trustedCAAnnotation                 = "externaldns.olm.openshift.io/trusted-ca-configmap-hash"
)

// providerStringTable maps ExternalDNSProviderType values from the
// ExternalDNS operator API to the provider string argument expected by ExternalDNS.
var providerStringTable = map[operatorv1beta1.ExternalDNSProviderType]string{
	operatorv1beta1.ProviderTypeAWS:      externalDNSProviderTypeAWS,
	operatorv1beta1.ProviderTypeGCP:      externalDNSProviderTypeGCP,
	operatorv1beta1.ProviderTypeAzure:    externalDNSProviderTypeAzure,
	operatorv1beta1.ProviderTypeBlueCat:  externalDNSProviderTypeBlueCat,
	operatorv1beta1.ProviderTypeInfoblox: externalDNSProviderTypeInfoblox,
}

// sourceStringTable maps ExternalDNSSourceType values from the
// ExternalDNS operator API to the source string argument expected by ExternalDNS.
var sourceStringTable = map[operatorv1beta1.ExternalDNSSourceType]string{
	operatorv1beta1.SourceTypeRoute:   "openshift-route",
	operatorv1beta1.SourceTypeService: "service",
}

type deploymentConfig struct {
	namespace              string
	image                  string
	serviceAccount         *corev1.ServiceAccount
	externalDNS            *operatorv1beta1.ExternalDNS
	isOpenShift            bool
	platformStatus         *configv1.PlatformStatus
	secret                 string
	secretHash             string
	trustedCAConfigMapName string
	trustedCAConfigMapHash string
}

// ensureExternalDNSDeployment ensures that the externalDNS deployment exists.
// Returns a Boolean value indicating whether the deployment exists, a pointer to the deployment, and an error when relevant.
func (r *reconciler) ensureExternalDNSDeployment(ctx context.Context, namespace, image string, serviceAccount *corev1.ServiceAccount, credSecret *corev1.Secret, trustCAConfigMap *corev1.ConfigMap, externalDNS *operatorv1beta1.ExternalDNS) (bool, *appsv1.Deployment, error) {
	nsName := types.NamespacedName{Namespace: namespace, Name: controller.ExternalDNSResourceName(externalDNS)}

	// build credentials secret's hash
	credSecretHash, err := buildMapHash(credSecret.Data)
	if err != nil {
		return false, nil, fmt.Errorf("failed to build the credentials secret's hash: %w", err)
	}

	// build trusted CA configmap's hash
	trustCAConfigMapName, trustCAConfigMapHash := "", ""
	if trustCAConfigMap != nil {
		trustCAConfigMapName = trustCAConfigMap.Name
		trustCAConfigMapHash, err = buildStringMapHash(trustCAConfigMap.Data)
		if err != nil {
			return false, nil, fmt.Errorf("failed to build the CA configmap's hash: %w", err)
		}
	}

	desired, err := desiredExternalDNSDeployment(&deploymentConfig{
		namespace,
		image,
		serviceAccount,
		externalDNS,
		r.config.IsOpenShift,
		r.config.PlatformStatus,
		credSecret.Name,
		credSecretHash,
		trustCAConfigMapName,
		trustCAConfigMapHash,
	})
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

// currentExternalDNSTrustedCAConfigMap gets the trusted CA configmap resource.
func (r *reconciler) currentExternalDNSTrustedCAConfigMap(ctx context.Context, nsName types.NamespacedName) (bool, *corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	if err := r.client.Get(ctx, nsName, cm); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, cm, nil
}

// desiredExternalDNSDeployment returns the desired deployment resource.
func desiredExternalDNSDeployment(cfg *deploymentConfig) (*appsv1.Deployment, error) {
	replicas := int32(1)

	matchLbl := map[string]string{
		appNameLabel:     controller.ExternalDNSBaseName,
		appInstanceLabel: cfg.externalDNS.Name,
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

	annotations := map[string]string{
		credentialsAnnotation: cfg.secretHash,
	}

	if cfg.trustedCAConfigMapHash != "" {
		annotations[trustedCAAnnotation] = cfg.trustedCAConfigMapHash
	}

	depl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.ExternalDNSResourceName(cfg.externalDNS),
			Namespace: cfg.namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLbl,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      matchLbl,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: cfg.serviceAccount.Name,
					NodeSelector:       nodeSelectorLbl,
					Tolerations:        tolerations,
				},
			},
		},
	}

	provider, ok := providerStringTable[cfg.externalDNS.Spec.Provider.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %q", cfg.externalDNS.Spec.Provider.Type)
	}
	source, ok := sourceStringTable[cfg.externalDNS.Spec.Source.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported source type: %q", cfg.externalDNS.Spec.Source.Type)
	}

	vbld := newExternalDNSVolumeBuilder(provider, cfg.secret, cfg.trustedCAConfigMapName)
	volumes := vbld.build()
	depl.Spec.Template.Spec.Volumes = append(depl.Spec.Template.Spec.Volumes, volumes...)

	cbld := &externalDNSContainerBuilder{
		image:          cfg.image,
		provider:       provider,
		source:         source,
		secretName:     cfg.secret,
		volumes:        volumes,
		externalDNS:    cfg.externalDNS,
		isOpenShift:    cfg.isOpenShift,
		platformStatus: cfg.platformStatus,
	}

	if len(cfg.externalDNS.Spec.Zones) == 0 {
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
		for _, zone := range cfg.externalDNS.Spec.Zones {
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
	annotationChanged := externalDNSAnnotationsChanged(current, expected, updated)
	containersChanged := externalDNSContainersChanged(current, expected, updated)
	volumesChanged := externalDNSVolumesChanged(current, expected, updated)
	return annotationChanged || containersChanged || volumesChanged, updated
}

// externalDNSAnnotationsChanged returns true if any annotation from the podspec differs from the expected.```
func externalDNSAnnotationsChanged(current, expected, updated *appsv1.Deployment) bool {
	changed := false
	if len(current.Spec.Template.Annotations) == 0 {
		if len(expected.Spec.Template.Annotations) == 0 {
			return false
		}
		updated.Spec.Template.Annotations = expected.Spec.Template.Annotations
		return true
	}
	for expectedKey, expectedValue := range expected.Spec.Template.Annotations {
		currentVal, currentExists := current.Spec.Template.Annotations[expectedKey]
		if !currentExists || currentVal != expectedValue {
			updated.Spec.Template.Annotations[expectedKey] = expectedValue
			changed = true
		}
	}
	return changed
}

// externalDNSContainersChanged returns true if the current containers differ from the expected.
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
			if !equalEnvVars(currCont.Env, expCont.Env) {
				updated.Spec.Template.Spec.Containers[currCont.Index].Env = expCont.Env
				changed = true
			}
			if vmChanged, updatedVolumeMounts := volumeMountsChanged(currCont.VolumeMounts, expCont.VolumeMounts, updated.Spec.Template.Spec.Containers[currCont.Index].VolumeMounts); vmChanged {
				updated.Spec.Template.Spec.Containers[currCont.Index].VolumeMounts = updatedVolumeMounts
				changed = true
			}
			if scChanged, updatedContext := securityContextChanged(currCont.SecurityContext, updated.Spec.Template.Spec.Containers[currCont.Index].SecurityContext, expCont.SecurityContext); scChanged {
				updated.Spec.Template.Spec.Containers[currCont.Index].SecurityContext = updatedContext
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

// externalDNSVolumesChanged returns true if the current volumes differ from the expected.
func externalDNSVolumesChanged(current, expected, updated *appsv1.Deployment) bool {
	if len(current.Spec.Template.Spec.Volumes) == 0 {
		if len(expected.Spec.Template.Spec.Volumes) == 0 {
			return false
		}
		updated.Spec.Template.Spec.Volumes = expected.Spec.Template.Spec.Volumes
		return true
	}

	changed := false

	currentVolumeMap := buildIndexedVolumeMap(current.Spec.Template.Spec.Volumes)
	expectedVolumeMap := buildIndexedVolumeMap(expected.Spec.Template.Spec.Volumes)

	// ensure all expected volumes are present,
	// unsolicited ones are kept (e.g. kube api token)
	for expName, expVol := range expectedVolumeMap {
		if currVol, currExists := currentVolumeMap[expName]; !currExists {
			updated.Spec.Template.Spec.Volumes = append(updated.Spec.Template.Spec.Volumes, expVol.Volume)
			changed = true
		} else {
			// deepequal is fine here as we don't have more than 1 item
			// neither in the secret nor in the configmap
			if !reflect.DeepEqual(currVol.Volume, expVol.Volume) {
				updated.Spec.Template.Spec.Volumes[currVol.Index] = expVol.Volume
				changed = true
			}
		}
	}

	return changed
}

// volumeMountsChanged checks that the current volume mounts have all expected ones,
// returns true if the current volume mounts had to be changed to match the expected.
func volumeMountsChanged(current, expected, updated []corev1.VolumeMount) (bool, []corev1.VolumeMount) {
	if len(current) == 0 {
		if len(expected) == 0 {
			return false, updated
		}
		return true, expected
	}

	changed := false

	currentVolumeMountMap := buildIndexedVolumeMountMap(current)
	expectedVolumeMountMap := buildIndexedVolumeMountMap(expected)

	// ensure all expected volume mounts are present,
	// unsolicited ones are kept (e.g. kube api token)
	for expName, expVol := range expectedVolumeMountMap {
		if currVol, currExists := currentVolumeMountMap[expName]; !currExists {
			updated = append(updated, expVol.VolumeMount)
			changed = true
		} else {
			if !reflect.DeepEqual(currVol.VolumeMount, expVol.VolumeMount) {
				updated[currVol.Index] = expVol.VolumeMount
				changed = true
			}
		}
	}

	return changed, updated
}

// equalEnvVars returns true if 2 env variable slices have the same content (order doesn't matter).
func equalEnvVars(current, expected []corev1.EnvVar) bool {
	var currentSorted, expectedSorted []string
	for _, env := range current {
		currentSorted = append(currentSorted, env.Name+" "+env.Value)
	}
	for _, env := range expected {
		expectedSorted = append(expectedSorted, env.Name+" "+env.Value)
	}
	sort.Strings(currentSorted)
	sort.Strings(expectedSorted)
	return cmp.Equal(currentSorted, expectedSorted)
}

// indexedContainer is the standard core POD's container with additional index field
type indexedContainer struct {
	corev1.Container
	Index int
}

// buildIndexedContainerMap builds a map from the given list of containers,
// key is the container name,
// value is the indexed container with the index being the sequence number of the given list.
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

// indexedVolume is the standard core POD's volume with additional index field
type indexedVolume struct {
	corev1.Volume
	Index int
}

// buildIndexedVolumeMap builds a map from the given list of volumes,
// key is the volume name,
// value is the indexed volume with the index being the sequence number of the given list.
func buildIndexedVolumeMap(volumes []corev1.Volume) map[string]indexedVolume {
	m := map[string]indexedVolume{}
	for i, vol := range volumes {
		m[vol.Name] = indexedVolume{
			Volume: vol,
			Index:  i,
		}
	}
	return m
}

// indexedVolumeMount is the standard core POD's voume mount with additional index field
type indexedVolumeMount struct {
	corev1.VolumeMount
	Index int
}

// buildIndexedVolumeMountMap builds a map from the given list of volume mounts,
// key is the volume name,
// value is the indexed volume mount with the index being the sequence number of the given list.
func buildIndexedVolumeMountMap(volumeMounts []corev1.VolumeMount) map[string]indexedVolumeMount {
	m := map[string]indexedVolumeMount{}
	for i, vol := range volumeMounts {
		m[vol.Name] = indexedVolumeMount{
			VolumeMount: vol,
			Index:       i,
		}
	}
	return m
}

// equalStringSliceContent returns true if 2 string slices have the same content (order doesn't matter).
func equalStringSliceContent(sl1, sl2 []string) bool {
	copy1 := append([]string{}, sl1...)
	copy2 := append([]string{}, sl2...)
	sort.Strings(copy1)
	sort.Strings(copy2)
	return cmp.Equal(copy1, copy2)
}

// buildMapHash is a utility function to get a checksum of a data map.
func buildMapHash(data map[string][]byte) (string, error) {
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

func buildStringMapHash(data map[string]string) (string, error) {
	m := map[string][]byte{}
	for k, v := range data {
		m[k] = []byte(v)
	}
	return buildMapHash(m)
}

func securityContextChanged(current, updated, desired *corev1.SecurityContext) (bool, *corev1.SecurityContext) {
	changed := false

	if desired == nil {
		return false, nil
	}

	if updated == nil {
		return true, desired
	}

	if current == nil {
		return true, desired
	}

	if desired.Capabilities != nil {
		cmpCapabilities := cmpopts.SortSlices(func(a, b corev1.Capability) bool { return a < b })
		if current.Capabilities == nil {
			updated.Capabilities = desired.Capabilities
			changed = true
		} else {
			if !cmp.Equal(desired.Capabilities.Add, current.Capabilities.Add, cmpCapabilities) {
				updated.Capabilities.Add = desired.Capabilities.Add
				changed = true
			}
			if !cmp.Equal(desired.Capabilities.Drop, current.Capabilities.Drop, cmpCapabilities) {
				updated.Capabilities.Drop = desired.Capabilities.Drop
				changed = true
			}
		}
	}

	if !equalBoolPtr(current.RunAsNonRoot, desired.RunAsNonRoot) {
		updated.RunAsNonRoot = desired.RunAsNonRoot
		changed = true
	}

	if !equalBoolPtr(current.Privileged, desired.Privileged) {
		updated.Privileged = desired.Privileged
		changed = true
	}
	if !equalBoolPtr(current.AllowPrivilegeEscalation, desired.AllowPrivilegeEscalation) {
		updated.AllowPrivilegeEscalation = desired.AllowPrivilegeEscalation
		changed = true
	}

	if desired.SeccompProfile != nil {
		if current.SeccompProfile == nil {
			updated.SeccompProfile = desired.SeccompProfile
			changed = true
		} else if desired.SeccompProfile.Type != "" && desired.SeccompProfile.Type != current.SeccompProfile.Type {
			updated.SeccompProfile = desired.SeccompProfile
			changed = true
		}
	}

	return changed, updated
}

func equalBoolPtr(current, desired *bool) bool {
	if desired == nil {
		return true
	}

	if current == nil {
		return false
	}

	if *current != *desired {
		return false
	}
	return true
}
