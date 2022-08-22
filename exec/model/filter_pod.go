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

package model

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	pkglabels "k8s.io/apimachinery/pkg/labels"

	"github.com/chaosblade-io/chaosblade-operator/channel"
)

const DefaultNamespace = "default"

func CheckPodFlags(flags map[string]string) *spec.Response {
	namespace := flags[ResourceNamespaceFlag.Name]
	if namespace == "" {
		return spec.ResponseFailWithFlags(spec.ParameterLess, ResourceNamespaceFlag.Name)
	}
	namespacesValue := strings.Split(namespace, ",")
	if len(namespacesValue) > 1 {
		return spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, ResourceNamespaceFlag.Name)
	}
	return CheckFlags(flags)
}

// GetMatchedPodResources return matched pods
func (b *BaseExperimentController) GetMatchedPodResources(ctx context.Context, expModel spec.ExpModel) ([]v1.Pod, *spec.Response) {
	flags := expModel.ActionFlags
	if flags[ResourceNamespaceFlag.Name] == "" {
		expModel.ActionFlags[ResourceNamespaceFlag.Name] = DefaultNamespace
	}
	if resp := CheckPodFlags(flags); !resp.Success {
		return nil, resp
	}
	pods, resp := resourceFunc(ctx, b.Client, flags)
	if !resp.Success {
		return pods, resp
	}
	return b.filterByOtherFlags(pods, flags)
}

func (b *BaseExperimentController) filterByOtherFlags(pods []v1.Pod, flags map[string]string) ([]v1.Pod, *spec.Response) {
	random := flags["random"] == "true"
	groupKey := flags[ResourceGroupKeyFlag.Name]
	if groupKey == "" {
		count, resp := GetResourceCount(len(pods), flags)
		if !resp.Success {
			return pods[:count], resp
		}
		if random {
			return randomPodSelected(pods, count), spec.Success()
		}
		return pods[:count], spec.Success()
	}
	groupPods := make(map[string][]v1.Pod, 0)
	keys := strings.Split(groupKey, ",")
	for _, pod := range pods {
		for _, key := range keys {
			labelValue := pod.Labels[key]
			podList := groupPods[labelValue]
			if podList == nil {
				podList = []v1.Pod{}
				groupPods[labelValue] = podList
			}
			groupPods[labelValue] = append(podList, pod)
		}
	}
	result := make([]v1.Pod, 0)
	for _, podList := range groupPods {
		count, resp := GetResourceCount(len(podList), flags)
		if !resp.Success {
			return pods[:count], resp
		}
		if random {
			result = append(result, randomPodSelected(podList, count)...)
		} else {
			result = append(result, podList[:count]...)
		}
	}
	if len(result) == 0 {
		return result, spec.ResponseFailWithFlags(spec.ParameterInvalidK8sPodQuery, ResourceGroupKeyFlag.Name)
	}
	return result, spec.Success()
}

// resourceFunc is used to query the target resource
var resourceFunc = func(ctx context.Context, client2 *channel.Client, flags map[string]string) ([]v1.Pod, *spec.Response) {
	namespace := flags[ResourceNamespaceFlag.Name]
	labels := flags[ResourceLabelsFlag.Name]
	requirements := ParseLabels(labels)
	logrusField := logrus.WithField("experiment", GetExperimentIdFromContext(ctx))
	pods := make([]v1.Pod, 0)
	names := flags[ResourceNamesFlag.Name]
	if names != "" {
		nameArr := strings.Split(names, ",")
		for _, name := range nameArr {
			pod := v1.Pod{}
			err := client2.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, &pod)
			if err != nil {
				logrusField.Warningf("can not find the pod by %s name in %s namespace, %v", name, namespace, err)
				continue
			}
			if MapContains(pod.Labels, requirements) {
				pods = append(pods, pod)
			}
		}
		logrusField.Infof("get pods by names %s, len is %d", names, len(pods))
		if len(pods) == 0 {
			return pods, spec.ResponseFailWithFlags(spec.ParameterInvalidK8sPodQuery, names)
		}
		return pods, spec.Success()
	}
	if labels != "" && len(requirements) == 0 {
		msg := spec.ParameterIllegal.Sprintf(ResourceLabelsFlag.Name, labels, "data format error")
		logrusField.Warningln(msg)
		return pods, spec.ResponseFailWithFlags(spec.ParameterLess, ResourceLabelsFlag.Name, labels, "data format error, example: key=value")
	}
	if len(requirements) > 0 {
		podList := v1.PodList{}
		selector := pkglabels.NewSelector().Add(requirements...)
		opts := client.ListOptions{Namespace: namespace, LabelSelector: selector}
		err := client2.List(context.TODO(), &podList, &opts)
		if err != nil {
			return pods, spec.ResponseFailWithFlags(spec.K8sExecFailed, "PodList", err)
		}
		if len(podList.Items) == 0 {
			return pods, spec.ResponseFailWithFlags(spec.ParameterInvalidK8sPodQuery, ResourceLabelsFlag.Name)
		}
		pods = podList.Items
		logrusField.Infof("get pods by labels %s, len is %d", labels, len(pods))
	}
	return pods, spec.Success()
}

func randomPodSelected(pods []v1.Pod, count int) []v1.Pod {
	if len(pods) == 0 {
		return pods
	}
	rand.Seed(time.Now().UnixNano())
	for i := len(pods) - 1; i > 0; i-- {
		num := rand.Intn(i + 1)
		pods[i], pods[num] = pods[num], pods[i]
	}
	return pods[:count]
}
