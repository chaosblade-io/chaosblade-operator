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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	// ChaosBladeTaintAnnotation indicates the node taint was modified by chaosblade
	ChaosBladeTaintAnnotation = "chaosblade.io/taint"
	// ChaosBladeOriginalTaintsAnnotation stores the original node taints
	ChaosBladeOriginalTaintsAnnotation = "chaosblade.io/original-taints"
	// ChaosBladeInjectedTaintAnnotation stores the taint injected by this experiment
	ChaosBladeInjectedTaintAnnotation = "chaosblade.io/injected-taint"
	// DefaultTaintKey is the default taint key for injection
	DefaultTaintKey = "chaosblade.io/unreachable"
	// DefaultTaintValue is the default taint value for injection
	DefaultTaintValue = "true"
	// DefaultTaintEffect is the default taint effect
	DefaultTaintEffect = "NoSchedule"
)

// chaosBladeTaintAnnotations returns the taintnode-owned annotation keys for cleanup.
func chaosBladeTaintAnnotations() []string {
	return []string{
		ChaosBladeOriginalTaintsAnnotation,
		ChaosBladeInjectedTaintAnnotation,
		ChaosBladeExperimentAnnotation,
	}
}

type PodTaintNodeActionSpec struct {
	spec.BaseExpActionCommandSpec
	client *channel.Client
}

func NewPodTaintNodeActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodTaintNodeActionSpec{
		BaseExpActionCommandSpec: spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:     "nodes",
					Desc:     "Node names to inject taint, multiple values separated by commas",
					Required: true,
				},
				&spec.ExpFlag{
					Name:    "taint-effect",
					Desc:    "Taint effect: NoSchedule (default), NoExecute, PreferNoSchedule. WARNING: NoExecute will evict running pods without matching tolerations",
					NoArgs:  false,
					Default: DefaultTaintEffect,
				},
				&spec.ExpFlag{
					Name:    "taint-key",
					Desc:    "Custom taint key. Default: chaosblade.io/unreachable",
					NoArgs:  false,
					Default: DefaultTaintKey,
				},
				&spec.ExpFlag{
					Name:    "taint-value",
					Desc:    "Custom taint value. Default: true",
					NoArgs:  false,
					Default: DefaultTaintValue,
				},
			},
			ActionExecutor: &PodTaintNodeActionExecutor{client: client},
			ActionExample: `# Add unreachable taint to nodes to prevent pod scheduling
blade create k8s pod-pod taintnode --nodes node1,node2 --kubeconfig ~/.kube/config

# Add taint with NoExecute effect (will evict running pods)
blade create k8s pod-pod taintnode --nodes node1 --taint-effect NoExecute --kubeconfig ~/.kube/config

# Add custom taint
blade create k8s pod-pod taintnode --nodes node1 --taint-key dedicated --taint-value gpu --taint-effect NoSchedule --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
		client: client,
	}
}

func (*PodTaintNodeActionSpec) Name() string {
	return "taintnode"
}

func (*PodTaintNodeActionSpec) Aliases() []string {
	return []string{}
}

func (*PodTaintNodeActionSpec) ShortDesc() string {
	return "Make pod scheduling fail by adding unreachable taint to nodes"
}

func (*PodTaintNodeActionSpec) LongDesc() string {
	return "Simulate the scenario where a Pod cannot be scheduled due to Taint/Toleration mismatch. " +
		"This fault is injected by adding an unreachable taint to the target nodes. " +
		"Pods without matching tolerations will not be scheduled to these nodes. " +
		"When the experiment is destroyed, the original taints will be restored. " +
		"WARNING: Using NoExecute effect will evict running pods that do not have matching tolerations."
}

type PodTaintNodeActionExecutor struct {
	client *channel.Client
}

func (*PodTaintNodeActionExecutor) Name() string {
	return "taintnode"
}

func (*PodTaintNodeActionExecutor) SetChannel(channel spec.Channel) {}

func (d *PodTaintNodeActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

// parseTaintNodeFlags parses and validates flags for taintnode action.
func parseTaintNodeFlags(expModel *spec.ExpModel) (nodeNames []string, taintKey, taintValue, taintEffect string, err error) {
	nodesFlag := expModel.ActionFlags["nodes"]
	if nodesFlag == "" {
		return nil, "", "", "", fmt.Errorf("nodes flag is required")
	}
	nodeNames, err = parseNodeNames(nodesFlag)
	if err != nil {
		return nil, "", "", "", err
	}
	taintKey = expModel.ActionFlags["taint-key"]
	if taintKey == "" {
		taintKey = DefaultTaintKey
	}
	taintValue = expModel.ActionFlags["taint-value"]
	if taintValue == "" {
		taintValue = DefaultTaintValue
	}
	taintEffect = expModel.ActionFlags["taint-effect"]
	if taintEffect == "" {
		taintEffect = DefaultTaintEffect
	}

	// Validate taint effect
	switch taintEffect {
	case string(v1.TaintEffectNoSchedule), string(v1.TaintEffectNoExecute), string(v1.TaintEffectPreferNoSchedule):
	default:
		return nil, "", "", "", fmt.Errorf("unsupported taint effect: %s, supported values: NoSchedule, NoExecute, PreferNoSchedule", taintEffect)
	}

	return nodeNames, taintKey, taintValue, taintEffect, nil
}

func (d *PodTaintNodeActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	nodeNames, taintKey, taintValue, taintEffect, err := parseTaintNodeFlags(expModel)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithFlags(spec.ParameterIllegal, err.Error())
	}

	var resourceStatuses []v1alpha1.ResourceStatus
	for _, nodeName := range nodeNames {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.NodeKind,
			Identifier: nodeName,
		}

		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Re-get latest node to avoid conflict
			latest := &v1.Node{}
			if err := d.client.Get(ctx, types.NamespacedName{Name: nodeName}, latest); err != nil {
				return err
			}
			return d.injectTaintToNode(ctx, latest, taintKey, taintValue, taintEffect, experimentId)
		}); err != nil {
			logrusField.Warningf("inject taint to node %s failed: %v", nodeName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject taint to node %s failed: %v", nodeName, err), spec.K8sExecFailed.Code)
			resourceStatuses = append(resourceStatuses, status)
			continue
		}
		logrusField.Infof("injected taint %s=%s:%s to node %s", taintKey, taintValue, taintEffect, nodeName)
		status = status.CreateSuccessResourceStatus()
		resourceStatuses = append(resourceStatuses, status)
	}

	// Check if all nodes failed
	allFailed := true
	for _, s := range resourceStatuses {
		if s.Success {
			allFailed = false
			break
		}
	}
	if allFailed {
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus("all nodes failed", resourceStatuses))
	}

	return spec.ReturnResultIgnoreCode(v1alpha1.CreateSuccessExperimentStatus(resourceStatuses))
}

func (d *PodTaintNodeActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	nodesFlag := expModel.ActionFlags["nodes"]
	nodeNames, err := parseNodeNames(nodesFlag)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithFlags(spec.ParameterIllegal, err.Error())
	}

	var resourceStatuses []v1alpha1.ResourceStatus
	for _, nodeName := range nodeNames {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.NodeKind,
			Identifier: nodeName,
		}

		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Re-get latest node to avoid conflict
			latest := &v1.Node{}
			if err := d.client.Get(ctx, types.NamespacedName{Name: nodeName}, latest); err != nil {
				return err
			}
			return d.restoreNodeTaints(ctx, latest, experimentId)
		}); err != nil {
			logrusField.Warningf("restore node %s taints failed: %v", nodeName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore node %s taints failed: %v", nodeName, err), spec.K8sExecFailed.Code)
			resourceStatuses = append(resourceStatuses, status)
			continue
		}
		logrusField.Infof("restored node %s taints", nodeName)
		status = status.CreateSuccessResourceStatus()
		status.State = v1alpha1.DestroyedState
		resourceStatuses = append(resourceStatuses, status)
	}

	allFailed := true
	for _, s := range resourceStatuses {
		if s.Success {
			allFailed = false
			break
		}
	}
	if allFailed {
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus("all nodes restore failed", resourceStatuses))
	}

	return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus(resourceStatuses))
}

// injectTaintToNode adds an unreachable taint to a node.
func (d *PodTaintNodeActionExecutor) injectTaintToNode(ctx context.Context, node *v1.Node, taintKey, taintValue, taintEffect, experimentId string) error {
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}

	// Check for conflicting experiment
	if existingId, ok := node.Annotations[ChaosBladeExperimentAnnotation]; ok && existingId != "" && existingId != experimentId {
		return fmt.Errorf("node is already modified by another chaosblade experiment: %s", existingId)
	}

	// Idempotent: if already modified by the same experiment, skip re-injection
	if node.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}

	// Backup original taints
	if len(node.Spec.Taints) > 0 {
		originalBytes, err := json.Marshal(node.Spec.Taints)
		if err != nil {
			return fmt.Errorf("marshal original taints failed: %v", err)
		}
		node.Annotations[ChaosBladeOriginalTaintsAnnotation] = string(originalBytes)
	}

	// Add the unreachable taint; refuse to overwrite an existing taint with the same key+effect
	newTaint := v1.Taint{
		Key:    taintKey,
		Value:  taintValue,
		Effect: v1.TaintEffect(taintEffect),
	}
	if idx := findTaintIndex(node.Spec.Taints, newTaint.Key, newTaint.Effect); idx >= 0 {
		existing := node.Spec.Taints[idx]
		return fmt.Errorf("node already has taint with key %q and effect %q (value %q); refusing to overwrite existing taint", existing.Key, existing.Effect, existing.Value)
	}
	node.Spec.Taints = append(node.Spec.Taints, newTaint)

	// Record injected taint for surgical removal during restore
	injectedBytes, _ := json.Marshal(newTaint)
	node.Annotations[ChaosBladeInjectedTaintAnnotation] = string(injectedBytes)

	// Set annotations
	node.Annotations[ChaosBladeTaintAnnotation] = ChaosBladeModifyAction
	node.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	return d.client.Update(ctx, node)
}

// restoreNodeTaints removes only the taint injected by this experiment,
// preserving any taints added by other controllers during the experiment.
func (d *PodTaintNodeActionExecutor) restoreNodeTaints(ctx context.Context, node *v1.Node, experimentId string) error {
	if node.Annotations == nil {
		return nil
	}

	// If this experiment did not modify the node, nothing to restore
	if node.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return nil
	}

	// Remove only the injected taint, preserving taints added by other controllers
	if injectedStr, ok := node.Annotations[ChaosBladeInjectedTaintAnnotation]; ok {
		var injected v1.Taint
		if err := json.Unmarshal([]byte(injectedStr), &injected); err != nil {
			return fmt.Errorf("unmarshal injected taint failed: %v", err)
		}
		node.Spec.Taints = removeTaintByKeyEffect(node.Spec.Taints, injected.Key, injected.Effect)
	} else {
		// Fallback: no injected-taint annotation, use original snapshot
		if originalTaintsStr, ok := node.Annotations[ChaosBladeOriginalTaintsAnnotation]; ok {
			var originalTaints []v1.Taint
			if err := json.Unmarshal([]byte(originalTaintsStr), &originalTaints); err != nil {
				return fmt.Errorf("unmarshal original taints failed: %v", err)
			}
			node.Spec.Taints = originalTaints
		} else {
			node.Spec.Taints = nil
		}
	}

	// Clean up annotations
	delete(node.Annotations, ChaosBladeTaintAnnotation)
	for _, key := range chaosBladeTaintAnnotations() {
		delete(node.Annotations, key)
	}

	return d.client.Update(ctx, node)
}

// findTaintIndex returns the index of the first taint matching key+effect, or -1.
func findTaintIndex(taints []v1.Taint, key string, effect v1.TaintEffect) int {
	for i, t := range taints {
		if t.Key == key && t.Effect == effect {
			return i
		}
	}
	return -1
}

// removeTaintByKeyEffect removes the first taint matching key+effect from the list.
// Kubernetes guarantees key+effect uniqueness per node, so this is sufficient for removal.
func removeTaintByKeyEffect(taints []v1.Taint, key string, effect v1.TaintEffect) []v1.Taint {
	for i, t := range taints {
		if t.Key == key && t.Effect == effect {
			return append(taints[:i], taints[i+1:]...)
		}
	}
	return taints
}

// parseNodeNames splits a comma-separated nodes flag, trims whitespace, and rejects empty entries.
func parseNodeNames(nodesFlag string) ([]string, error) {
	var result []string
	for _, n := range strings.Split(nodesFlag, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			return nil, fmt.Errorf("nodes flag contains empty node name")
		}
		result = append(result, n)
	}
	return result, nil
}

// validateTaintNodeFlags validates the nodes flag.
func validateTaintNodeFlags(nodes string) *spec.Response {
	if nodes == "" {
		return spec.ResponseFailWithFlags(spec.ParameterLess, "nodes")
	}
	return nil
}

// PreCreate implements model.ActionPreProcessor interface.
func (a *PodTaintNodeActionSpec) PreCreate(ctx context.Context, expModel *spec.ExpModel, client *channel.Client) (context.Context, *spec.Response) {
	nodes := expModel.ActionFlags["nodes"]
	if resp := validateTaintNodeFlags(nodes); resp != nil {
		return ctx, resp
	}

	nodeNames, err := parseNodeNames(nodes)
	if err != nil {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterIllegal, err.Error())
	}
	taintEffect := expModel.ActionFlags["taint-effect"]
	if taintEffect == "" {
		taintEffect = DefaultTaintEffect
	}
	// Validate taint effect in PreCreate to fail fast
	switch taintEffect {
	case string(v1.TaintEffectNoSchedule), string(v1.TaintEffectNoExecute), string(v1.TaintEffectPreferNoSchedule):
	default:
		return ctx, spec.ResponseFailWithFlags(spec.ParameterIllegal, fmt.Sprintf("unsupported taint effect: %s, supported values: NoSchedule, NoExecute, PreferNoSchedule", taintEffect))
	}

	containerObjectMetaList := model.ContainerMatchedList{}
	for _, nodeName := range nodeNames {
		containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
			Namespace: "",
			PodName:   fmt.Sprintf("chaosblade-tn-%s", nodeName),
			NodeName:  nodeName,
		})
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}

// PreDestroy implements model.ActionPreProcessor interface.
func (a *PodTaintNodeActionSpec) PreDestroy(ctx context.Context, expModel *spec.ExpModel, client *channel.Client, oldExpStatus v1alpha1.ExperimentStatus) (context.Context, *spec.Response) {
	nodes := expModel.ActionFlags["nodes"]
	if resp := validateTaintNodeFlags(nodes); resp != nil {
		return ctx, resp
	}

	nodeNames, err := parseNodeNames(nodes)
	if err != nil {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterIllegal, err.Error())
	}
	containerObjectMetaList := model.ContainerMatchedList{}
	for _, nodeName := range nodeNames {
		containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
			Namespace: "",
			PodName:   fmt.Sprintf("chaosblade-tn-%s", nodeName),
			NodeName:  nodeName,
		})
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}
