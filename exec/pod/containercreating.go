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

const (
	// ChaosBladePVCAnnotation is the annotation for PVC resources created by containercreating action
	ChaosBladePVCAnnotation = "chaosblade.io/pvc"
	// ChaosBladePodAnnotation is the annotation for Pod resources created by containercreating action
	ChaosBladePodAnnotation = "chaosblade.io/pod"
	// ChaosBladeExperimentAnnotation is the annotation key for experiment ID
	ChaosBladeExperimentAnnotation = "chaosblade.io/experiment"
	// ChaosBladeActionCreate is the annotation value for create action
	ChaosBladeActionCreate = "create"
)

type PodContainerCreatingActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewPodContainerCreatingActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodContainerCreatingActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: "storage-class",
					Desc: "StorageClass name for the faulty PVC, the PVC will be pending if the StorageClass does not exist. Default: chaosblade-fake-sc",
				},
				&spec.ExpFlag{
					Name: "volume-mount-path",
					Desc: "Volume mount path in the container. Default: /mnt/data",
				},
				&spec.ExpFlag{
					Name: "access-mode",
					Desc: "PVC access mode, values: ReadWriteOnce, ReadOnlyMany, ReadWriteMany. Default: ReadWriteOnce",
				},
				&spec.ExpFlag{
					Name: "storage-request",
					Desc: "PVC storage request size. Default: 1Gi",
				},
				&spec.ExpFlag{
					Name:   "random",
					Desc:   "Randomly select pod",
					NoArgs: true,
				},
			},
			ActionExecutor: &PodContainerCreatingActionExecutor{client: client},
			ActionExample: `# Create a pod stuck in ContainerCreating state in the default namespace
blade create k8s pod-pod containercreating --names nginx-app --namespace default --kubeconfig ~/.kube/config

# Create a pod stuck in ContainerCreating state by labels with custom StorageClass
blade create k8s pod-pod containercreating --labels app=guestbook --namespace default --storage-class fake-storage --kubeconfig ~/.kube/config

# Create a pod stuck in ContainerCreating state with custom PVC parameters
blade create k8s pod-pod containercreating --names nginx-app --namespace default --access-mode ReadWriteMany --storage-request 5Gi --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*PodContainerCreatingActionSpec) Name() string {
	return "containercreating"
}

func (*PodContainerCreatingActionSpec) Aliases() []string {
	return []string{}
}

func (*PodContainerCreatingActionSpec) ShortDesc() string {
	return "Make pod stuck in ContainerCreating state by PVC mount failure"
}

func (*PodContainerCreatingActionSpec) LongDesc() string {
	return "Simulate the scenario where a Pod is stuck in ContainerCreating state due to storage volume mount failure. " +
		"This fault is injected by creating a PVC that references a non-existent StorageClass (which keeps the PVC in Pending state), " +
		"then creating a Pod that mounts this PVC. Since the PVC cannot be bound, the Pod remains stuck in ContainerCreating state. " +
		"When the experiment is destroyed, the created Pod and PVC will be cleaned up."
}

type PodContainerCreatingActionExecutor struct {
	client *channel.Client
}

func (*PodContainerCreatingActionExecutor) Name() string {
	return "containercreating"
}

func (*PodContainerCreatingActionExecutor) SetChannel(channel spec.Channel) {}

func (d *PodContainerCreatingActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *PodContainerCreatingActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
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
	if storageClass == "" {
		storageClass = "chaosblade-fake-sc"
	}
	volumeMountPath := expModel.ActionFlags["volume-mount-path"]
	if volumeMountPath == "" {
		volumeMountPath = "/mnt/data"
	}
	accessMode := expModel.ActionFlags["access-mode"]
	if accessMode == "" {
		accessMode = string(v1.ReadWriteOnce)
	}
	storageRequest := expModel.ActionFlags["storage-request"]
	if storageRequest == "" {
		storageRequest = "1Gi"
	}

	// Validate storage request format
	storageQuantity, err := resource.ParseQuantity(storageRequest)
	if err != nil {
		errMsg := fmt.Sprintf("invalid storage-request %q: %v", storageRequest, err)
		logrusField.Errorln(errMsg)
		return spec.ResponseFailWithResult(spec.ParameterIllegal,
			v1alpha1.CreateFailExperimentStatus(errMsg, []v1alpha1.ResourceStatus{}),
			"storage-request", storageRequest, err)
	}

	// Deduplicate by namespace - create one faulty Pod+PVC per unique namespace
	seenNamespaces := make(map[string]bool)
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false

	for _, meta := range containerObjectMetaList {
		if seenNamespaces[meta.Namespace] {
			continue
		}
		seenNamespaces[meta.Namespace] = true

		pvcName := fmt.Sprintf("chaosblade-cc-%s-pvc", experimentId)
		podName := fmt.Sprintf("chaosblade-cc-%s-pod", experimentId)

		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: fmt.Sprintf("%s//%s", meta.Namespace, podName),
		}

		// Step 1: Create PVC referencing non-existent StorageClass
		if err := d.createPVC(ctx, meta.Namespace, pvcName, storageClass, accessMode, storageQuantity, experimentId); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrusField.Infof("PVC %s/%s already exists, skip creation", meta.Namespace, pvcName)
			} else {
				logrusField.Warningf("create PVC %s/%s failed: %v", meta.Namespace, pvcName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("create PVC failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				continue
			}
		} else {
			logrusField.Infof("created PVC %s/%s referencing non-existent StorageClass %s", meta.Namespace, pvcName, storageClass)
		}

		// Step 2: Create Pod that mounts the pending PVC
		if err := d.createPod(ctx, meta.Namespace, podName, pvcName, volumeMountPath, experimentId); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrusField.Infof("Pod %s/%s already exists, skip creation", meta.Namespace, podName)
			} else {
				logrusField.Warningf("create Pod %s/%s failed: %v", meta.Namespace, podName, err)
				// Best-effort rollback: delete the PVC we just created
				if delErr := d.deletePVC(ctx, meta.Namespace, pvcName); delErr != nil {
					logrusField.Warningf("rollback PVC %s/%s failed: %v", meta.Namespace, pvcName, delErr)
				}
				status = status.CreateFailResourceStatus(fmt.Sprintf("create Pod failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
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

func (d *PodContainerCreatingActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
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

		pvcName := fmt.Sprintf("chaosblade-cc-%s-pvc", experimentId)
		podName := meta.PodName
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

		// Step 2: Delete PVC (non-critical, warn only on failure)
		if err := d.deletePVC(ctx, namespace, pvcName); err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("PVC %s/%s already deleted", namespace, pvcName)
			} else {
				logrusField.Warningf("delete PVC %s/%s failed: %v", namespace, pvcName, err)
				// PVC deletion failure is not critical, just warn
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

// createPVC creates a PersistentVolumeClaim that references a non-existent StorageClass,
// which will cause it to remain in Pending state indefinitely.
func (d *PodContainerCreatingActionExecutor) createPVC(ctx context.Context, namespace, pvcName, storageClass, accessMode string, storageQuantity resource.Quantity, experimentId string) error {
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
				v1.PersistentVolumeAccessMode(accessMode),
			},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: storageQuantity,
				},
			},
		},
	}
	return d.client.Create(ctx, pvc)
}

// createPod creates a Pod that mounts the given PVC, which will cause it to be
// stuck in ContainerCreating state because the PVC is Pending.
func (d *PodContainerCreatingActionExecutor) createPod(ctx context.Context, namespace, podName, pvcName, volumeMountPath, experimentId string) error {
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
					Name:  "chaosblade-cc",
					Image: "busybox",
					Command: []string{
						"sleep",
						"infinity",
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "chaosblade-cc-volume",
							MountPath: volumeMountPath,
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "chaosblade-cc-volume",
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

// deletePod deletes a Pod by namespace and name
func (d *PodContainerCreatingActionExecutor) deletePod(ctx context.Context, namespace, podName string) error {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
	}
	return d.client.Delete(ctx, pod)
}

// deletePVC deletes a PersistentVolumeClaim by namespace and name
func (d *PodContainerCreatingActionExecutor) deletePVC(ctx context.Context, namespace, pvcName string) error {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
	}
	return d.client.Delete(ctx, pvc)
}
