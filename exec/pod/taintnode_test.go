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
	"encoding/json"
	"strings"
	"testing"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseTaintNodeFlags(t *testing.T) {
	tests := []struct {
		name        string
		actionFlags map[string]string
		wantNodes   []string
		wantKey     string
		wantValue   string
		wantEffect  string
		wantErr     bool
		errContains string
	}{
		{
			name:        "default values",
			actionFlags: map[string]string{"nodes": "node1"},
			wantNodes:   []string{"node1"},
			wantKey:     DefaultTaintKey,
			wantValue:   DefaultTaintValue,
			wantEffect:  DefaultTaintEffect,
		},
		{
			name:        "multiple nodes",
			actionFlags: map[string]string{"nodes": "node1,node2,node3"},
			wantNodes:   []string{"node1", "node2", "node3"},
			wantKey:     DefaultTaintKey,
			wantValue:   DefaultTaintValue,
			wantEffect:  DefaultTaintEffect,
		},
		{
			name: "custom taint params",
			actionFlags: map[string]string{
				"nodes":        "node1",
				"taint-key":    "dedicated",
				"taint-value":  "gpu",
				"taint-effect": "NoExecute",
			},
			wantNodes:  []string{"node1"},
			wantKey:    "dedicated",
			wantValue:  "gpu",
			wantEffect: "NoExecute",
		},
		{
			name:        "missing nodes flag",
			actionFlags: map[string]string{},
			wantErr:     true,
			errContains: "nodes flag is required",
		},
		{
			name: "unsupported taint effect",
			actionFlags: map[string]string{
				"nodes":        "node1",
				"taint-effect": "InvalidEffect",
			},
			wantErr:     true,
			errContains: "unsupported taint effect",
		},
		{
			name:        "PreferNoSchedule is valid",
			actionFlags: map[string]string{"nodes": "node1", "taint-effect": "PreferNoSchedule"},
			wantNodes:   []string{"node1"},
			wantKey:     DefaultTaintKey,
			wantValue:   DefaultTaintValue,
			wantEffect:  "PreferNoSchedule",
		},
		{
			name:        "whitespace trimmed",
			actionFlags: map[string]string{"nodes": " node1 , node2 "},
			wantNodes:   []string{"node1", "node2"},
			wantKey:     DefaultTaintKey,
			wantValue:   DefaultTaintValue,
			wantEffect:  DefaultTaintEffect,
		},
		{
			name:        "trailing comma rejected",
			actionFlags: map[string]string{"nodes": "node1,"},
			wantErr:     true,
			errContains: "empty node name",
		},
		{
			name:        "double comma rejected",
			actionFlags: map[string]string{"nodes": "node1,,node2"},
			wantErr:     true,
			errContains: "empty node name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expModel := &spec.ExpModel{ActionFlags: tt.actionFlags}
			nodes, key, value, effect, err := parseTaintNodeFlags(expModel)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseTaintNodeFlags() expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("parseTaintNodeFlags() error = %q, want containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("parseTaintNodeFlags() unexpected error: %v", err)
				return
			}
			if len(nodes) != len(tt.wantNodes) {
				t.Errorf("parseTaintNodeFlags() nodes = %v, want %v", nodes, tt.wantNodes)
			}
			for i, n := range nodes {
				if n != tt.wantNodes[i] {
					t.Errorf("parseTaintNodeFlags() nodes[%d] = %q, want %q", i, n, tt.wantNodes[i])
				}
			}
			if key != tt.wantKey {
				t.Errorf("parseTaintNodeFlags() key = %q, want %q", key, tt.wantKey)
			}
			if value != tt.wantValue {
				t.Errorf("parseTaintNodeFlags() value = %q, want %q", value, tt.wantValue)
			}
			if effect != tt.wantEffect {
				t.Errorf("parseTaintNodeFlags() effect = %q, want %q", effect, tt.wantEffect)
			}
		})
	}
}

func TestValidateTaintNodeFlags(t *testing.T) {
	tests := []struct {
		name     string
		nodes    string
		wantFail bool
	}{
		{"valid nodes", "node1", false},
		{"empty nodes", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := validateTaintNodeFlags(tt.nodes)
			if tt.wantFail && resp == nil {
				t.Error("validateTaintNodeFlags() expected failure, got nil")
			}
			if !tt.wantFail && resp != nil {
				t.Errorf("validateTaintNodeFlags() unexpected failure: %v", resp)
			}
		})
	}
}

func TestInjectTaintToNode_ConflictDetection(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				ChaosBladeExperimentAnnotation: "other-experiment",
			},
		},
	}

	executor := &PodTaintNodeActionExecutor{}
	// injectTaintToNode is a method, so we test the conflict logic directly
	err := executor.injectTaintToNode(nil, node, DefaultTaintKey, DefaultTaintValue, DefaultTaintEffect, "my-experiment")
	if err == nil {
		t.Error("expected conflict error when node is already modified by another experiment")
	}
	if !containsString(err.Error(), "already modified by another chaosblade experiment") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestInjectTaintToNode_Idempotent(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				ChaosBladeExperimentAnnotation: "my-experiment",
			},
		},
	}

	executor := &PodTaintNodeActionExecutor{}
	// Same experiment ID should skip injection (idempotent)
	err := executor.injectTaintToNode(nil, node, DefaultTaintKey, DefaultTaintValue, DefaultTaintEffect, "my-experiment")
	if err != nil {
		t.Errorf("expected nil for idempotent injection, got: %v", err)
	}
}

func TestInjectTaintToNode_BackupAndInject(t *testing.T) {
	originalTaints := []v1.Taint{
		{Key: "existing-key", Value: "existing-value", Effect: v1.TaintEffectNoSchedule},
	}
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: map[string]string{},
		},
		Spec: v1.NodeSpec{
			Taints: originalTaints,
		},
	}

	// Simulate the injection logic without calling injectTaintToNode
	// (which requires a real client for Update)
	backupBytes, _ := json.Marshal(originalTaints)
	node.Annotations[ChaosBladeOriginalTaintsAnnotation] = string(backupBytes)
	node.Spec.Taints = append(node.Spec.Taints, v1.Taint{
		Key:    DefaultTaintKey,
		Value:  DefaultTaintValue,
		Effect: v1.TaintEffect(DefaultTaintEffect),
	})
	node.Annotations[ChaosBladeTaintAnnotation] = ChaosBladeModifyAction
	node.Annotations[ChaosBladeExperimentAnnotation] = "my-experiment"

	// Verify original taints are backed up
	backupStr := node.Annotations[ChaosBladeOriginalTaintsAnnotation]
	if backupStr == "" {
		t.Error("expected original taints to be backed up in annotations")
	}
	var backedUp []v1.Taint
	if err := json.Unmarshal([]byte(backupStr), &backedUp); err != nil {
		t.Fatalf("failed to unmarshal backup: %v", err)
	}
	if len(backedUp) != 1 || backedUp[0].Key != "existing-key" {
		t.Errorf("backup = %v, want [{existing-key existing-value NoSchedule}]", backedUp)
	}

	// Verify new taint was appended
	found := false
	for _, tt := range node.Spec.Taints {
		if tt.Key == DefaultTaintKey {
			found = true
			if tt.Value != DefaultTaintValue || tt.Effect != v1.TaintEffectNoSchedule {
				t.Errorf("injected taint = %v, want key=%s value=%s effect=NoSchedule", tt, DefaultTaintKey, DefaultTaintValue)
			}
		}
	}
	if !found {
		t.Error("expected chaosblade taint to be added to node spec")
	}

	// Verify annotations
	if node.Annotations[ChaosBladeTaintAnnotation] != ChaosBladeModifyAction {
		t.Errorf("chaosblade.io/taint annotation = %q, want %q", node.Annotations[ChaosBladeTaintAnnotation], ChaosBladeModifyAction)
	}
	if node.Annotations[ChaosBladeExperimentAnnotation] != "my-experiment" {
		t.Errorf("chaosblade.io/experiment annotation = %q, want %q", node.Annotations[ChaosBladeExperimentAnnotation], "my-experiment")
	}
}

func TestInjectTaintToNode_DuplicateKeyEffect(t *testing.T) {
	// Node already has a taint with the same key+effect as the one we want to inject
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: map[string]string{},
		},
		Spec: v1.NodeSpec{
			Taints: []v1.Taint{
				{Key: DefaultTaintKey, Value: "existing-value", Effect: v1.TaintEffectNoSchedule},
			},
		},
	}

	executor := &PodTaintNodeActionExecutor{}
	err := executor.injectTaintToNode(nil, node, DefaultTaintKey, DefaultTaintValue, DefaultTaintEffect, "my-experiment")
	if err == nil {
		t.Error("expected error when node already has taint with same key+effect")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Verify node taints were NOT modified
	if len(node.Spec.Taints) != 1 {
		t.Fatalf("expected 1 taint (unchanged), got %d", len(node.Spec.Taints))
	}
	if node.Spec.Taints[0].Value != "existing-value" {
		t.Errorf("existing taint value should be unchanged, got %q", node.Spec.Taints[0].Value)
	}
}

func TestRestoreNodeTaints_NotModifiedByExperiment(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		experiment  string
		wantErr     bool
	}{
		{
			name:        "nil annotations - no error",
			annotations: nil,
			experiment:  "my-experiment",
			wantErr:     false,
		},
		{
			name:        "different experiment - skip restore",
			annotations: map[string]string{ChaosBladeExperimentAnnotation: "other-experiment"},
			experiment:  "my-experiment",
			wantErr:     false,
		},
		{
			name:        "empty annotations - skip restore",
			annotations: map[string]string{},
			experiment:  "my-experiment",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-node",
					Annotations: tt.annotations,
				},
			}
			executor := &PodTaintNodeActionExecutor{}
			err := executor.restoreNodeTaints(nil, node, tt.experiment)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRestoreNodeTaints_SuccessfulRestore(t *testing.T) {
	originalTaints := []v1.Taint{
		{Key: "sigma.ali/resource-pool", Value: "ackee_pool", Effect: v1.TaintEffectNoSchedule},
	}
	backupBytes, _ := json.Marshal(originalTaints)
	injectedTaint := v1.Taint{Key: DefaultTaintKey, Value: DefaultTaintValue, Effect: v1.TaintEffectNoSchedule}
	injectedBytes, _ := json.Marshal(injectedTaint)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				ChaosBladeExperimentAnnotation:     "my-experiment",
				ChaosBladeOriginalTaintsAnnotation: string(backupBytes),
				ChaosBladeInjectedTaintAnnotation:  string(injectedBytes),
				ChaosBladeTaintAnnotation:          ChaosBladeModifyAction,
			},
		},
		Spec: v1.NodeSpec{
			Taints: []v1.Taint{
				{Key: "sigma.ali/resource-pool", Value: "ackee_pool", Effect: v1.TaintEffectNoSchedule},
				{Key: DefaultTaintKey, Value: DefaultTaintValue, Effect: v1.TaintEffectNoSchedule},
			},
		},
	}

	// Simulate restore logic: remove only the injected taint
	if injectedStr, ok := node.Annotations[ChaosBladeInjectedTaintAnnotation]; ok {
		var injected v1.Taint
		if err := json.Unmarshal([]byte(injectedStr), &injected); err != nil {
			t.Fatalf("failed to unmarshal injected taint: %v", err)
		}
		node.Spec.Taints = removeTaintByKeyEffect(node.Spec.Taints, injected.Key, injected.Effect)
	}
	delete(node.Annotations, ChaosBladeTaintAnnotation)
	for _, key := range chaosBladeTaintAnnotations() {
		delete(node.Annotations, key)
	}

	// Verify injected taint removed, original preserved
	if len(node.Spec.Taints) != 1 {
		t.Fatalf("expected 1 taint after restore, got %d", len(node.Spec.Taints))
	}
	if node.Spec.Taints[0].Key != "sigma.ali/resource-pool" {
		t.Errorf("remaining taint key = %q, want sigma.ali/resource-pool", node.Spec.Taints[0].Key)
	}

	// Verify annotations cleaned up
	for _, key := range chaosBladeTaintAnnotations() {
		if _, ok := node.Annotations[key]; ok {
			t.Errorf("annotation %q should be cleaned up", key)
		}
	}
	if _, ok := node.Annotations[ChaosBladeTaintAnnotation]; ok {
		t.Error("chaosblade.io/taint annotation should be cleaned up")
	}
}

func TestRestoreNodeTaints_PreservesNewTaints(t *testing.T) {
	// Simulate: another controller added a taint while experiment was running
	originalTaints := []v1.Taint{
		{Key: "sigma.ali/resource-pool", Value: "ackee_pool", Effect: v1.TaintEffectNoSchedule},
	}
	backupBytes, _ := json.Marshal(originalTaints)
	injectedTaint := v1.Taint{Key: DefaultTaintKey, Value: DefaultTaintValue, Effect: v1.TaintEffectNoSchedule}
	injectedBytes, _ := json.Marshal(injectedTaint)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				ChaosBladeExperimentAnnotation:     "my-experiment",
				ChaosBladeOriginalTaintsAnnotation: string(backupBytes),
				ChaosBladeInjectedTaintAnnotation:  string(injectedBytes),
				ChaosBladeTaintAnnotation:          ChaosBladeModifyAction,
			},
		},
		Spec: v1.NodeSpec{
			Taints: []v1.Taint{
				{Key: "sigma.ali/resource-pool", Value: "ackee_pool", Effect: v1.TaintEffectNoSchedule},
				{Key: "other-controller/key", Value: "val", Effect: v1.TaintEffectNoSchedule},
				{Key: DefaultTaintKey, Value: DefaultTaintValue, Effect: v1.TaintEffectNoSchedule},
			},
		},
	}

	// Simulate restore logic
	if injectedStr, ok := node.Annotations[ChaosBladeInjectedTaintAnnotation]; ok {
		var injected v1.Taint
		if err := json.Unmarshal([]byte(injectedStr), &injected); err != nil {
			t.Fatalf("failed to unmarshal injected taint: %v", err)
		}
		node.Spec.Taints = removeTaintByKeyEffect(node.Spec.Taints, injected.Key, injected.Effect)
	}
	delete(node.Annotations, ChaosBladeTaintAnnotation)
	for _, key := range chaosBladeTaintAnnotations() {
		delete(node.Annotations, key)
	}

	// Verify: injected taint removed, other-controller taint preserved
	if len(node.Spec.Taints) != 2 {
		t.Fatalf("expected 2 taints after restore, got %d: %v", len(node.Spec.Taints), node.Spec.Taints)
	}
	found := false
	for _, t := range node.Spec.Taints {
		if t.Key == "other-controller/key" {
			found = true
		}
	}
	if !found {
		t.Error("expected other-controller taint to be preserved after restore")
	}
}

func TestRestoreNodeTaints_NoBackup(t *testing.T) {
	// Node had no taints before injection, so no backup exists
	injectedTaint := v1.Taint{Key: DefaultTaintKey, Value: DefaultTaintValue, Effect: v1.TaintEffectNoSchedule}
	injectedBytes, _ := json.Marshal(injectedTaint)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				ChaosBladeExperimentAnnotation:    "my-experiment",
				ChaosBladeInjectedTaintAnnotation: string(injectedBytes),
				ChaosBladeTaintAnnotation:         ChaosBladeModifyAction,
			},
		},
		Spec: v1.NodeSpec{
			Taints: []v1.Taint{
				{Key: DefaultTaintKey, Value: DefaultTaintValue, Effect: v1.TaintEffectNoSchedule},
			},
		},
	}

	// Simulate restore logic: remove only the injected taint
	if injectedStr, ok := node.Annotations[ChaosBladeInjectedTaintAnnotation]; ok {
		var injected v1.Taint
		if err := json.Unmarshal([]byte(injectedStr), &injected); err != nil {
			t.Fatalf("failed to unmarshal injected taint: %v", err)
		}
		node.Spec.Taints = removeTaintByKeyEffect(node.Spec.Taints, injected.Key, injected.Effect)
	}
	delete(node.Annotations, ChaosBladeTaintAnnotation)
	for _, key := range chaosBladeTaintAnnotations() {
		delete(node.Annotations, key)
	}

	// Should have no taints after restore
	if len(node.Spec.Taints) != 0 {
		t.Errorf("expected 0 taints after restore (node had no original taints), got %d: %v", len(node.Spec.Taints), node.Spec.Taints)
	}
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
