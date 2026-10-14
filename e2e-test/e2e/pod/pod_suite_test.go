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

// Package pod_test contains the Pod scope e2e suite. Each scope directory is
// its own Go package with its own Ginkgo bootstrap, so suites run through
// `go test ./e2e/...`.
package pod_test

import (
	"flag"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/chaosblade-io/chaosblade-operator/e2e-test/pkg/fixture"
)

var framework *fixture.Framework

func init() {
	fixture.RegisterFlags(flag.CommandLine)
}

func TestPod(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ChaosBlade Operator E2E: Pod Scope Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	f, err := fixture.NewFramework()
	Expect(err).NotTo(HaveOccurred(), "build the Kubernetes client from the local kubeconfig")
	framework = f

	Expect(framework.Preflight(ctx)).To(Succeed())
	Expect(framework.EnsureNamespace(ctx)).To(Succeed())
	GinkgoWriter.Printf("target namespace: %s (created by this run: %t)\n",
		framework.Namespace, framework.OwnsNamespace())
}, NodeTimeout(3*time.Minute))

var _ = AfterSuite(func(ctx SpecContext) {
	if framework == nil {
		return
	}
	if !framework.OwnsNamespace() {
		GinkgoWriter.Printf("namespace %s was not created by this run, leaving it in place\n", framework.Namespace)
		return
	}
	Expect(framework.CleanupNamespace(ctx)).To(Succeed())
}, NodeTimeout(3*time.Minute))
