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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	ChaosBladeFailedMountAction           = "failedmount"
	ChaosBladeOriginalVolumesAnnotation   = "chaosblade.io/original-volumes"
	ChaosBladeFailedMountVolumeNamePrefix = "chaosblade-fm-"
	ChaosBladeFailedMountVolumeMountPath  = "/chaosblade-fm-nonexistent"

	FailedMountVolumeTypeConfigMap = "configmap"
	FailedMountVolumeTypeSecret    = "secret"
	FailedMountVolumeTypePVC       = "pvc"
)

type FailedMountActionSpec struct {
	spec.BaseExpActionCommandSpec
	client *channel.Client
}

func NewFailedMountActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &FailedMountActionSpec{
		BaseExpActionCommandSpec: spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:     "workload-type",
					Desc:     "Workload type: deployment, daemonset, statefulset. Default: deployment",
					Required: false,
					Default:  "deployment",
				},
				&spec.ExpFlag{
					Name:     "workload-name",
					Desc:     "Workload name to inject failed mount volume",
					Required: true,
				},
				&spec.ExpFlag{
					Name:     "volume-type",
					Desc:     "Volume type to inject: configmap, secret, pvc",
					Required: true,
				},
				&spec.ExpFlag{
					Name:     "with-initcontainer",
					Desc:     "Mount the non-existent volume to init containers first. Default: false",
					Required: false,
					Default:  "false",
				},
			},
			ActionExecutor: &FailedMountActionExecutor{client: client},
			ActionExample: `# Mount a non-existent configmap volume to a deployment
blade create k8s pod-pod failedmount --namespace default --workload-type deployment --workload-name nginx-app --volume-type configmap --kubeconfig ~/.kube/config

# Mount a non-existent secret volume to a deployment with init container
blade create k8s pod-pod failedmount --namespace default --workload-type deployment --workload-name nginx-app --volume-type secret --with-initcontainer true --kubeconfig ~/.kube/config

# Mount a non-existent pvc volume to a statefulset
blade create k8s pod-pod failedmount --namespace default --workload-type statefulset --workload-name redis-app --volume-type pvc --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
		client: client,
	}
}

func (*FailedMountActionSpec) Name() string {
	return "failedmount"
}

func (*FailedMountActionSpec) Aliases() []string {
	return []string{}
}

func (*FailedMountActionSpec) ShortDesc() string {
	return "Mount a non-existent configmap/secret/pvc volume to simulate volume mount failure"
}

func (*FailedMountActionSpec) LongDesc() string {
	return "Inject a fault by adding a volume referencing a non-existent ConfigMap, Secret, or PVC " +
		"to the target workload (Deployment/DaemonSet/StatefulSet). The volume name is randomly generated. " +
		"When --with-initcontainer is true, the volume mount is added to init containers first. " +
		"The original volume configuration is backed up in an annotation and restored when the experiment is destroyed."
}

// PreCreate implements model.ActionPreProcessor.
func (a *FailedMountActionSpec) PreCreate(ctx context.Context, expModel *spec.ExpModel, client *channel.Client) (context.Context, *spec.Response) {
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	if namespace == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if strings.Contains(namespace, ",") {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}

	volumeType := expModel.ActionFlags["volume-type"]
	if volumeType == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "volume-type")
	}
	if volumeType != FailedMountVolumeTypeConfigMap && volumeType != FailedMountVolumeTypeSecret && volumeType != FailedMountVolumeTypePVC {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterIllegal, "volume-type",
			volumeType, "must be one of: configmap, secret, pvc")
	}

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-fm-%s-%s", workloadType, workloadName),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}

// PreDestroy implements model.ActionPreProcessor.
func (a *FailedMountActionSpec) PreDestroy(ctx context.Context, expModel *spec.ExpModel, client *channel.Client, oldExpStatus v1alpha1.ExperimentStatus) (context.Context, *spec.Response) {
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	if namespace == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if strings.Contains(namespace, ",") {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-fm-%s-%s", workloadType, workloadName),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}

type FailedMountActionExecutor struct {
	client *channel.Client
}

func (*FailedMountActionExecutor) Name() string {
	return "failedmount"
}

func (*FailedMountActionExecutor) SetChannel(channel spec.Channel) {}

func (d *FailedMountActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *FailedMountActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]
	volumeType := expModel.ActionFlags["volume-type"]
	withInitContainer := strings.EqualFold(expModel.ActionFlags["with-initcontainer"], "true")

	if namespace == "" {
		util.Errorf(uid, util.GetRunFuncName(), "namespace is required")
		return spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		util.Errorf(uid, util.GetRunFuncName(), "workload-name is required")
		return spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}
	if volumeType == "" {
		util.Errorf(uid, util.GetRunFuncName(), "volume-type is required")
		return spec.ResponseFailWithFlags(spec.ParameterLess, "volume-type")
	}
	if volumeType != FailedMountVolumeTypeConfigMap && volumeType != FailedMountVolumeTypeSecret && volumeType != FailedMountVolumeTypePVC {
		util.Errorf(uid, util.GetRunFuncName(), fmt.Sprintf("invalid volume-type: %s", volumeType))
		return spec.ResponseFailWithFlags(spec.ParameterIllegal, "volume-type",
			volumeType, "must be one of: configmap, secret, pvc")
	}

	status := v1alpha1.ResourceStatus{
		Kind:       v1alpha1.PodKind,
		Identifier: fmt.Sprintf("%s//%s//%s", namespace, workloadType, workloadName),
	}

	switch workloadType {
	case "deployment":
		deployment := &appsv1.Deployment{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, deployment)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("deployment %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("deployment not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get deployment %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get deployment failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.injectDeploymentFailedMount(ctx, deployment, volumeType, withInitContainer, experimentId); err != nil {
			logrusField.Warningf("inject failed mount to deployment %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject failed mount failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected failed mount to deployment %s/%s with volume-type=%s", namespace, workloadName, volumeType)

	case "daemonset":
		daemonset := &appsv1.DaemonSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, daemonset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("daemonset %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("daemonset not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get daemonset %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get daemonset failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.injectDaemonSetFailedMount(ctx, daemonset, volumeType, withInitContainer, experimentId); err != nil {
			logrusField.Warningf("inject failed mount to daemonset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject failed mount failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected failed mount to daemonset %s/%s with volume-type=%s", namespace, workloadName, volumeType)

	case "statefulset":
		statefulset := &appsv1.StatefulSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, statefulset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("statefulset %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("statefulset not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get statefulset %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get statefulset failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.injectStatefulSetFailedMount(ctx, statefulset, volumeType, withInitContainer, experimentId); err != nil {
			logrusField.Warningf("inject failed mount to statefulset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject failed mount failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected failed mount to statefulset %s/%s with volume-type=%s", namespace, workloadName, volumeType)

	default:
		status = status.CreateFailResourceStatus(fmt.Sprintf("unsupported workload type: %s", workloadType), spec.ParameterIllegal.Code)
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
	}

	status = status.CreateSuccessResourceStatus()
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

func (d *FailedMountActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	status := v1alpha1.ResourceStatus{
		Kind:       v1alpha1.PodKind,
		Identifier: fmt.Sprintf("%s//%s//%s", namespace, workloadType, workloadName),
	}

	switch workloadType {
	case "deployment":
		deployment := &appsv1.Deployment{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, deployment)
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
		}
		if err := d.restoreDeploymentVolumes(ctx, deployment, experimentId); err != nil {
			logrusField.Warningf("restore deployment %s/%s volumes failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore deployment volumes failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored deployment %s/%s volumes", namespace, workloadName)

	case "daemonset":
		daemonset := &appsv1.DaemonSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, daemonset)
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
		}
		if err := d.restoreDaemonSetVolumes(ctx, daemonset, experimentId); err != nil {
			logrusField.Warningf("restore daemonset %s/%s volumes failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore daemonset volumes failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored daemonset %s/%s volumes", namespace, workloadName)

	case "statefulset":
		statefulset := &appsv1.StatefulSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, statefulset)
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
		}
		if err := d.restoreStatefulSetVolumes(ctx, statefulset, experimentId); err != nil {
			logrusField.Warningf("restore statefulset %s/%s volumes failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore statefulset volumes failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored statefulset %s/%s volumes", namespace, workloadName)

	default:
		status = status.CreateFailResourceStatus(fmt.Sprintf("unsupported workload type: %s", workloadType), spec.ParameterIllegal.Code)
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
	}

	status = status.CreateSuccessResourceStatus()
	status.State = v1alpha1.DestroyedState
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

// volumeBackup stores the injected volume name so destroy knows exactly what to remove.
type volumeBackup struct {
	VolumeName string `json:"volumeName"`
	VolumeType string `json:"volumeType"`
	MountedTo  string `json:"mountedTo"` // "initContainers" or "containers"
}

// generateRandomHash generates a 12-character hex string.
func generateRandomHash() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "a1b2c3d4e5f6"
	}
	return hex.EncodeToString(b)
}

// buildFailedMountVolume creates a Volume with a non-existent configmap/secret/pvc reference.
func buildFailedMountVolume(volumeName, volumeType string) v1.Volume {
	fakeRef := "chaosblade-nonexistent-" + generateRandomHash()
	vol := v1.Volume{Name: volumeName}

	switch volumeType {
	case FailedMountVolumeTypeConfigMap:
		vol.VolumeSource = v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{Name: fakeRef},
			},
		}
	case FailedMountVolumeTypeSecret:
		vol.VolumeSource = v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: fakeRef,
			},
		}
	case FailedMountVolumeTypePVC:
		vol.VolumeSource = v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: fakeRef,
			},
		}
	}
	return vol
}

// injectFailedMountVolume adds a non-existent volume and mounts it to the target containers.
// If withInitContainer is true, mounts to init containers; otherwise mounts to regular containers.
func injectFailedMountVolume(podSpec *v1.PodSpec, annotations map[string]string, volumeType string, withInitContainer bool) (*volumeBackup, error) {
	volumeName := ChaosBladeFailedMountVolumeNamePrefix + generateRandomHash()

	mountedTo := "containers"
	if withInitContainer {
		if len(podSpec.InitContainers) == 0 {
			return nil, fmt.Errorf("the specified pod has no initContainers")
		}
		mountedTo = "initContainers"
	}

	vol := buildFailedMountVolume(volumeName, volumeType)
	podSpec.Volumes = append(podSpec.Volumes, vol)

	volumeMount := v1.VolumeMount{
		Name:      volumeName,
		MountPath: ChaosBladeFailedMountVolumeMountPath + "/" + volumeName,
	}

	if withInitContainer {
		for i := range podSpec.InitContainers {
			podSpec.InitContainers[i].VolumeMounts = append(podSpec.InitContainers[i].VolumeMounts, volumeMount)
		}
	} else {
		for i := range podSpec.Containers {
			podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts, volumeMount)
		}
	}

	backup := &volumeBackup{
		VolumeName: volumeName,
		VolumeType: volumeType,
		MountedTo:  mountedTo,
	}
	backupBytes, err := json.Marshal(backup)
	if err != nil {
		return nil, fmt.Errorf("marshal volume backup failed: %v", err)
	}
	annotations[ChaosBladeOriginalVolumesAnnotation] = string(backupBytes)

	return backup, nil
}

// removeInjectedVolume removes the injected volume and its mounts from the pod spec.
// It also validates that the volume being removed matches the expected type from backup.
func removeInjectedVolume(podSpec *v1.PodSpec, backup *volumeBackup) error {
	found := false
	newVolumes := make([]v1.Volume, 0, len(podSpec.Volumes))
	for _, vol := range podSpec.Volumes {
		if vol.Name == backup.VolumeName {
			found = true
			if err := validateVolumeType(&vol, backup.VolumeType); err != nil {
				return fmt.Errorf("sanity check failed for volume %q: %w", backup.VolumeName, err)
			}
			continue
		}
		newVolumes = append(newVolumes, vol)
	}
	if !found {
		logrus.Warnf("injected volume %q not found in pod spec, it may have been removed externally", backup.VolumeName)
	}
	podSpec.Volumes = newVolumes

	if backup.MountedTo == "initContainers" {
		for i := range podSpec.InitContainers {
			mounts := make([]v1.VolumeMount, 0, len(podSpec.InitContainers[i].VolumeMounts))
			for _, m := range podSpec.InitContainers[i].VolumeMounts {
				if m.Name != backup.VolumeName {
					mounts = append(mounts, m)
				}
			}
			podSpec.InitContainers[i].VolumeMounts = mounts
		}
	} else {
		for i := range podSpec.Containers {
			mounts := make([]v1.VolumeMount, 0, len(podSpec.Containers[i].VolumeMounts))
			for _, m := range podSpec.Containers[i].VolumeMounts {
				if m.Name != backup.VolumeName {
					mounts = append(mounts, m)
				}
			}
			podSpec.Containers[i].VolumeMounts = mounts
		}
	}
	return nil
}

// validateVolumeType checks that the volume's actual source type matches the expected type from backup.
func validateVolumeType(vol *v1.Volume, expectedType string) error {
	switch expectedType {
	case FailedMountVolumeTypeConfigMap:
		if vol.ConfigMap == nil {
			return fmt.Errorf("expected configmap volume but found different type")
		}
	case FailedMountVolumeTypeSecret:
		if vol.Secret == nil {
			return fmt.Errorf("expected secret volume but found different type")
		}
	case FailedMountVolumeTypePVC:
		if vol.PersistentVolumeClaim == nil {
			return fmt.Errorf("expected pvc volume but found different type")
		}
	default:
		return fmt.Errorf("unknown volume type %q in backup annotation", expectedType)
	}
	return nil
}

// restoreVolumeFromAnnotation parses the backup annotation and removes the injected volume.
func restoreVolumeFromAnnotation(podSpec *v1.PodSpec, annotations map[string]string) error {
	backupStr, ok := annotations[ChaosBladeOriginalVolumesAnnotation]
	if !ok || backupStr == "" {
		return fmt.Errorf("volume backup annotation not found")
	}

	var backup volumeBackup
	if err := json.Unmarshal([]byte(backupStr), &backup); err != nil {
		return fmt.Errorf("unmarshal volume backup failed: %v", err)
	}

	return removeInjectedVolume(podSpec, &backup)
}

// --- Deployment ---

func (d *FailedMountActionExecutor) injectDeploymentFailedMount(ctx context.Context, deployment *appsv1.Deployment, volumeType string, withInitContainer bool, experimentId string) error {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(deployment.Annotations, experimentId); err != nil {
		return err
	}
	if deployment.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	deployment.Annotations[ChaosBladeDeploymentAnnotation] = ChaosBladeFailedMountAction
	deployment.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if _, err := injectFailedMountVolume(&deployment.Spec.Template.Spec, deployment.Annotations, volumeType, withInitContainer); err != nil {
		return err
	}

	return d.client.Update(ctx, deployment)
}

func (d *FailedMountActionExecutor) restoreDeploymentVolumes(ctx context.Context, deployment *appsv1.Deployment, experimentId string) error {
	if deployment.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("deployment was not modified by experiment %s", experimentId)
	}

	if err := restoreVolumeFromAnnotation(&deployment.Spec.Template.Spec, deployment.Annotations); err != nil {
		return err
	}

	delete(deployment.Annotations, ChaosBladeDeploymentAnnotation)
	delete(deployment.Annotations, ChaosBladeExperimentAnnotation)
	delete(deployment.Annotations, ChaosBladeOriginalVolumesAnnotation)

	return d.client.Update(ctx, deployment)
}

// --- DaemonSet ---

func (d *FailedMountActionExecutor) injectDaemonSetFailedMount(ctx context.Context, daemonset *appsv1.DaemonSet, volumeType string, withInitContainer bool, experimentId string) error {
	if daemonset.Annotations == nil {
		daemonset.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(daemonset.Annotations, experimentId); err != nil {
		return err
	}
	if daemonset.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	daemonset.Annotations[ChaosBladeDaemonSetAnnotation] = ChaosBladeFailedMountAction
	daemonset.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if _, err := injectFailedMountVolume(&daemonset.Spec.Template.Spec, daemonset.Annotations, volumeType, withInitContainer); err != nil {
		return err
	}

	return d.client.Update(ctx, daemonset)
}

func (d *FailedMountActionExecutor) restoreDaemonSetVolumes(ctx context.Context, daemonset *appsv1.DaemonSet, experimentId string) error {
	if daemonset.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("daemonset was not modified by experiment %s", experimentId)
	}

	if err := restoreVolumeFromAnnotation(&daemonset.Spec.Template.Spec, daemonset.Annotations); err != nil {
		return err
	}

	delete(daemonset.Annotations, ChaosBladeDaemonSetAnnotation)
	delete(daemonset.Annotations, ChaosBladeExperimentAnnotation)
	delete(daemonset.Annotations, ChaosBladeOriginalVolumesAnnotation)

	return d.client.Update(ctx, daemonset)
}

// --- StatefulSet ---

func (d *FailedMountActionExecutor) injectStatefulSetFailedMount(ctx context.Context, statefulset *appsv1.StatefulSet, volumeType string, withInitContainer bool, experimentId string) error {
	if statefulset.Annotations == nil {
		statefulset.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(statefulset.Annotations, experimentId); err != nil {
		return err
	}
	if statefulset.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	statefulset.Annotations[ChaosBladeStatefulSetAnnotation] = ChaosBladeFailedMountAction
	statefulset.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if _, err := injectFailedMountVolume(&statefulset.Spec.Template.Spec, statefulset.Annotations, volumeType, withInitContainer); err != nil {
		return err
	}

	return d.client.Update(ctx, statefulset)
}

func (d *FailedMountActionExecutor) restoreStatefulSetVolumes(ctx context.Context, statefulset *appsv1.StatefulSet, experimentId string) error {
	if statefulset.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("statefulset was not modified by experiment %s", experimentId)
	}

	if err := restoreVolumeFromAnnotation(&statefulset.Spec.Template.Spec, statefulset.Annotations); err != nil {
		return err
	}

	delete(statefulset.Annotations, ChaosBladeStatefulSetAnnotation)
	delete(statefulset.Annotations, ChaosBladeExperimentAnnotation)
	delete(statefulset.Annotations, ChaosBladeOriginalVolumesAnnotation)

	return d.client.Update(ctx, statefulset)
}
