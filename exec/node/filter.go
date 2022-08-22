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

func (e *ExpController) getMatchedNodeResources(ctx context.Context, expModel spec.ExpModel) ([]v1.Node, *spec.Response) {
	flags := expModel.ActionFlags
	if resp := model.CheckFlags(flags); !resp.Success {
		return nil, resp
	}
	nodes, resp := resourceFunc(ctx, e.Client, flags)
	if !resp.Success {
		return nil, resp
	}
	return e.filterByOtherFlags(nodes, flags)
}

func (e *ExpController) filterByOtherFlags(nodes []v1.Node, flags map[string]string) ([]v1.Node, *spec.Response) {
	groupKey := flags[model.ResourceGroupKeyFlag.Name]
	if groupKey == "" {
		count, resp := model.GetResourceCount(len(nodes), flags)
		return nodes[:count], resp
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
		count, resp := model.GetResourceCount(len(nodeList), flags)
		if !resp.Success {
			return nodes[:count], resp
		}
		result = append(result, nodeList[:count]...)
	}
	return result, spec.Success()
}

var resourceFunc = func(ctx context.Context, client2 *channel.Client, flags map[string]string) ([]v1.Node, *spec.Response) {
	labels := flags[model.ResourceLabelsFlag.Name]
	requirements := model.ParseLabels(labels)
	logrusField := logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx))
	nodes := make([]v1.Node, 0)
	names := flags[model.ResourceNamesFlag.Name]
	if names != "" {
		nameArr := strings.Split(names, ",")
		for _, name := range nameArr {
			node := v1.Node{}
			err := client2.Get(context.TODO(), types.NamespacedName{Name: name}, &node)
			if err != nil {
				// Skip the invalid name
				logrusField.Warningf("can not find the node by %s name, %v", name, err)
				continue
			}
			if model.MapContains(node.Labels, requirements) {
				nodes = append(nodes, node)
			}
		}
		logrusField.Infof("get nodes by name %s, len is %d", names, len(nodes))
		if len(nodes) == 0 {
			return nodes, spec.ResponseFailWithFlags(spec.ParameterInvalidK8sNodeQuery, names)
		}
		return nodes, spec.Success()
	}
	if labels != "" && len(requirements) == 0 {
		logrusField.Warningln(spec.ParameterIllegal.Sprintf(model.ResourceLabelsFlag.Name, labels, "illegal labels"))
		return nodes, spec.ResponseFailWithFlags(spec.ParameterIllegal, model.ResourceLabelsFlag.Name, labels, "illegal labels")
	}
	if len(requirements) > 0 {
		nodeList := v1.NodeList{}
		selector := pkglabels.NewSelector().Add(requirements...)
		opts := client.ListOptions{LabelSelector: selector}
		err := client2.List(context.TODO(), &nodeList, &opts)
		if err != nil {
			return nodes, spec.ResponseFailWithFlags(spec.K8sExecFailed, "ListNode", err)
		}
		nodes = nodeList.Items
		logrusField.Infof("get nodes by labels %s, len is %d", labels, len(nodes))
	}
	if len(nodes) == 0 {
		return nodes, spec.ResponseFailWithFlags(spec.ParameterInvalidK8sNodeQuery, labels)
	}
	return nodes, spec.Success()
}
