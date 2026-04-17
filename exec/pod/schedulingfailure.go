/*
 * Copyright 2025 The ChaosBlade Authors
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

package pod

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	// ChaosBladeWorkloadAnnotation is the annotation for workload resources modified by schedulingfailure action
	ChaosBladeWorkloadAnnotation = "chaosblade.io/workload"
	// ChaosBladeOriginalAffinityAnnotation stores the original affinity configuration
	ChaosBladeOriginalAffinityAnnotation = "chaosblade.io/original-affinity"
	// ChaosBladeOriginalNodeSelectorAnnotation stores the original node selector configuration
	ChaosBladeOriginalNodeSelectorAnnotation = "chaosblade.io/original-nodeselector"
	// ChaosBladeSchedulingFailureAction indicates scheduling failure action
	ChaosBladeSchedulingFailureAction = "schedulingfailure"
	// UnreachableNodeLabel is a label that no node will have
	UnreachableNodeLabelKey   = "chaosblade.io/unreachable"
	UnreachableNodeLabelValue = "true"
)

type PodSchedulingFailureActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewPodSchedulingFailureActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodSchedulingFailureActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: "workload-type",
					Desc: "Workload type: deployment, daemonset, statefulset. Default: deployment",
				},
				&spec.ExpFlag{
					Name: "workload-name",
					Desc: "Workload name to inject scheduling failure",
				},
				&spec.ExpFlag{
					Name: "affinity-type",
					Desc: "Affinity type to inject: node-affinity, node-selector, pod-affinity, pod-anti-affinity. Default: node-affinity",
				},
			},
			ActionExecutor: &PodSchedulingFailureActionExecutor{client: client},
			ActionExample: `# Inject scheduling failure to a deployment by node affinity
blade create k8s pod-pod schedulingfailure --namespace default --workload-type deployment --workload-name nginx-deployment --kubeconfig ~/.kube/config

# Inject scheduling failure using node-selector
blade create k8s pod-pod schedulingfailure --namespace default --workload-type deployment --workload-name nginx-deployment --affinity-type node-selector --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*PodSchedulingFailureActionSpec) Name() string {
	return "schedulingfailure"
}

func (*PodSchedulingFailureActionSpec) Aliases() []string {
	return []string{}
}

func (*PodSchedulingFailureActionSpec) ShortDesc() string {
	return "Make pod scheduling fail by injecting unreachable affinity rules"
}

func (*PodSchedulingFailureActionSpec) LongDesc() string {
	return "Simulate the scenario where a Pod cannot be scheduled due to affinity configuration issues. " +
		"This fault is injected by modifying the target workload's (Deployment/DaemonSet/StatefulSet) Pod template " +
		"to add an unreachable node affinity or node selector. The scheduler will not find any node matching the rules, " +
		"causing the Pod to remain in Pending state. When the experiment is destroyed, the original affinity " +
		"configuration will be restored."
}

type PodSchedulingFailureActionExecutor struct {
	client *channel.Client
}

func (*PodSchedulingFailureActionExecutor) Name() string {
	return "schedulingfailure"
}

func (*PodSchedulingFailureActionExecutor) SetChannel(channel spec.Channel) {}

func (d *PodSchedulingFailureActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *PodSchedulingFailureActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	// Parse flags
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]
	affinityType := expModel.ActionFlags["affinity-type"]
	if affinityType == "" {
		affinityType = "node-affinity"
	}

	// Validate required flags
	if namespace == "" {
		util.Errorf(uid, util.GetRunFuncName(), "namespace is required")
		return spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		util.Errorf(uid, util.GetRunFuncName(), "workload-name is required")
		return spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}

	status := v1alpha1.ResourceStatus{
		Kind:       v1alpha1.PodKind,
		Identifier: fmt.Sprintf("%s//%s//%s", namespace, workloadType, workloadName),
	}

	// Get and modify the workload
	switch workloadType {
	case "deployment":
		deployment := &appsv1.Deployment{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, deployment)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("deployment %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("deployment not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get deployment %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get deployment failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		// Inject scheduling failure
		if err := d.injectDeploymentSchedulingFailure(ctx, deployment, affinityType, experimentId); err != nil {
			logrusField.Warningf("inject scheduling failure to deployment %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject scheduling failure failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected scheduling failure to deployment %s/%s with affinity type %s", namespace, workloadName, affinityType)

	case "daemonset":
		daemonset := &appsv1.DaemonSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, daemonset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("daemonset %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("daemonset not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get daemonset %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get daemonset failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.injectDaemonSetSchedulingFailure(ctx, daemonset, affinityType, experimentId); err != nil {
			logrusField.Warningf("inject scheduling failure to daemonset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject scheduling failure failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected scheduling failure to daemonset %s/%s with affinity type %s", namespace, workloadName, affinityType)

	case "statefulset":
		statefulset := &appsv1.StatefulSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, statefulset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("statefulset %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("statefulset not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get statefulset %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get statefulset failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.injectStatefulSetSchedulingFailure(ctx, statefulset, affinityType, experimentId); err != nil {
			logrusField.Warningf("inject scheduling failure to statefulset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject scheduling failure failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected scheduling failure to statefulset %s/%s with affinity type %s", namespace, workloadName, affinityType)

	default:
		status = status.CreateFailResourceStatus(fmt.Sprintf("unsupported workload type: %s", workloadType), spec.ParameterIllegal.Code)
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
	}

	status = status.CreateSuccessResourceStatus()
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

func (d *PodSchedulingFailureActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	// Parse flags
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	status := v1alpha1.ResourceStatus{
		Kind:       v1alpha1.PodKind,
		Identifier: fmt.Sprintf("%s//%s//%s", namespace, workloadType, workloadName),
	}

	// Restore the workload
	switch workloadType {
	case "deployment":
		deployment := &appsv1.Deployment{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, deployment)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("deployment %s/%s already deleted", namespace, workloadName)
				status = status.CreateSuccessResourceStatus()
				status.State = v1alpha1.DestroyedState
				return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{status}))
			}
			logrusField.Warningf("get deployment %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("get deployment failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.restoreDeployment(ctx, deployment, experimentId); err != nil {
			logrusField.Warningf("restore deployment %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore deployment failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored deployment %s/%s", namespace, workloadName)

	case "daemonset":
		daemonset := &appsv1.DaemonSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, daemonset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("daemonset %s/%s already deleted", namespace, workloadName)
				status = status.CreateSuccessResourceStatus()
				status.State = v1alpha1.DestroyedState
				return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{status}))
			}
			logrusField.Warningf("get daemonset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("get daemonset failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.restoreDaemonSet(ctx, daemonset, experimentId); err != nil {
			logrusField.Warningf("restore daemonset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore daemonset failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored daemonset %s/%s", namespace, workloadName)

	case "statefulset":
		statefulset := &appsv1.StatefulSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, statefulset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Infof("statefulset %s/%s already deleted", namespace, workloadName)
				status = status.CreateSuccessResourceStatus()
				status.State = v1alpha1.DestroyedState
				return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{status}))
			}
			logrusField.Warningf("get statefulset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("get statefulset failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.restoreStatefulSet(ctx, statefulset, experimentId); err != nil {
			logrusField.Warningf("restore statefulset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore statefulset failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored statefulset %s/%s", namespace, workloadName)

	default:
		status = status.CreateFailResourceStatus(fmt.Sprintf("unsupported workload type: %s", workloadType), spec.ParameterIllegal.Code)
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
	}

	status = status.CreateSuccessResourceStatus()
	status.State = v1alpha1.DestroyedState
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

// injectDeploymentSchedulingFailure injects scheduling failure to a Deployment
func (d *PodSchedulingFailureActionExecutor) injectDeploymentSchedulingFailure(ctx context.Context, deployment *appsv1.Deployment, affinityType, experimentId string) error {
	// Backup original configuration
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	deployment.Annotations[ChaosBladeWorkloadAnnotation] = ChaosBladeSchedulingFailureAction
	deployment.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	// Backup and inject affinity
	if err := d.backupAndInjectAffinity(&deployment.Spec.Template.Spec, deployment.Annotations, affinityType); err != nil {
		return err
	}

	return d.client.Update(ctx, deployment)
}

// injectDaemonSetSchedulingFailure injects scheduling failure to a DaemonSet
func (d *PodSchedulingFailureActionExecutor) injectDaemonSetSchedulingFailure(ctx context.Context, daemonset *appsv1.DaemonSet, affinityType, experimentId string) error {
	if daemonset.Annotations == nil {
		daemonset.Annotations = make(map[string]string)
	}
	daemonset.Annotations[ChaosBladeWorkloadAnnotation] = ChaosBladeSchedulingFailureAction
	daemonset.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if err := d.backupAndInjectAffinity(&daemonset.Spec.Template.Spec, daemonset.Annotations, affinityType); err != nil {
		return err
	}

	return d.client.Update(ctx, daemonset)
}

// injectStatefulSetSchedulingFailure injects scheduling failure to a StatefulSet
func (d *PodSchedulingFailureActionExecutor) injectStatefulSetSchedulingFailure(ctx context.Context, statefulset *appsv1.StatefulSet, affinityType, experimentId string) error {
	if statefulset.Annotations == nil {
		statefulset.Annotations = make(map[string]string)
	}
	statefulset.Annotations[ChaosBladeWorkloadAnnotation] = ChaosBladeSchedulingFailureAction
	statefulset.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if err := d.backupAndInjectAffinity(&statefulset.Spec.Template.Spec, statefulset.Annotations, affinityType); err != nil {
		return err
	}

	return d.client.Update(ctx, statefulset)
}

// backupAndInjectAffinity backs up original affinity and injects unreachable affinity rules
func (d *PodSchedulingFailureActionExecutor) backupAndInjectAffinity(podSpec *v1.PodSpec, annotations map[string]string, affinityType string) error {
	switch affinityType {
	case "node-affinity":
		// Backup original affinity
		if podSpec.Affinity != nil && podSpec.Affinity.NodeAffinity != nil {
			originalBytes, err := json.Marshal(podSpec.Affinity.NodeAffinity)
			if err != nil {
				return fmt.Errorf("marshal original node affinity failed: %v", err)
			}
			annotations[ChaosBladeOriginalAffinityAnnotation] = string(originalBytes)
		}

		// Inject unreachable node affinity
		unreachableAffinity := &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      UnreachableNodeLabelKey,
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{UnreachableNodeLabelValue},
							},
						},
					},
				},
			},
		}

		if podSpec.Affinity == nil {
			podSpec.Affinity = &v1.Affinity{}
		}
		podSpec.Affinity.NodeAffinity = unreachableAffinity

	case "node-selector":
		// Backup original node selector
		if podSpec.NodeSelector != nil && len(podSpec.NodeSelector) > 0 {
			originalBytes, err := json.Marshal(podSpec.NodeSelector)
			if err != nil {
				return fmt.Errorf("marshal original node selector failed: %v", err)
			}
			annotations[ChaosBladeOriginalNodeSelectorAnnotation] = string(originalBytes)
		}

		// Inject unreachable node selector
		podSpec.NodeSelector = map[string]string{
			UnreachableNodeLabelKey: UnreachableNodeLabelValue,
		}

	case "pod-affinity":
		// Backup original pod affinity
		if podSpec.Affinity != nil && podSpec.Affinity.PodAffinity != nil {
			originalBytes, err := json.Marshal(podSpec.Affinity.PodAffinity)
			if err != nil {
				return fmt.Errorf("marshal original pod affinity failed: %v", err)
			}
			annotations[ChaosBladeOriginalAffinityAnnotation] = string(originalBytes)
		}

		// Inject unreachable pod affinity
		unreachablePodAffinity := &v1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      UnreachableNodeLabelKey,
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{UnreachableNodeLabelValue},
							},
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		}

		if podSpec.Affinity == nil {
			podSpec.Affinity = &v1.Affinity{}
		}
		podSpec.Affinity.PodAffinity = unreachablePodAffinity

	case "pod-anti-affinity":
		// Backup original pod anti-affinity
		if podSpec.Affinity != nil && podSpec.Affinity.PodAntiAffinity != nil {
			originalBytes, err := json.Marshal(podSpec.Affinity.PodAntiAffinity)
			if err != nil {
				return fmt.Errorf("marshal original pod anti-affinity failed: %v", err)
			}
			annotations[ChaosBladeOriginalAffinityAnnotation] = string(originalBytes)
		}

		// Inject unreachable pod anti-affinity (this will block scheduling on any node)
		unreachablePodAntiAffinity := &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: metav1.LabelSelectorOpExists,
							},
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		}

		if podSpec.Affinity == nil {
			podSpec.Affinity = &v1.Affinity{}
		}
		podSpec.Affinity.PodAntiAffinity = unreachablePodAntiAffinity

	default:
		return fmt.Errorf("unsupported affinity type: %s", affinityType)
	}

	return nil
}

// restoreDeployment restores a Deployment's original affinity configuration
func (d *PodSchedulingFailureActionExecutor) restoreDeployment(ctx context.Context, deployment *appsv1.Deployment, experimentId string) error {
	// Verify this deployment was modified by the same experiment
	if deployment.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("deployment was not modified by experiment %s", experimentId)
	}

	if err := d.restoreAffinity(&deployment.Spec.Template.Spec, deployment.Annotations); err != nil {
		return err
	}

	// Clean up annotations
	delete(deployment.Annotations, ChaosBladeWorkloadAnnotation)
	delete(deployment.Annotations, ChaosBladeExperimentAnnotation)
	delete(deployment.Annotations, ChaosBladeOriginalAffinityAnnotation)
	delete(deployment.Annotations, ChaosBladeOriginalNodeSelectorAnnotation)

	return d.client.Update(ctx, deployment)
}

// restoreDaemonSet restores a DaemonSet's original affinity configuration
func (d *PodSchedulingFailureActionExecutor) restoreDaemonSet(ctx context.Context, daemonset *appsv1.DaemonSet, experimentId string) error {
	if daemonset.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("daemonset was not modified by experiment %s", experimentId)
	}

	if err := d.restoreAffinity(&daemonset.Spec.Template.Spec, daemonset.Annotations); err != nil {
		return err
	}

	delete(daemonset.Annotations, ChaosBladeWorkloadAnnotation)
	delete(daemonset.Annotations, ChaosBladeExperimentAnnotation)
	delete(daemonset.Annotations, ChaosBladeOriginalAffinityAnnotation)
	delete(daemonset.Annotations, ChaosBladeOriginalNodeSelectorAnnotation)

	return d.client.Update(ctx, daemonset)
}

// restoreStatefulSet restores a StatefulSet's original affinity configuration
func (d *PodSchedulingFailureActionExecutor) restoreStatefulSet(ctx context.Context, statefulset *appsv1.StatefulSet, experimentId string) error {
	if statefulset.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("statefulset was not modified by experiment %s", experimentId)
	}

	if err := d.restoreAffinity(&statefulset.Spec.Template.Spec, statefulset.Annotations); err != nil {
		return err
	}

	delete(statefulset.Annotations, ChaosBladeWorkloadAnnotation)
	delete(statefulset.Annotations, ChaosBladeExperimentAnnotation)
	delete(statefulset.Annotations, ChaosBladeOriginalAffinityAnnotation)
	delete(statefulset.Annotations, ChaosBladeOriginalNodeSelectorAnnotation)

	return d.client.Update(ctx, statefulset)
}

// restoreAffinity restores the original affinity configuration from annotations
func (d *PodSchedulingFailureActionExecutor) restoreAffinity(podSpec *v1.PodSpec, annotations map[string]string) error {
	// Restore node affinity
	if originalAffinityStr, ok := annotations[ChaosBladeOriginalAffinityAnnotation]; ok {
		var originalAffinity interface{}
		if err := json.Unmarshal([]byte(originalAffinityStr), &originalAffinity); err != nil {
			return fmt.Errorf("unmarshal original affinity failed: %v", err)
		}

		// Try to determine the type and restore
		switch v := originalAffinity.(type) {
		case map[string]interface{}:
			// Could be NodeAffinity or PodAffinity or PodAntiAffinity
			// Check which one was backed up by looking at the structure
			if _, hasNodeSelectorTerms := v["nodeSelectorTerms"]; hasNodeSelectorTerms {
				var nodeAffinity v1.NodeAffinity
				if err := json.Unmarshal([]byte(originalAffinityStr), &nodeAffinity); err != nil {
					return fmt.Errorf("unmarshal node affinity failed: %v", err)
				}
				if podSpec.Affinity == nil {
					podSpec.Affinity = &v1.Affinity{}
				}
				podSpec.Affinity.NodeAffinity = &nodeAffinity
			} else if _, hasRequiredDuringScheduling := v["requiredDuringSchedulingIgnoredDuringExecution"]; hasRequiredDuringScheduling {
				// Could be PodAffinity or PodAntiAffinity - restore to both for simplicity
				var podAffinity v1.PodAffinity
				if err := json.Unmarshal([]byte(originalAffinityStr), &podAffinity); err != nil {
					return fmt.Errorf("unmarshal pod affinity failed: %v", err)
				}
				if podSpec.Affinity == nil {
					podSpec.Affinity = &v1.Affinity{}
				}
				podSpec.Affinity.PodAffinity = &podAffinity
			}
		}
	} else {
		// No backup means there was no original affinity, clear injected one
		if podSpec.Affinity != nil {
			podSpec.Affinity.NodeAffinity = nil
			podSpec.Affinity.PodAffinity = nil
			podSpec.Affinity.PodAntiAffinity = nil
			// If all affinity fields are nil, set Affinity to nil
			if podSpec.Affinity.NodeAffinity == nil &&
				podSpec.Affinity.PodAffinity == nil &&
				podSpec.Affinity.PodAntiAffinity == nil {
				podSpec.Affinity = nil
			}
		}
	}

	// Restore node selector
	if originalNodeSelectorStr, ok := annotations[ChaosBladeOriginalNodeSelectorAnnotation]; ok {
		var originalNodeSelector map[string]string
		if err := json.Unmarshal([]byte(originalNodeSelectorStr), &originalNodeSelector); err != nil {
			return fmt.Errorf("unmarshal original node selector failed: %v", err)
		}
		podSpec.NodeSelector = originalNodeSelector
	} else {
		// No backup means there was no original node selector, clear injected one
		podSpec.NodeSelector = nil
	}

	return nil
}
