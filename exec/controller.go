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

package exec

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/container"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/exec/node"
	"github.com/chaosblade-io/chaosblade-operator/exec/pod"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type ResourceDispatchedController struct {
	Controllers map[string]model.ExperimentController
}

var executor *ResourceDispatchedController
var once sync.Once

func NewDispatcherExecutor(client *channel.Client) *ResourceDispatchedController {
	once.Do(func() {
		executor = &ResourceDispatchedController{
			Controllers: make(map[string]model.ExperimentController, 0),
		}
		executor.register(
			node.NewExpController(client),
			pod.NewExpController(client),
			container.NewExpController(client),
		)
	})
	return executor
}

func (e *ResourceDispatchedController) Name() string {
	return "dispatch"
}

func (e *ResourceDispatchedController) Create(bladeName string, expSpec v1alpha1.ExperimentSpec) v1alpha1.ExperimentStatus {
	controller := e.Controllers[expSpec.Scope]
	if controller == nil {
		return v1alpha1.ExperimentStatus{
			State: "Error",
			Error: "can not find the scope controller for creating",
		}
	}
	response := controller.Create(context.Background(), expSpec)
	experimentStatus := createExperimentStatusByResponse(response)
	experimentStatus.Scope = expSpec.Scope
	experimentStatus.Target = expSpec.Target
	experimentStatus.Action = expSpec.Action
	return experimentStatus
}

func (e *ResourceDispatchedController) Destroy(bladeName string, expSpec v1alpha1.ExperimentSpec, oldExpStatus v1alpha1.ExperimentStatus) v1alpha1.ExperimentStatus {
	controller := e.Controllers[expSpec.Scope]
	if controller == nil {
		return v1alpha1.ExperimentStatus{
			State: "Error",
			Error: "can not find the scope controller for destroying",
		}
	}

	if oldExpStatus.ResStatuses == nil ||
		len(oldExpStatus.ResStatuses) == 0 {
		return model.CreateDestroyedStatus(oldExpStatus)
	}
	ctx := spec.SetDestroyFlag(context.Background(), bladeName)
	ctx = setNodeNamesAndExpIdsToContextForDestroy(ctx, &oldExpStatus.ResStatuses)

	response := controller.Destroy(ctx, expSpec, oldExpStatus)
	newExpStatus := createExperimentStatusByResponse(response)
	newExpStatus.Scope = oldExpStatus.Scope
	newExpStatus.Target = oldExpStatus.Target
	newExpStatus.Action = oldExpStatus.Action
	return newExpStatus
}

func createExperimentStatusByResponse(response *spec.Response) v1alpha1.ExperimentStatus {
	experimentStatus := v1alpha1.ExperimentStatus{}
	if response.Result != nil {
		experimentStatus = response.Result.(v1alpha1.ExperimentStatus)
	} else {
		if response.Success {
			experimentStatus = v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{})
		} else {
			experimentStatus = v1alpha1.CreateFailExperimentStatus(response.Err, []v1alpha1.ResourceStatus{})
		}
	}
	return experimentStatus
}

func setNodeNamesAndExpIdsToContextForDestroy(ctx context.Context, resourceStatus *[]v1alpha1.ResourceStatus) context.Context {
	logrus.Infof("oldStatus for destroying, %+v", *resourceStatus)
	nodeNameUidMap := model.NodeNameUidMap{}
	nodeNameExpObjectMetaMap := model.NodeNameExpObjectMetasMap{}
	for _, status := range *resourceStatus {
		if status.Id == "" {
			logrus.Warningf("the status id is empty, name: %s, uid: %s", status.Name, status.Uid)
		}
		// Because uid is unuseful in destroy operator
		nodeNameUidMap[status.NodeName] = ""
		expObjectMeta := model.ExpObjectMeta{
			Id:   status.Id,
			Name: status.Name,
			Uid:  status.Uid,
		}
		expObjectMetas := nodeNameExpObjectMetaMap[status.NodeName]
		if expObjectMetas == nil {
			expObjectMetas = make([]model.ExpObjectMeta, 0)
		}
		expObjectMetas = append(expObjectMetas, expObjectMeta)
		nodeNameExpObjectMetaMap[status.NodeName] = expObjectMetas
	}
	ctx = context.WithValue(ctx, model.NodeNameUidMapKey, nodeNameUidMap)
	ctx = context.WithValue(ctx, model.NodeNameExpObjectMetaMapKey, nodeNameExpObjectMetaMap)
	return ctx
}

func (e *ResourceDispatchedController) register(cs ...model.ExperimentController) {
	for _, c := range cs {
		e.Controllers[c.Name()] = c
	}
}
