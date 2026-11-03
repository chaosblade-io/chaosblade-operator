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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

func TestBuildContainerImagePatchNoChanges(t *testing.T) {
	for name, changes := range map[string][]containerImageChange{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := buildContainerImagePatch(changes)
			if err != nil {
				t.Fatalf("buildContainerImagePatch() error = %v", err)
			}
			if data != nil {
				t.Errorf("buildContainerImagePatch() = %s, want nil so that no request is sent", data)
			}
		})
	}
}

func TestBuildContainerImagePatch(t *testing.T) {
	tests := []struct {
		name            string
		changes         []containerImageChange
		wantAnnotations map[string]string
		wantContainers  []containerImagePatch
	}{
		{
			name: "single container",
			changes: []containerImageChange{{
				ContainerName: "app",
				NewImage:      "nginx:1.19-fault-injection",
				AnnotationKey: "failPod-app",
				OriginalImage: "nginx:1.19",
			}},
			wantAnnotations: map[string]string{"failPod-app": "nginx:1.19"},
			wantContainers: []containerImagePatch{
				{Name: "app", Image: "nginx:1.19-fault-injection"},
			},
		},
		{
			name: "multiple containers keep their own annotation",
			changes: []containerImageChange{
				{
					ContainerName: "app",
					NewImage:      "nginx:1.19-fault-injection",
					AnnotationKey: "failPod-app",
					OriginalImage: "nginx:1.19",
				},
				{
					ContainerName: "sidecar",
					NewImage:      "envoy:v2-image-config-error",
					AnnotationKey: "chaosblade.io/imageconfig-sidecar",
					OriginalImage: "envoy:v2",
				},
			},
			wantAnnotations: map[string]string{
				"failPod-app":                       "nginx:1.19",
				"chaosblade.io/imageconfig-sidecar": "envoy:v2",
			},
			wantContainers: []containerImagePatch{
				{Name: "app", Image: "nginx:1.19-fault-injection"},
				{Name: "sidecar", Image: "envoy:v2-image-config-error"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := buildContainerImagePatch(tt.changes)
			if err != nil {
				t.Fatalf("buildContainerImagePatch() error = %v", err)
			}

			var patch podImagePatch
			if err := json.Unmarshal(data, &patch); err != nil {
				t.Fatalf("patch %s is not valid json: %v", data, err)
			}

			if len(patch.Metadata.Annotations) != len(tt.wantAnnotations) {
				t.Errorf("annotations = %v, want %v", patch.Metadata.Annotations, tt.wantAnnotations)
			}
			for key, want := range tt.wantAnnotations {
				if got := patch.Metadata.Annotations[key]; got != want {
					t.Errorf("annotations[%q] = %q, want %q", key, got, want)
				}
			}
			if len(patch.Spec.Containers) != len(tt.wantContainers) {
				t.Fatalf("containers = %v, want %v", patch.Spec.Containers, tt.wantContainers)
			}
			for i, want := range tt.wantContainers {
				if got := patch.Spec.Containers[i]; got != want {
					t.Errorf("containers[%d] = %+v, want %+v", i, got, want)
				}
			}
		})
	}
}

// TestBuildContainerImagePatchCarriesOnlyMutableFields guards the reason the
// patch exists: a running pod may only change a handful of spec fields, so the
// request body must not carry anything else. A full object Update is rejected
// by the API server as soon as an unrelated field differs from the live object.
func TestBuildContainerImagePatchCarriesOnlyMutableFields(t *testing.T) {
	changes := []containerImageChange{{
		ContainerName: "app",
		NewImage:      "nginx:1.19-fault-injection",
		AnnotationKey: "failPod-app",
		OriginalImage: "nginx:1.19",
	}}
	data, err := buildContainerImagePatch(changes)
	if err != nil {
		t.Fatalf("buildContainerImagePatch() error = %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("patch %s is not valid json: %v", data, err)
	}
	assertKeys(t, "root", raw, []string{"metadata", "spec"})

	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(raw["metadata"], &metadata); err != nil {
		t.Fatalf("metadata is not an object: %v", err)
	}
	assertKeys(t, "metadata", metadata, []string{"annotations"})

	var podSpec map[string]json.RawMessage
	if err := json.Unmarshal(raw["spec"], &podSpec); err != nil {
		t.Fatalf("spec is not an object: %v", err)
	}
	assertKeys(t, "spec", podSpec, []string{"containers"})

	var containers []map[string]json.RawMessage
	if err := json.Unmarshal(podSpec["containers"], &containers); err != nil {
		t.Fatalf("containers is not a list: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("containers has %d entries, want 1", len(containers))
	}
	// name is the strategic merge key, image is the only field being changed.
	assertKeys(t, "spec.containers[0]", containers[0], []string{"name", "image"})
}

// TestBuildContainerImagePatchEscapesValues proves the payload is marshalled
// rather than concatenated, so a value carrying JSON metacharacters cannot
// break the request body.
func TestBuildContainerImagePatchEscapesValues(t *testing.T) {
	changes := []containerImageChange{{
		ContainerName: "app",
		NewImage:      `nginx:"odd"-fault-injection`,
		AnnotationKey: "failPod-app",
		OriginalImage: `nginx:"odd"`,
	}}
	data, err := buildContainerImagePatch(changes)
	if err != nil {
		t.Fatalf("buildContainerImagePatch() error = %v", err)
	}

	var patch podImagePatch
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatalf("patch %s is not valid json: %v", data, err)
	}
	if got, want := patch.Metadata.Annotations["failPod-app"], `nginx:"odd"`; got != want {
		t.Errorf("annotations[failPod-app] = %q, want %q", got, want)
	}
	if got, want := patch.Spec.Containers[0].Image, `nginx:"odd"-fault-injection`; got != want {
		t.Errorf("containers[0].image = %q, want %q", got, want)
	}
}

// TestStrategicMergePatchPreservesImmutableFields replays the API server merge
// offline. The pod carries the two fields that made a full object Update fail:
// a probe level TerminationGracePeriodSeconds and a spec.DNSConfig. Neither may
// change on a running pod, so if the merge touched either one the request would
// be rejected with "pod updates may not change fields other than ...".
func TestStrategicMergePatchPreservesImmutableFields(t *testing.T) {
	gracePeriod := int64(60)
	ndots := "2"
	const (
		originalImage = "registry.k8s.io/descheduler:v0.29.0"
		faultImage    = originalImage + "-fault-injection"
	)
	original := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "descheduler-315bb44774-5psaa",
			Namespace:   "default",
			Annotations: map[string]string{"unrelated": "keep-me"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "app",
					Image: originalImage,
					LivenessProbe: &v1.Probe{
						SuccessThreshold:              1,
						FailureThreshold:              3,
						TerminationGracePeriodSeconds: &gracePeriod,
					},
				},
				{Name: "sidecar", Image: "envoy:v2"},
			},
			DNSConfig: &v1.PodDNSConfig{
				Options: []v1.PodDNSConfigOption{{Name: "ndots", Value: &ndots}},
			},
		},
	}

	originalJSON, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original pod: %v", err)
	}
	patchData, err := buildContainerImagePatch([]containerImageChange{{
		ContainerName: "app",
		NewImage:      faultImage,
		AnnotationKey: "failPod-app",
		OriginalImage: originalImage,
	}})
	if err != nil {
		t.Fatalf("buildContainerImagePatch() error = %v", err)
	}

	mergedJSON, err := strategicpatch.StrategicMergePatch(originalJSON, patchData, v1.Pod{})
	if err != nil {
		t.Fatalf("StrategicMergePatch(%s, %s) error = %v", originalJSON, patchData, err)
	}
	merged := &v1.Pod{}
	if err := json.Unmarshal(mergedJSON, merged); err != nil {
		t.Fatalf("merged result %s is not a pod: %v", mergedJSON, err)
	}

	// the intended change went through
	if got := merged.Spec.Containers[0].Image; got != faultImage {
		t.Errorf("containers[app].image = %q, want %q", got, faultImage)
	}
	if got := merged.Annotations["failPod-app"]; got != originalImage {
		t.Errorf("annotations[failPod-app] = %q, want %q", got, originalImage)
	}

	// containers merge by name: the unlisted one survives untouched
	if len(merged.Spec.Containers) != 2 {
		t.Fatalf("containers = %d entries, want 2 (list must merge by name, not replace)", len(merged.Spec.Containers))
	}
	if got := merged.Spec.Containers[1]; got.Name != "sidecar" || got.Image != "envoy:v2" {
		t.Errorf("containers[1] = %+v, want sidecar/envoy:v2", got)
	}

	// the fields that are immutable on a running pod are untouched
	probe := merged.Spec.Containers[0].LivenessProbe
	if probe == nil || probe.TerminationGracePeriodSeconds == nil {
		t.Fatalf("livenessProbe.terminationGracePeriodSeconds was dropped: %+v", probe)
	}
	if got := *probe.TerminationGracePeriodSeconds; got != gracePeriod {
		t.Errorf("livenessProbe.terminationGracePeriodSeconds = %d, want %d", got, gracePeriod)
	}
	if merged.Spec.DNSConfig == nil {
		t.Fatal("spec.dnsConfig was dropped")
	}
	options := merged.Spec.DNSConfig.Options
	if len(options) != 1 || options[0].Name != "ndots" || options[0].Value == nil || *options[0].Value != ndots {
		t.Errorf("spec.dnsConfig.options = %+v, want [{ndots %s}]", options, ndots)
	}

	// unrelated annotations survive the map merge
	if got := merged.Annotations["unrelated"]; got != "keep-me" {
		t.Errorf("annotations[unrelated] = %q, want %q", got, "keep-me")
	}
}

// TestBuildFinalizerRemovePatch pins the directive name. It is the load bearing
// detail of the whole remove path: a plain `finalizers` list is merged as a
// union and removes nothing, so a typo here would silently leave every target
// pod stuck in Terminating with no error surfaced anywhere.
func TestBuildFinalizerRemovePatch(t *testing.T) {
	data, err := buildFinalizerRemovePatch("chaosblade.io/pod-terminating")
	if err != nil {
		t.Fatalf("buildFinalizerRemovePatch() error = %v", err)
	}

	var decoded map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("patch %s is not valid json: %v", data, err)
	}
	metadata, ok := decoded["metadata"]
	if !ok {
		t.Fatalf("patch %s has no metadata", data)
	}
	assertKeys(t, "metadata", metadata, []string{"$deleteFromPrimitiveList/finalizers"})

	var removed []string
	if err := json.Unmarshal(metadata["$deleteFromPrimitiveList/finalizers"], &removed); err != nil {
		t.Fatalf("directive value is not a string list: %v", err)
	}
	if len(removed) != 1 || removed[0] != "chaosblade.io/pod-terminating" {
		t.Errorf("$deleteFromPrimitiveList/finalizers = %v, want [chaosblade.io/pod-terminating]", removed)
	}
	if _, present := metadata["finalizers"]; present {
		t.Errorf("patch %s also carries a plain finalizers list, which would re-add the entry", data)
	}
}

func TestBuildFinalizerAddPatch(t *testing.T) {
	data, err := buildFinalizerAddPatch("chaosblade.io/pod-terminating")
	if err != nil {
		t.Fatalf("buildFinalizerAddPatch() error = %v", err)
	}

	var decoded map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("patch %s is not valid json: %v", data, err)
	}
	metadata, ok := decoded["metadata"]
	if !ok {
		t.Fatalf("patch %s has no metadata", data)
	}
	assertKeys(t, "metadata", metadata, []string{"finalizers"})
	if _, present := decoded["spec"]; present {
		t.Errorf("patch %s carries a spec, which has no business in a finalizer write", data)
	}
}

// TestStrategicMergePatchFinalizerRoundTrip replays the add and remove cycle
// offline through the same merge implementation the API server runs. The
// expected outcomes mirror what was measured against a live v1.24 cluster:
// add is a de-duplicated union, remove needs the directive, and both are
// idempotent.
func TestStrategicMergePatchFinalizerRoundTrip(t *testing.T) {
	const ours = "chaosblade.io/pod-terminating"
	original := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "nginx-app",
			Namespace:  "default",
			Finalizers: []string{"example.com/fa", "example.com/fb"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "app", Image: "nginx:1.25"}},
		},
	}
	originalJSON, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original pod: %v", err)
	}

	addData, err := buildFinalizerAddPatch(ours)
	if err != nil {
		t.Fatalf("buildFinalizerAddPatch() error = %v", err)
	}
	removeData, err := buildFinalizerRemovePatch(ours)
	if err != nil {
		t.Fatalf("buildFinalizerRemovePatch() error = %v", err)
	}

	merge := func(t *testing.T, current []byte, patch []byte) []byte {
		t.Helper()
		out, err := strategicpatch.StrategicMergePatch(current, patch, v1.Pod{})
		if err != nil {
			t.Fatalf("StrategicMergePatch(%s, %s) error = %v", current, patch, err)
		}
		return out
	}
	finalizers := func(t *testing.T, raw []byte) []string {
		t.Helper()
		pod := &v1.Pod{}
		if err := json.Unmarshal(raw, pod); err != nil {
			t.Fatalf("merged result %s is not a pod: %v", raw, err)
		}
		return pod.GetFinalizers()
	}

	// add is a union: the pre-existing finalizers are kept
	afterAdd := merge(t, originalJSON, addData)
	if got := finalizers(t, afterAdd); !sameStrings(got, []string{ours, "example.com/fa", "example.com/fb"}) {
		t.Errorf("after add = %v, want %v in any order", got, []string{ours, "example.com/fa", "example.com/fb"})
	}

	// add is idempotent: repeating it must not duplicate the entry
	afterAddTwice := merge(t, afterAdd, addData)
	if got := finalizers(t, afterAddTwice); !sameStrings(got, []string{ours, "example.com/fa", "example.com/fb"}) {
		t.Errorf("after second add = %v, want no duplicate", got)
	}

	// a plain list does NOT remove, which is why the directive is required
	plain := podFinalizersAddPatch{}
	plain.Metadata.Finalizers = []string{"example.com/fa", "example.com/fb"}
	plainData, err := json.Marshal(plain)
	if err != nil {
		t.Fatalf("marshal plain patch: %v", err)
	}
	if got := finalizers(t, merge(t, afterAddTwice, plainData)); !hasFinalizer(got, ours) {
		t.Errorf("submitting only the desired list removed %q; strategic merge of a primitive list is a union", ours)
	}

	// the directive removes exactly one entry and keeps the rest
	afterRemove := merge(t, afterAddTwice, removeData)
	if got := finalizers(t, afterRemove); !sameStrings(got, []string{"example.com/fa", "example.com/fb"}) {
		t.Errorf("after remove = %v, want [example.com/fa example.com/fb] in any order", got)
	}

	// remove is idempotent: naming an entry that is already gone is not an error
	afterRemoveTwice := merge(t, afterRemove, removeData)
	if got := finalizers(t, afterRemoveTwice); !sameStrings(got, []string{"example.com/fa", "example.com/fb"}) {
		t.Errorf("after second remove = %v, want the other finalizers untouched", got)
	}

	// a metadata only patch leaves the spec alone, so it cannot trip the
	// immutability check that rejects a full object Update
	pod := &v1.Pod{}
	if err := json.Unmarshal(afterRemoveTwice, pod); err != nil {
		t.Fatalf("final result is not a pod: %v", err)
	}
	if len(pod.Spec.Containers) != 1 || pod.Spec.Containers[0].Image != "nginx:1.25" {
		t.Errorf("spec.containers = %+v, want the single untouched nginx:1.25 container", pod.Spec.Containers)
	}
}

// assertKeys fails when the decoded object carries any key outside want, which
// is how a field leaking into the request body gets caught.
func assertKeys(t *testing.T, path string, got map[string]json.RawMessage, want []string) {
	t.Helper()
	allowed := make(map[string]bool, len(want))
	for _, key := range want {
		allowed[key] = true
	}
	for key := range got {
		if !allowed[key] {
			t.Errorf("%s carries unexpected field %q, want only %v", path, key, want)
		}
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("%s is missing field %q", path, key)
		}
	}
}

// sameStrings compares two string slices ignoring order, because the API
// documents that finalizer order is not enforced.
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(got))
	for _, s := range got {
		seen[s]++
	}
	for _, s := range want {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}

func hasFinalizer(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
