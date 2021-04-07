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
	"errors"
	"fmt"
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

func CheckPodFlags(flags map[string]string) (error, int32) {
	namespace := flags[ResourceNamespaceFlag.Name]
	if namespace == "" {
		return fmt.Errorf(spec.ResponseErr[spec.ParameterLess].ErrInfo, ResourceNamespaceFlag.Name), spec.ParameterLess
	}
	namespacesValue := strings.Split(namespace, ",")
	if len(namespacesValue) > 1 {
		return fmt.Errorf(spec.ResponseErr[spec.ParameterInvalidNSNotOne].ErrInfo, ResourceNamespaceFlag.Name), spec.ParameterInvalidNSNotOne
	}
	return CheckFlags(flags)
}

// GetMatchedPodResources return matched pods
func (b *BaseExperimentController) GetMatchedPodResources(ctx context.Context, expModel spec.ExpModel) ([]v1.Pod, error, int32) {
	flags := expModel.ActionFlags
	if flags[ResourceNamespaceFlag.Name] == "" {
		expModel.ActionFlags[ResourceNamespaceFlag.Name] = DefaultNamespace
	}
	if err, code := CheckPodFlags(flags); err != nil {
		return nil, err, code
	}
	pods, err, code := resourceFunc(ctx, b.Client, flags)
	if err != nil {
		return nil, err, code
	}
	if pods == nil || len(pods) == 0 {
		return pods, fmt.Errorf(spec.ResponseErr[spec.ParameterInvalidK8sPodQuery].ErrInfo, ResourceNamespaceFlag.Name+"|"+ResourceLabelsFlag.Name), spec.ParameterInvalidK8sPodQuery
	}
	return b.filterByOtherFlags(pods, flags)
}

func (b *BaseExperimentController) filterByOtherFlags(pods []v1.Pod, flags map[string]string) ([]v1.Pod, error, int32) {
	random := flags["random"] == "true"
	groupKey := flags[ResourceGroupKeyFlag.Name]
	if groupKey == "" {
		count, err, code := GetResourceCount(len(pods), flags)
		if err != nil {
			return nil, err, code
		}
		if random {
			return randomPodSelected(pods, count), nil, spec.Success
		}
		return pods[:count], nil, spec.Success
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
		count, err, code := GetResourceCount(len(podList), flags)
		if err != nil {
			return nil, err, code
		}
		if random {
			result = append(result, randomPodSelected(podList, count)...)
		} else {
			result = append(result, podList[:count]...)
		}
	}
	return result, nil, spec.Success
}

// resourceFunc is used to query the target resource
var resourceFunc = func(ctx context.Context, client2 *channel.Client, flags map[string]string) ([]v1.Pod, error, int32) {
	namespace := flags[ResourceNamespaceFlag.Name]
	labels := flags[ResourceLabelsFlag.Name]
	labelsMap := ParseLabels(labels)
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
			if MapContains(pod.Labels, labelsMap) {
				pods = append(pods, pod)
			}
		}
		logrusField.Infof("get pods by names %s, len is %d", names, len(pods))
		return pods, nil, spec.Success
	}
	if labels != "" && len(labelsMap) == 0 {
		msg := fmt.Sprintf(spec.ResponseErr[spec.ParameterIllegal].ErrInfo, ResourceLabelsFlag.Name)
		logrusField.Warningln(msg)
		return pods, errors.New(msg), spec.ParameterIllegal
	}
	if len(labelsMap) > 0 {
		podList := v1.PodList{}
		opts := client.ListOptions{Namespace: namespace, LabelSelector: pkglabels.SelectorFromSet(labelsMap)}
		err := client2.List(context.TODO(), &podList, &opts)
		if err != nil {
			return pods, fmt.Errorf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "GetPodList", err.Error()), spec.K8sExecFailed
		}
		if len(podList.Items) == 0 {
			return pods, nil, spec.Success
		}
		pods = podList.Items
		logrusField.Infof("get pods by labels %s, len is %d", labels, len(pods))
	}
	return pods, nil, spec.Success
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
