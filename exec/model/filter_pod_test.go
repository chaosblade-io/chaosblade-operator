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
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_randomSelected(t *testing.T) {
	originList := []v1.Pod{
		{ObjectMeta: v12.ObjectMeta{Name: "1"}},
		{ObjectMeta: v12.ObjectMeta{Name: "2"}},
		{ObjectMeta: v12.ObjectMeta{Name: "3"}},
		{ObjectMeta: v12.ObjectMeta{Name: "4"}},
		{ObjectMeta: v12.ObjectMeta{Name: "5"}},
		{ObjectMeta: v12.ObjectMeta{Name: "6"}},
		{ObjectMeta: v12.ObjectMeta{Name: "7"}},
		{ObjectMeta: v12.ObjectMeta{Name: "8"}},
		{ObjectMeta: v12.ObjectMeta{Name: "9"}},
		{ObjectMeta: v12.ObjectMeta{Name: "10"}},
	}
	randomList := randomPodSelected(originList, 5)
	var randomNameList []string
	for _, item := range randomList {
		randomNameList = append(randomNameList, item.ObjectMeta.Name)
	}
	t.Logf("randomNameList()=%v", randomNameList)
	if reflect.DeepEqual(randomNameList, []string{"1", "2", "3", "4", "5"}) {
		t.Errorf("randomPodSelected() is invalid")
	}
}
