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

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	pkglabels "k8s.io/apimachinery/pkg/labels"

	"github.com/chaosblade-io/chaosblade-operator/channel"
)

const DefaultNamespace = "default"

func CheckPodFlags(flags map[string]string) error {
	namespace := flags[ResourceNamespaceFlag.Name]
	if namespace == "" {
		return fmt.Errorf("must specify %s flag", ResourceNamespaceFlag.Name)
	}
	namespacesValue := strings.Split(namespace, ",")
	if len(namespacesValue) > 1 {
		return fmt.Errorf("only one %s value can be specified", ResourceNamespaceFlag.Name)
	}
	// Must include one flag in the count, percent, labels and names
	expFlags := []*spec.ExpFlag{
		ResourceCountFlag,
		ResourcePercentFlag,
		ResourceLabelsFlag,
		ResourceNamesFlag,
	}
	value := ""
	flagsNames := make([]string, 0)
	for _, flag := range expFlags {
		flagsNames = append(flagsNames, flag.Name)
		value = fmt.Sprintf("%s%s", value, flags[flag.Name])
	}
	if value == "" {
		return fmt.Errorf("must specify one flag in %s", strings.Join(flagsNames, ","))
	}
	return nil
}

func (b *BaseExperimentController) GetMatchedPodResources(expModel spec.ExpModel) ([]v1.Pod, error) {
	flags := expModel.ActionFlags
	if flags[ResourceNamespaceFlag.Name] == "" {
		expModel.ActionFlags[ResourceNamespaceFlag.Name] = DefaultNamespace
	}
	if err := CheckPodFlags(flags); err != nil {
		return nil, err
	}
	pods, err := resourceFunc(b.Client, flags)
	if err != nil {
		return nil, err
	}
	if pods == nil || len(pods) == 0 {
		return pods, fmt.Errorf("can not find the pods in %s namespace",
			expModel.ActionFlags[ResourceNamespaceFlag.Name])
	}
	return b.filterByOtherFlags(pods, flags)
}

func (b *BaseExperimentController) filterByOtherFlags(pods []v1.Pod, flags map[string]string) ([]v1.Pod, error) {
	groupKey := flags[ResourceGroupKeyFlag.Name]
	if groupKey == "" {
		count, err := GetResourceCount(len(pods), flags)
		if err != nil {
			return nil, err
		}
		return pods[:count], nil
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
		count, err := GetResourceCount(len(podList), flags)
		if err != nil {
			return nil, err
		}
		result = append(result, podList[:count]...)
	}
	return result, nil
}

// resourceFunc is used to query the target resource
var resourceFunc = func(client2 *channel.Client, flags map[string]string) ([]v1.Pod, error) {
	pods := make([]v1.Pod, 0)
	namespace := flags[ResourceNamespaceFlag.Name]
	// labels
	labels := flags[ResourceLabelsFlag.Name]
	if labels != "" {
		labelArr := strings.Split(labels, ",")
		labelMap := make(map[string]string, 0)
		for _, label := range labelArr {
			keyValue := strings.SplitN(label, "=", 2)
			if len(keyValue) != 2 {
				logrus.Warningf("label %s is illegal", label)
				continue
			}
			labelMap[keyValue[0]] = keyValue[1]
		}
		if len(labelMap) == 0 {
			return pods, fmt.Errorf("illegal labels %s", labels)
		}
		podList := v1.PodList{}
		opts := client.ListOptions{Namespace: namespace, LabelSelector: pkglabels.SelectorFromSet(labelMap)}
		err := client2.List(context.TODO(), &opts, &podList)
		if err != nil {
			return pods, err
		}
		if len(podList.Items) == 0 {
			return pods, nil
		}
		pods = podList.Items
		logrus.Infof("get pods by labels %s, len is %d", labels, len(pods))
	}

	podsWithName := make([]v1.Pod, 0)
	// names
	names := flags[ResourceNamesFlag.Name]
	if names != "" {
		nameArr := strings.Split(names, ",")
		if len(pods) == 0 {
			for _, name := range nameArr {
				pod := v1.Pod{}
				err := client2.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, &pod)
				if err != nil {
					logrus.Warningf("can not find the pod by %s name in %s namespace, %v", name, namespace, err)
				} else {
					podsWithName = append(podsWithName, pod)
				}
			}
		} else {
			for _, pod := range pods {
				for _, name := range nameArr {
					if pod.Name == name {
						podsWithName = append(podsWithName, pod)
					}
				}
			}
		}
		logrus.Infof("get pods by names %s, len is %d", names, len(podsWithName))
		if len(podsWithName) == 0 {
			return podsWithName, nil
		}
		pods = podsWithName
	}
	return pods, nil
}
