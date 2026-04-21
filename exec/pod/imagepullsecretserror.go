/*
 * Copyright 2025 The ChaosBlade Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package pod

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	// ChaosBladeIPSBackupAnnotation marks a Secret as a backup created by this action
	ChaosBladeIPSBackupAnnotation = "chaosblade.io/ips-backup"
	// ChaosBladeIPSOriginalNameAnnotation stores the original Secret name
	ChaosBladeIPSOriginalNameAnnotation = "chaosblade.io/ips-original-name"
	// ChaosBladeIPSOriginalNamespaceAnnotation stores the original Secret namespace
	ChaosBladeIPSOriginalNamespaceAnnotation = "chaosblade.io/ips-original-namespace"
	// ChaosBladeIPSExperimentLabel is the label key for experiment ID on backup Secrets
	ChaosBladeIPSExperimentLabel = "chaosblade.io/experiment"
)

type ImagePullSecretsErrorActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewImagePullSecretsErrorActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &ImagePullSecretsErrorActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: "secret-name",
					Desc: "The name of the imagePullSecret to corrupt. If not specified, all imagePullSecrets of the target Pod will be corrupted",
				},
			},
			ActionExecutor: &ImagePullSecretsErrorActionExecutor{client: client},
			ActionExample: `# Simulate image pull authentication failure for a specific pod
blade create k8s pod-pod imagepullsecretserror --names my-app-pod --namespace default --kubeconfig ~/.kube/config

# Simulate image pull authentication failure for pods selected by labels
blade create k8s pod-pod imagepullsecretserror --labels app=nginx --namespace default --kubeconfig ~/.kube/config

# Corrupt only a specific imagePullSecret
blade create k8s pod-pod imagepullsecretserror --names my-app-pod --namespace default --secret-name my-registry-secret --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*ImagePullSecretsErrorActionSpec) Name() string {
	return "imagepullsecretserror"
}

func (*ImagePullSecretsErrorActionSpec) Aliases() []string {
	return []string{}
}

func (*ImagePullSecretsErrorActionSpec) ShortDesc() string {
	return "Simulate image pull authentication failure by corrupting imagePullSecrets"
}

func (*ImagePullSecretsErrorActionSpec) LongDesc() string {
	return "Simulate the scenario where a Pod fails to pull images from a private registry due to " +
		"authentication failure. This fault is injected by corrupting the credentials in the Secret " +
		"referenced by the Pod's imagePullSecrets field. The original Secret data is backed up to a " +
		"separate Secret for recovery. After corruption, the Pod is deleted so the controller recreates " +
		"it, and the new Pod will fail to pull images with ErrImagePull/ImagePullBackOff status. " +
		"When the experiment is destroyed, the original Secret is restored and the Pod is deleted again " +
		"to trigger a successful image pull."
}

type ImagePullSecretsErrorActionExecutor struct {
	client *channel.Client
}

func (*ImagePullSecretsErrorActionExecutor) Name() string {
	return "imagepullsecretserror"
}

func (*ImagePullSecretsErrorActionExecutor) SetChannel(channel spec.Channel) {}

func (d *ImagePullSecretsErrorActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *ImagePullSecretsErrorActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	secretNameFilter := expModel.ActionFlags["secret-name"]

	// Track processed Secrets to avoid corrupting the same Secret multiple times
	// when multiple Pods reference the same Secret
	processedSecrets := make(map[string]bool)
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false

	for _, meta := range containerObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: meta.GetIdentifier(),
		}

		// Get the Pod
		pod := &v1.Pod{}
		err := d.client.Get(ctx, types.NamespacedName{Name: meta.PodName, Namespace: meta.Namespace}, pod)
		if err != nil {
			logrusField.Warningf("get pod %s/%s failed: %v", meta.Namespace, meta.PodName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("get pod failed: %v", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		// Check imagePullSecrets
		if len(pod.Spec.ImagePullSecrets) == 0 {
			logrusField.Warningf("pod %s/%s has no imagePullSecrets", meta.Namespace, meta.PodName)
			status = status.CreateFailResourceStatus("pod has no imagePullSecrets", spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		// Filter by --secret-name if specified
		targetSecretRefs := pod.Spec.ImagePullSecrets
		if secretNameFilter != "" {
			targetSecretRefs = filterSecretRefs(targetSecretRefs, secretNameFilter)
			if len(targetSecretRefs) == 0 {
				logrusField.Warningf("pod %s/%s does not have imagePullSecret %s", meta.Namespace, meta.PodName, secretNameFilter)
				status = status.CreateFailResourceStatus(
					fmt.Sprintf("pod does not have imagePullSecret %s", secretNameFilter), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				continue
			}
		}

		// Process each target Secret for this Pod.
		// All Secrets must be corrupted successfully before deleting the Pod.
		// If any corruption fails, roll back the ones that succeeded for this Pod
		// to avoid partial corruption leading to unpredictable behavior.
		corruptedInThisRound := make([]string, 0, len(targetSecretRefs))
		allSecretsOk := true
		for _, secretRef := range targetSecretRefs {
			secretKey := fmt.Sprintf("%s/%s", meta.Namespace, secretRef.Name)
			if processedSecrets[secretKey] {
				// Already corrupted by a previous Pod in this experiment
				continue
			}

			if err := d.corruptSecret(ctx, logrusField, experimentId, meta.Namespace, secretRef.Name); err != nil {
				logrusField.Warningf("corrupt secret %s failed: %v", secretKey, err)
				allSecretsOk = false
				break
			}
			corruptedInThisRound = append(corruptedInThisRound, secretKey)
		}

		if !allSecretsOk {
			// Roll back Secrets corrupted in this round to avoid partial corruption.
			// Secrets corrupted by previous Pods (already in processedSecrets) are not
			// rolled back here because those Pods have already been deleted successfully.
			for _, secretKey := range corruptedInThisRound {
				if err := d.rollbackSecret(ctx, logrusField, experimentId, secretKey); err != nil {
					logrusField.Warningf("rollback secret %s failed: %v", secretKey, err)
				}
			}
			status = status.CreateFailResourceStatus("failed to corrupt all imagePullSecrets, rolled back", spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		// Mark all newly corrupted Secrets as processed
		for _, secretKey := range corruptedInThisRound {
			processedSecrets[secretKey] = true
		}

		// Delete the Pod to trigger recreation with corrupted credentials
		if err := d.client.Delete(ctx, pod); err != nil {
			if !apierrors.IsNotFound(err) {
				logrusField.Warningf("delete pod %s/%s failed: %v, rolling back corrupted secrets", meta.Namespace, meta.PodName, err)
				// Roll back Secrets corrupted in this round since Pod won't be recreated
				for _, secretKey := range corruptedInThisRound {
					if rbErr := d.rollbackSecret(ctx, logrusField, experimentId, secretKey); rbErr != nil {
						logrusField.Warningf("rollback secret %s after pod delete failure: %v", secretKey, rbErr)
					}
					delete(processedSecrets, secretKey)
				}
				status = status.CreateFailResourceStatus(fmt.Sprintf("delete pod failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				continue
			}
		}

		logrusField.Infof("corrupted imagePullSecrets and deleted pod %s/%s", meta.Namespace, meta.PodName)
		status = status.CreateSuccessResourceStatus()
		statuses = append(statuses, status)
		success = true
	}

	var experimentStatus v1alpha1.ExperimentStatus
	if success {
		experimentStatus = v1alpha1.CreateSuccessExperimentStatus(statuses)
	} else {
		experimentStatus = v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses)
	}
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

func (d *ImagePullSecretsErrorActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	// Collect all unique namespaces involved
	namespaces := make(map[string]bool)
	for _, meta := range containerObjectMetaList {
		namespaces[meta.Namespace] = true
	}

	// Find and restore all backup Secrets for this experiment
	allSuccess := true
	for ns := range namespaces {
		if err := d.restoreSecretsInNamespace(ctx, logrusField, experimentId, ns); err != nil {
			logrusField.Warningf("restore secrets in namespace %s failed: %v", ns, err)
			allSuccess = false
		}
	}

	// Delete Pods to trigger recreation with restored credentials
	statuses := make([]v1alpha1.ResourceStatus, 0)
	for _, meta := range containerObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: meta.GetIdentifier(),
		}

		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meta.PodName,
				Namespace: meta.Namespace,
			},
		}
		if err := d.client.Delete(ctx, pod); err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("pod %s/%s already deleted", meta.Namespace, meta.PodName)
			} else {
				logrusField.Warningf("delete pod %s/%s failed: %v", meta.Namespace, meta.PodName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("delete pod failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				allSuccess = false
				continue
			}
		}

		status = status.CreateSuccessResourceStatus()
		status.State = v1alpha1.DestroyedState
		statuses = append(statuses, status)
	}

	if allSuccess {
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus(statuses))
	}
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses))
}

// corruptSecret backs up the original Secret data, then corrupts the credentials
func (d *ImagePullSecretsErrorActionExecutor) corruptSecret(ctx context.Context, logrusField *logrus.Entry, experimentId, namespace, secretName string) error {
	// Get the Secret
	secret := &v1.Secret{}
	if err := d.client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return fmt.Errorf("get secret %s/%s failed: %v", namespace, secretName, err)
	}

	// Validate Secret type
	if secret.Type != v1.SecretTypeDockerConfigJson && secret.Type != v1.SecretTypeDockercfg {
		return fmt.Errorf("secret %s/%s type is %s, expected %s or %s",
			namespace, secretName, secret.Type, v1.SecretTypeDockerConfigJson, v1.SecretTypeDockercfg)
	}

	// Create backup Secret
	backupName := generateBackupSecretName(experimentId, namespace, secretName)
	backupSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: namespace,
			Labels: map[string]string{
				ChaosBladeIPSExperimentLabel: experimentId,
			},
			Annotations: map[string]string{
				ChaosBladeIPSBackupAnnotation:            "true",
				ChaosBladeIPSOriginalNameAnnotation:      secretName,
				ChaosBladeIPSOriginalNamespaceAnnotation: namespace,
			},
		},
		Type: secret.Type,
		Data: copySecretData(secret.Data),
	}

	createdBackup := false
	if err := d.client.Create(ctx, backupSecret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Check if the existing backup belongs to this experiment
			existingBackup := &v1.Secret{}
			if getErr := d.client.Get(ctx, types.NamespacedName{Name: backupName, Namespace: namespace}, existingBackup); getErr != nil {
				return fmt.Errorf("backup secret %s/%s already exists and failed to verify owner: %v", namespace, backupName, getErr)
			}
			if existingBackup.Labels[ChaosBladeIPSExperimentLabel] != experimentId {
				return fmt.Errorf("secret %s/%s is already being used by another experiment %s",
					namespace, secretName, existingBackup.Labels[ChaosBladeIPSExperimentLabel])
			}
			logrusField.Infof("backup secret %s/%s already exists for this experiment, skip creation", namespace, backupName)
		} else {
			return fmt.Errorf("create backup secret %s/%s failed: %v", namespace, backupName, err)
		}
	} else {
		createdBackup = true
		logrusField.Infof("created backup secret %s/%s for original %s", namespace, backupName, secretName)
	}

	// Corrupt the credentials
	var corruptedData []byte
	var corruptErr error
	if secret.Type == v1.SecretTypeDockerConfigJson {
		dataKey := v1.DockerConfigJsonKey
		originalData, ok := secret.Data[dataKey]
		if !ok {
			return fmt.Errorf("secret %s/%s has no %s key", namespace, secretName, dataKey)
		}
		corruptedData, corruptErr = corruptDockerConfigJSON(originalData)
	} else {
		// kubernetes.io/dockercfg
		dataKey := v1.DockerConfigKey
		originalData, ok := secret.Data[dataKey]
		if !ok {
			return fmt.Errorf("secret %s/%s has no %s key", namespace, secretName, dataKey)
		}
		corruptedData, corruptErr = corruptDockerCfg(originalData)
	}

	if corruptErr != nil {
		// Rollback: only delete the backup if we created it in this call
		if createdBackup {
			if delErr := d.client.Delete(ctx, backupSecret); delErr != nil {
				logrusField.Warningf("rollback: delete backup secret %s/%s failed: %v", namespace, backupName, delErr)
			}
		}
		return fmt.Errorf("corrupt secret data failed: %v", corruptErr)
	}

	// Update the Secret with corrupted data
	if secret.Type == v1.SecretTypeDockerConfigJson {
		secret.Data[v1.DockerConfigJsonKey] = corruptedData
	} else {
		secret.Data[v1.DockerConfigKey] = corruptedData
	}

	if err := d.client.Update(ctx, secret); err != nil {
		// Rollback: only delete the backup if we created it in this call
		if createdBackup {
			if delErr := d.client.Delete(ctx, backupSecret); delErr != nil {
				logrusField.Warningf("rollback: delete backup secret %s/%s failed: %v", namespace, backupName, delErr)
			}
		}
		return fmt.Errorf("update secret %s/%s failed: %v", namespace, secretName, err)
	}

	logrusField.Infof("corrupted credentials in secret %s/%s", namespace, secretName)
	return nil
}

// rollbackSecret restores a single Secret from its backup and deletes the backup.
// secretKey is in the format "namespace/secretName".
func (d *ImagePullSecretsErrorActionExecutor) rollbackSecret(ctx context.Context, logrusField *logrus.Entry, experimentId, secretKey string) error {
	parts := strings.SplitN(secretKey, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid secret key format: %s", secretKey)
	}
	namespace, secretName := parts[0], parts[1]
	backupName := generateBackupSecretName(experimentId, namespace, secretName)

	// Get the backup Secret
	backup := &v1.Secret{}
	if err := d.client.Get(ctx, types.NamespacedName{Name: backupName, Namespace: namespace}, backup); err != nil {
		if apierrors.IsNotFound(err) {
			logrusField.Infof("rollback: backup secret %s/%s not found, nothing to restore", namespace, backupName)
			return nil
		}
		return fmt.Errorf("rollback: get backup secret %s/%s failed: %v", namespace, backupName, err)
	}

	// Get the original Secret and restore its data
	originalSecret := &v1.Secret{}
	if err := d.client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, originalSecret); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("rollback: get original secret %s/%s failed: %v", namespace, secretName, err)
		}
		// Original was deleted externally; just clean up the backup
	} else {
		originalSecret.Data = copySecretData(backup.Data)
		originalSecret.Type = backup.Type
		if err := d.client.Update(ctx, originalSecret); err != nil {
			return fmt.Errorf("rollback: restore secret %s/%s failed: %v", namespace, secretName, err)
		}
		logrusField.Infof("rollback: restored secret %s/%s from backup", namespace, secretName)
	}

	// Delete the backup Secret
	if err := d.client.Delete(ctx, backup); err != nil && !apierrors.IsNotFound(err) {
		logrusField.Warningf("rollback: delete backup secret %s/%s failed: %v", namespace, backupName, err)
	}
	return nil
}

// restoreSecretsInNamespace finds all backup Secrets in the given namespace for this experiment
// and restores the original Secrets from the backups
func (d *ImagePullSecretsErrorActionExecutor) restoreSecretsInNamespace(ctx context.Context, logrusField *logrus.Entry, experimentId, namespace string) error {
	backupList := &v1.SecretList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{ChaosBladeIPSExperimentLabel: experimentId},
	}
	if err := d.client.List(ctx, backupList, listOpts...); err != nil {
		return fmt.Errorf("list backup secrets in namespace %s failed: %v", namespace, err)
	}

	var errs []string
	for i := range backupList.Items {
		backup := &backupList.Items[i]

		// Only process Secrets explicitly marked as IPS backups
		if backup.Annotations[ChaosBladeIPSBackupAnnotation] != "true" {
			continue
		}

		originalName := backup.Annotations[ChaosBladeIPSOriginalNameAnnotation]
		originalNamespace := backup.Annotations[ChaosBladeIPSOriginalNamespaceAnnotation]

		if originalName == "" || originalNamespace == "" {
			logrusField.Warningf("backup secret %s/%s missing original name/namespace annotations, skip",
				namespace, backup.Name)
			errs = append(errs, fmt.Sprintf("backup %s/%s missing annotations", namespace, backup.Name))
			continue
		}

		// Get the original Secret
		originalSecret := &v1.Secret{}
		err := d.client.Get(ctx, types.NamespacedName{Name: originalName, Namespace: originalNamespace}, originalSecret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("original secret %s/%s not found, deleting backup", originalNamespace, originalName)
			} else {
				logrusField.Warningf("get original secret %s/%s failed: %v", originalNamespace, originalName, err)
				errs = append(errs, fmt.Sprintf("get secret %s/%s failed: %v", originalNamespace, originalName, err))
				continue
			}
		} else {
			// Restore the original data and type
			originalSecret.Data = copySecretData(backup.Data)
			originalSecret.Type = backup.Type
			if err := d.client.Update(ctx, originalSecret); err != nil {
				logrusField.Warningf("restore secret %s/%s failed: %v", originalNamespace, originalName, err)
				errs = append(errs, fmt.Sprintf("restore secret %s/%s failed: %v", originalNamespace, originalName, err))
				continue
			}
			logrusField.Infof("restored secret %s/%s from backup", originalNamespace, originalName)
		}

		// Delete the backup Secret
		if err := d.client.Delete(ctx, backup); err != nil {
			if !apierrors.IsNotFound(err) {
				logrusField.Warningf("delete backup secret %s/%s failed: %v", namespace, backup.Name, err)
				errs = append(errs, fmt.Sprintf("delete backup %s/%s failed: %v", namespace, backup.Name, err))
			}
		} else {
			logrusField.Infof("deleted backup secret %s/%s", namespace, backup.Name)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("restore secrets in namespace %s had %d error(s): %s", namespace, len(errs), strings.Join(errs, "; "))
	}
	return nil
}

// generateBackupSecretName creates a deterministic backup Secret name
// Format: chaosblade-ips-<experimentId[:8]>-<sha256(namespace/secretName)[:8]>
func generateBackupSecretName(experimentId, namespace, secretName string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s/%s", namespace, secretName)))
	hashStr := fmt.Sprintf("%x", hash[:4])

	expIdPrefix := experimentId
	if len(expIdPrefix) > 8 {
		expIdPrefix = expIdPrefix[:8]
	}

	return fmt.Sprintf("chaosblade-ips-%s-%s", expIdPrefix, hashStr)
}

// corruptDockerConfigJSON corrupts the auth credentials in a .dockerconfigjson format Secret
// The JSON structure is preserved but all credentials are replaced with invalid values
func corruptDockerConfigJSON(data []byte) ([]byte, error) {
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal dockerconfigjson failed: %v", err)
	}

	auths, ok := config["auths"]
	if !ok {
		return nil, fmt.Errorf("dockerconfigjson has no auths key")
	}

	authsMap, ok := auths.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("auths field is not a map")
	}

	if len(authsMap) == 0 {
		return nil, fmt.Errorf("dockerconfigjson auths is empty, no credentials to corrupt")
	}

	// Corrupt each registry's credentials
	if n := corruptRegistryCredentials(authsMap); n == 0 {
		return nil, fmt.Errorf("dockerconfigjson: no valid registry credentials found to corrupt")
	}

	config["auths"] = authsMap
	result, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal corrupted dockerconfigjson failed: %v", err)
	}
	return result, nil
}

// corruptDockerCfg corrupts the auth credentials in a .dockercfg format Secret
// The .dockercfg format is: {"registry": {"username": "...", "password": "...", "auth": "..."}}
func corruptDockerCfg(data []byte) ([]byte, error) {
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal dockercfg failed: %v", err)
	}

	if len(config) == 0 {
		return nil, fmt.Errorf("dockercfg is empty, no credentials to corrupt")
	}

	if n := corruptRegistryCredentials(config); n == 0 {
		return nil, fmt.Errorf("dockercfg: no valid registry credentials found to corrupt")
	}

	result, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal corrupted dockercfg failed: %v", err)
	}
	return result, nil
}

// corruptRegistryCredentials replaces the auth credentials in each registry entry with invalid values.
// Returns the number of registry entries that were actually corrupted.
func corruptRegistryCredentials(registries map[string]interface{}) int {
	invalidAuth := base64.StdEncoding.EncodeToString([]byte("chaosblade-invalid-user:chaosblade-invalid-pass"))
	corrupted := 0
	for registry, creds := range registries {
		credsMap, ok := creds.(map[string]interface{})
		if !ok {
			continue
		}
		credsMap["username"] = "chaosblade-invalid-user"
		credsMap["password"] = "chaosblade-invalid-pass"
		credsMap["auth"] = invalidAuth
		delete(credsMap, "identitytoken")
		delete(credsMap, "registrytoken")
		registries[registry] = credsMap
		corrupted++
	}
	return corrupted
}

// filterSecretRefs filters the imagePullSecrets list by a specific Secret name
func filterSecretRefs(refs []v1.LocalObjectReference, name string) []v1.LocalObjectReference {
	filtered := make([]v1.LocalObjectReference, 0)
	for _, ref := range refs {
		if ref.Name == name {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

// copySecretData creates a deep copy of Secret data
func copySecretData(data map[string][]byte) map[string][]byte {
	if data == nil {
		return nil
	}
	result := make(map[string][]byte, len(data))
	for k, v := range data {
		copied := make([]byte, len(v))
		copy(copied, v)
		result[k] = copied
	}
	return result
}
