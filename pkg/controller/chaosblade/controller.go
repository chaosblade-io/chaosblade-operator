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

package chaosblade

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

var log = logf.Log.WithName("controller_chaosblade")

const chaosbladeFinalizer = "finalizer.chaosblade.io"

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ChaosBlade Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileChaosBlade {
	cbClient := mgr.GetClient().(*channel.Client)
	return &ReconcileChaosBlade{
		client:   cbClient,
		scheme:   mgr.GetScheme(),
		Executor: exec.NewDispatcherExecutor(cbClient),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, rcb *ReconcileChaosBlade) error {
	// Create a new controller
	c, err := controller.New("chaosblade-controller", mgr, controller.Options{Reconciler: rcb})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ChaosBlade
	chaosBlade := v1alpha1.ChaosBlade{}
	err = c.Watch(
		&source.Kind{Type: &chaosBlade},
		&handler.EnqueueRequestForObject{},
		&SpecUpdatedPredicateForRunningPhase{})
	if err != nil {
		return err
	}
	return nil
}

// blank assignment to verify that ReconcileChaosBlade implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileChaosBlade{}

// ReconcileChaosBlade reconciles a ChaosBlade object
type ReconcileChaosBlade struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   *channel.Client
	scheme   *runtime.Scheme
	Executor model.ExpController
}

// Reconcile reads that state of the cluster for a ChaosBlade object and makes changes based on the state read
// and what is in the ChaosBlade.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileChaosBlade) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	requeue := reconcile.Result{Requeue: true}
	forget := reconcile.Result{}

	// Fetch the RC instance
	cb := &v1alpha1.ChaosBlade{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cb)
	if err != nil {
		if errors.IsNotFound(err) {
			// Return and don't requeue
			return forget, nil
		}
		// Error reading the object - requeue the request.
		return forget, err
	}

	if len(cb.Spec.Experiments) == 0 {
		return forget, nil
	}

	// Remove the Finalizer if the CR object status is destroyed
	if cb.Status.Phase == v1alpha1.ClusterPhaseDestroyed {
		cb.SetFinalizers(remove(cb.GetFinalizers(), chaosbladeFinalizer))
		err := r.client.Update(context.TODO(), cb)
		return forget, err
	}

	// Add finalizer for this CR
	if cb.Status.Phase != v1alpha1.ClusterPhaseDestroyed &&
		!contains(cb.GetFinalizers(), chaosbladeFinalizer) {
		if err := r.addFinalizer(reqLogger, cb); err != nil {
			return requeue, err
		}
		return forget, nil
	}

	// Create experiment
	if cb.Status.Phase == v1alpha1.ClusterPhaseInitial ||
		cb.Status.Phase == v1alpha1.ClusterPhaseUpdating {
		expStatusList := make([]v1alpha1.ExperimentStatus, 0)
		var phase = v1alpha1.ClusterPhaseError
		for _, exp := range cb.Spec.Experiments {
			var experimentStatus = r.Executor.Create(cb.Name, exp)
			if experimentStatus.Success {
				phase = v1alpha1.ClusterPhaseRunning
			}
			expStatusList = append(expStatusList, experimentStatus)
		}

		cb.Status.ExpStatuses = expStatusList
		cb.Status.Phase = phase
		err := r.client.Status().Update(context.TODO(), cb)
		if err != nil {
			logrus.Warningf("update chaosblade err, %v", err)
		}
		return forget, err
	}

	// delete the CR object
	if cb.GetDeletionTimestamp() != nil {
		if contains(cb.GetFinalizers(), chaosbladeFinalizer) {
			err := r.finalizeChaosBlade(reqLogger, cb)
			return forget, err
		}
		return forget, nil
	}

	// Update CR, firstly destroy it and re-create the new CR
	phase := v1alpha1.ClusterPhaseUpdating
	for idx, expStatus := range cb.Status.ExpStatuses {
		logrus.Infof("update cb: %+v", *cb)
		var experimentStatus = r.Executor.Destroy(cb.Name, cb.Spec.Experiments[idx], expStatus)
		if !experimentStatus.Success {
			phase = v1alpha1.ClusterPhaseDestroying
		}
		cb.Status.ExpStatuses[idx] = experimentStatus
	}
	cb.Status.Phase = phase
	err = r.client.Status().Update(context.TODO(), cb.DeepCopy())
	if err != nil {
		logrus.Warningf("update chaosblade to updating err, %v", err)
	}
	return forget, nil
}

// finalizeChaosBlade
func (r *ReconcileChaosBlade) finalizeChaosBlade(reqLogger logr.Logger, cb *v1alpha1.ChaosBlade) error {
	var phase = v1alpha1.ClusterPhaseDestroyed
	for idx, exp := range cb.Spec.Experiments {
		logrus.Infof("finalize cb: %+v", *cb)
		oldExpStatus := cb.Status.ExpStatuses[idx]
		oldExpStatus = r.Executor.Destroy(cb.Name, exp, oldExpStatus)
		if !oldExpStatus.Success {
			phase = v1alpha1.ClusterPhaseDestroying
		}
		cb.Status.ExpStatuses[idx] = oldExpStatus
	}
	cb.Status.Phase = phase
	err := r.client.Status().Update(context.TODO(), cb.DeepCopy())
	if err != nil {
		logrus.Warningf("update chaosblade status failed in finalize phase, %v", err)
		return err
	}
	if cb.Status.Phase == v1alpha1.ClusterPhaseDestroying {
		return fmt.Errorf("failed to destory, please see the experiment status")
	}
	reqLogger.Info("Successfully finalized chaosblade")
	return nil
}

func (r *ReconcileChaosBlade) addFinalizer(reqLogger logr.Logger, cb *v1alpha1.ChaosBlade) error {
	reqLogger.Info("Adding Finalizer for the ChaosBlade")
	cb.SetFinalizers(append(cb.GetFinalizers(), chaosbladeFinalizer))
	// Update CR
	err := r.client.Update(context.TODO(), cb)
	if err != nil {
		reqLogger.Error(err, "Failed to update ChaosBlade with finalizer")
		return err
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}
	return list
}
