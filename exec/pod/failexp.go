/*
 * Copyright 1999-2020 Alibaba Group Holding Ltd.
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
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type FailPodActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewFailPodActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &FailPodActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{},
			},
			ActionExecutor: &FailPodActionExecutor{client: client},
			ActionExample: `# Specify POD exception
blade create k8s pod-pod fail --labels "app=test" --namespace default
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*FailPodActionSpec) Name() string {
	return "fail"
}

func (*FailPodActionSpec) Aliases() []string {
	return []string{}
}

func (*FailPodActionSpec) ShortDesc() string {
	return "Fail pods"
}

func (*FailPodActionSpec) LongDesc() string {
	return "Fail pods"
}

type FailPodActionExecutor struct {
	client *channel.Client
}

func (*FailPodActionExecutor) Name() string {
	return "fail"
}

func (*FailPodActionExecutor) SetChannel(channel spec.Channel) {
}

func (d *FailPodActionExecutor) Exec(uid string, ctx context.Context, model *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(ctx, model)
	} else {
		return d.create(ctx, model)
	}
}

func (d *FailPodActionExecutor) create(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)
	containerMatchedList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(experimentId, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWaitResult(spec.ParameterLess, fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].Err, "container object meta"),
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].ErrInfo, "container object meta"), nil))
	}
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	for _, c := range containerMatchedList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: c.GetIdentifier(),
		}
		objectMeta := types.NamespacedName{Name: c.PodName, Namespace: c.Namespace}
		pod := &v1.Pod{}
		err := d.client.Get(context.TODO(), objectMeta, pod)
		if err != nil {
			logrusField.Errorf("get pod %s err, %v", c.PodName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed)
		}

		if !isPodReady(pod) {
			logrusField.Infof("pod %s is not ready", c.PodName)
			statuses = append(statuses, status.CreateFailResourceStatus("pod is not read", spec.K8sExecFailed))
			continue
		}

		if err := d.failPod(ctx, pod); err != nil {
			logrusField.Warningf("fail pod %s err, %v", c.PodName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed)
		} else {
			status = status.CreateSuccessResourceStatus()
			success = true
		}
		statuses = append(statuses, status)
	}
	var experimentStatus v1alpha1.ExperimentStatus
	if success {
		experimentStatus = v1alpha1.CreateSuccessExperimentStatus(statuses)
	} else {
		experimentStatus = v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses)
	}
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

func (d *FailPodActionExecutor) destroy(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	containerMatchedList, err := model.GetContainerObjectMetaListFromContext(ctx)
	experimentId := model.GetExperimentIdFromContext(ctx)
	if err != nil {
		util.Errorf(experimentId, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWaitResult(spec.ParameterLess, fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].Err, "container object meta"),
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].ErrInfo, "container object meta"), nil))
	}
	logrusField := logrus.WithField("experiment", experimentId)
	experimentStatus := v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{})
	statuses := experimentStatus.ResStatuses
	for _, c := range containerMatchedList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: c.GetIdentifier(),
		}
		objectMeta := types.NamespacedName{Name: c.PodName, Namespace: c.Namespace}
		pod := &v1.Pod{}
		err := d.client.Get(context.TODO(), objectMeta, pod)
		if err != nil {
			logrusField.Errorf("get pod %s err, %v", c.PodName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed)
			continue
		}

		err = d.client.Delete(context.TODO(), pod)
		if err != nil {
			logrusField.Errorf("delete pod %s err, %v", c.PodName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed)
			continue
		}
	}
	experimentStatus.ResStatuses = statuses
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

// failPod will exec failPod experiment
func (d *FailPodActionExecutor) failPod(ctx context.Context, pod *v1.Pod) error {
	for i, container := range pod.Spec.Containers {
		key := fmt.Sprintf("%s-%s", "failPod", container.Name)
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		if isAnnotationExist(pod.Annotations, key) {
			continue
		}
		pod.Annotations[key] = container.Image
		pod.Spec.Containers[i].Image = fmt.Sprintf("%s-fault-injection", container.Image)
	}
	if err := d.client.Update(ctx, pod); err != nil {
		return err
	}
	return nil
}

// isAnnotationExist will check this pod has been tested
func isAnnotationExist(annotation map[string]string, key string) bool {
	_, ok := annotation[key]
	if !ok {
		return false
	}
	return true
}
