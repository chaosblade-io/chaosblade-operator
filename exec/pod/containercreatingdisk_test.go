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
	"testing"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

func TestPodContainerCreatingDiskActionSpec_Name(t *testing.T) {
	s := NewPodContainerCreatingDiskActionSpec(nil)
	if s.Name() != "containercreating-disk" {
		t.Errorf("expected name 'containercreating-disk', got '%s'", s.Name())
	}
}

func TestPodContainerCreatingDiskActionSpec_ShortDesc(t *testing.T) {
	s := NewPodContainerCreatingDiskActionSpec(nil)
	if s.ShortDesc() == "" {
		t.Error("ShortDesc should not be empty")
	}
}

func TestPodContainerCreatingDiskActionSpec_LongDesc(t *testing.T) {
	s := NewPodContainerCreatingDiskActionSpec(nil)
	if s.LongDesc() == "" {
		t.Error("LongDesc should not be empty")
	}
}

func TestPodContainerCreatingDiskActionSpec_Aliases(t *testing.T) {
	s := NewPodContainerCreatingDiskActionSpec(nil)
	if len(s.Aliases()) != 0 {
		t.Errorf("expected no aliases, got %v", s.Aliases())
	}
}

func TestPodContainerCreatingDiskActionSpec_ActionFlags(t *testing.T) {
	s := NewPodContainerCreatingDiskActionSpec(nil)
	flags := s.Flags()
	if flags == nil {
		t.Fatal("Flags should not be nil")
	}

	flagNames := make(map[string]bool)
	for _, f := range flags {
		flagNames[f.FlagName()] = true
	}
	if !flagNames["storage-class"] {
		t.Error("storage-class flag should exist")
	}
	if !flagNames["pv-capacity"] {
		t.Error("pv-capacity flag should exist")
	}
	if !flagNames["volume-mount-path"] {
		t.Error("volume-mount-path flag should exist")
	}
}

func TestPodContainerCreatingDiskActionSpec_ActionCategories(t *testing.T) {
	s := NewPodContainerCreatingDiskActionSpec(nil)
	categories := s.Categories()
	found := false
	for _, c := range categories {
		if c == model.CategorySystemContainer {
			found = true
			break
		}
	}
	if !found {
		t.Error("CategorySystemContainer should be present")
	}
}

func TestPodContainerCreatingDiskActionSpec_ActionExample(t *testing.T) {
	s := NewPodContainerCreatingDiskActionSpec(nil)
	example := s.Example()
	if example == "" {
		t.Error("Example should not be empty")
	}
}

func TestPodContainerCreatingDiskActionExecutor_Name(t *testing.T) {
	executor := &PodContainerCreatingDiskActionExecutor{}
	if executor.Name() != "containercreating-disk" {
		t.Errorf("expected name 'containercreating-disk', got '%s'", executor.Name())
	}
}

func TestPreCreate_NamespaceEmpty(t *testing.T) {
	actionSpec := NewPodContainerCreatingDiskActionSpec(nil).(*PodContainerCreatingDiskActionSpec)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			model.ResourceNamespaceFlag.Name: "",
			"storage-class":                  "alicloud-disk-ssd",
		},
	}

	ctx := context.Background()
	ctx = model.SetExperimentIdToContext(ctx, "test-exp-001")

	_, resp := actionSpec.PreCreate(ctx, expModel, nil)
	if resp == nil {
		t.Fatal("expected error response for empty namespace")
	}
	if resp.Success {
		t.Error("expected PreCreate to fail for empty namespace")
	}
}

func TestPreCreate_NamespaceWithComma(t *testing.T) {
	actionSpec := NewPodContainerCreatingDiskActionSpec(nil).(*PodContainerCreatingDiskActionSpec)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			model.ResourceNamespaceFlag.Name: "ns1,ns2",
			"storage-class":                  "alicloud-disk-ssd",
		},
	}

	ctx := context.Background()
	ctx = model.SetExperimentIdToContext(ctx, "test-exp-002")

	_, resp := actionSpec.PreCreate(ctx, expModel, nil)
	if resp == nil {
		t.Fatal("expected error response for multi-value namespace")
	}
	if resp.Success {
		t.Error("expected PreCreate to fail for namespace with comma")
	}
}

func TestPreCreate_StorageClassEmpty(t *testing.T) {
	actionSpec := NewPodContainerCreatingDiskActionSpec(nil).(*PodContainerCreatingDiskActionSpec)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			model.ResourceNamespaceFlag.Name: "default",
			"storage-class":                  "",
		},
	}

	ctx := context.Background()
	ctx = model.SetExperimentIdToContext(ctx, "test-exp-003")

	_, resp := actionSpec.PreCreate(ctx, expModel, nil)
	if resp == nil {
		t.Fatal("expected error response for empty storage-class")
	}
	if resp.Success {
		t.Error("expected PreCreate to fail for empty storage-class")
	}
}

func TestPreCreate_Success(t *testing.T) {
	actionSpec := NewPodContainerCreatingDiskActionSpec(nil).(*PodContainerCreatingDiskActionSpec)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			model.ResourceNamespaceFlag.Name: "default",
			"storage-class":                  "alicloud-disk-ssd",
		},
	}

	ctx := context.Background()
	ctx = model.SetExperimentIdToContext(ctx, "test-exp-004")

	newCtx, resp := actionSpec.PreCreate(ctx, expModel, nil)
	if resp != nil {
		t.Fatalf("expected no error, got response: %+v", resp)
	}

	list, err := model.GetContainerObjectMetaListFromContext(newCtx)
	if err != nil {
		t.Fatalf("failed to get containerObjectMetaList: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 container meta, got %d", len(list))
	}
	if list[0].Namespace != "default" {
		t.Errorf("expected namespace 'default', got '%s'", list[0].Namespace)
	}
}

func TestPreDestroy_Success(t *testing.T) {
	actionSpec := NewPodContainerCreatingDiskActionSpec(nil).(*PodContainerCreatingDiskActionSpec)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			model.ResourceNamespaceFlag.Name: "default",
			"storage-class":                  "alicloud-disk-ssd",
		},
	}

	ctx := context.Background()
	ctx = model.SetExperimentIdToContext(ctx, "test-exp-005")

	newCtx, resp := actionSpec.PreDestroy(ctx, expModel, nil, v1alpha1.ExperimentStatus{})
	if resp != nil {
		t.Fatalf("expected no error, got response: %+v", resp)
	}

	list, err := model.GetContainerObjectMetaListFromContext(newCtx)
	if err != nil {
		t.Fatalf("failed to get containerObjectMetaList: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 container meta, got %d", len(list))
	}
	if list[0].Namespace != "default" {
		t.Errorf("expected namespace 'default', got '%s'", list[0].Namespace)
	}
}

func TestPreDestroy_NamespaceEmpty(t *testing.T) {
	actionSpec := NewPodContainerCreatingDiskActionSpec(nil).(*PodContainerCreatingDiskActionSpec)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			model.ResourceNamespaceFlag.Name: "",
			"storage-class":                  "alicloud-disk-ssd",
		},
	}

	ctx := context.Background()
	ctx = model.SetExperimentIdToContext(ctx, "test-exp-006")

	_, resp := actionSpec.PreDestroy(ctx, expModel, nil, v1alpha1.ExperimentStatus{})
	if resp == nil {
		t.Fatal("expected error response for empty namespace")
	}
	if resp.Success {
		t.Error("expected PreDestroy to fail for empty namespace")
	}
}

func TestPreCreate_PVCapacityInvalid(t *testing.T) {
	actionSpec := NewPodContainerCreatingDiskActionSpec(nil).(*PodContainerCreatingDiskActionSpec)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			model.ResourceNamespaceFlag.Name: "default",
			"storage-class":                  "alicloud-disk-ssd",
			"pv-capacity":                    "invalid-capacity",
		},
	}

	ctx := context.Background()
	ctx = model.SetExperimentIdToContext(ctx, "test-exp-007")

	_, resp := actionSpec.PreCreate(ctx, expModel, nil)
	if resp == nil {
		t.Fatal("expected error response for invalid pv-capacity")
	}
	if resp.Success {
		t.Error("expected PreCreate to fail for invalid pv-capacity")
	}
}

func TestPreCreate_PVCapacityValid(t *testing.T) {
	actionSpec := NewPodContainerCreatingDiskActionSpec(nil).(*PodContainerCreatingDiskActionSpec)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			model.ResourceNamespaceFlag.Name: "default",
			"storage-class":                  "alicloud-disk-ssd",
			"pv-capacity":                    "50Gi",
		},
	}

	ctx := context.Background()
	ctx = model.SetExperimentIdToContext(ctx, "test-exp-008")

	newCtx, resp := actionSpec.PreCreate(ctx, expModel, nil)
	if resp != nil {
		t.Fatalf("expected no error, got response: %+v", resp)
	}

	list, err := model.GetContainerObjectMetaListFromContext(newCtx)
	if err != nil {
		t.Fatalf("failed to get containerObjectMetaList: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 container meta, got %d", len(list))
	}
	if list[0].Namespace != "default" {
		t.Errorf("expected namespace 'default', got '%s'", list[0].Namespace)
	}
}
