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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
	runtime2 "github.com/chaosblade-io/chaosblade-operator/pkg/runtime"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
	"github.com/chaosblade-io/chaosblade-operator/version"
)

const chaosbladeFinalizer = "finalizer.chaosblade.io"

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ChaosBlade Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if err := add(mgr, newReconciler(mgr)); err != nil {
		return err
	}
	// add periodically clean up blade ticker
	return mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		startPeriodicallyCleanUpBlade(ctx, mgr)
		return nil
	}))
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
	c, err := controller.New("chaosblade-controller", mgr, controller.Options{
		Reconciler:              rcb,
		MaxConcurrentReconciles: runtime2.MaxConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ChaosBlade
	cb := v1alpha1.ChaosBlade{}
	err = c.Watch(source.Kind(mgr.GetCache(), &cb, &handler.TypedEnqueueRequestForObject[*v1alpha1.ChaosBlade]{}))
	if err != nil {
		return err
	}
	if chaosblade.DaemonsetEnable {
		//namespace, err := k8sutil.GetOperatorNamespace()
		//if err != nil {
		//	return err
		//}
		//chaosblade.DaemonsetPodNamespace = namespace
		//// deploy chaosblade tool
		//if err := deployChaosBladeTool(rcb); err != nil {
		//	logrus.WithField("product", version.Product).WithError(err).Errorln("Failed to deploy chaosblade tool")
		//	return err
		//}
		logrus.WithField("product", version.Product).WithField("daemonset.enable", chaosblade.DaemonsetEnable).
			Infoln("enable chaosblade-tool deamonset")
	}
	return nil
}

// if blade status is destroying
func startPeriodicallyCleanUpBlade(ctx context.Context, mgr manager.Manager) {
	go func() {
		cli := mgr.GetClient()
		duration, err := time.ParseDuration(chaosblade.RemoveBladeInterval)
		if err != nil {
			logrus.Errorf("parse interval error: %v, use default interval: %s", err, chaosblade.DefaultRemoveBladeInterval)
			duration, err = time.ParseDuration(chaosblade.DefaultRemoveBladeInterval)
			chaosblade.RemoveBladeInterval = chaosblade.DefaultRemoveBladeInterval
			if err != nil {
				logrus.Fatalf("start periodically clean up blade, ticker error: %v", err)
			}
		}
		// first clean up
		periodicallyCleanUpBlade(ctx, cli, duration)

		// ticker clean up
		ticker := time.NewTicker(time.Second * time.Duration(duration.Seconds()))
		logrus.Infof("start periodically clean up blade ticker, interval: %s", chaosblade.RemoveBladeInterval)
		for range ticker.C {
			periodicallyCleanUpBlade(ctx, cli, duration)
		}
	}()
}

func periodicallyCleanUpBlade(ctx context.Context, cli client.Client, interval time.Duration) {
	results := &v1alpha1.ChaosBladeList{}
	if err := cli.List(ctx, results, &client.ListOptions{}); err != nil {
		logrus.Errorf("periodically clean up, list blade error: %v", err)
	}
	logrus.Infof("periodically clean up blade, blade size: %d", len(results.Items))
	for _, item := range results.Items {
		if item.DeletionTimestamp == nil {
			continue
		}
		sub := time.Now().Sub(item.DeletionTimestamp.Time)
		if item.Status.Phase == v1alpha1.ClusterPhaseDestroying && sub.Seconds() > interval.Seconds() {
			logrus.Infof("periodically clean up blade %s, deletion time: %s", item.Name, item.DeletionTimestamp.String())
			// patch blade
			if err := cli.Patch(ctx,
				&v1alpha1.ChaosBlade{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "chaosblade.io/v1alpha1",
						Kind:       "ChaosBlade",
					},
					ObjectMeta: metav1.ObjectMeta{Name: item.Name},
				},
				client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":[]}}`)),
			); err != nil {
				logrus.Errorf("patch blade: %s, error: %v", item.Name, err)
			}
		}
	}
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
func (r *ReconcileChaosBlade) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := logrus.WithField("Request.Name", request.Name)
	forget := reconcile.Result{}
	// Fetch the RC instance
	cb := &v1alpha1.ChaosBlade{}
	err := r.client.Get(ctx, request.NamespacedName, cb)
	if err != nil {
		return forget, nil
	}
	if len(cb.Spec.Experiments) == 0 {
		return forget, nil
	}
	// reqLogger.Info(fmt.Sprintf("chaosblade obj: %+v", cb))

	// Destroyed->delete
	// Remove the Finalizer if the CR object status is destroyed to delete it
	if cb.Status.Phase == v1alpha1.ClusterPhaseDestroyed {
		cb.SetFinalizers(remove(cb.GetFinalizers(), chaosbladeFinalizer))
		err := r.client.Update(ctx, cb)
		if err != nil {
			reqLogger.WithError(err).Errorln("remove chaosblade finalizer failed at destroyed phase")
		}
		return forget, nil
	}
	if cb.Status.Phase == v1alpha1.ClusterPhaseDestroying || cb.GetDeletionTimestamp() != nil {
		err := r.finalizeChaosBlade(ctx, reqLogger, cb)
		if err != nil {
			reqLogger.WithError(err).Errorln("finalize chaosblade failed at destroying phase")
		}
		return forget, nil
	}
	// Initial->Initialized
	if cb.Status.Phase == v1alpha1.ClusterPhaseInitial {
		if contains(cb.GetFinalizers(), chaosbladeFinalizer) {
			cb.Status.Phase = v1alpha1.ClusterPhaseInitialized
			cb.Status.ExpStatuses = make([]v1alpha1.ExperimentStatus, 0)
			if err := r.client.Status().Update(ctx, cb); err != nil {
				reqLogger.WithError(err).Errorln("update chaosblade phase to Initialized failed")
			}
		} else {
			cb.SetFinalizers(append(cb.GetFinalizers(), chaosbladeFinalizer))
			// Update CR
			if err := r.client.Update(ctx, cb); err != nil {
				reqLogger.WithError(err).Errorln("add finalizer to chaosblade failed")
			}
		}
		return forget, nil
	}
	// Initialized->Running/Error
	// TODO When all the master nodes are inaccessible, there is the possibility of re-execution.
	if cb.Status.Phase == v1alpha1.ClusterPhaseInitialized ||
		cb.Status.Phase == v1alpha1.ClusterPhaseUpdating {
		originalPhase := cb.Status.Phase
		expStatusList := make([]v1alpha1.ExperimentStatus, 0)
		phase := v1alpha1.ClusterPhaseError
		for _, exp := range cb.Spec.Experiments {
			experimentStatus := r.Executor.Create(cb.Name, exp)
			if experimentStatus.Success {
				phase = v1alpha1.ClusterPhaseRunning
			}
			expStatusList = append(expStatusList, experimentStatus)
		}
		cb.Status.ExpStatuses = expStatusList
		cb.Status.Phase = phase
		if err := r.client.Status().Update(ctx, cb); err != nil {
			reqLogger.WithError(err).Errorf("Important!!!!!update phase from %s to %s failed", originalPhase, phase)
		}
		return forget, nil
	}

	// Running/Error->Updating/Destroying
	if cb.Status.Phase == v1alpha1.ClusterPhaseRunning ||
		cb.Status.Phase == v1alpha1.ClusterPhaseError {
		// Update CR, firstly destroy it and re-create the new CR
		phase := v1alpha1.ClusterPhaseUpdating
		originalPhase := cb.Status.Phase
		logrus.Infof("update cb: %+v", *cb)
		matchersString := cb.GetAnnotations()["preSpec"]
		if matchersString != "" {
			var oldSpec v1alpha1.ChaosBladeSpec
			err := json.Unmarshal([]byte(matchersString), &oldSpec)
			if err != nil {
				reqLogger.WithError(err).Errorf("unmarshal old spec failed, %s", matchersString)
				return forget, nil
			}
			// update annotation to cb
			if err = r.client.Update(ctx, cb); err != nil {
				reqLogger.WithError(err).Errorln("add annotation to chaosblade failed")
			}
			if cb.Status.ExpStatuses != nil {
				for idx, expStatus := range cb.Status.ExpStatuses {
					experimentStatus := r.Executor.Destroy(cb.Name, oldSpec.Experiments[idx], expStatus)
					if !experimentStatus.Success {
						phase = v1alpha1.ClusterPhaseDestroying
					}
					cb.Status.ExpStatuses[idx] = experimentStatus
				}
			}
			cb.Status.Phase = phase
			if err := r.client.Status().Update(ctx, cb); err != nil {
				reqLogger.WithError(err).Errorf("update phase from %s to %s failed", originalPhase, phase)
			}
			return forget, nil
		}
		reqLogger.Errorln("can not found matchers in annotations field")
	}
	return forget, nil
}

// finalizeChaosBlade
func (r *ReconcileChaosBlade) finalizeChaosBlade(ctx context.Context, reqLogger *logrus.Entry, cb *v1alpha1.ChaosBlade) error {
	phase := v1alpha1.ClusterPhaseDestroyed
	reqLogger.Infoln("Finalize the chaosblade")
	if cb.Status.ExpStatuses != nil &&
		len(cb.Spec.Experiments) == len(cb.Status.ExpStatuses) {
		for idx, exp := range cb.Spec.Experiments {
			oldExpStatus := cb.Status.ExpStatuses[idx]
			oldExpStatus = r.Executor.Destroy(cb.Name, exp, oldExpStatus)
			if !oldExpStatus.Success {
				phase = v1alpha1.ClusterPhaseDestroying
			}
			cb.Status.ExpStatuses[idx] = oldExpStatus
		}
	}
	cb.Status.Phase = phase
	err := r.client.Status().Update(ctx, cb)
	if err != nil {
		return fmt.Errorf("update chaosblade status failed in finalize phase, %v", err)
	}
	if cb.Status.Phase == v1alpha1.ClusterPhaseDestroying {
		return fmt.Errorf("failed to destory, please see the experiment status")
	}
	reqLogger.Info("Successfully finalized chaosblade")
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
