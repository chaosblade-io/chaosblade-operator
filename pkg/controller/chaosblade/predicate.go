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
	"reflect"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type SpecUpdatedPredicateForRunningPhase struct {
}

func (sup *SpecUpdatedPredicateForRunningPhase) Create(e event.CreateEvent) bool {
	logrus.Infof("trigger create event")
	if e.Object == nil {
		return false
	}
	obj, ok := e.Object.(*v1alpha1.ChaosBlade)
	if !ok {
		return false
	}
	if obj.GetDeletionTimestamp() != nil {
		if contains(obj.GetFinalizers(), chaosbladeFinalizer) {
			return true
		}
		logrus.Infof("cannot find the %s finalizer, so skip the create event", chaosbladeFinalizer)
		return false
	}
	if obj.Status.Phase == v1alpha1.ClusterPhaseInitial {
		return true
	}
	logrus.Infof("unexpected status for cb created, name: %s, phase: %s", obj.Name, obj.Status.Phase)
	return false
}

func (*SpecUpdatedPredicateForRunningPhase) Delete(e event.DeleteEvent) bool {
	logrus.Infof("trigger delete event")
	if e.Object == nil {
		return false
	}
	obj, ok := e.Object.(*v1alpha1.ChaosBlade)
	if !ok {
		return false
	}
	logrus.Infof("deleteObj: %+v", obj)
	// 虽然版本是最新的，但是此对象会包含 Finalizers:[finalizer.chaosblade.io]
	if obj.Status.Phase == v1alpha1.ClusterPhaseDestroyed {
		return false
	}
	return contains(obj.GetFinalizers(), chaosbladeFinalizer)
}

func (*SpecUpdatedPredicateForRunningPhase) Update(e event.UpdateEvent) bool {
	logrus.Infof("trigger update event")
	if e.ObjectOld == nil {
		return false
	}
	oldObj, ok := e.ObjectOld.(*v1alpha1.ChaosBlade)
	if !ok {
		return false
	}
	newObj, ok := e.ObjectNew.(*v1alpha1.ChaosBlade)
	if !ok {
		return false
	}

	logrus.Infof("oldObject: %+v", oldObj)
	logrus.Infof("newObject: %+v", newObj)

	if !reflect.DeepEqual(newObj.Spec, oldObj.Spec) {
		return true
	}

	logrus.Infof("oldVersion:%s, newVersion: %s", oldObj.ResourceVersion, newObj.ResourceVersion)

	// This update is end if the old cr status is UPDATING
	if oldObj.Status.Phase == v1alpha1.ClusterPhaseInitial {
		if newObj.Status.Phase == v1alpha1.ClusterPhaseInitial {
			return true
		}
		logrus.Infof("this is the end result for initial, so skip the update event")
		return false
	}
	if oldObj.Status.Phase == v1alpha1.ClusterPhaseUpdating {
		logrus.Infof("this is the end result for updating, so skip the update event")
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

	logrus.Infof("spec not changed under running phase, so skip the update event")
	return false
}

func (*SpecUpdatedPredicateForRunningPhase) Generic(e event.GenericEvent) bool {
	return false
}
