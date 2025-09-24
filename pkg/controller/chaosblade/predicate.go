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

package chaosblade

import (
	"encoding/json"
	"reflect"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type SpecUpdatedPredicateForRunningPhase struct{}

func (sup *SpecUpdatedPredicateForRunningPhase) Create(e event.CreateEvent) bool {
	if e.Object == nil {
		return false
	}
	obj, ok := e.Object.(*v1alpha1.ChaosBlade)
	if !ok {
		return false
	}
	logrus.Infof("trigger create event, name: %s", obj.Name)
	logrus.Debugf("creating obj: %+v", obj)
	if obj.GetDeletionTimestamp() != nil {
		logrus.Infof("unexpected phase for cb creating, name: %s, phase: %s", obj.Name, obj.Status.Phase)
		return false
	}
	if obj.Status.Phase == v1alpha1.ClusterPhaseInitial {
		return true
	}
	logrus.Infof("unexpected phase for cb creating, name: %s, phase: %s", obj.Name, obj.Status.Phase)
	return false
}

func (*SpecUpdatedPredicateForRunningPhase) Delete(e event.DeleteEvent) bool {
	if e.Object == nil {
		return false
	}
	obj, ok := e.Object.(*v1alpha1.ChaosBlade)
	if !ok {
		return false
	}
	logrus.Infof("trigger delete event, name: %s", obj.Name)
	logrus.Debugf("deleting obj: %+v", obj)
	return contains(obj.GetFinalizers(), chaosbladeFinalizer)
}

func (*SpecUpdatedPredicateForRunningPhase) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil {
		return false
	}
	oldObj, ok := e.ObjectOld.(*v1alpha1.ChaosBlade)
	if !ok {
		return false
	}
	logrus.Infof("trigger update event, name: %s", oldObj.Name)
	newObj, ok := e.ObjectNew.(*v1alpha1.ChaosBlade)
	if !ok {
		return false
	}
	logrus.Debugf("updating oldObj: %+v", oldObj)
	logrus.Debugf("updating newObj: %+v", newObj)
	if !reflect.DeepEqual(newObj.Spec, oldObj.Spec) {
		bytes, err := json.Marshal(oldObj.Spec.DeepCopy())
		if err != nil {
			logrus.Warningf("marshal old spec failed, %+v", err)
			return false
		}
		newObj.SetAnnotations(map[string]string{"preSpec": string(bytes)})
		return true
	}

	if newObj.Status.Phase == v1alpha1.ClusterPhaseInitial {
		return true
	}
	// delete Error chaosblade
	if oldObj.GetDeletionTimestamp() == nil &&
		newObj.GetDeletionTimestamp() != nil {
		return true
	}
	if newObj.Status.Phase == v1alpha1.ClusterPhaseRunning ||
		newObj.Status.Phase == v1alpha1.ClusterPhaseError ||
		newObj.Status.Phase == v1alpha1.ClusterPhaseDestroying {
		return false
	}
	if newObj.Status.Phase != oldObj.Status.Phase {
		return true
	}
	if !reflect.DeepEqual(newObj.Status, oldObj.Status) {
		return true
	}
	if newObj.GetDeletionTimestamp() != nil {
		if contains(newObj.GetFinalizers(), chaosbladeFinalizer) {
			return true
		}
		logrus.Infof("cannot find the %s finalizer, so skip the update event", chaosbladeFinalizer)
		return false
	}
	logrus.Infof("spec not changed under %s phase, so skip the update event", newObj.Status.Phase)
	return false
}

func (*SpecUpdatedPredicateForRunningPhase) Generic(e event.GenericEvent) bool {
	return false
}
