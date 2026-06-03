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

package model

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

// TestParallelExec_PreservesIndexMapping locks in the ordering contract: each
// worker must write into its own slot, so result[i] == i regardless of the
// non-deterministic completion order produced by the worker pool.
func TestParallelExec_PreservesIndexMapping(t *testing.T) {
	const n = 200
	result := make([]int, n)

	ParallelizeExec(n, func(i int) {
		time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
		result[i] = i
	})

	for i := 0; i < n; i++ {
		if result[i] != i {
			t.Fatalf("index mapping broken: result[%d] = %d, want %d", i, result[i], i)
		}
	}
}

// TestCheckExperimentStatus_LengthMismatchGuard verifies the defensive guard:
// when statuses and identifiers have mismatched lengths, the function returns
// immediately without spawning the status-check goroutine or dispatching any
// exec calls.
func TestCheckExperimentStatus_LengthMismatchGuard(t *testing.T) {
	ctx := SetExperimentIdToContext(context.Background(), "exp-mismatch")
	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{"timeout": "1"},
	}

	statuses := []v1alpha1.ResourceStatus{
		{Identifier: "ns/node-0/pod-0", Success: true},
		{Identifier: "ns/node-1/pod-1", Success: true},
	}
	identifiers := []ExperimentIdentifierInPod{
		{ChaosBladePodName: "cb-node-0"},
	}

	// A nil-config client: if the guard failed to short-circuit, the spawned
	// goroutine would eventually invoke client.Exec and panic. The guard must
	// return before any goroutine is started.
	client := &channel.Client{}

	done := make(chan struct{})
	go func() {
		checkExperimentStatus(ctx, expModel, statuses, identifiers, client)
		close(done)
	}()

	select {
	case <-done:
		// Returned synchronously as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("checkExperimentStatus did not return promptly on length mismatch")
	}
}
