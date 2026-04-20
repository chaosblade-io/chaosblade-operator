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
	"time"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	// ChaosBladePVAnnotation is the annotation for PV resources created by containercreating action
	ChaosBladePVAnnotation = "chaosblade.io/pv"
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
	client *channel.Client
}

func NewPodContainerCreatingActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodContainerCreatingActionSpec{
		BaseExpActionCommandSpec: spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: "volume-mount-path",
					Desc: "Volume mount path in the container. Default: /mnt/data",
				},
			},
			ActionExecutor: &PodContainerCreatingActionExecutor{client: client},
			ActionExample: `# Create a pod stuck in ContainerCreating state in the default namespace
blade create k8s pod-pod containercreating --namespace default --kubeconfig ~/.kube/config

# Create a pod stuck in ContainerCreating state with custom volume mount path
blade create k8s pod-pod containercreating --namespace default --volume-mount-path /data --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
		client: client,
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
		"This fault is injected by creating a PV with an unreachable NFS server and a PVC bound to it, " +
		"then creating a Pod that mounts this PVC. Since the NFS server is unreachable, the volume mount fails " +
		"and the Pod remains stuck in ContainerCreating state. " +
		"When the experiment is destroyed, the created Pod, PVC, and PV will be cleaned up."
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
	volumeMountPath := expModel.ActionFlags["volume-mount-path"]
	if volumeMountPath == "" {
		volumeMountPath = "/mnt/data"
	}

	// Deduplicate by namespace - create one faulty PV+PVC+Pod per unique namespace
	seenNamespaces := make(map[string]bool)
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false

	for _, meta := range containerObjectMetaList {
		if seenNamespaces[meta.Namespace] {
			continue
		}
		seenNamespaces[meta.Namespace] = true

		pvName := fmt.Sprintf("chaosblade-cc-%s-pv", experimentId)
		pvcName := fmt.Sprintf("chaosblade-cc-%s-pvc", experimentId)
		podName := fmt.Sprintf("chaosblade-cc-%s-pod", experimentId)

		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: fmt.Sprintf("%s//%s", meta.Namespace, podName),
		}

		// Step 1: Create PV with unreachable NFS server
		if err := d.createPV(ctx, pvName, experimentId); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrusField.Infof("PV %s already exists, skip creation", pvName)
			} else {
				logrusField.Warningf("create PV %s failed: %v", pvName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("create PV failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				continue
			}
		} else {
			logrusField.Infof("created PV %s with unreachable NFS server", pvName)
		}

		// Step 2: Create PVC bound to the PV (PVC will be Bound, but mount will fail)
		if err := d.createPVC(ctx, meta.Namespace, pvcName, pvName, experimentId); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrusField.Infof("PVC %s/%s already exists, skip creation", meta.Namespace, pvcName)
			} else {
				logrusField.Warningf("create PVC %s/%s failed: %v", meta.Namespace, pvcName, err)
				// Best-effort rollback: delete the PV we just created.
				// If rollback fails, the PV will be leaked because we record
				// a failed status and Destroy only processes successful ones.
				// To prevent leaks, record success so Destroy will retry cleanup.
				pvDeleted := true
				if delErr := d.deletePV(ctx, pvName); delErr != nil {
					logrusField.Warningf("rollback PV %s failed: %v", pvName, delErr)
					pvDeleted = false
				}
				if pvDeleted {
					status = status.CreateFailResourceStatus(fmt.Sprintf("create PVC failed: %v", err), spec.K8sExecFailed.Code)
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
			logrusField.Infof("created PVC %s/%s bound to PV %s", meta.Namespace, pvcName, pvName)
		}

		// Step 3: Wait for PVC to be Bound before creating Pod
		if err := d.waitForPVCBound(ctx, meta.Namespace, pvcName, 30*time.Second); err != nil {
			logrusField.Warningf("PVC %s/%s is not bound yet: %v", meta.Namespace, pvcName, err)
		}

		// Step 4: Create Pod that mounts the PVC (will be stuck in ContainerCreating)
		if err := d.createPod(ctx, meta.Namespace, podName, pvcName, volumeMountPath, experimentId); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrusField.Infof("Pod %s/%s already exists, skip creation", meta.Namespace, podName)
			} else {
				logrusField.Warningf("create Pod %s/%s failed: %v", meta.Namespace, podName, err)
				// Best-effort rollback: delete PVC and PV.
				// If rollback fails, resources will be leaked because we record
				// a failed status and Destroy only processes successful ones.
				// To prevent leaks, we still record success so Destroy will
				// attempt cleanup (destroy is idempotent and handles NotFound).
				pvcDeleted := false
				if delErr := d.deletePVC(ctx, meta.Namespace, pvcName); delErr != nil {
					logrusField.Warningf("rollback PVC %s/%s failed: %v", meta.Namespace, pvcName, delErr)
				} else {
					pvcDeleted = true
				}
				pvDeleted := true
				if delErr := d.deletePV(ctx, pvName); delErr != nil {
					logrusField.Warningf("rollback PV %s failed: %v", pvName, delErr)
					pvDeleted = false
				}
				// If rollback fully succeeded, record failure (no leaked resources).
				// If any rollback step failed, record success so Destroy will retry cleanup.
				if pvcDeleted && pvDeleted {
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

		pvName := fmt.Sprintf("chaosblade-cc-%s-pv", experimentId)
		pvcName := fmt.Sprintf("chaosblade-cc-%s-pvc", experimentId)
		podName := fmt.Sprintf("chaosblade-cc-%s-pod", experimentId)
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

		// Step 3: Delete PV
		if err := d.deletePV(ctx, pvName); err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("PV %s already deleted", pvName)
			} else {
				logrusField.Warningf("delete PV %s failed: %v", pvName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("delete PV failed: %v", err), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				allSuccess = false
				continue
			}
		} else {
			logrusField.Infof("deleted PV %s", pvName)
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

// createPV creates a PersistentVolume with an unreachable NFS server.
// The PV will be Available, allowing PVC binding, but the NFS mount will fail
// when a Pod tries to use it, causing the Pod to be stuck in ContainerCreating.
func (d *PodContainerCreatingActionExecutor) createPV(ctx context.Context, pvName, experimentId string) error {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Annotations: map[string]string{
				ChaosBladePVAnnotation:         ChaosBladeActionCreate,
				ChaosBladeExperimentAnnotation: experimentId,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse("1Gi"),
			},
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
			PersistentVolumeSource: v1.PersistentVolumeSource{
				NFS: &v1.NFSVolumeSource{
					// Use a non-routable IP address to simulate unreachable NFS server
					Server:   "10.255.255.1",
					Path:     "/chaosblade-fake-nfs",
					ReadOnly: false,
				},
			},
		},
	}
	return d.client.Create(ctx, pv)
}

// createPVC creates a PersistentVolumeClaim that binds to the specified PV.
// The PVC will be Bound to the PV, but the actual volume mount will fail
// because the NFS server is unreachable.
func (d *PodContainerCreatingActionExecutor) createPVC(ctx context.Context, namespace, pvcName, pvName, experimentId string) error {
	emptyStr := ""
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
			StorageClassName: &emptyStr,
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
			VolumeName: pvName,
		},
	}
	return d.client.Create(ctx, pvc)
}

// createPod creates a Pod that mounts the given PVC, which will cause it to be
// stuck in ContainerCreating state because the NFS mount fails.
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
			// Tolerate all taints so the Pod can be scheduled on any node
			Tolerations: []v1.Toleration{
				{
					Operator: v1.TolerationOpExists,
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

// waitForPVCBound polls until the PVC is in Bound state or timeout is reached
func (d *PodContainerCreatingActionExecutor) waitForPVCBound(ctx context.Context, namespace, pvcName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pvc := &v1.PersistentVolumeClaim{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: pvcName}, pvc)
		if err != nil {
			return err
		}
		if pvc.Status.Phase == v1.ClaimBound {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("PVC %s/%s is not bound after %v", namespace, pvcName, timeout)
}

// deletePV deletes a PersistentVolume by name
func (d *PodContainerCreatingActionExecutor) deletePV(ctx context.Context, pvName string) error {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
	}
	return d.client.Delete(ctx, pv)
}

// PreCreate implements model.ActionPreProcessor interface.
// It validates the namespace and prepares the context for containercreating action.
func (a *PodContainerCreatingActionSpec) PreCreate(ctx context.Context, expModel *spec.ExpModel, client *channel.Client) (context.Context, *spec.Response) {
	experimentId := model.GetExperimentIdFromContext(ctx)

	// Validate namespace: must be specified and only one value
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	if namespace == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if strings.Contains(namespace, ",") {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, model.ResourceNamespaceFlag.Name)
	}

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-cc-%s-pod", experimentId),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}

// PreDestroy implements model.ActionPreProcessor interface.
// It prepares the context for containercreating destroy flow.
func (a *PodContainerCreatingActionSpec) PreDestroy(ctx context.Context, expModel *spec.ExpModel, client *channel.Client, oldExpStatus v1alpha1.ExperimentStatus) (context.Context, *spec.Response) {
	experimentId := model.GetExperimentIdFromContext(ctx)
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-cc-%s-pod", experimentId),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}
