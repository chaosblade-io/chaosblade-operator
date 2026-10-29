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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/channel"
)

// containerImageChange describes one container image rewrite performed by an
// experiment, together with the annotation recording the original image.
type containerImageChange struct {
	// ContainerName is the strategic merge key identifying the container.
	ContainerName string
	// NewImage replaces the current image of the container.
	NewImage string
	// AnnotationKey is the annotation storing OriginalImage for recovery.
	AnnotationKey string
	// OriginalImage is the image the container is restored to.
	OriginalImage string
}

// containerImagePatch is one entry of the patched spec.containers list. Only
// the merge key and the image are carried, so every other field of the
// container stays out of the request body.
type containerImagePatch struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// podSpecPatch carries the PodSpec fields an experiment rewrites. A running
// pod may only change `spec.containers[].image`, `spec.initContainers[].image`,
// `spec.activeDeadlineSeconds`, additions to `spec.tolerations` and, in one
// special case, `spec.terminationGracePeriodSeconds`.
type podSpecPatch struct {
	Containers []containerImagePatch `json:"containers,omitempty"`
}

// podMetadataPatch carries the bookkeeping annotations. Merging a map only adds
// or overwrites the listed keys, so the remaining annotations are preserved.
type podMetadataPatch struct {
	Annotations map[string]string `json:"annotations,omitempty"`
}

// podImagePatch is the strategic merge patch submitted to the API server.
type podImagePatch struct {
	Metadata podMetadataPatch `json:"metadata"`
	Spec     podSpecPatch     `json:"spec"`
}

// podFinalizersAdd is the metadata half of a patch adding finalizers.
// `finalizers` is a list of primitives tagged `patchStrategy:"merge"` with no
// merge key, so the strategic merge is a union and the API server
// de-duplicates it: adding an entry that is already there changes nothing.
type podFinalizersAdd struct {
	Finalizers []string `json:"finalizers"`
}

type podFinalizersAddPatch struct {
	Metadata podFinalizersAdd `json:"metadata"`
}

// podFinalizersRemove is the metadata half of a patch removing finalizers.
//
// Removal cannot be expressed as a plain list. Verified against a live API
// server: sending only the desired finalizers leaves the removed entry in
// place, because the strategic merge of a primitive list is a union. The
// `$deleteFromPrimitiveList/finalizers` directive is what actually removes it,
// and it is idempotent - naming an absent entry is accepted and changes
// nothing, including when the list is empty or unset.
type podFinalizersRemove struct {
	DeleteFromFinalizers []string `json:"$deleteFromPrimitiveList/finalizers"`
}

type podFinalizersRemovePatch struct {
	Metadata podFinalizersRemove `json:"metadata"`
}

// buildFinalizerAddPatch renders the patch adding one finalizer.
func buildFinalizerAddPatch(finalizer string) ([]byte, error) {
	patch := podFinalizersAddPatch{}
	patch.Metadata.Finalizers = []string{finalizer}
	return json.Marshal(patch)
}

// buildFinalizerRemovePatch renders the patch removing one finalizer.
func buildFinalizerRemovePatch(finalizer string) ([]byte, error) {
	patch := podFinalizersRemovePatch{}
	patch.Metadata.DeleteFromFinalizers = []string{finalizer}
	return json.Marshal(patch)
}

// buildContainerImagePatch renders the changes into a strategic merge patch,
// and returns nil when there is nothing to change.
//
// The patch relies on the strategic merge semantics of PodSpec: `containers`
// is tagged `patchStrategy:"merge" patchMergeKey:"name"`, so the listed
// containers are merged by name while the unlisted ones, and every other pod
// field, are left untouched.
//
// Keeping the payload down to the merge key and the image is what makes the
// write safe. A pod is immutable outside the whitelist above, so writing the
// whole object back with Update is rejected as soon as any field outside that
// whitelist differs from the live object - even though the experiment only
// ever meant to touch an image, and even when the difference comes from a
// field the client structs do not model and silently dropped on the way back.
func buildContainerImagePatch(changes []containerImageChange) ([]byte, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	patch := podImagePatch{
		Metadata: podMetadataPatch{Annotations: make(map[string]string, len(changes))},
		Spec:     podSpecPatch{Containers: make([]containerImagePatch, 0, len(changes))},
	}
	for _, change := range changes {
		patch.Metadata.Annotations[change.AnnotationKey] = change.OriginalImage
		patch.Spec.Containers = append(patch.Spec.Containers, containerImagePatch{
			Name:  change.ContainerName,
			Image: change.NewImage,
		})
	}
	return json.Marshal(patch)
}

// patchContainerImages submits the image rewrites of one pod as a single
// strategic merge patch. It is a no-op when changes is empty.
func patchContainerImages(ctx context.Context, cli *channel.Client, pod *v1.Pod, changes []containerImageChange) error {
	data, err := buildContainerImagePatch(changes)
	if err != nil {
		return err
	}
	return strategicPatchPod(ctx, cli, pod, data)
}

// strategicPatchPod submits data as a strategic merge patch, and is a no-op
// when data is nil.
//
// On success pod is refreshed from the API server response, because
// controller-runtime decodes the body back into the object it was given.
// Callers that inspect pod after the write depend on that refresh - the
// rollback path of the terminating experiment reads the finalizers back to
// decide whether there is anything to undo.
func strategicPatchPod(ctx context.Context, cli *channel.Client, pod *v1.Pod, data []byte) error {
	if data == nil {
		return nil
	}
	return cli.Patch(ctx, pod, client.RawPatch(types.StrategicMergePatchType, data))
}
