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
	"strings"

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
	// ChaosBladeDeploymentAnnotation indicates the deployment resource is modified
	ChaosBladeDeploymentAnnotation = "chaosblade.io/deployment"
	// ChaosBladeDaemonSetAnnotation indicates the daemonset resource is modified
	ChaosBladeDaemonSetAnnotation = "chaosblade.io/daemonset"
	// ChaosBladeStatefulSetAnnotation indicates the statefulset resource is modified
	ChaosBladeStatefulSetAnnotation = "chaosblade.io/statefulset"
	// ChaosBladeModifyAction indicates modify action
	ChaosBladeModifyAction = "modify"
	// ChaosBladeOriginalNodeAffinityAnnotation stores the original node affinity configuration
	ChaosBladeOriginalNodeAffinityAnnotation = "chaosblade.io/original-node-affinity"
	// ChaosBladeOriginalPodAffinityAnnotation stores the original pod affinity configuration
	ChaosBladeOriginalPodAffinityAnnotation = "chaosblade.io/original-pod-affinity"
	// ChaosBladeOriginalPodAntiAffinityAnnotation stores the original pod anti-affinity configuration
	ChaosBladeOriginalPodAntiAffinityAnnotation = "chaosblade.io/original-pod-anti-affinity"
	// ChaosBladeOriginalNodeSelectorAnnotation stores the original node selector configuration
	ChaosBladeOriginalNodeSelectorAnnotation = "chaosblade.io/original-nodeselector"
	// ChaosBladeAffinityTypeAnnotation records which affinity type was injected
	ChaosBladeAffinityTypeAnnotation = "chaosblade.io/affinity-type"
	// ChaosBladeSchedulingFailureAction indicates scheduling failure action
	ChaosBladeSchedulingFailureAction = "schedulingfailure"
	// UnreachableNodeLabel is a label that no node will have
	UnreachableNodeLabelKey   = "chaosblade.io/unreachable"
	UnreachableNodeLabelValue = "true"
)

type PodSchedulingFailureActionSpec struct {
	spec.BaseExpActionCommandSpec
	client *channel.Client
}

func NewPodSchedulingFailureActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodSchedulingFailureActionSpec{
		BaseExpActionCommandSpec: spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:     "workload-type",
					Desc:     "Workload type: deployment, daemonset, statefulset. Default: deployment",
					Required: false,
					Default:  "deployment",
				},
				&spec.ExpFlag{
					Name:     "workload-name",
					Desc:     "Workload name to inject scheduling failure",
					Required: true,
				},
				&spec.ExpFlag{
					Name:     "affinity-type",
					Desc:     "Affinity type to inject: node-affinity, node-selector, pod-affinity, pod-anti-affinity. Default: node-affinity",
					Required: false,
					Default:  "node-affinity",
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
		client: client,
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

// handleGetError handles errors from client.Get operations in destroy.
// Returns true if the error was handled (NotFound or other error), false otherwise.
// When returning true, the response pointer is set with the appropriate status.
func handleGetError(err error, namespace, workloadType, workloadName string, status *v1alpha1.ResourceStatus, logrusField *logrus.Entry) (*spec.Response, bool) {
	if err == nil {
		return nil, false
	}
	if apierrors.IsNotFound(err) {
		logrusField.Infof("%s %s/%s already deleted", workloadType, namespace, workloadName)
		*status = status.CreateSuccessResourceStatus()
		status.State = v1alpha1.DestroyedState
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{*status})), true
	}
	logrusField.Warningf("get %s %s/%s failed: %v", workloadType, namespace, workloadName, err)
	*status = status.CreateFailResourceStatus(fmt.Sprintf("get %s failed: %v", workloadType, err), spec.K8sExecFailed.Code)
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{*status})), true
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
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
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
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
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
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
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

// ensureNoConflictingExperiment checks whether the workload is already modified by
// another chaosblade experiment. This prevents overwriting backup data from a
// running experiment, which would make the workload unrecoverable on destroy.
func ensureNoConflictingExperiment(annotations map[string]string, experimentId string) error {
	if existingId, ok := annotations[ChaosBladeExperimentAnnotation]; ok && existingId != "" && existingId != experimentId {
		return fmt.Errorf("workload is already modified by another chaosblade experiment: %s", existingId)
	}
	return nil
}

// injectDeploymentSchedulingFailure injects scheduling failure to a Deployment
func (d *PodSchedulingFailureActionExecutor) injectDeploymentSchedulingFailure(ctx context.Context, deployment *appsv1.Deployment, affinityType, experimentId string) error {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(deployment.Annotations, experimentId); err != nil {
		return err
	}
	// Idempotent: if already modified by the same experiment, skip re-injection
	// to avoid overwriting the saved original affinity backup.
	if deployment.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	deployment.Annotations[ChaosBladeDeploymentAnnotation] = ChaosBladeModifyAction
	deployment.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	// Backup and inject affinity
	if err := d.backupAndInjectAffinity(&deployment.Spec.Template.Spec, deployment.Annotations, affinityType, deployment.Spec.Template.Labels); err != nil {
		return err
	}

	return d.client.Update(ctx, deployment)
}

// injectDaemonSetSchedulingFailure injects scheduling failure to a DaemonSet
func (d *PodSchedulingFailureActionExecutor) injectDaemonSetSchedulingFailure(ctx context.Context, daemonset *appsv1.DaemonSet, affinityType, experimentId string) error {
	if daemonset.Annotations == nil {
		daemonset.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(daemonset.Annotations, experimentId); err != nil {
		return err
	}
	if daemonset.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	daemonset.Annotations[ChaosBladeDaemonSetAnnotation] = ChaosBladeModifyAction
	daemonset.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if err := d.backupAndInjectAffinity(&daemonset.Spec.Template.Spec, daemonset.Annotations, affinityType, daemonset.Spec.Template.Labels); err != nil {
		return err
	}

	return d.client.Update(ctx, daemonset)
}

// injectStatefulSetSchedulingFailure injects scheduling failure to a StatefulSet
func (d *PodSchedulingFailureActionExecutor) injectStatefulSetSchedulingFailure(ctx context.Context, statefulset *appsv1.StatefulSet, affinityType, experimentId string) error {
	if statefulset.Annotations == nil {
		statefulset.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(statefulset.Annotations, experimentId); err != nil {
		return err
	}
	if statefulset.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	statefulset.Annotations[ChaosBladeStatefulSetAnnotation] = ChaosBladeModifyAction
	statefulset.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if err := d.backupAndInjectAffinity(&statefulset.Spec.Template.Spec, statefulset.Annotations, affinityType, statefulset.Spec.Template.Labels); err != nil {
		return err
	}

	return d.client.Update(ctx, statefulset)
}

// backupAndInjectAffinity backs up original affinity and injects unreachable affinity rules
// podLabels is used by pod-anti-affinity to target the workload's own labels
func (d *PodSchedulingFailureActionExecutor) backupAndInjectAffinity(podSpec *v1.PodSpec, annotations map[string]string, affinityType string, podLabels map[string]string) error {
	annotations[ChaosBladeAffinityTypeAnnotation] = affinityType

	switch affinityType {
	case "node-affinity":
		// Backup original affinity
		if podSpec.Affinity != nil && podSpec.Affinity.NodeAffinity != nil {
			originalBytes, err := json.Marshal(podSpec.Affinity.NodeAffinity)
			if err != nil {
				return fmt.Errorf("marshal original node affinity failed: %v", err)
			}
			annotations[ChaosBladeOriginalNodeAffinityAnnotation] = string(originalBytes)
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
		if len(podSpec.NodeSelector) > 0 {
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
			annotations[ChaosBladeOriginalPodAffinityAnnotation] = string(originalBytes)
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
			annotations[ChaosBladeOriginalPodAntiAffinityAnnotation] = string(originalBytes)
		}

		// Inject pod anti-affinity against the workload's own labels
		// This creates a "one pod per node" constraint: new pods can't be scheduled
		// on nodes that already have a pod with the same labels.
		// With enough replicas (> number of available nodes), pods will be Pending.
		var matchExpressions []metav1.LabelSelectorRequirement
		for key, value := range podLabels {
			matchExpressions = append(matchExpressions, metav1.LabelSelectorRequirement{
				Key:      key,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{value},
			})
		}
		if len(matchExpressions) == 0 {
			return fmt.Errorf("pod template has no labels, cannot inject pod-anti-affinity")
		}

		unreachablePodAntiAffinity := &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: matchExpressions,
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

// PreCreate implements model.ActionPreProcessor interface.
// It validates the required flags and prepares the context for schedulingfailure action.
func (a *PodSchedulingFailureActionSpec) PreCreate(ctx context.Context, expModel *spec.ExpModel, client *channel.Client) (context.Context, *spec.Response) {
	// Validate required flags
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	if namespace == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if strings.Contains(namespace, ",") {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-sf-%s-%s", workloadType, workloadName),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}

// PreDestroy implements model.ActionPreProcessor interface.
// It prepares the context for schedulingfailure destroy flow.
// Unlike the default destroy path, it always attempts to restore the workload
// regardless of the old experiment status.
func (a *PodSchedulingFailureActionSpec) PreDestroy(ctx context.Context, expModel *spec.ExpModel, client *channel.Client, oldExpStatus v1alpha1.ExperimentStatus) (context.Context, *spec.Response) {
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	if namespace == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if strings.Contains(namespace, ",") {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-sf-%s-%s", workloadType, workloadName),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
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
	delete(deployment.Annotations, ChaosBladeDeploymentAnnotation)
	delete(deployment.Annotations, ChaosBladeExperimentAnnotation)
	delete(deployment.Annotations, ChaosBladeAffinityTypeAnnotation)
	delete(deployment.Annotations, ChaosBladeOriginalNodeAffinityAnnotation)
	delete(deployment.Annotations, ChaosBladeOriginalPodAffinityAnnotation)
	delete(deployment.Annotations, ChaosBladeOriginalPodAntiAffinityAnnotation)
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

	delete(daemonset.Annotations, ChaosBladeDaemonSetAnnotation)
	delete(daemonset.Annotations, ChaosBladeExperimentAnnotation)
	delete(daemonset.Annotations, ChaosBladeAffinityTypeAnnotation)
	delete(daemonset.Annotations, ChaosBladeOriginalNodeAffinityAnnotation)
	delete(daemonset.Annotations, ChaosBladeOriginalPodAffinityAnnotation)
	delete(daemonset.Annotations, ChaosBladeOriginalPodAntiAffinityAnnotation)
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

	delete(statefulset.Annotations, ChaosBladeStatefulSetAnnotation)
	delete(statefulset.Annotations, ChaosBladeExperimentAnnotation)
	delete(statefulset.Annotations, ChaosBladeAffinityTypeAnnotation)
	delete(statefulset.Annotations, ChaosBladeOriginalNodeAffinityAnnotation)
	delete(statefulset.Annotations, ChaosBladeOriginalPodAffinityAnnotation)
	delete(statefulset.Annotations, ChaosBladeOriginalPodAntiAffinityAnnotation)
	delete(statefulset.Annotations, ChaosBladeOriginalNodeSelectorAnnotation)

	return d.client.Update(ctx, statefulset)
}

// restoreAffinity restores only the affinity field that was modified during injection.
// It uses the ChaosBladeAffinityTypeAnnotation to determine which field to restore,
// avoiding unintentional clearing of pre-existing affinity/selector settings.
func (d *PodSchedulingFailureActionExecutor) restoreAffinity(podSpec *v1.PodSpec, annotations map[string]string) error {
	affinityType := annotations[ChaosBladeAffinityTypeAnnotation]
	if affinityType == "" {
		return fmt.Errorf("affinity type annotation not found, cannot determine which field to restore")
	}

	switch affinityType {
	case "node-affinity":
		if originalNodeAffinityStr, ok := annotations[ChaosBladeOriginalNodeAffinityAnnotation]; ok {
			var nodeAffinity v1.NodeAffinity
			if err := json.Unmarshal([]byte(originalNodeAffinityStr), &nodeAffinity); err != nil {
				return fmt.Errorf("unmarshal original node affinity failed: %v", err)
			}
			if podSpec.Affinity == nil {
				podSpec.Affinity = &v1.Affinity{}
			}
			podSpec.Affinity.NodeAffinity = &nodeAffinity
		} else {
			if podSpec.Affinity != nil {
				podSpec.Affinity.NodeAffinity = nil
			}
		}

	case "pod-affinity":
		if originalPodAffinityStr, ok := annotations[ChaosBladeOriginalPodAffinityAnnotation]; ok {
			var podAffinity v1.PodAffinity
			if err := json.Unmarshal([]byte(originalPodAffinityStr), &podAffinity); err != nil {
				return fmt.Errorf("unmarshal original pod affinity failed: %v", err)
			}
			if podSpec.Affinity == nil {
				podSpec.Affinity = &v1.Affinity{}
			}
			podSpec.Affinity.PodAffinity = &podAffinity
		} else {
			if podSpec.Affinity != nil {
				podSpec.Affinity.PodAffinity = nil
			}
		}

	case "pod-anti-affinity":
		if originalPodAntiAffinityStr, ok := annotations[ChaosBladeOriginalPodAntiAffinityAnnotation]; ok {
			var podAntiAffinity v1.PodAntiAffinity
			if err := json.Unmarshal([]byte(originalPodAntiAffinityStr), &podAntiAffinity); err != nil {
				return fmt.Errorf("unmarshal original pod anti-affinity failed: %v", err)
			}
			if podSpec.Affinity == nil {
				podSpec.Affinity = &v1.Affinity{}
			}
			podSpec.Affinity.PodAntiAffinity = &podAntiAffinity
		} else {
			if podSpec.Affinity != nil {
				podSpec.Affinity.PodAntiAffinity = nil
			}
		}

	case "node-selector":
		if originalNodeSelectorStr, ok := annotations[ChaosBladeOriginalNodeSelectorAnnotation]; ok {
			var originalNodeSelector map[string]string
			if err := json.Unmarshal([]byte(originalNodeSelectorStr), &originalNodeSelector); err != nil {
				return fmt.Errorf("unmarshal original node selector failed: %v", err)
			}
			podSpec.NodeSelector = originalNodeSelector
		} else {
			podSpec.NodeSelector = nil
		}

	default:
		return fmt.Errorf("unknown affinity type in annotation: %s", affinityType)
	}

	// Clean up empty Affinity struct
	if podSpec.Affinity != nil &&
		podSpec.Affinity.NodeAffinity == nil &&
		podSpec.Affinity.PodAffinity == nil &&
		podSpec.Affinity.PodAntiAffinity == nil {
		podSpec.Affinity = nil
	}

	return nil
}
