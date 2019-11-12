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

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/meta"
)

const (
	// For create operation
	// nodeName:nodeUid
	NodeNameUidMapKey = "NodeNameUidMap"
	// nodeName: []{}
	NodeNameContainerObjectMetasMapKey = "NodeNameContainerObjectMetasMapKey"
	//[{Name: xx, Namespace: xx, Uid: xxx, NodeName: xxx}]
	PodObjectMetaListKey = "PodObjectMetaListKey"

	// For destroy operation
	// nodeName:[{uid:expId}, {uid:expId}]
	NodeNameExpObjectMetaMapKey = "NodeNameExpObjectMetasMap"

	UidKey  = "Uid"
	NameKey = "Name"
)

type NodeNameUidMap map[string]string

type NodeNameExpObjectMetasMap map[string][]ExpObjectMeta

type PodListOption client.ListOptions

type ExpObjectMeta struct {
	Id   string
	Name string
	Uid  string
}

type PodObjectMeta struct {
	Name      string
	Namespace string
	Uid       string
	NodeName  string
}

type ContainerObjectMeta struct {
	Name     string
	Uid      string
	PodName  string
	PodUid   string
	NodeName string
}

type NodeNameContainerObjectMetasMap map[string][]ContainerObjectMeta

type PodObjectMetaList []PodObjectMeta

func ExtractNodeNameUidMapFromContext(ctx context.Context) (NodeNameUidMap, error) {
	nodeNameUidMapValue := ctx.Value(NodeNameUidMapKey)
	if nodeNameUidMapValue == nil {
		return nil, fmt.Errorf("less node names in context")
	}
	nodeNameUidMap := nodeNameUidMapValue.(NodeNameUidMap)
	return nodeNameUidMap, nil
}

func ExtractNodeNameExpObjectMetasMapFromContext(ctx context.Context) (NodeNameExpObjectMetasMap, error) {
	nodeNameExpIdsMapValue := ctx.Value(NodeNameExpObjectMetaMapKey)
	if nodeNameExpIdsMapValue == nil {
		return nil, fmt.Errorf("less expriment ids in context")
	}
	nodeNameExpIdsMap := nodeNameExpIdsMapValue.(NodeNameExpObjectMetasMap)
	return nodeNameExpIdsMap, nil
}

func ExtractNodeNameContainerMetasMapFromContext(ctx context.Context) (NodeNameContainerObjectMetasMap, error) {
	containerObjectMetaValues := ctx.Value(NodeNameContainerObjectMetasMapKey)
	if containerObjectMetaValues == nil {
		return nil, fmt.Errorf("less container values in context")
	}
	containerObjectMetas := containerObjectMetaValues.(NodeNameContainerObjectMetasMap)
	return containerObjectMetas, nil
}

func ExtractPodObjectMetasFromContext(ctx context.Context) (PodObjectMetaList, error) {
	podObjectMetaValues := ctx.Value(PodObjectMetaListKey)
	if podObjectMetaValues == nil {
		return nil, fmt.Errorf("less pod object meta parameter")
	}
	podObjectMetas := podObjectMetaValues.(PodObjectMetaList)
	return podObjectMetas, nil
}

func GetChaosBladePodListOptions() *client.ListOptions {
	return &client.ListOptions{
		Namespace:     meta.Constant.Namespace,
		LabelSelector: labels.SelectorFromSet(meta.Constant.PodLabels),
	}
}
