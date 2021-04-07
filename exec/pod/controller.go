/*
 * Copyright 1999-2019 Alibaba Group Holding Ltd.
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

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type ExpController struct {
	model.BaseExperimentController
}

func NewExpController(client *channel.Client) model.ExperimentController {
	return &ExpController{
		model.BaseExperimentController{
			Client:            client,
			ResourceModelSpec: NewResourceModelSpec(client),
		},
	}
}

func (*ExpController) Name() string {
	return "pod"
}

// Create pod resource experiments
func (e *ExpController) Create(ctx context.Context, expSpec v1alpha1.ExperimentSpec) *spec.Response {
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)
	pods, err, code := e.GetMatchedPodResources(ctx, *expModel)
	if err != nil {
		logrusField.Errorf("uid: %s, get matched pod experiment failed, %v", experimentId, err)
		return spec.ResponseFailWaitResult(code, err.Error(), v1alpha1.CreateFailExperimentStatus(err.Error(), v1alpha1.CreateFailResStatuses(code, err.Error(), experimentId)))
	}
	if len(pods) == 0 {
		msg := fmt.Sprintf(spec.ResponseErr[spec.ParameterInvalidK8sPodQuery].Err, "namespace|labels")
		logrusField.Errorln(msg)
		return spec.ResponseFailWaitResult(spec.ParameterInvalidK8sPodQuery, msg,
			v1alpha1.CreateFailExperimentStatus(msg, v1alpha1.CreateFailResStatuses(spec.ParameterInvalidK8sPodQuery, msg, experimentId)))
	}
	logrusField.Infof("creating pod experiment, pod count is %d", len(pods))
	containerObjectMetaList := getContainerMatchedList(experimentId, pods)
	if len(containerObjectMetaList) == 0 {
		msg := fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "getContainerMatchedList", "pod container not found")
		logrusField.Errorln(msg)
		return spec.ResponseFailWaitResult(spec.K8sExecFailed, fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].Err, experimentId),
			v1alpha1.CreateFailExperimentStatus(msg, v1alpha1.CreateFailResStatuses(spec.K8sExecFailed, msg, experimentId)))
	}
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return e.Exec(ctx, expModel)
}

func (e *ExpController) Destroy(ctx context.Context, expSpec v1alpha1.ExperimentSpec, oldExpStatus v1alpha1.ExperimentStatus) *spec.Response {
	logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx)).Infoln("start to destroy")
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	statuses := oldExpStatus.ResStatuses
	if statuses == nil {
		return spec.ReturnSuccess(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{}))
	}
	containerObjectMetaList := model.ContainerMatchedList{}
	for _, status := range statuses {
		if !status.Success {
			continue
		}
		containerObjectMeta := model.ParseIdentifier(status.Identifier)
		containerObjectMeta.Id = status.Id
		containerObjectMetaList = append(containerObjectMetaList, containerObjectMeta)
	}
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return e.Exec(ctx, expModel)
}

// getContainerMatchedList transports selected pods
func getContainerMatchedList(experimentId string, pods []v1.Pod) model.ContainerMatchedList {
	containerObjectMetaList := model.ContainerMatchedList{}
	for _, p := range pods {
		containerId, containerName, err := model.GetOneAvailableContainerIdFromPod(p)
		if err != nil {
			logrus.WithField("experiment", experimentId).WithField("pod", p.Name).
				Errorf("get an available container failed, %v", err)
			continue
		}
		containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
			ContainerId:   containerId[:12],
			ContainerName: containerName,
			PodName:       p.Name,
			NodeName:      p.Spec.NodeName,
			Namespace:     p.Namespace,
		})
	}
	return containerObjectMetaList
}
