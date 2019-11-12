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

package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/meta"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

// ExperimentIdentifier contains the necessary experiment fields of the resource
type ExperimentIdentifier struct {
	Id      string
	Uid     string
	Name    string
	Command string
}

func NewExperimentIdentifier(id, uid, name, command string) ExperimentIdentifier {
	return ExperimentIdentifier{
		Id:      id,
		Uid:     uid,
		Name:    name,
		Command: command,
	}
}

type ExecCommandInPodExecutor struct {
	Client      *channel.Client
	CommandFunc func(ctx context.Context, model *spec.ExpModel,
		resourceIdentifier *ResourceIdentifier) ([]ExperimentIdentifier, error) `json:"-"`
}

func (e *ExecCommandInPodExecutor) Name() string {
	return "execInPod"
}

func (e *ExecCommandInPodExecutor) SetChannel(channel spec.Channel) {
}

func (e *ExecCommandInPodExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	nodeNameUidMap, err := ExtractNodeNameUidMapFromContext(ctx)
	if err != nil {
		return spec.ReturnFailWitResult(spec.Code[spec.IllegalParameters], err.Error(),
			v1alpha1.CreateFailExperimentStatus(err.Error(), nil))
	}
	podList := &v1.PodList{}
	if err := e.Client.List(context.TODO(), GetChaosBladePodListOptions(), podList); err != nil {
		return spec.ReturnFailWitResult(spec.Code[spec.IllegalParameters], err.Error(),
			v1alpha1.CreateFailExperimentStatus(err.Error(), nil))
	}
	return e.execInMatchedPod(ctx, nodeNameUidMap, podList, expModel)
}

// ResourceIdentifier is used to pass the necessary values in context
type ResourceIdentifier struct {
	NodeName    string
	NodeUid     string
	PodName     string
	PodUid      string
	ContainerId string
}

func NewResourceIdentifier(nodeName, nodeUid, podName, podUid, containerId string) *ResourceIdentifier {
	return &ResourceIdentifier{
		NodeName:    nodeName,
		NodeUid:     nodeUid,
		PodName:     podName,
		PodUid:      podUid,
		ContainerId: containerId,
	}
}

// execInMatchedPod will execute the experiment in the target pod
func (e *ExecCommandInPodExecutor) execInMatchedPod(ctx context.Context, nodeNameUidMap NodeNameUidMap,
	podList *v1.PodList, expModel *spec.ExpModel) *spec.Response {

	experimentStatus := v1alpha1.ExperimentStatus{
		ResStatuses: make([]v1alpha1.ResourceStatus, 0),
	}
	statuses := experimentStatus.ResStatuses
	success := false
	logrus.Infof("nodeNameUidMap: %+v", nodeNameUidMap)
	for nodeName, nodeUid := range nodeNameUidMap {
		rsStatus := v1alpha1.ResourceStatus{
			Kind:     expModel.Scope,
			NodeName: nodeName,
		}
		if _, ok := spec.IsDestroy(ctx); ok {
			// Destroy
			nodeNameExpObjectMetasMaps, err := ExtractNodeNameExpObjectMetasMapFromContext(ctx)
			if err != nil {
				return spec.ReturnFailWitResult(spec.Code[spec.IllegalParameters], err.Error(),
					v1alpha1.CreateFailExperimentStatus(err.Error(), nil))
			}
			expObjectMetas := nodeNameExpObjectMetasMaps[nodeName]
			if len(expObjectMetas) == 0 {
				rsStatus.CreateFailResourceStatus("can not find the experiment id")
				statuses = append(statuses, rsStatus)
				continue
			}
			expIdSet := sets.String{}
			for _, objectMeta := range expObjectMetas {
				expIdSet.Insert(objectMeta.Id)
			}
			if expIdSet.Len() == 0 {
				rsStatus.CreateFailResourceStatus(fmt.Sprintf("cannot find the experiment id for %s node", nodeName))
				statuses = append(statuses, rsStatus)
				continue
			}
			expIds := strings.Join(expIdSet.List(), ",")
			ctx = spec.SetDestroyFlag(ctx, expIds)
		}

		targetPod := e.getExecPod(nodeName, podList)
		if targetPod == nil {
			rsStatus.CreateFailResourceStatus(fmt.Sprintf("can not find the target pod on %s node", nodeName))
			statuses = append(statuses, rsStatus)
			continue
		}
		// get the first container from pod
		containerId, _ := GetOneAvailableContainerIdFromPod(*targetPod)
		resourceIdentifier := NewResourceIdentifier(nodeName, nodeUid, targetPod.Name, string(targetPod.UID), containerId)
		success, statuses = e.execCommands(ctx, expModel, rsStatus, targetPod, statuses, resourceIdentifier)
	}
	logrus.Infof("success: %t, statuses: %+v", success, statuses)
	if success {
		experimentStatus.State = v1alpha1.SuccessState
	} else {
		experimentStatus.State = v1alpha1.ErrorState
		if len(statuses) == 0 {
			experimentStatus.Error = "the resources not found"
		} else {
			experimentStatus.Error = "see resStatus for the error details"
		}
	}
	experimentStatus.Success = success
	experimentStatus.ResStatuses = statuses
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

func (e *ExecCommandInPodExecutor) execCommands(ctx context.Context, expModel *spec.ExpModel,
	rsStatus v1alpha1.ResourceStatus, targetPod *v1.Pod, statuses []v1alpha1.ResourceStatus,
	resourceIdentifier *ResourceIdentifier) (bool, []v1alpha1.ResourceStatus) {

	success := false
	experimentIdentifiers, err := e.CommandFunc(ctx, expModel, resourceIdentifier)
	logrus.Infof("experimentIdentifiers: %+v", experimentIdentifiers)
	if err != nil {
		newStatus := rsStatus.CreateFailResourceStatus(err.Error())
		statuses = append(statuses, newStatus)
		return success, statuses
	}
	for _, identifier := range experimentIdentifiers {
		newStatus := rsStatus
		newStatus.Id = identifier.Id
		newStatus.Uid = identifier.Uid
		newStatus.Name = identifier.Name
		response := e.Client.Exec(targetPod, meta.Constant.PodName, identifier.Command, time.Second*30)
		if response.Success {
			if _, ok := spec.IsDestroy(ctx); !ok {
				newStatus.Id = response.Result.(string)
			}
			newStatus = newStatus.CreateSuccessResourceStatus()
			success = true
		} else {
			newStatus = newStatus.CreateFailResourceStatus(response.Err)
		}
		statuses = append(statuses, newStatus)
	}
	return success, statuses
}

func (e *ExecCommandInPodExecutor) getExecPod(nodeName string, podList *v1.PodList) *v1.Pod {
	var targetPod *v1.Pod
	for _, pod := range podList.Items {
		if pod.Spec.NodeName == nodeName {
			targetPod = &pod
			break
		}
	}
	return targetPod
}

func ExcludeKeyFunc() func() map[string]spec.Empty {
	return GetResourceFlagNames
}

func TruncateContainerObjectMetaUid(uid string) string {
	return strings.ReplaceAll(uid, "docker://", "")
}
