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
	podObjectMetaList, err := model.ExtractPodObjectMetasFromContext(ctx)
	if err != nil {
		return spec.ReturnFailWitResult(spec.Code[spec.IllegalParameters], err.Error(),
			v1alpha1.CreateFailExperimentStatus(err.Error(), nil))
	}
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	for _, meta := range podObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Uid:      meta.Uid,
			Name:     meta.Name,
			Kind:     v1alpha1.PodKind,
			NodeName: meta.NodeName,
		}

		objectMeta := types.NamespacedName{Name: meta.Name, Namespace: meta.Namespace}
		pod := &v1.Pod{}
		err := d.client.Get(context.TODO(),objectMeta, pod)
		if err != nil {
			logrus.Errorf("get pod %s err, %v", meta.Name, err)
			status = status.CreateFailResourceStatus(err.Error())
		}

		if !isPodReady(pod) {
			logrus.Infof("pod %s is not ready", meta.Name)
			statuses = append(statuses, status.CreateFailResourceStatus("pod is not read"))
			continue
		}

		if err := d.failPod(ctx, pod); err != nil {
			logrus.Warningf("fail pod %s err, %v", meta.Name, err)
			status = status.CreateFailResourceStatus(err.Error())
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
	logrus.Info("start destroy pod inject")
	podObjectMetaList, err := model.ExtractPodObjectMetasFromContext(ctx)
	logrus.Infof("podObjectMetaList: %v", podObjectMetaList)
	if err != nil {
		return spec.ReturnFailWitResult(spec.Code[spec.IllegalParameters], err.Error(),
			v1alpha1.CreateFailExperimentStatus(err.Error(), nil))
	}
	experimentStatus := v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{})
	statuses := experimentStatus.ResStatuses
	for _, meta := range podObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Uid:      meta.Uid,
			Name:     meta.Name,
			Kind:     v1alpha1.PodKind,
			NodeName: meta.NodeName,
		}
		objectMeta := types.NamespacedName{Name: meta.Name, Namespace: meta.Namespace}
		pod := &v1.Pod{}
		err := d.client.Get(context.TODO(),objectMeta, pod)
		if err != nil {
			logrus.Errorf("get pod %s err, %v", meta.Name, err)
			status = status.CreateFailResourceStatus(err.Error())
			continue
		}

		err = d.client.Delete(context.TODO(), pod)
		if err != nil {
			logrus.Errorf("delete pod %s err, %v", meta.Name, err)
			status = status.CreateFailResourceStatus(err.Error())
			continue
		}
	}
	experimentStatus.ResStatuses = statuses
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

// failPod will exec failPod experiment
func (d *FailPodActionExecutor) failPod(ctx context.Context,  pod *v1.Pod) error {
	for i, container := range pod.Spec.Containers{
		key := fmt.Sprintf("%s-%s","failPod", container.Name)
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
func isAnnotationExist(annotation map[string]string, key string)  bool {
	_, ok := annotation[key]
	if !ok {
		return false
	}
	return true
}