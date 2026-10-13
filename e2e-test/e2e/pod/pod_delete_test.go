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

package pod_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/chaosblade-io/chaosblade-operator/e2e-test/pkg/fixture"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

var _ = Describe("[E2E] [Pod Scope] Pod Delete", Label("pod-delete"), func() {
	var (
		podName        string
		controlPodName string
		crName         string
	)

	BeforeEach(func() {
		podName = "e2e-pod-" + fixture.RandomSuffix()
		controlPodName = "e2e-control-pod-" + fixture.RandomSuffix()
		crName = "e2e-delete-pod-" + fixture.RandomSuffix()
	})

	It("should delete the target pod and report success, then release the CR", func(ctx SpecContext) {
		By("creating the target pod and a control pod in the same namespace")
		Expect(framework.CreatePod(ctx, podName, map[string]string{fixture.TestLabel: "true"})).To(Succeed())
		Expect(framework.CreatePod(ctx, controlPodName, map[string]string{fixture.TestLabel: "true"})).To(Succeed())
		Expect(framework.WaitForPodRunning(ctx, podName)).To(Succeed())
		Expect(framework.WaitForPodRunning(ctx, controlPodName)).To(Succeed())
		controlBefore, err := framework.GetPod(ctx, controlPodName)
		Expect(err).NotTo(HaveOccurred())

		By("creating the ChaosBlade CR")
		cb := fixture.NewChaosBladeBuilder(crName).
			WithScope("pod").
			WithTarget("pod").
			WithAction("delete").
			WithDesc("delete pod by names (e2e)").
			WithMatcher("names", podName).
			WithMatcher("namespace", framework.Namespace).
			Build()
		Expect(framework.CreateChaosBlade(ctx, cb)).To(Succeed())

		By("asserting the physical effect: the target pod is deleted")
		Expect(framework.WaitForPodDeleted(ctx, podName)).To(Succeed())

		// The control pod guards the selection boundary: without it the suite
		// would also pass if a regression ignored the names matcher and deleted
		// every pod in the namespace.
		By("asserting the selection boundary: the control pod keeps its UID and stays undeleted")
		controlAfter, err := framework.GetPod(ctx, controlPodName)
		Expect(err).NotTo(HaveOccurred())
		Expect(controlAfter.UID).To(Equal(controlBefore.UID))
		Expect(controlAfter.DeletionTimestamp).To(BeNil())

		By("asserting the CR status: phase Running and experiment identity")
		got, err := framework.WaitForChaosBladeSuccess(ctx, crName)
		Expect(err).NotTo(HaveOccurred())
		expStatus := got.Status.ExpStatuses[0]
		Expect(expStatus.Scope).To(Equal("pod"))
		Expect(expStatus.Target).To(Equal("pod"))
		Expect(expStatus.Action).To(Equal("delete"))

		By("asserting the resource statuses point at the target pod and report success")
		Expect(expStatus.ResStatuses).To(HaveLen(1))
		resStatus := expStatus.ResStatuses[0]
		Expect(resStatus.Success).To(BeTrue())
		Expect(resStatus.State).To(Equal(v1alpha1.SuccessState))
		Expect(resStatus.Error).To(BeEmpty())
		Expect(resStatus.Identifier).To(HaveSuffix("/" + podName))

		// The finalizer must be attached before cleanup: waiting for NotFound
		// alone would also pass without one, because a CR without a finalizer
		// disappears immediately on deletion and the release flow would never
		// be exercised.
		Expect(got.Finalizers).To(ContainElement(fixture.ChaosBladeFinalizer))
	}, NodeTimeout(8*time.Minute))

	AfterEach(func(ctx SpecContext) {
		By("deleting the ChaosBlade CR and waiting for the finalizer to release it")
		Expect(framework.DeleteChaosBlade(ctx, crName)).To(Succeed())
		Expect(framework.WaitForChaosBladeGone(ctx, crName)).To(Succeed())
	}, NodeTimeout(3*time.Minute))
})
