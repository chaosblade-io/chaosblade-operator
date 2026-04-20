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
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	// PodTerminatingFinalizer is the finalizer added to the target pod to block its deletion
	PodTerminatingFinalizer = "chaosblade.io/pod-terminating"
)

type PodTerminatingActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewPodTerminatingActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodTerminatingActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags:    []spec.ExpFlagSpec{},
			ActionExecutor: &PodTerminatingActionExecutor{client: client},
			ActionExample: `# Make the pod stuck in Terminating state in the default namespace
blade create k8s pod-pod terminating --names nginx-app --namespace default --kubeconfig ~/.kube/config

# Make pods stuck in Terminating state by labels
blade create k8s pod-pod terminating --labels app=guestbook --namespace default --evict-count 2 --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*PodTerminatingActionSpec) Name() string {
	return "terminating"
}

func (*PodTerminatingActionSpec) Aliases() []string {
	return []string{}
}

func (*PodTerminatingActionSpec) ShortDesc() string {
	return "Make pod stuck in Terminating state by adding a finalizer"
}

func (*PodTerminatingActionSpec) LongDesc() string {
	return "Simulate the scenario where a Pod is stuck in Terminating state due to uncleaned finalizers. " +
		"This fault injects by adding a custom finalizer to the target Pod and then deleting it, " +
		"which causes the Pod to remain in Terminating state because the finalizer prevents garbage collection. " +
		"When the experiment is destroyed, the finalizer will be removed so the Pod can be fully deleted."
}

type PodTerminatingActionExecutor struct {
	client *channel.Client
}

func (*PodTerminatingActionExecutor) Name() string {
	return "terminating"
}

func (*PodTerminatingActionExecutor) SetChannel(channel spec.Channel) {}

func (d *PodTerminatingActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *PodTerminatingActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false

	for _, meta := range containerObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: meta.GetIdentifier(),
		}

		pod := &v1.Pod{}
		err := d.client.Get(ctx, types.NamespacedName{Name: meta.PodName, Namespace: meta.Namespace}, pod)
		if err != nil {
			logrusField.Warningf("get pod %s/%s err, %v", meta.Namespace, meta.PodName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		// Skip if pod is already being deleted
		if pod.DeletionTimestamp != nil {
			logrusField.Warningf("pod %s/%s is already terminating, cannot inject fault", meta.Namespace, meta.PodName)
			status = status.CreateFailResourceStatus("pod is already in Terminating state, no fault injected", spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		// Step 1: Add the finalizer to the pod
		if err := d.addFinalizer(ctx, pod); err != nil {
			logrusField.Warningf("add finalizer to pod %s/%s err, %v", meta.Namespace, meta.PodName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("add finalizer failed: %v", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		// Step 2: Delete the pod, it will be stuck in Terminating because of the finalizer
		if err := d.client.Delete(ctx, pod); err != nil {
			logrusField.Warningf("delete pod %s/%s err, %v", meta.Namespace, meta.PodName, err)
			// Best-effort rollback: remove the finalizer we just added
			if rbErr := d.removeFinalizer(ctx, pod); rbErr != nil {
				logrusField.Warningf("rollback finalizer for pod %s/%s failed: %v", meta.Namespace, meta.PodName, rbErr)
			}
			status = status.CreateFailResourceStatus(fmt.Sprintf("delete pod failed: %v", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		logrusField.Infof("pod %s/%s is now stuck in Terminating state with finalizer %s",
			meta.Namespace, meta.PodName, PodTerminatingFinalizer)

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

func (d *PodTerminatingActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
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

	for _, meta := range containerObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: meta.GetIdentifier(),
		}

		pod := &v1.Pod{}
		err := d.client.Get(ctx, types.NamespacedName{Name: meta.PodName, Namespace: meta.Namespace}, pod)
		if err != nil {
			// Distinguish between NotFound and other errors (RBAC, API server unreachable, etc.)
			if apierrors.IsNotFound(err) {
				// Pod is already fully deleted, treat as success
				logrusField.Infof("pod %s/%s already deleted", meta.Namespace, meta.PodName)
				status = status.CreateSuccessResourceStatus()
				status.State = v1alpha1.DestroyedState
			} else {
				// Other errors mean the finalizer may still be present
				logrusField.Warningf("get pod %s/%s err, %v", meta.Namespace, meta.PodName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get pod failed: %v", err), spec.K8sExecFailed.Code)
				allSuccess = false
			}
			statuses = append(statuses, status)
			continue
		}

		// Remove the finalizer to allow the pod to be fully deleted
		if err := d.removeFinalizer(ctx, pod); err != nil {
			logrusField.Warningf("remove finalizer from pod %s/%s err, %v", meta.Namespace, meta.PodName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("remove finalizer failed: %v", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			allSuccess = false
			continue
		}

		logrusField.Infof("removed finalizer from pod %s/%s, pod will be fully deleted",
			meta.Namespace, meta.PodName)

		status = status.CreateSuccessResourceStatus()
		status.State = v1alpha1.DestroyedState
		statuses = append(statuses, status)
	}

	if allSuccess {
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus(statuses))
	}
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses))
}

// addFinalizer adds the chaosblade pod-terminating finalizer to the pod
func (d *PodTerminatingActionExecutor) addFinalizer(ctx context.Context, pod *v1.Pod) error {
	finalizers := pod.GetFinalizers()
	for _, f := range finalizers {
		if f == PodTerminatingFinalizer {
			// Finalizer already exists
			return nil
		}
	}
	pod.SetFinalizers(append(finalizers, PodTerminatingFinalizer))
	return d.client.Update(ctx, pod)
}

// removeFinalizer removes the chaosblade pod-terminating finalizer from the pod
func (d *PodTerminatingActionExecutor) removeFinalizer(ctx context.Context, pod *v1.Pod) error {
	finalizers := pod.GetFinalizers()
	newFinalizers := make([]string, 0, len(finalizers))
	found := false
	for _, f := range finalizers {
		if f == PodTerminatingFinalizer {
			found = true
			continue
		}
		newFinalizers = append(newFinalizers, f)
	}
	if !found {
		// Finalizer not found, nothing to remove
		return nil
	}
	pod.SetFinalizers(newFinalizers)
	return d.client.Update(ctx, pod)
}
