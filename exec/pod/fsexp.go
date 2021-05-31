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
	"strconv"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
	chaosfs "github.com/chaosblade-io/chaosblade-operator/pkg/hookfs"
	webhook "github.com/chaosblade-io/chaosblade-operator/pkg/webhook/pod"
)

type PodIOActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewPodIOActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodIOActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: "method",
					Desc: "inject methods, only support read and write",
				},
				&spec.ExpFlag{
					Name: "delay",
					Desc: "file io delay time, ms",
				},
			},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: "path",
					Desc: "I/O exception path or file",
				},
				&spec.ExpFlag{
					Name:   "random",
					Desc:   "random inject I/O code",
					NoArgs: true,
				},
				&spec.ExpFlag{
					Name: "percent",
					Desc: "I/O error percent [0-100],",
				},
				&spec.ExpFlag{
					Name: "errno",
					Desc: "I/O error code",
				},
			},
			ActionExecutor: &PodIOActionExecutor{client: client},
			ActionExample: `# Two types of exceptions were injected for the READ operation, with an exception rate of 60 percent
blade create k8s pod-pod IO --method read --delay 1000 --path /home --percent 60 --errno 28 --labels "app=test" --namespace default`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*PodIOActionSpec) Name() string {
	return "IO"
}

func (*PodIOActionSpec) Aliases() []string {
	return []string{}
}

func (*PodIOActionSpec) ShortDesc() string {
	return "Pod File System IO Exception"
}

func (*PodIOActionSpec) LongDesc() string {
	return "Pod File System IO Exception"
}

type PodIOActionExecutor struct {
	client *channel.Client
}

func (*PodIOActionExecutor) Name() string {
	return "IO"
}

func (*PodIOActionExecutor) SetChannel(channel spec.Channel) {
}

func (d *PodIOActionExecutor) Exec(uid string, ctx context.Context, model *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(ctx, model)
	} else {
		return d.create(ctx, model)
	}
}

func (d *PodIOActionExecutor) create(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	containerMatchedList, err := model.GetContainerObjectMetaListFromContext(ctx)
	experimentId := model.GetExperimentIdFromContext(ctx)
	if err != nil {
		util.Errorf(experimentId, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWaitResult(spec.ParameterLess, fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].Err, "container object meta"),
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].ErrInfo, "container object meta"), nil))
	}
	logrusField := logrus.WithField("experiment", experimentId)
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	for _, c := range containerMatchedList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: c.GetIdentifier(),
		}
		pod := &v1.Pod{}
		err := d.client.Get(context.TODO(), client.ObjectKey{Namespace: c.Namespace, Name: c.PodName}, pod)
		if err != nil {
			logrusField.Errorf("get pod %s err, %v", c.PodName, err)
			statuses = append(statuses, status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed))
			continue
		}
		if !isPodReady(pod) {
			logrusField.Infof("pod %s is not ready", c.PodName)
			statuses = append(statuses, status.CreateFailResourceStatus("pod is not read", spec.K8sExecFailed))
			continue
		}
		methods, ok := expModel.ActionFlags["method"]
		if !ok && len(methods) != 0 {
			logrusField.Error("method cannot be empty")
			statuses = append(statuses, status.CreateFailResourceStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterLess].Err, "method"), spec.ParameterLess))
			continue
		}

		var delay, percent, errno int
		delayStr, ok := expModel.ActionFlags["delay"]
		if ok && len(delayStr) != 0 {
			delay, err = strconv.Atoi(delayStr)
			if err != nil {
				logrusField.Error("delay must be integer")
				statuses = append(statuses, status.CreateFailResourceStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterIllegal].Err, "delay"), spec.ParameterIllegal))
				continue
			}
		}
		percentStr, ok := expModel.ActionFlags["percent"]
		if ok && len(percentStr) != 0 {
			if percent, err = strconv.Atoi(percentStr); err != nil {
				logrusField.Error("percent must be integer")
				statuses = append(statuses, status.CreateFailResourceStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterIllegal].Err, "percent"), spec.ParameterIllegal))
				continue
			}
		}

		errnoStr, ok := expModel.ActionFlags["errno"]
		if ok && len(errnoStr) != 0 {
			if errno, err = strconv.Atoi(errnoStr); err != nil {
				logrusField.Error("errno must be integer")
				statuses = append(statuses, status.CreateFailResourceStatus(fmt.Sprintf(spec.ResponseErr[spec.ParameterIllegal].Err, "errno"), spec.ParameterIllegal))
				continue
			}
		}

		random := false
		randomStr, ok := expModel.ActionFlags["random"]
		if ok && randomStr == "true" {
			random = true
		}

		request := &chaosfs.InjectMessage{
			Methods: strings.Split(methods, ","),
			Path:    expModel.ActionFlags["path"],
			Delay:   uint32(delay),
			Percent: uint32(percent),
			Random:  random,
			Errno:   uint32(errno),
		}

		chaosfsClient, err := getChaosfsClient(pod)
		if err != nil {
			logrusField.WithField("pod", c.PodName).WithField("request", request).
				Errorf("init chaosfs client failed: %v", err)
			statuses = append(statuses, status.CreateFailResourceStatus(fmt.Sprintf(spec.ResponseErr[spec.ChaosfsClientFailed].Err, pod.Name, err), spec.ChaosfsClientFailed))
			continue
		}
		err = chaosfsClient.InjectFault(ctx, request)
		if err != nil {
			logrusField.Errorf("inject io exception in pod %s failed, request %v, err: %v", c.PodName, request, err)
			statuses = append(statuses, status.CreateFailResourceStatus(fmt.Sprintf(spec.ResponseErr[spec.ChaosfsInjectFailed].Err, pod.Name, request, err), spec.ChaosfsInjectFailed))
			continue
		}
		statuses = append(statuses, status.CreateSuccessResourceStatus())
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

func (d *PodIOActionExecutor) destroy(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
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
		pod := &v1.Pod{}
		err := d.client.Get(context.TODO(), client.ObjectKey{Namespace: c.Namespace, Name: c.PodName}, pod)
		if err != nil {
			logrusField.Errorf("get pod %s err, %v", c.PodName, err)
			continue
		}
		if !isPodReady(pod) {
			logrusField.Errorf("pod %s is not ready", c.PodName)
			continue
		}

		chaosfsClient, err := getChaosfsClient(pod)
		if err != nil {
			logrusField.Errorf("init chaosfs client failed in pod %v, err: %v", pod.Name, err)
			statuses = append(statuses, status.CreateFailResourceStatus(fmt.Sprintf(spec.ResponseErr[spec.ChaosfsClientFailed].Err, pod.Name, err), spec.ChaosfsClientFailed))
			continue
		}
		err = chaosfsClient.Revoke(ctx)
		if err != nil {
			logrusField.Errorf("recover io exception failed in pod  %v, err: %v", c.PodName, err)
			statuses = append(statuses, status.CreateFailResourceStatus(fmt.Sprintf(spec.ResponseErr[spec.ChaosfsRecoverFailed].Err, pod.Name, err), spec.ChaosfsRecoverFailed))
			continue
		}
	}
	experimentStatus.ResStatuses = statuses
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

func isPodReady(pod *v1.Pod) bool {
	if pod.ObjectMeta.DeletionTimestamp != nil {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodReady &&
			condition.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

func getChaosfsClient(pod *v1.Pod) (*chaosfs.ChaosBladeHookClient, error) {
	port, err := getContainerPort(webhook.FuseServerPortName, pod)
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", pod.Status.PodIP, port)
	return chaosfs.NewChabladeHookClient(addr), nil

}
func getContainerPort(portName string, pod *v1.Pod) (int32, error) {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == portName {
				return port.ContainerPort, nil
			}
		}
	}
	return 0, fmt.Errorf("can not found fuse-server container port ")
}
