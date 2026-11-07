/*
 * Copyright 2026 The ChaosBlade Authors
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

package main

import (
	"testing"
	"time"

	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestConfigureLeaderElection(t *testing.T) {
	options := manager.Options{}

	configureLeaderElection(&options, "chaosblade")

	if !options.LeaderElection {
		t.Fatal("leader election must be enabled")
	}
	if options.LeaderElectionID != leaderElectionID {
		t.Fatalf("unexpected leader election ID: %q", options.LeaderElectionID)
	}
	if options.LeaderElectionNamespace != "chaosblade" {
		t.Fatalf("unexpected leader election namespace: %q", options.LeaderElectionNamespace)
	}
	if options.LeaderElectionResourceLock != resourcelock.LeasesResourceLock {
		t.Fatalf("unexpected leader election resource lock: %q", options.LeaderElectionResourceLock)
	}
	assertDuration(t, "lease duration", options.LeaseDuration, 30*time.Second)
	assertDuration(t, "renew deadline", options.RenewDeadline, 20*time.Second)
	assertDuration(t, "retry period", options.RetryPeriod, 5*time.Second)
}

func assertDuration(t *testing.T, name string, actual *time.Duration, expected time.Duration) {
	t.Helper()
	if actual == nil || *actual != expected {
		t.Fatalf("unexpected %s: %v, want %s", name, actual, expected)
	}
}
