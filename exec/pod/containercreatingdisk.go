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
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

// PodContainerCreatingDiskActionSpec defines the action spec for containercreating-disk.
// It creates a PVC with a specified StorageClass (triggering cloud disk provisioning)
// and a Pod that mounts this PVC. When the cloud disk provisioner fails (due to zone
// mismatch, disk type not supported, or quota exceeded), the PVC remains Pending and
// the Pod is stuck in ContainerCreating.
type PodContainerCreatingDiskActionSpec struct {
	spec.BaseExpActionCommandSpec
	client *channel.Client
}

func NewPodContainerCreatingDiskActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodContainerCreatingDiskActionSpec{
		BaseExpActionCommandSpec: spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: "storage-class",
					Desc: "StorageClass name for PVC creation",
				},
				&spec.ExpFlag{
					Name: "pv-capacity",
					Desc: "PVC storage capacity, default: 20Gi",
				},
				&spec.ExpFlag{
					Name: "volume-mount-path",
					Desc: "Volume mount path in the container, default: /mnt/data",
				},
			},
			ActionExecutor: &PodContainerCreatingDiskActionExecutor{client: client},
			ActionExample: `# Create a pod stuck in ContainerCreating state by cloud disk PVC failure
blade create k8s pod-pod containercreating-disk --namespace default --storage-class alicloud-disk-ssd --kubeconfig ~/.kube/config

# Specify custom PV capacity
blade create k8s pod-pod containercreating-disk --namespace default --storage-class alicloud-disk-ssd --pv-capacity 50Gi --kubeconfig ~/.kube/config

# Specify custom volume mount path
blade create k8s pod-pod containercreating-disk --namespace default --storage-class alicloud-disk-ssd --volume-mount-path /data --kubeconfig ~/.kube/config`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
		client: client,
	}
}

func (*PodContainerCreatingDiskActionSpec) Name() string {
	return "containercreating-disk"
}

func (*PodContainerCreatingDiskActionSpec) Aliases() []string {
	return []string{}
}

func (*PodContainerCreatingDiskActionSpec) ShortDesc() string {
	return "Make pod stuck in ContainerCreating state by cloud disk PVC creation failure"
}

func (*PodContainerCreatingDiskActionSpec) LongDesc() string {
	return "Simulate the scenario where a Pod is stuck in ContainerCreating state due to cloud disk PVC creation failure. " +
		"This fault is injected by creating a PVC with the specified StorageClass (which triggers cloud disk provisioning), " +
		"then creating a Pod that mounts this PVC. When the cloud disk provisioner fails (due to zone mismatch, " +
		"disk type not supported, or quota exceeded), the PVC remains Pending and the Pod is stuck in ContainerCreating. " +
		"When the experiment is destroyed, the created Pod and PVC will be cleaned up."
}

// PodContainerCreatingDiskActionExecutor implements the create and destroy logic
// for the containercreating-disk action.
type PodContainerCreatingDiskActionExecutor struct {
	client *channel.Client
}

func (*PodContainerCreatingDiskActionExecutor) Name() string {
	return "containercreating-disk"
}

func (*PodContainerCreatingDiskActionExecutor) SetChannel(channel spec.Channel) {}

func (d *PodContainerCreatingDiskActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *PodContainerCreatingDiskActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	// Parse flags with defaults
	storageClass := expModel.ActionFlags["storage-class"]
	pvCapacity := expModel.ActionFlags["pv-capacity"]
	if pvCapacity == "" {
		pvCapacity = "20Gi"
	}
	volumeMountPath := expModel.ActionFlags["volume-mount-path"]
	if volumeMountPath == "" {
		volumeMountPath = "/mnt/data"
	}

	// Deduplicate by namespace - create one PVC+Pod per unique namespace
	seenNamespaces := make(map[string]bool)
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false

	for _, meta := range containerObjectMetaList {
		if seenNamespaces[meta.Namespace] {
			continue
		}
		seenNamespaces[meta.Namespace] = true

		pvcName := fmt.Sprintf("chaosblade-ccd-%s-pvc", experimentId)
		podName := fmt.Sprintf("chaosblade-ccd-%s-pod", experimentId)

		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: fmt.Sprintf("%s//%s", meta.Namespace, podName),
		}

		// Step 1: Create PVC with the specified StorageClass.
		// In clusters without a cloud disk provisioner, the PVC will remain Pending.
		if err := d.createPVC(ctx, meta.Namespace, pvcName, storageClass, pvCapacity, experimentId); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrusField.Infof("PVC %s/%s already exists, skip creation", meta.Namespace, pvcName)
			} else {
				logrusField.Warningf("create PVC %s/%s failed: %v", meta.Namespace, pvcName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("create PVC failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				continue
			}
		} else {
			logrusField.Infof("created PVC %s/%s with StorageClass %s", meta.Namespace, pvcName, storageClass)
		}

		// Step 2: Create Pod that mounts the PVC.
		// The Pod will be stuck in ContainerCreating since the PVC is not bound.
		if err := d.createPod(ctx, meta.Namespace, podName, pvcName, volumeMountPath, experimentId); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrusField.Infof("Pod %s/%s already exists, skip creation", meta.Namespace, podName)
			} else {
				logrusField.Warningf("create Pod %s/%s failed: %v", meta.Namespace, podName, err)
				// Best-effort rollback: delete the PVC we just created.
				// If rollback fails, resources will be leaked because we record
				// a failed status and Destroy only processes successful ones.
				// To prevent leaks, we still record success so Destroy will
				// attempt cleanup (destroy is idempotent and handles NotFound).
				pvcDeleted := true
				if delErr := d.deletePVC(ctx, meta.Namespace, pvcName); delErr != nil {
					logrusField.Warningf("rollback PVC %s/%s failed: %v", meta.Namespace, pvcName, delErr)
					pvcDeleted = false
				}
				if pvcDeleted {
					status = status.CreateFailResourceStatus(fmt.Sprintf("create Pod failed: %v", err), spec.K8sExecFailed.Code)
					statuses = append(statuses, status)
				} else {
					logrusField.Warningf("rollback incomplete, recording success status to ensure Destroy can clean up")
					status = status.CreateSuccessResourceStatus()
					statuses = append(statuses, status)
					success = true
				}
				continue
			}
		} else {
			logrusField.Infof("created Pod %s/%s which will be stuck in ContainerCreating state", meta.Namespace, podName)
		}

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

func (d *PodContainerCreatingDiskActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	statuses := make([]v1alpha1.ResourceStatus, 0)
	allSuccess := true
	seenNamespaces := make(map[string]bool)

	for _, meta := range containerObjectMetaList {
		if seenNamespaces[meta.Namespace] {
			continue
		}
		seenNamespaces[meta.Namespace] = true

		pvcName := fmt.Sprintf("chaosblade-ccd-%s-pvc", experimentId)
		podName := fmt.Sprintf("chaosblade-ccd-%s-pod", experimentId)
		namespace := meta.Namespace

		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: fmt.Sprintf("%s//%s", namespace, podName),
		}

		// Step 1: Delete Pod
		if err := d.deletePod(ctx, namespace, podName); err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("Pod %s/%s already deleted", namespace, podName)
			} else {
				logrusField.Warningf("delete Pod %s/%s failed: %v", namespace, podName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("delete Pod failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				allSuccess = false
				continue
			}
		} else {
			logrusField.Infof("deleted Pod %s/%s", namespace, podName)
		}

		// Step 2: Delete PVC
		if err := d.deletePVC(ctx, namespace, pvcName); err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("PVC %s/%s already deleted", namespace, pvcName)
			} else {
				logrusField.Warningf("delete PVC %s/%s failed: %v", namespace, pvcName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("delete PVC failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				allSuccess = false
				continue
			}
		} else {
			logrusField.Infof("deleted PVC %s/%s", namespace, pvcName)
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

// createPVC creates a PersistentVolumeClaim with the specified StorageClass.
// The PVC will remain Pending if the cloud disk provisioner is unavailable or
// misconfigured (zone mismatch, disk type not supported, quota exceeded).
func (d *PodContainerCreatingDiskActionExecutor) createPVC(ctx context.Context, namespace, pvcName, storageClass, pvCapacity, experimentId string) error {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Annotations: map[string]string{
				ChaosBladePVCAnnotation:        ChaosBladeActionCreate,
				ChaosBladeExperimentAnnotation: experimentId,
			},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClass,
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse(pvCapacity),
				},
			},
		},
	}
	return d.client.Create(ctx, pvc)
}

// createPod creates a Pod that mounts the given PVC, which will cause it to be
// stuck in ContainerCreating state because the PVC is not bound (cloud disk
// provisioner failure).
func (d *PodContainerCreatingDiskActionExecutor) createPod(ctx context.Context, namespace, podName, pvcName, volumeMountPath, experimentId string) error {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Annotations: map[string]string{
				ChaosBladePodAnnotation:        ChaosBladeActionCreate,
				ChaosBladeExperimentAnnotation: experimentId,
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    "chaosblade-ccd-container",
					Image:   "busybox",
					Command: []string{"sleep", "infinity"},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "chaosblade-ccd-volume",
							MountPath: volumeMountPath,
						},
					},
				},
			},
			Tolerations: []v1.Toleration{
				{
					Operator: v1.TolerationOpExists,
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "chaosblade-ccd-volume",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}
	return d.client.Create(ctx, pod)
}

// deletePod deletes a Pod by namespace and name.
func (d *PodContainerCreatingDiskActionExecutor) deletePod(ctx context.Context, namespace, podName string) error {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
	}
	return d.client.Delete(ctx, pod)
}

// deletePVC deletes a PersistentVolumeClaim by namespace and name.
func (d *PodContainerCreatingDiskActionExecutor) deletePVC(ctx context.Context, namespace, pvcName string) error {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
	}
	return d.client.Delete(ctx, pvc)
}

// PreCreate implements model.ActionPreProcessor interface.
// It validates namespace and storage-class, and prepares the context for
// containercreating-disk action.
func (a *PodContainerCreatingDiskActionSpec) PreCreate(ctx context.Context, expModel *spec.ExpModel, client *channel.Client) (context.Context, *spec.Response) {
	experimentId := model.GetExperimentIdFromContext(ctx)

	// Validate namespace: must be specified and only one value
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	if namespace == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if strings.Contains(namespace, ",") {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, model.ResourceNamespaceFlag.Name)
	}

	// Validate storage-class: must be specified
	storageClass := expModel.ActionFlags["storage-class"]
	if storageClass == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "storage-class")
	}

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-ccd-%s-pod", experimentId),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}

// PreDestroy implements model.ActionPreProcessor interface.
// It prepares the context for containercreating-disk destroy flow.
func (a *PodContainerCreatingDiskActionSpec) PreDestroy(ctx context.Context, expModel *spec.ExpModel, client *channel.Client, oldExpStatus v1alpha1.ExperimentStatus) (context.Context, *spec.Response) {
	experimentId := model.GetExperimentIdFromContext(ctx)
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-ccd-%s-pod", experimentId),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}
