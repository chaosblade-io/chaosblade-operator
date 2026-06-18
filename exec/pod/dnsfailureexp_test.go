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
	"strings"
	"testing"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	sigsclient "sigs.k8s.io/controller-runtime/pkg/client"
	sigsfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newDnsTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("register corev1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("register appsv1 scheme: %v", err)
	}
	return scheme
}

func newDnsTestClient(t *testing.T, objs ...sigsclient.Object) *channel.Client {
	t.Helper()
	fake := sigsfake.NewClientBuilder().
		WithScheme(newDnsTestScheme(t)).
		WithObjects(objs...).
		Build()
	return &channel.Client{Client: fake}
}

// controllerRef returns an OwnerReference with controller=true for use in
// pod / replicaset metadata.
func controllerRef(kind, name string) metav1.OwnerReference {
	ctrl := true
	return metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       kind,
		Name:       name,
		Controller: &ctrl,
		UID:        types.UID(kind + "-" + name + "-uid"),
	}
}

func newPod(name, namespace string, owners ...metav1.OwnerReference) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: owners,
		},
	}
}

func newReplicaSet(name, namespace string, owners ...metav1.OwnerReference) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: owners,
		},
	}
}

func newDeployment(name, namespace string, podSpec v1.PodSpec, annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{Spec: podSpec},
		},
	}
}

func newDaemonSet(name, namespace string, podSpec v1.PodSpec, annotations map[string]string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: v1.PodTemplateSpec{Spec: podSpec},
		},
	}
}

func newStatefulSet(name, namespace string, podSpec v1.PodSpec, annotations map[string]string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: appsv1.StatefulSetSpec{
			Template: v1.PodTemplateSpec{Spec: podSpec},
		},
	}
}

// ---------------------------------------------------------------------------
// Pure logic: podUsesNameserver
// ---------------------------------------------------------------------------

// TestPodUsesNameserver locks in the documented "best-effort precondition"
// contract: strict only when DNSPolicy=None with explicit DNSConfig.Nameservers,
// hint-accepted in every other configuration. If a future change tightens any
// of the hint-mode cases without also updating the LongDesc / flag Desc, this
// table will catch the drift.
func TestPodUsesNameserver(t *testing.T) {
	tests := []struct {
		name       string
		pod        *v1.Pod
		nameserver string
		want       bool
	}{
		{
			name: "DNSConfig nil with ClusterFirst — accepted as hint",
			pod: &v1.Pod{Spec: v1.PodSpec{
				DNSPolicy: v1.DNSClusterFirst,
			}},
			nameserver: "10.96.0.10",
			want:       true,
		},
		{
			name: "DNSConfig nil with Default policy — accepted as hint",
			pod: &v1.Pod{Spec: v1.PodSpec{
				DNSPolicy: v1.DNSDefault,
			}},
			nameserver: "10.96.0.10",
			want:       true,
		},
		{
			name: "DNSConfig nil with ClusterFirstWithHostNet — accepted as hint",
			pod: &v1.Pod{Spec: v1.PodSpec{
				DNSPolicy: v1.DNSClusterFirstWithHostNet,
			}},
			nameserver: "10.96.0.10",
			want:       true,
		},
		{
			name: "DNSConfig set with matching nameserver",
			pod: &v1.Pod{Spec: v1.PodSpec{
				DNSPolicy: v1.DNSClusterFirst,
				DNSConfig: &v1.PodDNSConfig{
					Nameservers: []string{"10.96.0.10", "8.8.8.8"},
				},
			}},
			nameserver: "10.96.0.10",
			want:       true,
		},
		{
			name: "DNSConfig set, nameserver not listed, policy ClusterFirst — still accepted (hint mode)",
			pod: &v1.Pod{Spec: v1.PodSpec{
				DNSPolicy: v1.DNSClusterFirst,
				DNSConfig: &v1.PodDNSConfig{
					Nameservers: []string{"8.8.8.8"},
				},
			}},
			nameserver: "10.96.0.10",
			want:       true,
		},
		{
			name: "DNSConfig set, nameserver not listed, policy None — strict mode rejects",
			pod: &v1.Pod{Spec: v1.PodSpec{
				DNSPolicy: v1.DNSNone,
				DNSConfig: &v1.PodDNSConfig{
					Nameservers: []string{"8.8.8.8"},
				},
			}},
			nameserver: "10.96.0.10",
			want:       false,
		},
		{
			name: "DNSConfig set, nameserver matches, policy None — accepted",
			pod: &v1.Pod{Spec: v1.PodSpec{
				DNSPolicy: v1.DNSNone,
				DNSConfig: &v1.PodDNSConfig{
					Nameservers: []string{"10.96.0.10"},
				},
			}},
			nameserver: "10.96.0.10",
			want:       true,
		},
		{
			name: "empty Nameservers list, policy None — strict mode rejects",
			pod: &v1.Pod{Spec: v1.PodSpec{
				DNSPolicy: v1.DNSNone,
				DNSConfig: &v1.PodDNSConfig{},
			}},
			nameserver: "10.96.0.10",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podUsesNameserver(tt.pod, tt.nameserver)
			if got != tt.want {
				t.Errorf("podUsesNameserver() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure logic: beginPodDnsFailure
// ---------------------------------------------------------------------------

func TestBeginPodDnsFailure_BackupAndInject_WithExistingDnsConfig(t *testing.T) {
	annotations := map[string]string{}
	originalCfg := &v1.PodDNSConfig{
		Nameservers: []string{"10.96.0.10"},
		Searches:    []string{"svc.cluster.local"},
	}
	podSpec := &v1.PodSpec{
		DNSPolicy: v1.DNSClusterFirst,
		DNSConfig: originalCfg,
	}

	if err := beginPodDnsFailure(annotations, podSpec, "exp-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if annotations[ChaosBladeOriginalDnsPolicyAnnotation] != `"ClusterFirst"` {
		t.Errorf("original policy annotation = %q, want %q",
			annotations[ChaosBladeOriginalDnsPolicyAnnotation], `"ClusterFirst"`)
	}
	rawCfg := annotations[ChaosBladeOriginalDnsConfigAnnotation]
	if rawCfg == "" {
		t.Fatal("original DNSConfig annotation should be non-empty when DNSConfig was set")
	}
	var restoredCfg v1.PodDNSConfig
	if err := json.Unmarshal([]byte(rawCfg), &restoredCfg); err != nil {
		t.Fatalf("backed-up DNSConfig is not valid JSON: %v", err)
	}
	if len(restoredCfg.Nameservers) != 1 || restoredCfg.Nameservers[0] != "10.96.0.10" {
		t.Errorf("backed-up nameservers = %v, want [10.96.0.10]", restoredCfg.Nameservers)
	}

	if annotations[ChaosBladePodDnsFailureAnnotation] != ChaosBladePodDnsFailureAction {
		t.Errorf("dns-failure marker annotation = %q, want %q",
			annotations[ChaosBladePodDnsFailureAnnotation], ChaosBladePodDnsFailureAction)
	}
	if annotations[ChaosBladeExperimentAnnotation] != "exp-1" {
		t.Errorf("experiment annotation = %q, want %q",
			annotations[ChaosBladeExperimentAnnotation], "exp-1")
	}

	if podSpec.DNSPolicy != v1.DNSNone {
		t.Errorf("injected DNSPolicy = %v, want DNSNone", podSpec.DNSPolicy)
	}
	if podSpec.DNSConfig == nil || len(podSpec.DNSConfig.Nameservers) != 1 ||
		podSpec.DNSConfig.Nameservers[0] != UnreachableDnsNameserver {
		t.Errorf("injected DNSConfig.Nameservers = %v, want [%s]",
			podSpec.DNSConfig, UnreachableDnsNameserver)
	}
}

func TestBeginPodDnsFailure_NilDnsConfigYieldsEmptyAnnotation(t *testing.T) {
	annotations := map[string]string{}
	podSpec := &v1.PodSpec{DNSPolicy: v1.DNSClusterFirst, DNSConfig: nil}

	if err := beginPodDnsFailure(annotations, podSpec, "exp-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The empty string is the explicit marker for "was nil"; without this
	// signal, endPodDnsFailure would leave the injected DNSConfig in place.
	if v, ok := annotations[ChaosBladeOriginalDnsConfigAnnotation]; !ok {
		t.Error("original DNSConfig annotation must be present even when value was nil")
	} else if v != "" {
		t.Errorf("original DNSConfig annotation = %q, want empty string sentinel", v)
	}
}

func TestBeginPodDnsFailure_ConflictWithOtherExperiment(t *testing.T) {
	annotations := map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-other",
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
	}
	podSpec := &v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
	}

	err := beginPodDnsFailure(annotations, podSpec, "exp-1")
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "already modified by another chaosblade experiment") {
		t.Errorf("unexpected error: %v", err)
	}
	// The other experiment's backup must remain intact.
	if annotations[ChaosBladeOriginalDnsPolicyAnnotation] != `"ClusterFirst"` {
		t.Errorf("conflict path mutated other experiment's backup: %q",
			annotations[ChaosBladeOriginalDnsPolicyAnnotation])
	}
	if annotations[ChaosBladeExperimentAnnotation] != "exp-other" {
		t.Errorf("conflict path mutated experiment owner annotation: %q",
			annotations[ChaosBladeExperimentAnnotation])
	}
}

func TestBeginPodDnsFailure_IdempotentSameExperimentIsNoOp(t *testing.T) {
	// Workload already injected (DNSPolicy=None, fake nameserver) with the same
	// experiment id. Re-running must not re-marshal the *injected* spec back
	// into the backup annotation — otherwise the backup would record "None"
	// and the restore would never reach the original ClusterFirst.
	annotations := map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-1",
		ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
		ChaosBladeOriginalDnsConfigAnnotation: "",
	}
	podSpec := &v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
	}

	if err := beginPodDnsFailure(annotations, podSpec, "exp-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if annotations[ChaosBladeOriginalDnsPolicyAnnotation] != `"ClusterFirst"` {
		t.Errorf("idempotent re-injection clobbered policy backup: %q",
			annotations[ChaosBladeOriginalDnsPolicyAnnotation])
	}
	if annotations[ChaosBladeOriginalDnsConfigAnnotation] != "" {
		t.Errorf("idempotent re-injection clobbered DNSConfig backup: %q",
			annotations[ChaosBladeOriginalDnsConfigAnnotation])
	}
}

func TestBeginPodDnsFailure_EmptyExperimentAnnotationIsNotConflict(t *testing.T) {
	// An empty string for the experiment annotation is treated as "no owner";
	// proceeding to inject under the current experiment is allowed.
	annotations := map[string]string{
		ChaosBladeExperimentAnnotation: "",
	}
	podSpec := &v1.PodSpec{DNSPolicy: v1.DNSClusterFirst}

	if err := beginPodDnsFailure(annotations, podSpec, "exp-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if annotations[ChaosBladeExperimentAnnotation] != "exp-1" {
		t.Errorf("expected experiment annotation set to exp-1, got %q",
			annotations[ChaosBladeExperimentAnnotation])
	}
}

// ---------------------------------------------------------------------------
// Pure logic: endPodDnsFailure
// ---------------------------------------------------------------------------

func TestEndPodDnsFailure_RestoresPolicyAndConfig(t *testing.T) {
	originalCfg := &v1.PodDNSConfig{Nameservers: []string{"10.96.0.10"}}
	originalCfgBytes, _ := json.Marshal(originalCfg)

	annotations := map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-1",
		ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
		ChaosBladeOriginalDnsConfigAnnotation: string(originalCfgBytes),
		"unrelated":                           "keep-me",
	}
	podSpec := &v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
	}

	mutated, err := endPodDnsFailure(annotations, podSpec, "exp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mutated {
		t.Fatal("endPodDnsFailure should report a mutation when experiment id matches")
	}
	if podSpec.DNSPolicy != v1.DNSClusterFirst {
		t.Errorf("DNSPolicy after restore = %v, want ClusterFirst", podSpec.DNSPolicy)
	}
	if podSpec.DNSConfig == nil || len(podSpec.DNSConfig.Nameservers) != 1 ||
		podSpec.DNSConfig.Nameservers[0] != "10.96.0.10" {
		t.Errorf("DNSConfig after restore = %+v, want nameserver 10.96.0.10", podSpec.DNSConfig)
	}
	for _, k := range []string{
		ChaosBladeExperimentAnnotation,
		ChaosBladePodDnsFailureAnnotation,
		ChaosBladeOriginalDnsPolicyAnnotation,
		ChaosBladeOriginalDnsConfigAnnotation,
	} {
		if _, exists := annotations[k]; exists {
			t.Errorf("annotation %q must be cleared after restore", k)
		}
	}
	if annotations["unrelated"] != "keep-me" {
		t.Error("unrelated annotation must be preserved")
	}
}

func TestEndPodDnsFailure_RestoresNilDnsConfigViaEmptySentinel(t *testing.T) {
	annotations := map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-1",
		ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
		ChaosBladeOriginalDnsConfigAnnotation: "",
	}
	podSpec := &v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
	}

	mutated, err := endPodDnsFailure(annotations, podSpec, "exp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mutated {
		t.Fatal("endPodDnsFailure should return true on matching experiment id")
	}
	if podSpec.DNSConfig != nil {
		t.Errorf("DNSConfig after restore = %+v, want nil (original was nil)", podSpec.DNSConfig)
	}
}

func TestEndPodDnsFailure_NoMutationWhenExperimentMismatches(t *testing.T) {
	annotations := map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-other",
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
	}
	podSpec := &v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
	}

	mutated, err := endPodDnsFailure(annotations, podSpec, "exp-1")
	if err != nil {
		t.Errorf("foreign experiment id must not surface as an error, got: %v", err)
	}
	if mutated {
		t.Error("endPodDnsFailure must not report a mutation for a foreign experiment id")
	}
	if podSpec.DNSPolicy != v1.DNSNone {
		t.Errorf("podSpec must remain untouched, got DNSPolicy=%v", podSpec.DNSPolicy)
	}
	if annotations[ChaosBladeExperimentAnnotation] != "exp-other" {
		t.Errorf("foreign annotation must not be cleared, got %q",
			annotations[ChaosBladeExperimentAnnotation])
	}
}

func TestEndPodDnsFailure_NilAnnotationsIsNoOp(t *testing.T) {
	podSpec := &v1.PodSpec{DNSPolicy: v1.DNSNone}
	mutated, err := endPodDnsFailure(nil, podSpec, "exp-1")
	if err != nil {
		t.Errorf("nil annotations must not surface as an error, got: %v", err)
	}
	if mutated {
		t.Error("endPodDnsFailure on nil annotations must return false")
	}
	if podSpec.DNSPolicy != v1.DNSNone {
		t.Errorf("podSpec must remain untouched, got DNSPolicy=%v", podSpec.DNSPolicy)
	}
}

func TestEndPodDnsFailure_MissingActionMarkerIsSafeNoOp(t *testing.T) {
	// A workload that carries the experiment annotation but no DNS-failure
	// action marker may have been touched by a different chaosblade action
	// sharing the same experiment id, or already half-restored. Either way,
	// the pod-DNS-failure restore must not touch it.
	annotations := map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-1",
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
		ChaosBladeOriginalDnsConfigAnnotation: "",
	}
	podSpec := &v1.PodSpec{DNSPolicy: v1.DNSNone}

	mutated, err := endPodDnsFailure(annotations, podSpec, "exp-1")
	if err != nil {
		t.Errorf("missing action marker must not surface as an error, got: %v", err)
	}
	if mutated {
		t.Error("endPodDnsFailure must report no mutation when the action marker is absent")
	}
	if _, exists := annotations[ChaosBladeExperimentAnnotation]; !exists {
		t.Error("experiment annotation must remain in place when action marker is absent")
	}
}

// TestEndPodDnsFailure_StrictOnInvalidBackup is the regression test for the
// high-severity code review finding: when the backed-up DNSPolicy/DNSConfig
// cannot be decoded, the action MUST refuse to restore AND MUST keep the
// chaosblade annotations in place so the workload can still be identified
// and recovered (manually or via a retry after the annotation is repaired).
//
// The previous behaviour swallowed JSON errors and still deleted the
// chaosblade.io/experiment annotation, which left the pod stranded with the
// injected unreachable nameserver and no metadata to find it again.
func TestEndPodDnsFailure_StrictOnInvalidBackup(t *testing.T) {
	cases := []struct {
		name        string
		annotations map[string]string
		wantErrFrag string
	}{
		{
			name: "invalid policy JSON",
			annotations: map[string]string{
				ChaosBladeExperimentAnnotation:        "exp-1",
				ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
				ChaosBladeOriginalDnsPolicyAnnotation: `not-json`,
				ChaosBladeOriginalDnsConfigAnnotation: "",
			},
			wantErrFrag: ChaosBladeOriginalDnsPolicyAnnotation,
		},
		{
			name: "invalid config JSON",
			annotations: map[string]string{
				ChaosBladeExperimentAnnotation:        "exp-1",
				ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
				ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
				ChaosBladeOriginalDnsConfigAnnotation: `not-json-either`,
			},
			wantErrFrag: ChaosBladeOriginalDnsConfigAnnotation,
		},
		{
			name: "missing policy annotation",
			annotations: map[string]string{
				ChaosBladeExperimentAnnotation:        "exp-1",
				ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
				ChaosBladeOriginalDnsConfigAnnotation: "",
			},
			wantErrFrag: ChaosBladeOriginalDnsPolicyAnnotation,
		},
		{
			name: "missing config annotation",
			annotations: map[string]string{
				ChaosBladeExperimentAnnotation:        "exp-1",
				ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
				ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
			},
			wantErrFrag: ChaosBladeOriginalDnsConfigAnnotation,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// Snapshot the annotations so we can prove they are not mutated.
			original := copyMap(tt.annotations)
			podSpec := &v1.PodSpec{
				DNSPolicy: v1.DNSNone,
				DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
			}

			mutated, err := endPodDnsFailure(tt.annotations, podSpec, "exp-1")
			if err == nil {
				t.Fatal("expected error, got nil — corruption must not be silently accepted")
			}
			if mutated {
				t.Error("endPodDnsFailure must NOT report a mutation when the backup is corrupted")
			}
			if !strings.Contains(err.Error(), tt.wantErrFrag) {
				t.Errorf("error %q must name the offending annotation %q", err.Error(), tt.wantErrFrag)
			}

			// Critical safety property: annotations MUST be left intact so a
			// later destroy (after the operator repairs the value) can still
			// identify and restore the workload.
			for k, v := range original {
				if got, ok := tt.annotations[k]; !ok || got != v {
					t.Errorf("annotation %q was modified or removed on error path: got %q, want %q", k, got, v)
				}
			}
			// And the podSpec MUST remain at the injected state so the
			// workload is still in a known, identifiable configuration.
			if podSpec.DNSPolicy != v1.DNSNone {
				t.Errorf("podSpec.DNSPolicy was mutated on error path: %v", podSpec.DNSPolicy)
			}
			if podSpec.DNSConfig == nil ||
				len(podSpec.DNSConfig.Nameservers) != 1 ||
				podSpec.DNSConfig.Nameservers[0] != UnreachableDnsNameserver {
				t.Errorf("podSpec.DNSConfig was mutated on error path: %+v", podSpec.DNSConfig)
			}
		})
	}
}

// TestEndPodDnsFailure_AtomicityOnPartialCorruption locks in the "all-or-
// nothing" invariant: even when ONE backup is valid and the OTHER is not,
// neither half of the podSpec may be restored. This prevents a half-restored
// workload that the next destroy attempt cannot fully recover.
func TestEndPodDnsFailure_AtomicityOnPartialCorruption(t *testing.T) {
	annotations := map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-1",
		ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`, // valid
		ChaosBladeOriginalDnsConfigAnnotation: `not-json`,       // corrupted
	}
	podSpec := &v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
	}

	mutated, err := endPodDnsFailure(annotations, podSpec, "exp-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if mutated {
		t.Fatal("partial corruption must not yield a mutation")
	}
	// DNSPolicy was perfectly decodable, but the function must NOT have
	// applied it because DNSConfig failed to decode.
	if podSpec.DNSPolicy != v1.DNSNone {
		t.Errorf("DNSPolicy was applied even though DNSConfig decode failed: %v", podSpec.DNSPolicy)
	}
}

// ---------------------------------------------------------------------------
// resolveTopLevelWorkload (fake client)
// ---------------------------------------------------------------------------

func TestResolveTopLevelWorkload(t *testing.T) {
	const ns = "default"
	ctx := context.Background()

	t.Run("pod owned by ReplicaSet which is owned by Deployment", func(t *testing.T) {
		rs := newReplicaSet("nginx-7b8d", ns, controllerRef("Deployment", "nginx"))
		c := newDnsTestClient(t, rs)
		pod := newPod("nginx-7b8d-abc", ns, controllerRef("ReplicaSet", "nginx-7b8d"))

		kind, name, err := resolveTopLevelWorkload(ctx, c, pod)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != "Deployment" || name != "nginx" {
			t.Errorf("got (%s, %s), want (Deployment, nginx)", kind, name)
		}
	})

	t.Run("pod owned directly by DaemonSet", func(t *testing.T) {
		c := newDnsTestClient(t)
		pod := newPod("fluentd-x", ns, controllerRef("DaemonSet", "fluentd"))

		kind, name, err := resolveTopLevelWorkload(ctx, c, pod)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != "DaemonSet" || name != "fluentd" {
			t.Errorf("got (%s, %s), want (DaemonSet, fluentd)", kind, name)
		}
	})

	t.Run("pod owned directly by StatefulSet", func(t *testing.T) {
		c := newDnsTestClient(t)
		pod := newPod("redis-0", ns, controllerRef("StatefulSet", "redis"))

		kind, name, err := resolveTopLevelWorkload(ctx, c, pod)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != "StatefulSet" || name != "redis" {
			t.Errorf("got (%s, %s), want (StatefulSet, redis)", kind, name)
		}
	})

	t.Run("naked pod with no controller owner", func(t *testing.T) {
		c := newDnsTestClient(t)
		pod := newPod("loose-pod", ns)

		_, _, err := resolveTopLevelWorkload(ctx, c, pod)
		if err == nil || !strings.Contains(err.Error(), "no controller owner") {
			t.Fatalf("expected no-controller-owner error, got: %v", err)
		}
	})

	t.Run("pod owned by Job — unsupported", func(t *testing.T) {
		c := newDnsTestClient(t)
		pod := newPod("batch-pod", ns, controllerRef("Job", "batch-job"))

		_, _, err := resolveTopLevelWorkload(ctx, c, pod)
		if err == nil || !strings.Contains(err.Error(), "unsupported pod owner kind Job") {
			t.Fatalf("expected unsupported-kind error for Job, got: %v", err)
		}
	})

	t.Run("ReplicaSet not owned by a Deployment is rejected", func(t *testing.T) {
		// Bare ReplicaSet (or owned by something other than Deployment) — the
		// supported workload set requires the top-level controller to be a
		// Deployment so it can be re-injected by name.
		rs := newReplicaSet("bare-rs", ns)
		c := newDnsTestClient(t, rs)
		pod := newPod("bare-rs-x", ns, controllerRef("ReplicaSet", "bare-rs"))

		_, _, err := resolveTopLevelWorkload(ctx, c, pod)
		if err == nil || !strings.Contains(err.Error(), "not owned by a Deployment") {
			t.Fatalf("expected not-owned-by-deployment error, got: %v", err)
		}
	})

	t.Run("missing ReplicaSet surfaces the lookup error", func(t *testing.T) {
		c := newDnsTestClient(t)
		pod := newPod("orphan-x", ns, controllerRef("ReplicaSet", "missing-rs"))

		_, _, err := resolveTopLevelWorkload(ctx, c, pod)
		if err == nil || !strings.Contains(err.Error(), "get replicaset") {
			t.Fatalf("expected replicaset lookup error, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// injectWorkloadDnsFailure / restoreWorkloadDnsFailure (fake client)
// ---------------------------------------------------------------------------

func TestInjectAndRestoreWorkloadDnsFailure_Deployment(t *testing.T) {
	const ns = "default"
	original := v1.PodSpec{
		DNSPolicy: v1.DNSClusterFirst,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{"10.96.0.10"}},
	}
	dep := newDeployment("nginx", ns, original, nil)
	c := newDnsTestClient(t, dep)
	executor := &PodDnsFailureActionExecutor{client: c}

	if err := executor.injectWorkloadDnsFailure(context.Background(), ns, "Deployment", "nginx", "exp-1"); err != nil {
		t.Fatalf("inject failed: %v", err)
	}

	mid := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, mid); err != nil {
		t.Fatalf("get mid-state failed: %v", err)
	}
	if mid.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
		t.Errorf("post-inject DNSPolicy = %v, want DNSNone", mid.Spec.Template.Spec.DNSPolicy)
	}
	if mid.Spec.Template.Spec.DNSConfig == nil ||
		mid.Spec.Template.Spec.DNSConfig.Nameservers[0] != UnreachableDnsNameserver {
		t.Errorf("post-inject DNSConfig = %+v, want unreachable nameserver",
			mid.Spec.Template.Spec.DNSConfig)
	}
	if mid.Annotations[ChaosBladeExperimentAnnotation] != "exp-1" {
		t.Errorf("post-inject experiment annotation = %q, want exp-1",
			mid.Annotations[ChaosBladeExperimentAnnotation])
	}

	if err := executor.restoreWorkloadDnsFailure(context.Background(), ns, "Deployment", "nginx", "exp-1"); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	post := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, post); err != nil {
		t.Fatalf("get post-state failed: %v", err)
	}
	if post.Spec.Template.Spec.DNSPolicy != v1.DNSClusterFirst {
		t.Errorf("post-restore DNSPolicy = %v, want ClusterFirst", post.Spec.Template.Spec.DNSPolicy)
	}
	if post.Spec.Template.Spec.DNSConfig == nil ||
		post.Spec.Template.Spec.DNSConfig.Nameservers[0] != "10.96.0.10" {
		t.Errorf("post-restore DNSConfig = %+v, want nameserver 10.96.0.10",
			post.Spec.Template.Spec.DNSConfig)
	}
	for _, k := range []string{
		ChaosBladeExperimentAnnotation,
		ChaosBladePodDnsFailureAnnotation,
		ChaosBladeOriginalDnsPolicyAnnotation,
		ChaosBladeOriginalDnsConfigAnnotation,
	} {
		if _, exists := post.Annotations[k]; exists {
			t.Errorf("annotation %q must be cleared post-restore", k)
		}
	}
}

func TestInjectAndRestoreWorkloadDnsFailure_DaemonSet(t *testing.T) {
	const ns = "default"
	ds := newDaemonSet("fluentd", ns, v1.PodSpec{DNSPolicy: v1.DNSClusterFirstWithHostNet}, nil)
	c := newDnsTestClient(t, ds)
	executor := &PodDnsFailureActionExecutor{client: c}

	if err := executor.injectWorkloadDnsFailure(context.Background(), ns, "DaemonSet", "fluentd", "exp-1"); err != nil {
		t.Fatalf("inject failed: %v", err)
	}
	mid := &appsv1.DaemonSet{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "fluentd"}, mid)
	if mid.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
		t.Errorf("DaemonSet DNSPolicy after inject = %v, want DNSNone", mid.Spec.Template.Spec.DNSPolicy)
	}

	if err := executor.restoreWorkloadDnsFailure(context.Background(), ns, "DaemonSet", "fluentd", "exp-1"); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	post := &appsv1.DaemonSet{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "fluentd"}, post)
	if post.Spec.Template.Spec.DNSPolicy != v1.DNSClusterFirstWithHostNet {
		t.Errorf("DaemonSet DNSPolicy after restore = %v, want ClusterFirstWithHostNet",
			post.Spec.Template.Spec.DNSPolicy)
	}
}

func TestInjectAndRestoreWorkloadDnsFailure_StatefulSet(t *testing.T) {
	const ns = "default"
	sts := newStatefulSet("redis", ns, v1.PodSpec{DNSPolicy: v1.DNSDefault}, nil)
	c := newDnsTestClient(t, sts)
	executor := &PodDnsFailureActionExecutor{client: c}

	if err := executor.injectWorkloadDnsFailure(context.Background(), ns, "StatefulSet", "redis", "exp-1"); err != nil {
		t.Fatalf("inject failed: %v", err)
	}
	if err := executor.restoreWorkloadDnsFailure(context.Background(), ns, "StatefulSet", "redis", "exp-1"); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	post := &appsv1.StatefulSet{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "redis"}, post)
	if post.Spec.Template.Spec.DNSPolicy != v1.DNSDefault {
		t.Errorf("StatefulSet DNSPolicy after restore = %v, want Default", post.Spec.Template.Spec.DNSPolicy)
	}
}

func TestInjectWorkloadDnsFailure_UnsupportedKindFailsFast(t *testing.T) {
	c := newDnsTestClient(t)
	executor := &PodDnsFailureActionExecutor{client: c}

	err := executor.injectWorkloadDnsFailure(context.Background(), "default", "Job", "batch", "exp-1")
	if err == nil || !strings.Contains(err.Error(), "unsupported owner kind Job") {
		t.Fatalf("expected unsupported-kind error, got: %v", err)
	}
}

func TestRestoreWorkloadDnsFailure_MissingWorkloadIsNoOp(t *testing.T) {
	c := newDnsTestClient(t) // no objects
	executor := &PodDnsFailureActionExecutor{client: c}

	if err := executor.restoreWorkloadDnsFailure(
		context.Background(), "default", "Deployment", "ghost", "exp-1",
	); err != nil {
		t.Errorf("restore of missing workload should be a no-op, got: %v", err)
	}
}

func TestRestoreWorkloadDnsFailure_ForeignExperimentIsNoOp(t *testing.T) {
	const ns = "default"
	dep := newDeployment(
		"nginx", ns,
		v1.PodSpec{
			DNSPolicy: v1.DNSNone,
			DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
		},
		map[string]string{
			ChaosBladeExperimentAnnotation:        "exp-other",
			ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
			ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
		},
	)
	c := newDnsTestClient(t, dep)
	executor := &PodDnsFailureActionExecutor{client: c}

	if err := executor.restoreWorkloadDnsFailure(context.Background(), ns, "Deployment", "nginx", "exp-1"); err != nil {
		t.Fatalf("unexpected error on foreign restore: %v", err)
	}
	post := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, post)
	if post.Annotations[ChaosBladeExperimentAnnotation] != "exp-other" {
		t.Errorf("foreign restore mutated experiment owner: %q",
			post.Annotations[ChaosBladeExperimentAnnotation])
	}
	if post.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
		t.Errorf("foreign restore mutated DNSPolicy: %v", post.Spec.Template.Spec.DNSPolicy)
	}
}

// ---------------------------------------------------------------------------
// restoreNamespaceWorkloads (fallback path)
// ---------------------------------------------------------------------------

func TestRestoreNamespaceWorkloads(t *testing.T) {
	const ns = "default"
	const expId = "exp-1"

	injectedPodSpec := v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
	}
	ownAnnotations := map[string]string{
		ChaosBladeExperimentAnnotation:        expId,
		ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
		ChaosBladeOriginalDnsConfigAnnotation: "",
	}

	// Owned by our experiment — should be restored.
	depOwn := newDeployment("nginx", ns, injectedPodSpec, copyMap(ownAnnotations))
	dsOwn := newDaemonSet("fluentd", ns, injectedPodSpec, copyMap(ownAnnotations))
	stsOwn := newStatefulSet("redis", ns, injectedPodSpec, copyMap(ownAnnotations))

	// Owned by a different experiment — must be left alone.
	depForeign := newDeployment("nginx-foreign", ns, injectedPodSpec, map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-other",
		ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
		ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
		ChaosBladeOriginalDnsConfigAnnotation: "",
	})

	// Carries our experiment annotation but is owned by another chaosblade
	// *action* (e.g. taint, badresource). The DNS failure restore must not
	// touch workloads it did not inject.
	depWrongAction := newDeployment(
		"nginx-wrong-action", ns, v1.PodSpec{DNSPolicy: v1.DNSClusterFirst},
		map[string]string{
			ChaosBladeExperimentAnnotation: expId,
			// Note: no ChaosBladePodDnsFailureAnnotation marker.
			"chaosblade.io/some-other-action": "true",
		},
	)

	// No annotations — must be ignored.
	depPlain := newDeployment("nginx-plain", ns,
		v1.PodSpec{DNSPolicy: v1.DNSClusterFirst}, nil)

	c := newDnsTestClient(t, depOwn, dsOwn, stsOwn, depForeign, depWrongAction, depPlain)
	executor := &PodDnsFailureActionExecutor{client: c}

	processed := map[string]bool{}
	if err := executor.restoreNamespaceWorkloads(context.Background(), ns, expId, processed); err != nil {
		t.Fatalf("restoreNamespaceWorkloads failed: %v", err)
	}

	// Owned workloads restored.
	verifyDeploymentRestored(t, c, ns, "nginx")
	verifyDaemonSetRestored(t, c, ns, "fluentd")
	verifyStatefulSetRestored(t, c, ns, "redis")

	// Foreign workload untouched.
	gotForeign := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx-foreign"}, gotForeign)
	if gotForeign.Annotations[ChaosBladeExperimentAnnotation] != "exp-other" {
		t.Errorf("foreign-experiment workload was modified: annotation=%q",
			gotForeign.Annotations[ChaosBladeExperimentAnnotation])
	}
	if gotForeign.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
		t.Errorf("foreign-experiment workload DNSPolicy was restored: %v",
			gotForeign.Spec.Template.Spec.DNSPolicy)
	}

	// Wrong-action workload untouched (our annotation present but no DNS
	// failure marker).
	gotWrongAction := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx-wrong-action"}, gotWrongAction)
	if gotWrongAction.Annotations[ChaosBladeExperimentAnnotation] != expId {
		t.Errorf("wrong-action workload annotation was cleared by DNS restore: %q",
			gotWrongAction.Annotations[ChaosBladeExperimentAnnotation])
	}

	// Verify processed map is populated so a second invocation is a no-op.
	wantKeys := []string{
		ns + "/Deployment/nginx",
		ns + "/DaemonSet/fluentd",
		ns + "/StatefulSet/redis",
	}
	for _, k := range wantKeys {
		if !processed[k] {
			t.Errorf("processed map missing key %q", k)
		}
	}
}

func TestRestoreNamespaceWorkloads_RespectsAlreadyProcessed(t *testing.T) {
	const ns = "default"
	const expId = "exp-1"

	dep := newDeployment(
		"nginx", ns,
		v1.PodSpec{
			DNSPolicy: v1.DNSNone,
			DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
		},
		map[string]string{
			ChaosBladeExperimentAnnotation:        expId,
			ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
			ChaosBladeOriginalDnsPolicyAnnotation: `"ClusterFirst"`,
			ChaosBladeOriginalDnsConfigAnnotation: "",
		},
	)
	c := newDnsTestClient(t, dep)
	executor := &PodDnsFailureActionExecutor{client: c}

	// Pre-mark the workload as already processed; the namespace fallback must
	// honor it and skip the Update entirely.
	processed := map[string]bool{ns + "/Deployment/nginx": true}
	if err := executor.restoreNamespaceWorkloads(context.Background(), ns, expId, processed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	post := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, post)
	if post.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
		t.Errorf("workload was unexpectedly restored despite being pre-processed: %v",
			post.Spec.Template.Spec.DNSPolicy)
	}
}

// ---------------------------------------------------------------------------
// End-to-end: Exec create → destroy round-trip
// ---------------------------------------------------------------------------

// TestExec_CreateThenDestroy_RoundTrip is the safety net the code review
// asked for: it stitches together container context, owner resolution,
// annotation backup, injection, and the namespace-wide restore fallback.
func TestExec_CreateThenDestroy_RoundTrip(t *testing.T) {
	const ns = "default"
	originalSpec := v1.PodSpec{
		DNSPolicy: v1.DNSClusterFirst,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{"10.96.0.10"}},
	}

	dep := newDeployment("nginx", ns, originalSpec, nil)
	rs := newReplicaSet("nginx-7b8d", ns, controllerRef("Deployment", "nginx"))
	pod := newPod("nginx-7b8d-abc", ns, controllerRef("ReplicaSet", "nginx-7b8d"))
	pod.Spec = originalSpec

	c := newDnsTestClient(t, dep, rs, pod)
	executor := &PodDnsFailureActionExecutor{client: c}

	containerMetas := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Id:        "container-1",
			PodName:   "nginx-7b8d-abc",
			Namespace: ns,
		},
	}
	ctx := model.SetExperimentIdToContext(context.Background(), "exp-roundtrip")
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerMetas)

	expModel := &spec.ExpModel{ActionFlags: map[string]string{}}

	// Phase 1: create.
	createResp := executor.Exec("uid-1", ctx, expModel)
	if createResp == nil {
		t.Fatal("create response is nil")
	}
	if status, ok := createResp.Result.(v1alpha1.ExperimentStatus); ok && !status.Success {
		t.Fatalf("create failed: %s", status.Error)
	}

	mid := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, mid); err != nil {
		t.Fatalf("get mid-state deployment failed: %v", err)
	}
	if mid.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
		t.Errorf("mid-state DNSPolicy = %v, want DNSNone", mid.Spec.Template.Spec.DNSPolicy)
	}
	if mid.Annotations[ChaosBladeExperimentAnnotation] != "exp-roundtrip" {
		t.Errorf("mid-state experiment annotation = %q, want exp-roundtrip",
			mid.Annotations[ChaosBladeExperimentAnnotation])
	}
	// The pod should have been deleted as part of injection so the controller
	// recreates it with the faulty DNS config.
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx-7b8d-abc"}, &v1.Pod{}); err == nil {
		t.Error("expected the originally matched pod to be deleted after create")
	}

	// Phase 2: destroy. The original pod is gone, so the destroy path takes
	// the namespace-wide fallback. This must still restore the workload.
	destroyCtx := spec.SetDestroyFlag(ctx, "uid-1")
	destroyResp := executor.Exec("uid-1", destroyCtx, expModel)
	if destroyResp == nil {
		t.Fatal("destroy response is nil")
	}

	verifyDeploymentRestored(t, c, ns, "nginx")
}

func TestExec_CreateFailsNameserverPrecondition(t *testing.T) {
	const ns = "default"
	dep := newDeployment("nginx", ns,
		v1.PodSpec{
			DNSPolicy: v1.DNSNone,
			DNSConfig: &v1.PodDNSConfig{Nameservers: []string{"8.8.8.8"}},
		}, nil)
	rs := newReplicaSet("nginx-7b8d", ns, controllerRef("Deployment", "nginx"))
	pod := newPod("nginx-7b8d-abc", ns, controllerRef("ReplicaSet", "nginx-7b8d"))
	pod.Spec = v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{"8.8.8.8"}},
	}

	c := newDnsTestClient(t, dep, rs, pod)
	executor := &PodDnsFailureActionExecutor{client: c}

	containerMetas := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Id:        "container-1",
			PodName:   "nginx-7b8d-abc",
			Namespace: ns,
		},
	}
	ctx := model.SetExperimentIdToContext(context.Background(), "exp-precondition")
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerMetas)

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{DnsFailureNameserverFlag: "10.96.0.10"},
	}

	resp := executor.Exec("uid-1", ctx, expModel)
	if resp == nil {
		t.Fatal("response is nil")
	}
	// The Deployment must NOT have been modified, because the strict-mode
	// nameserver precondition was not satisfied.
	post := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, post)
	if post.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
		t.Errorf("workload DNSPolicy after failed precondition = %v, want unchanged DNSNone",
			post.Spec.Template.Spec.DNSPolicy)
	}
	if _, ok := post.Annotations[ChaosBladeExperimentAnnotation]; ok {
		t.Errorf("workload was annotated even though precondition failed: %v",
			post.Annotations)
	}
	// Pod was not deleted.
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx-7b8d-abc"}, &v1.Pod{}); err != nil {
		t.Errorf("pod must not be deleted when precondition fails: %v", err)
	}

	// The error reported back to ChaosBlade must explain exactly why the
	// strict-mode precondition kicked in. Locking this string in prevents
	// future refactors from silently regressing the message back to the
	// vague "does not reference nameserver" wording that the code review
	// found misleading.
	status, ok := resp.Result.(v1alpha1.ExperimentStatus)
	if !ok {
		t.Fatalf("resp.Result is %T, want v1alpha1.ExperimentStatus", resp.Result)
	}
	if len(status.ResStatuses) != 1 {
		t.Fatalf("got %d resource statuses, want 1", len(status.ResStatuses))
	}
	gotErr := status.ResStatuses[0].Error
	for _, want := range []string{
		"DNSPolicy=None",
		"DNSConfig.Nameservers does not contain 10.96.0.10",
		"PodSpec is authoritative",
	} {
		if !strings.Contains(gotErr, want) {
			t.Errorf("precondition error %q must explain %q", gotErr, want)
		}
	}
}

// TestExec_CreateProceedsWhenPodSpecCannotDisprovePrecondition locks in the
// other half of the documented contract: when the pod's PodSpec cannot prove
// or disprove the requested nameserver (DNSConfig is nil, or DNSPolicy is
// ClusterFirst with extra nameservers), the action treats the flag as a hint
// and proceeds with injection. This is the half that the code review pointed
// out is easy to miss when reading the (previously misleading) docs.
func TestExec_CreateProceedsWhenPodSpecCannotDisprovePrecondition(t *testing.T) {
	const ns = "default"

	cases := []struct {
		name    string
		podSpec v1.PodSpec
	}{
		{
			name:    "DNSConfig nil with ClusterFirst",
			podSpec: v1.PodSpec{DNSPolicy: v1.DNSClusterFirst},
		},
		{
			name: "DNSConfig set with extra nameservers under ClusterFirst",
			podSpec: v1.PodSpec{
				DNSPolicy: v1.DNSClusterFirst,
				DNSConfig: &v1.PodDNSConfig{Nameservers: []string{"8.8.8.8"}},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dep := newDeployment("nginx", ns, tt.podSpec, nil)
			rs := newReplicaSet("nginx-7b8d", ns, controllerRef("Deployment", "nginx"))
			pod := newPod("nginx-7b8d-abc", ns, controllerRef("ReplicaSet", "nginx-7b8d"))
			pod.Spec = tt.podSpec

			c := newDnsTestClient(t, dep, rs, pod)
			executor := &PodDnsFailureActionExecutor{client: c}

			containerMetas := model.ContainerMatchedList{
				model.ContainerObjectMeta{
					Id:        "container-1",
					PodName:   "nginx-7b8d-abc",
					Namespace: ns,
				},
			}
			ctx := model.SetExperimentIdToContext(context.Background(), "exp-hint")
			ctx = model.SetContainerObjectMetaListToContext(ctx, containerMetas)

			expModel := &spec.ExpModel{
				ActionFlags: map[string]string{DnsFailureNameserverFlag: "10.96.0.10"},
			}

			if resp := executor.Exec("uid-1", ctx, expModel); resp == nil {
				t.Fatal("response is nil")
			}

			// The Deployment WAS modified because the hint-mode precondition
			// accepts what it cannot verify.
			post := &appsv1.Deployment{}
			_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, post)
			if post.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
				t.Errorf("hint-mode precondition must allow injection, but DNSPolicy = %v",
					post.Spec.Template.Spec.DNSPolicy)
			}
			if post.Annotations[ChaosBladeExperimentAnnotation] != "exp-hint" {
				t.Errorf("workload was not marked with experiment annotation: %v",
					post.Annotations)
			}
		})
	}
}

// TestExec_DestroyWithCorruptedAnnotationSurfacesFailureAndAllowsRetry is the
// end-to-end regression test for the high-severity code review finding. It
// proves three properties at the same time:
//
//  1. A destroy that hits a corrupted backup annotation reports a FAILURE
//     status (not a misleading success).
//  2. Every chaosblade.io annotation is preserved on the failure path so the
//     workload is still identifiable for a retry.
//  3. After the operator repairs the annotation, a follow-up destroy
//     completes the restore — the workload does NOT get stranded.
//
// The previous lenient implementation silently swallowed the JSON error,
// deleted chaosblade.io/experiment, and reported success — which left the
// pod with the unreachable nameserver permanently and no way to identify it
// for cleanup.
func TestExec_DestroyWithCorruptedAnnotationSurfacesFailureAndAllowsRetry(t *testing.T) {
	const ns = "default"

	injectedSpec := v1.PodSpec{
		DNSPolicy: v1.DNSNone,
		DNSConfig: &v1.PodDNSConfig{Nameservers: []string{UnreachableDnsNameserver}},
	}
	// The deployment is in the "injected and corrupted" state: the marker
	// and experiment annotations are correct, but the DNSPolicy backup is
	// not parseable JSON (simulating an out-of-band edit or a serializer
	// bug).
	corruptedAnnotations := map[string]string{
		ChaosBladeExperimentAnnotation:        "exp-retry",
		ChaosBladePodDnsFailureAnnotation:     ChaosBladePodDnsFailureAction,
		ChaosBladeOriginalDnsPolicyAnnotation: `corrupt-not-json`,
		ChaosBladeOriginalDnsConfigAnnotation: "",
	}

	dep := newDeployment("nginx", ns, injectedSpec, corruptedAnnotations)
	rs := newReplicaSet("nginx-7b8d", ns, controllerRef("Deployment", "nginx"))
	pod := newPod("nginx-7b8d-abc", ns, controllerRef("ReplicaSet", "nginx-7b8d"))
	pod.Spec = injectedSpec

	c := newDnsTestClient(t, dep, rs, pod)
	executor := &PodDnsFailureActionExecutor{client: c}

	containerMetas := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Id:        "container-1",
			PodName:   "nginx-7b8d-abc",
			Namespace: ns,
		},
	}
	ctx := model.SetExperimentIdToContext(context.Background(), "exp-retry")
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerMetas)
	destroyCtx := spec.SetDestroyFlag(ctx, "uid-1")
	expModel := &spec.ExpModel{ActionFlags: map[string]string{}}

	// ----- First destroy: must surface a failure.
	resp := executor.Exec("uid-1", destroyCtx, expModel)
	if resp == nil {
		t.Fatal("response is nil")
	}
	status, ok := resp.Result.(v1alpha1.ExperimentStatus)
	if !ok {
		t.Fatalf("unexpected result type %T", resp.Result)
	}
	if status.Success {
		t.Fatalf("destroy with corrupted annotation must NOT report success, got: %+v", status)
	}
	if len(status.ResStatuses) == 0 {
		t.Fatal("expected at least one resource status describing the failure")
	}
	gotErr := status.ResStatuses[0].Error
	for _, want := range []string{"decode backed-up", ChaosBladeOriginalDnsPolicyAnnotation, "can be retried"} {
		if !strings.Contains(gotErr, want) {
			t.Errorf("failure status error %q must mention %q", gotErr, want)
		}
	}

	// ----- The workload MUST be untouched: annotations preserved + podSpec
	// still at the injected state. This is exactly the property the fix
	// guarantees.
	midDep := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, midDep); err != nil {
		t.Fatalf("get mid-state deployment failed: %v", err)
	}
	for k, want := range corruptedAnnotations {
		if got := midDep.Annotations[k]; got != want {
			t.Errorf("annotation %q was modified on the failure path: got %q, want %q", k, got, want)
		}
	}
	if midDep.Spec.Template.Spec.DNSPolicy != v1.DNSNone {
		t.Errorf("podSpec.DNSPolicy was mutated on the failure path: %v",
			midDep.Spec.Template.Spec.DNSPolicy)
	}
	if midDep.Spec.Template.Spec.DNSConfig == nil ||
		midDep.Spec.Template.Spec.DNSConfig.Nameservers[0] != UnreachableDnsNameserver {
		t.Errorf("podSpec.DNSConfig was mutated on the failure path: %+v",
			midDep.Spec.Template.Spec.DNSConfig)
	}

	// ----- Operator repairs the annotation out-of-band. The remaining
	// chaosblade metadata is still in place, so a follow-up destroy can
	// pick up where the first one left off.
	midDep.Annotations[ChaosBladeOriginalDnsPolicyAnnotation] = `"ClusterFirst"`
	if err := c.Update(context.Background(), midDep); err != nil {
		t.Fatalf("manual annotation repair failed: %v", err)
	}

	// ----- Second destroy: must complete the restore.
	resp2 := executor.Exec("uid-1", destroyCtx, expModel)
	if resp2 == nil {
		t.Fatal("retry response is nil")
	}
	status2, ok := resp2.Result.(v1alpha1.ExperimentStatus)
	if !ok {
		t.Fatalf("unexpected retry result type %T", resp2.Result)
	}
	if !status2.Success {
		t.Fatalf("retry destroy must succeed after repair, got: %+v", status2)
	}

	postDep := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "nginx"}, postDep); err != nil {
		t.Fatalf("get post-state deployment failed: %v", err)
	}
	if postDep.Spec.Template.Spec.DNSPolicy != v1.DNSClusterFirst {
		t.Errorf("retry destroy must restore DNSPolicy, got %v", postDep.Spec.Template.Spec.DNSPolicy)
	}
	if postDep.Spec.Template.Spec.DNSConfig != nil {
		t.Errorf("retry destroy must restore DNSConfig to nil (empty-string sentinel), got %+v",
			postDep.Spec.Template.Spec.DNSConfig)
	}
	for _, k := range []string{
		ChaosBladeExperimentAnnotation,
		ChaosBladePodDnsFailureAnnotation,
		ChaosBladeOriginalDnsPolicyAnnotation,
		ChaosBladeOriginalDnsConfigAnnotation,
	} {
		if _, exists := postDep.Annotations[k]; exists {
			t.Errorf("retry destroy must clear annotation %q after successful restore", k)
		}
	}
}

// ---------------------------------------------------------------------------
// Shared assertion helpers
// ---------------------------------------------------------------------------

func verifyDeploymentRestored(t *testing.T, c *channel.Client, ns, name string) {
	t.Helper()
	got := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, got); err != nil {
		t.Fatalf("get %s/%s failed: %v", ns, name, err)
	}
	if got.Spec.Template.Spec.DNSPolicy != v1.DNSClusterFirst {
		t.Errorf("%s DNSPolicy after restore = %v, want ClusterFirst",
			name, got.Spec.Template.Spec.DNSPolicy)
	}
	for _, k := range []string{
		ChaosBladeExperimentAnnotation,
		ChaosBladePodDnsFailureAnnotation,
		ChaosBladeOriginalDnsPolicyAnnotation,
		ChaosBladeOriginalDnsConfigAnnotation,
	} {
		if _, exists := got.Annotations[k]; exists {
			t.Errorf("%s annotation %q must be cleared after restore", name, k)
		}
	}
}

func verifyDaemonSetRestored(t *testing.T, c *channel.Client, ns, name string) {
	t.Helper()
	got := &appsv1.DaemonSet{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, got); err != nil {
		t.Fatalf("get %s/%s failed: %v", ns, name, err)
	}
	if got.Spec.Template.Spec.DNSPolicy != v1.DNSClusterFirst {
		t.Errorf("%s DNSPolicy after restore = %v, want ClusterFirst",
			name, got.Spec.Template.Spec.DNSPolicy)
	}
}

func verifyStatefulSetRestored(t *testing.T, c *channel.Client, ns, name string) {
	t.Helper()
	got := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, got); err != nil {
		t.Fatalf("get %s/%s failed: %v", ns, name, err)
	}
	if got.Spec.Template.Spec.DNSPolicy != v1.DNSClusterFirst {
		t.Errorf("%s DNSPolicy after restore = %v, want ClusterFirst",
			name, got.Spec.Template.Spec.DNSPolicy)
	}
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
