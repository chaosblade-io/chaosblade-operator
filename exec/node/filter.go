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

package node

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	pkglabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
)

func (e *ExpController) getMatchedNodeResources(expModel spec.ExpModel) ([]v1.Node, error) {
	flags := expModel.ActionFlags
	if err := model.CheckFlags(flags); err != nil {
		return nil, err
	}
	nodes, err := resourceFunc(e.Client, flags)
	if err != nil {
		return nil, err
	}
	if nodes == nil || len(nodes) == 0 {
		return nodes, fmt.Errorf("can not find the nodes")
	}
	return e.filterByOtherFlags(nodes, flags)
}

func (e *ExpController) filterByOtherFlags(nodes []v1.Node, flags map[string]string) ([]v1.Node, error) {
	groupKey := flags[model.ResourceGroupKeyFlag.Name]
	if groupKey == "" {
		count, err := model.GetResourceCount(len(nodes), flags)
		if err != nil {
			return nil, err
		}
		return nodes[:count], nil
	}
	groupNodes := make(map[string][]v1.Node, 0)
	keys := strings.Split(groupKey, ",")
	for _, node := range nodes {
		for _, key := range keys {
			nodeList := groupNodes[node.Labels[key]]
			if nodeList == nil {
				nodeList = make([]v1.Node, 0)
			}
			nodeList = append(nodeList, node)
		}
	}
	result := make([]v1.Node, 0)
	for _, nodeList := range groupNodes {
		count, err := model.GetResourceCount(len(nodeList), flags)
		if err != nil {
			return nil, err
		}
		result = append(result, nodeList[:count]...)
	}
	return result, nil
}

var resourceFunc = func(client2 *channel.Client, flags map[string]string) ([]v1.Node, error) {
	nodes := make([]v1.Node, 0)
	// labels
	labels := flags[model.ResourceLabelsFlag.Name]
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
			return nodes, fmt.Errorf("illegal labels %s", labels)
		}
		nodeList := v1.NodeList{}
		opts := client.ListOptions{LabelSelector: pkglabels.SelectorFromSet(labelMap)}
		err := client2.List(context.TODO(), &opts, &nodeList)
		if err != nil {
			return nodes, err
		}
		if len(nodeList.Items) == 0 {
			return nodes, nil
		}
		nodes = nodeList.Items
	}

	nodesWithName := make([]v1.Node, 0)
	// names
	names := flags[model.ResourceNamesFlag.Name]
	if names != "" {
		nameArr := strings.Split(names, ",")
		if len(nodes) == 0 {
			// 根据资源名查询
			for _, name := range nameArr {
				node := v1.Node{}
				err := client2.Get(context.TODO(), types.NamespacedName{Name: name}, &node)
				if err != nil {
					logrus.Warningf("can not find the node by %s name, %v", name, err)
				} else {
					nodesWithName = append(nodesWithName, node)
				}
			}
		} else {
			for _, node := range nodes {
				for _, name := range nameArr {
					if node.Name == name {
						nodesWithName = append(nodesWithName, node)
					}
				}
			}
		}
		if len(nodesWithName) == 0 {
			return nodesWithName, nil
		}
		nodes = nodesWithName
		return nodes, nil
	}
	nodeList := v1.NodeList{}
	err := client2.List(context.TODO(), &client.ListOptions{}, &nodeList)
	if err != nil {
		logrus.Warningf("can not find all the nodes, %v", err)
		return nodes, err
	}
	nodes = nodeList.Items
	return nodes, nil
}
