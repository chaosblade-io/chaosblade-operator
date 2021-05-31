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

package container

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"

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
	return "container"
}

// Create an experiment about container
func (e *ExpController) Create(ctx context.Context, expSpec v1alpha1.ExperimentSpec) *spec.Response {
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	// priority: id > name > index
	containerIdsValue := strings.TrimSpace(expModel.ActionFlags[model.ContainerIdsFlag.Name])
	containerNamesValue := strings.TrimSpace(expModel.ActionFlags[model.ContainerNamesFlag.Name])
	containerIndexValue := strings.TrimSpace(expModel.ActionFlags[model.ContainerIndexFlag.Name])
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId).WithField("location", util.GetRunFuncName())
	lessParameter := fmt.Sprintf("%s|%s|%s", model.ContainerIdsFlag.Name, model.ContainerNamesFlag.Name, model.ContainerIndexFlag.Name)
	if containerIdsValue == "" && containerNamesValue == "" && containerIndexValue == "" {
		errMsg := fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].Err, lessParameter)
		logrusField.Errorln(errMsg)
		return spec.ResponseFailWaitResult(spec.ParameterLess, errMsg,
			v1alpha1.CreateFailExperimentStatus(errMsg, v1alpha1.CreateFailResStatuses(spec.ParameterLess, errMsg, experimentId)))
	}
	pods, err, code := e.GetMatchedPodResources(ctx, *expModel)
	if err != nil {
		logrusField.Errorf("uid: %s, get matched pod resources failed, %v", experimentId, err)
		return spec.ResponseFailWaitResult(code, err.Error(), v1alpha1.CreateFailExperimentStatus(err.Error(), v1alpha1.CreateFailResStatuses(code, err.Error(), experimentId)))
	}
	if len(pods) == 0 {
		msg := fmt.Sprintf(spec.ResponseErr[spec.ParameterInvalidK8sPodQuery].Err, "namespace|labels")
		logrusField.Errorln(msg)
		return spec.ResponseFailWaitResult(spec.ParameterInvalidK8sPodQuery, msg,
			v1alpha1.CreateFailExperimentStatus(msg, v1alpha1.CreateFailResStatuses(spec.ParameterInvalidK8sPodQuery, msg, experimentId)))
	}
	containerObjectMetaList, err := getMatchedContainerMetaList(pods, containerIdsValue, containerNamesValue, containerIndexValue)
	if err != nil {
		errMsg := fmt.Sprintf(spec.ResponseErr[spec.ParameterIllegal].Err+" ,"+err.Error(), "container-index")
		logrusField.Errorf("get matched container meta list failed, %v", err)
		return spec.ResponseFailWaitResult(spec.ParameterIllegal, fmt.Sprintf(spec.ResponseErr[spec.ParameterIllegal].Err, "container-index"),
			v1alpha1.CreateFailExperimentStatus(errMsg, v1alpha1.CreateFailResStatuses(spec.ParameterIllegal, errMsg, experimentId)))
	}
	if len(containerObjectMetaList) == 0 {
		errMsg := fmt.Sprintf(spec.ResponseErr[spec.ParameterInvalid].ErrInfo+" ,"+"container not found by `%s`", lessParameter, lessParameter)
		logrusField.Errorln(errMsg)
		return spec.ResponseFailWaitResult(spec.ParameterInvalid, errMsg,
			v1alpha1.CreateFailExperimentStatus(errMsg, v1alpha1.CreateFailResStatuses(spec.ParameterInvalid, errMsg, experimentId)))
	}
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return e.Exec(ctx, expModel)
}

// Destroy
func (e *ExpController) Destroy(ctx context.Context, expSpec v1alpha1.ExperimentSpec, oldExpStatus v1alpha1.ExperimentStatus) *spec.Response {
	logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx)).WithField("location", util.GetRunFuncName()).Infof("start to destroy")
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	statuses := oldExpStatus.ResStatuses
	if statuses == nil {
		return spec.ReturnSuccess(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{}))
	}
	containerObjectMetaList := model.ContainerMatchedList{}
	for _, status := range statuses {
		if !status.Success {
			// does not need to destroy
			continue
		}
		containerObjectMeta := model.ParseIdentifier(status.Identifier)
		containerObjectMeta.Id = status.Id
		containerObjectMetaList = append(containerObjectMetaList, containerObjectMeta)
	}
	if len(containerObjectMetaList) == 0 {
		return spec.ReturnSuccess(v1alpha1.CreateSuccessExperimentStatus(statuses))
	}
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return e.Exec(ctx, expModel)
}

// getMatchedContainerMetaList which will be used in the executor
func getMatchedContainerMetaList(pods []v1.Pod, containerIdsValue, containerNamesValue, containerIndexValue string) (model.ContainerMatchedList, error) {
	containerObjectMetaList := model.ContainerMatchedList{}
	expectedContainerIds := strings.Split(containerIdsValue, ",")
	expectedContainerNames := strings.Split(containerNamesValue, ",")
	// priority id>name>index
	for _, pod := range pods {
		containerStatuses := pod.Status.ContainerStatuses
		if containerStatuses == nil {
			continue
		}
		for _, containerStatus := range containerStatuses {
			containerId := model.TruncateContainerObjectMetaUid(containerStatus.ContainerID)
			containerName := containerStatus.Name
			if containerIdsValue != "" {
				for _, expectedContainerId := range expectedContainerIds {
					if expectedContainerId == "" {
						continue
					}
					if strings.HasPrefix(containerId, expectedContainerId) {
						containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
							ContainerId:   containerId[:12],
							ContainerName: containerName,
							PodName:       pod.Name,
							Namespace:     pod.Namespace,
							NodeName:      pod.Spec.NodeName,
						})
					}
				}
			} else if containerNamesValue != "" {
				for _, expectedName := range expectedContainerNames {
					if expectedName == "" {
						continue
					}
					if expectedName == containerName {
						// matched
						containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
							ContainerId:   containerId[:12],
							ContainerName: containerName,
							PodName:       pod.Name,
							Namespace:     pod.Namespace,
							NodeName:      pod.Spec.NodeName,
						})
					}
				}
			}
		}
		if containerIdsValue == "" && containerNamesValue == "" && containerIndexValue != "" {
			idx, err := strconv.Atoi(containerIndexValue)
			if err != nil {
				return containerObjectMetaList, err
			}
			if idx > len(containerStatuses)-1 {
				return containerObjectMetaList, fmt.Errorf("%s value is out of bound", containerIndexValue)
			}
			containerId := model.TruncateContainerObjectMetaUid(containerStatuses[idx].ContainerID)
			containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
				ContainerId:   containerId[:12],
				ContainerName: containerStatuses[idx].Name,
				PodName:       pod.Name,
				Namespace:     pod.Namespace,
				NodeName:      pod.Spec.NodeName,
			})
		}
	}
	return containerObjectMetaList, nil
}
