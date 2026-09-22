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

package fixture

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

// ChaosBladeBuilder fluently builds a ChaosBlade CR holding a single
// experiment, mirroring the YAML examples under examples/.
type ChaosBladeBuilder struct {
	name string
	exp  v1alpha1.ExperimentSpec
}

// NewChaosBladeBuilder starts building a ChaosBlade CR with the given name.
func NewChaosBladeBuilder(name string) *ChaosBladeBuilder {
	return &ChaosBladeBuilder{name: name}
}

// WithScope sets the experiment scope, e.g. "pod", "node" or "container".
func (b *ChaosBladeBuilder) WithScope(scope string) *ChaosBladeBuilder {
	b.exp.Scope = scope
	return b
}

// WithTarget sets the experiment target, e.g. "pod", "cpu" or "network".
func (b *ChaosBladeBuilder) WithTarget(target string) *ChaosBladeBuilder {
	b.exp.Target = target
	return b
}

// WithAction sets the experiment action, e.g. "delete", "fullload" or "delay".
func (b *ChaosBladeBuilder) WithAction(action string) *ChaosBladeBuilder {
	b.exp.Action = action
	return b
}

// WithDesc sets the experiment description.
func (b *ChaosBladeBuilder) WithDesc(desc string) *ChaosBladeBuilder {
	b.exp.Desc = desc
	return b
}

// WithMatcher appends a matcher flag, e.g. WithMatcher("names", podName).
func (b *ChaosBladeBuilder) WithMatcher(name string, values ...string) *ChaosBladeBuilder {
	b.exp.Matchers = append(b.exp.Matchers, v1alpha1.FlagSpec{Name: name, Value: values})
	return b
}

// Build returns the ChaosBlade CR. The CRD is cluster-scoped, so no
// namespace is set; target workloads are selected via experiment matchers.
func (b *ChaosBladeBuilder) Build() *v1alpha1.ChaosBlade {
	return &v1alpha1.ChaosBlade{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.name,
			Labels: map[string]string{TestLabel: "true"},
		},
		Spec: v1alpha1.ChaosBladeSpec{
			Experiments: []v1alpha1.ExperimentSpec{b.exp},
		},
	}
}
