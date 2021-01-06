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

	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type DeletePodActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewDeletePodActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &DeletePodActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:   "random",
					Desc:   "Randomly select pod",
					NoArgs: true,
				},
			},
			ActionExecutor: &DeletePodActionExecutor{client: client},
			ActionExample:
			`# Deletes the POD under the specified default namespace that is app=guestbook
blade create k8s pod-pod delete --labels app=guestbook --namespace default --evict-count 2 --kubeconfig ~/.kube/config`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*DeletePodActionSpec) Name() string {
	return "delete"
}

func (*DeletePodActionSpec) Aliases() []string {
	return []string{}
}

func (*DeletePodActionSpec) ShortDesc() string {
	return "Delete pods"
}

func (*DeletePodActionSpec) LongDesc() string {
	return "Delete pods"
}

type DeletePodActionExecutor struct {
	client *channel.Client
}

func (*DeletePodActionExecutor) Name() string {
	return "delete"
}

func (*DeletePodActionExecutor) SetChannel(channel spec.Channel) {
}

func (d *DeletePodActionExecutor) Exec(uid string, ctx context.Context, model *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, model)
	} else {
		return d.create(uid, ctx, model)
	}
}

func (d *DeletePodActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWaitResult(spec.ParameterLess, fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].Err, "container object meta"),
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].ErrInfo, "container object meta"), nil))
	}
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	for _, meta := range containerObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: fmt.Sprintf("%s/%s/%s", meta.Namespace, meta.NodeName, meta.PodName),
		}
		objectMeta := metav1.ObjectMeta{Name: meta.PodName, Namespace: meta.Namespace}
		err := d.client.Delete(context.TODO(), &v1.Pod{ObjectMeta: objectMeta})
		if err != nil {
			logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx)).
				Warningf("delete pod %s err, %v", meta.PodName, err)
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

func (d *DeletePodActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWaitResult(spec.ParameterLess, fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].Err, "container object meta"),
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].ErrInfo, "container object meta"), nil))
	}
	experimentStatus := v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{})
	statuses := experimentStatus.ResStatuses
	for _, c := range containerObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Id:         c.Id,
			Kind:       v1alpha1.PodKind,
			State:      v1alpha1.DestroyedState,
			Success:    true,
			Identifier: c.GetIdentifier(),
		}
		statuses = append(statuses, status)
	}
	experimentStatus.ResStatuses = statuses
	return spec.ReturnResultIgnoreCode(experimentStatus)
}
