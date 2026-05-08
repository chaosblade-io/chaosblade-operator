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
	"encoding/json"
	"fmt"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	ConfigMapNameFlag = "configmap-name"

	ChaosBladeExperimentLabel      = "chaosblade.io/experiment-id"
	ChaosBladeBackupLabel          = "chaosblade.io/backup"
	ChaosBladeOriginalNameAnn      = "chaosblade.io/original-name"
	ChaosBladeOriginalNamespaceAnn = "chaosblade.io/original-namespace"
	ChaosBladeOriginalLabelsAnn    = "chaosblade.io/original-labels"
	ChaosBladeOriginalAnnsAnn      = "chaosblade.io/original-annotations"
)

type ConfigMapDeleteActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewConfigMapDeleteActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &ConfigMapDeleteActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: ConfigMapNameFlag,
					Desc: "The ConfigMap name to delete. If not specified, the first non-optional ConfigMap from the Pod spec will be selected",
				},
			},
			ActionExecutor: &ConfigMapDeleteActionExecutor{client: client},
			ActionExample: `# Delete the auto-selected required ConfigMap for pods matching labels
blade create k8s pod-pod configmapdelete --labels "app=test" --namespace default

# Delete a specific ConfigMap
blade create k8s pod-pod configmapdelete --labels "app=test" --namespace default --configmap-name my-config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*ConfigMapDeleteActionSpec) Name() string {
	return "configmapdelete"
}

func (*ConfigMapDeleteActionSpec) Aliases() []string {
	return []string{}
}

func (*ConfigMapDeleteActionSpec) ShortDesc() string {
	return "Delete ConfigMap to simulate Pod startup failure"
}

func (*ConfigMapDeleteActionSpec) LongDesc() string {
	return "Delete a ConfigMap that a Pod depends on, then restart the Pod to simulate startup failure " +
		"caused by missing ConfigMap. The original ConfigMap is backed up and restored when the experiment is destroyed."
}

type ConfigMapDeleteActionExecutor struct {
	client *channel.Client
}

func (*ConfigMapDeleteActionExecutor) Name() string {
	return "configmapdelete"
}

func (*ConfigMapDeleteActionExecutor) SetChannel(channel spec.Channel) {}

func (d *ConfigMapDeleteActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(ctx, expModel)
	}
	return d.create(ctx, expModel)
}

// create backs up the target ConfigMap, deletes it, then deletes the Pod to trigger a restart failure.
func (d *ConfigMapDeleteActionExecutor) create(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	configMapName := expModel.ActionFlags[ConfigMapNameFlag]
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerMatchedList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(experimentId, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	processedCMs := make(map[string]bool)

	for _, c := range containerMatchedList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: c.GetIdentifier(),
		}

		// Step 1: Fetch the Pod
		pod := &v1.Pod{}
		if err := d.client.Get(ctx, types.NamespacedName{Name: c.PodName, Namespace: c.Namespace}, pod); err != nil {
			logrusField.Errorf("get pod %s/%s failed: %v", c.Namespace, c.PodName, err)
			status = status.CreateFailResourceStatus(spec.K8sExecFailed.Sprintf("get pod", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		// Skip pods that are not in a healthy state
		if !isPodReady(pod) {
			logrusField.Infof("pod %s/%s is not ready, skip", c.Namespace, c.PodName)
			status = status.CreateFailResourceStatus(
				fmt.Sprintf("pod %s is not ready", c.PodName), spec.K8sExecFailed.Code,
			)
			statuses = append(statuses, status)
			continue
		}

		// Step 2: Resolve the target ConfigMap
		resolvedCMName, resolveErr := resolveTargetConfigMap(pod, configMapName)
		if resolveErr != nil {
			logrusField.Errorf("resolve configmap for pod %s/%s failed: %v", c.Namespace, c.PodName, resolveErr)
			status = status.CreateFailResourceStatus(resolveErr.Error(), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		cmKey := fmt.Sprintf("%s/%s", c.Namespace, resolvedCMName)

		// Step 3: Deduplicate — only backup & delete each ConfigMap once
		if !processedCMs[cmKey] {
			// Step 4: Fetch the original ConfigMap
			originalCM := &v1.ConfigMap{}
			if err := d.client.Get(ctx, types.NamespacedName{Name: resolvedCMName, Namespace: c.Namespace}, originalCM); err != nil {
				logrusField.Errorf("get configmap %s/%s failed: %v", c.Namespace, resolvedCMName, err)
				status = status.CreateFailResourceStatus(
					fmt.Sprintf("configmap %s not found in namespace %s", resolvedCMName, c.Namespace), spec.K8sExecFailed.Code,
				)
				statuses = append(statuses, status)
				continue
			}
			// Step 5: Create backup ConfigMap
			// (originalCM is declared inside the dedup block above)
			if err := d.createBackupConfigMap(ctx, experimentId, originalCM); err != nil {
				logrusField.Errorf("create backup configmap for %s/%s failed: %v", c.Namespace, resolvedCMName, err)
				status = status.CreateFailResourceStatus(
					fmt.Sprintf("create backup configmap failed: %v", err), spec.K8sExecFailed.Code,
				)
				statuses = append(statuses, status)
				continue
			}
			logrusField.Infof("created backup configmap for %s/%s", c.Namespace, resolvedCMName)

			// Step 6: Delete the original ConfigMap
			if err := d.client.Delete(ctx, originalCM); err != nil && !apierrors.IsNotFound(err) {
				logrusField.Errorf("delete configmap %s/%s failed: %v", c.Namespace, resolvedCMName, err)
				// Rollback: delete the backup
				backupName := getBackupConfigMapName(experimentId, c.Namespace, resolvedCMName)
				if rbErr := d.client.Delete(ctx, &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: backupName, Namespace: c.Namespace},
				}); rbErr != nil && !apierrors.IsNotFound(rbErr) {
					logrusField.Warningf("rollback: delete backup configmap %s failed: %v", backupName, rbErr)
				}
				status = status.CreateFailResourceStatus(
					fmt.Sprintf("delete configmap %s failed: %v", resolvedCMName, err), spec.K8sExecFailed.Code,
				)
				statuses = append(statuses, status)
				continue
			}
			logrusField.Infof("deleted configmap %s/%s", c.Namespace, resolvedCMName)

			// Mark as processed only after both backup and delete succeed
			processedCMs[cmKey] = true
		}

		// Step 7: Delete the Pod to trigger rebuild
		if err := d.client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
			logrusField.Errorf("delete pod %s/%s failed: %v", c.Namespace, c.PodName, err)
			// The ConfigMap is already deleted. Attempt to restore it from backup.
			backupName := getBackupConfigMapName(experimentId, c.Namespace, resolvedCMName)
			if restoreErr := d.restoreAndCleanupBackup(ctx, c.Namespace, backupName); restoreErr != nil {
				logrusField.Errorf("rollback: restore configmap from backup %s failed: %v, manual intervention required", backupName, restoreErr)
				status = status.CreateFailResourceStatus(
					fmt.Sprintf("configmap %s has been deleted but restore failed: %v, manual intervention required", resolvedCMName, restoreErr),
					spec.K8sExecFailed.Code,
				)
				statuses = append(statuses, status)
				continue
			}
			status = status.CreateFailResourceStatus(
				fmt.Sprintf("delete pod %s failed: %v", c.PodName, err), spec.K8sExecFailed.Code,
			)
			statuses = append(statuses, status)
			continue
		}
		logrusField.Infof("deleted pod %s/%s to trigger rebuild", c.Namespace, c.PodName)

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

// destroy restores the backed-up ConfigMap and deletes the Pod to trigger a healthy restart.
func (d *ConfigMapDeleteActionExecutor) destroy(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerMatchedList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(experimentId, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	// Find all backup ConfigMaps for this experiment across all relevant namespaces
	namespaces := make(map[string]bool)
	for _, c := range containerMatchedList {
		namespaces[c.Namespace] = true
	}

	// Collect backup CMs by namespace
	allSuccess := true
	backupCMs := make(map[string][]*v1.ConfigMap) // namespace -> list of backup CMs
	for ns := range namespaces {
		cmList := &v1.ConfigMapList{}
		labelSelector := labels.SelectorFromSet(labels.Set{
			ChaosBladeExperimentLabel: experimentId,
			ChaosBladeBackupLabel:     "configmap",
		})
		if err := d.client.List(ctx, cmList, &client.ListOptions{
			Namespace:     ns,
			LabelSelector: labelSelector,
		}); err != nil {
			logrusField.Errorf("list backup configmaps in namespace %s failed: %v", ns, err)
			allSuccess = false
			continue
		}
		for i := range cmList.Items {
			backupCMs[ns] = append(backupCMs[ns], &cmList.Items[i])
		}
	}

	// Restore all backup ConfigMaps
	restoredCMs := make(map[string]bool) // key: namespace/backupName
	for ns, cms := range backupCMs {
		for _, backupCM := range cms {
			backupKey := fmt.Sprintf("%s/%s", ns, backupCM.Name)
			if restoredCMs[backupKey] {
				continue
			}
			restoredCMs[backupKey] = true

			if err := d.restoreConfigMapFromBackup(ctx, backupCM); err != nil {
				logrusField.Errorf("restore configmap from backup %s/%s failed: %v", ns, backupCM.Name, err)
				allSuccess = false
				// Do NOT delete the backup when restore fails — preserve it for retry
				continue
			}

			if err := d.deleteBackupConfigMap(ctx, backupCM); err != nil {
				logrusField.Warningf("delete backup configmap %s/%s failed: %v", ns, backupCM.Name, err)
			}
		}
	}

	// Delete all matched Pods to trigger healthy rebuilds
	statuses := make([]v1alpha1.ResourceStatus, 0)
	for _, c := range containerMatchedList {
		status := v1alpha1.ResourceStatus{
			Id:         c.Id,
			Kind:       v1alpha1.PodKind,
			Identifier: c.GetIdentifier(),
		}

		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.PodName,
				Namespace: c.Namespace,
			},
		}
		if err := d.client.Delete(ctx, pod); err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("pod %s/%s already deleted", c.Namespace, c.PodName)
			} else {
				logrusField.Errorf("delete pod %s/%s failed: %v", c.Namespace, c.PodName, err)
				status = status.CreateFailResourceStatus(
					fmt.Sprintf("delete pod %s failed: %v", c.PodName, err), spec.K8sExecFailed.Code,
				)
				statuses = append(statuses, status)
				allSuccess = false
				continue
			}
		} else {
			logrusField.Infof("deleted pod %s/%s to trigger healthy rebuild", c.Namespace, c.PodName)
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

// createBackupConfigMap creates a backup copy of the original ConfigMap.
func (d *ConfigMapDeleteActionExecutor) createBackupConfigMap(ctx context.Context, experimentId string, originalCM *v1.ConfigMap) error {
	backupName := getBackupConfigMapName(experimentId, originalCM.Namespace, originalCM.Name)

	annotations := map[string]string{
		ChaosBladeOriginalNameAnn:      originalCM.Name,
		ChaosBladeOriginalNamespaceAnn: originalCM.Namespace,
		ChaosBladeExperimentAnnotation: experimentId,
	}

	// Preserve original labels and annotations as JSON
	if len(originalCM.Labels) > 0 {
		if labelsJSON, err := json.Marshal(originalCM.Labels); err != nil {
			logrus.Warningf("failed to marshal original labels for configmap %s/%s: %v", originalCM.Namespace, originalCM.Name, err)
		} else {
			annotations[ChaosBladeOriginalLabelsAnn] = string(labelsJSON)
		}
	}
	if len(originalCM.Annotations) > 0 {
		if annsJSON, err := json.Marshal(originalCM.Annotations); err != nil {
			logrus.Warningf("failed to marshal original annotations for configmap %s/%s: %v", originalCM.Namespace, originalCM.Name, err)
		} else {
			annotations[ChaosBladeOriginalAnnsAnn] = string(annsJSON)
		}
	}

	backupCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: originalCM.Namespace,
			Labels: map[string]string{
				ChaosBladeExperimentLabel: experimentId,
				ChaosBladeBackupLabel:     "configmap",
			},
			Annotations: annotations,
		},
		Data:       copyStringMap(originalCM.Data),
		BinaryData: copyByteMap(originalCM.BinaryData),
	}

	if err := d.client.Create(ctx, backupCM); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.WithField("experiment", experimentId).Infof("backup configmap %s already exists, skip creation", backupName)
			return nil
		}
		return err
	}
	return nil
}

// restoreConfigMapFromBackup recreates the original ConfigMap from a backup ConfigMap object.
// It reads the original name, namespace, labels, and annotations from the backup's annotations.
// Returns nil if the original ConfigMap already exists (AlreadyExists is treated as success).
func (d *ConfigMapDeleteActionExecutor) restoreConfigMapFromBackup(ctx context.Context, backupCM *v1.ConfigMap) error {
	originalName := backupCM.Annotations[ChaosBladeOriginalNameAnn]
	originalNamespace := backupCM.Annotations[ChaosBladeOriginalNamespaceAnn]
	if originalNamespace == "" {
		originalNamespace = backupCM.Namespace
	}

	// Restore original labels
	var originalLabels map[string]string
	if labelsJSON := backupCM.Annotations[ChaosBladeOriginalLabelsAnn]; labelsJSON != "" {
		if err := json.Unmarshal([]byte(labelsJSON), &originalLabels); err != nil {
			logrus.Warningf("failed to unmarshal original labels from backup %s/%s: %v", backupCM.Namespace, backupCM.Name, err)
		}
	}
	// Restore original annotations
	var originalAnnotations map[string]string
	if annsJSON := backupCM.Annotations[ChaosBladeOriginalAnnsAnn]; annsJSON != "" {
		if err := json.Unmarshal([]byte(annsJSON), &originalAnnotations); err != nil {
			logrus.Warningf("failed to unmarshal original annotations from backup %s/%s: %v", backupCM.Namespace, backupCM.Name, err)
		}
	}

	restoredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        originalName,
			Namespace:   originalNamespace,
			Labels:      originalLabels,
			Annotations: originalAnnotations,
		},
		Data:       backupCM.Data,
		BinaryData: backupCM.BinaryData,
	}
	if err := d.client.Create(ctx, restoredCM); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.Infof("configmap %s/%s already exists, skip restore", originalNamespace, originalName)
			return nil
		}
		return fmt.Errorf("restore configmap %s/%s: %w", originalNamespace, originalName, err)
	}
	return nil
}

// deleteBackupConfigMap removes a backup ConfigMap. NotFound is treated as success.
func (d *ConfigMapDeleteActionExecutor) deleteBackupConfigMap(ctx context.Context, backupCM *v1.ConfigMap) error {
	if err := d.client.Delete(ctx, backupCM); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete backup configmap %s/%s: %w", backupCM.Namespace, backupCM.Name, err)
	}
	return nil
}

// restoreAndCleanupBackup fetches the backup ConfigMap by name, restores the original, and deletes the backup.
// Used during create-phase rollback when we need to look up the backup by name.
func (d *ConfigMapDeleteActionExecutor) restoreAndCleanupBackup(ctx context.Context, namespace, backupName string) error {
	backupCM := &v1.ConfigMap{}
	if err := d.client.Get(ctx, types.NamespacedName{Name: backupName, Namespace: namespace}, backupCM); err != nil {
		return fmt.Errorf("get backup configmap %s/%s: %w", namespace, backupName, err)
	}

	if err := d.restoreConfigMapFromBackup(ctx, backupCM); err != nil {
		return err
	}
	return d.deleteBackupConfigMap(ctx, backupCM)
}

// ConfigMapRef describes a ConfigMap reference found in a Pod spec.
type ConfigMapRef struct {
	Name     string
	Optional bool
	Source   string // "volume", "envFrom", "envValueFrom"
}

// collectConfigMapReferences scans a Pod spec and returns all ConfigMap references.
// It scans Volumes, EnvFrom and Env ValueFrom across all containers and init containers.
// Duplicate names are merged: if any reference is required, the result is required.
func collectConfigMapReferences(pod *v1.Pod) []ConfigMapRef {
	seen := make(map[string]int) // name -> index in result
	var refs []ConfigMapRef

	addRef := func(name string, optional bool, source string) {
		if idx, exists := seen[name]; exists {
			// If any reference is required, mark as required
			if !optional {
				refs[idx].Optional = false
			}
			return
		}
		seen[name] = len(refs)
		refs = append(refs, ConfigMapRef{Name: name, Optional: optional, Source: source})
	}

	// Scan volumes first (highest priority for auto-selection)
	for _, vol := range pod.Spec.Volumes {
		if vol.ConfigMap != nil {
			addRef(vol.ConfigMap.LocalObjectReference.Name, isOptional(vol.ConfigMap.Optional), "volume")
		}
	}

	// Scan all containers (regular + init)
	allContainers := make([]v1.Container, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))
	allContainers = append(allContainers, pod.Spec.Containers...)
	allContainers = append(allContainers, pod.Spec.InitContainers...)
	for _, ctr := range allContainers {
		for _, envFrom := range ctr.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				addRef(envFrom.ConfigMapRef.LocalObjectReference.Name, isOptional(envFrom.ConfigMapRef.Optional), "envFrom")
			}
		}
		for _, env := range ctr.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				addRef(env.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name, isOptional(env.ValueFrom.ConfigMapKeyRef.Optional), "envValueFrom")
			}
		}
	}

	return refs
}

// resolveTargetConfigMap determines which ConfigMap to target for chaos injection.
// If userSpecifiedName is provided, it validates that the ConfigMap is referenced and non-optional.
// Otherwise, it returns the first non-optional ConfigMap.
func resolveTargetConfigMap(pod *v1.Pod, userSpecifiedName string) (string, error) {
	refs := collectConfigMapReferences(pod)

	if len(refs) == 0 {
		return "", fmt.Errorf("pod %s has no ConfigMap dependency", pod.Name)
	}

	if userSpecifiedName != "" {
		for _, ref := range refs {
			if ref.Name == userSpecifiedName {
				if ref.Optional {
					return "", fmt.Errorf("configmap %s is optional in pod %s, only required ConfigMaps can be deleted for chaos", userSpecifiedName, pod.Name)
				}
				return userSpecifiedName, nil
			}
		}
		return "", fmt.Errorf("configmap %s is not referenced by pod %s", userSpecifiedName, pod.Name)
	}

	// Auto-select the first non-optional ConfigMap
	for _, ref := range refs {
		if !ref.Optional {
			return ref.Name, nil
		}
	}
	return "", fmt.Errorf("pod %s has no required (non-optional) ConfigMap dependency", pod.Name)
}

// isOptional returns true only when the *bool pointer is non-nil and true.
func isOptional(opt *bool) bool {
	return opt != nil && *opt
}

// getBackupConfigMapName generates a deterministic backup name from experiment ID and ConfigMap identity.
// The experimentId is truncated to 8 chars to keep names short; the full ID is stored in labels for querying.
func getBackupConfigMapName(experimentId, namespace, cmName string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s", experimentId, namespace, cmName)))
	hashStr := fmt.Sprintf("%x", hash[:4])

	expIdPrefix := experimentId
	if len(expIdPrefix) > 8 {
		expIdPrefix = expIdPrefix[:8]
	}

	return fmt.Sprintf("chaosblade-backup-%s-%s", expIdPrefix, hashStr)
}

func copyStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyByteMap(src map[string][]byte) map[string][]byte {
	if src == nil {
		return nil
	}
	dst := make(map[string][]byte, len(src))
	for k, v := range src {
		cp := make([]byte, len(v))
		copy(cp, v)
		dst[k] = cp
	}
	return dst
}
