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

// Package fixture provides the e2e test framework: Kubernetes client setup
// from the local kubeconfig, workload/ChaosBlade CR helpers and polling
// utilities shared by all scope suites.
package fixture

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	// PollInterval is the interval between two polls in the wait helpers.
	PollInterval = 2 * time.Second

	podRunningTimeout    = 2 * time.Minute
	podDeletedTimeout    = 2 * time.Minute
	crExecutedTimeout    = 2 * time.Minute
	crDeletedTimeout     = 2 * time.Minute
	namespaceGoneTimeout = 2 * time.Minute

	// DefaultImage is the amd64 chaosblade-tool image. A cluster running the
	// operator already caches it on every node through the tool DaemonSet, so
	// it needs no reachable public registry, unlike busybox. Arm64 clusters
	// must override it, e.g.
	//   make e2e-run IMAGE=ghcr.io/chaosblade-io/chaosblade-tool-arm64:1.8.0
	DefaultImage = "ghcr.io/chaosblade-io/chaosblade-tool:1.8.0"

	// TestLabel marks every resource this framework creates, so leftovers of
	// an interrupted run can be removed by `make e2e-cleanup`.
	// Keep in sync with TEST_LABEL in the Makefile.
	TestLabel = "chaosblade-e2e"

	namespacePrefix   = TestLabel
	chaosBladeCRDName = "chaosblades.chaosblade.io"
)

var (
	namespaceFlag string
	imageFlag     string
)

// RegisterFlags registers the shared e2e flags on the given flag set. It must
// be called before flags are parsed, typically from an init() function in
// each suite bootstrap.
func RegisterFlags(fs *flag.FlagSet) {
	if fs == nil {
		fs = flag.CommandLine
	}
	fs.StringVar(&namespaceFlag, "namespace", "",
		"namespace for e2e target workloads; empty creates a unique chaosblade-e2e-<suffix> namespace per run")
	fs.StringVar(&imageFlag, "image", DefaultImage,
		"image used by e2e target workloads")
}

// RandomSuffix returns a random suffix used to build unique resource names.
func RandomSuffix() string {
	return fmt.Sprintf("%08x", rand.Uint32())
}

// Framework wraps the Kubernetes client and holds the namespace that carries
// this run's target workloads.
type Framework struct {
	Client    client.Client
	Namespace string

	// ownsNamespace records whether this run created Namespace. Cleanup never
	// deletes a namespace it did not create, so pointing the suite at an
	// existing namespace (even "default") cannot destroy unrelated workloads.
	ownsNamespace bool
}

// NewFramework builds the client from the local kubeconfig, resolved from the
// KUBECONFIG environment variable or ~/.kube/config.
func NewFramework() (*Framework, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig (set KUBECONFIG or ~/.kube/config): %w", err)
	}
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1alpha1.SchemeBuilder.AddToScheme(scheme); err != nil {
		return nil, err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	namespace := namespaceFlag
	if namespace == "" {
		namespace = namespacePrefix + "-" + RandomSuffix()
	}
	return &Framework{Client: c, Namespace: namespace}, nil
}

// Preflight fails fast when the cluster cannot run these tests, instead of
// letting every spec time out with an opaque error. The ChaosBlade CRD must
// be installed and cluster-scoped, because the fixtures build CRs without a
// namespace and select targets through experiment matchers.
func (f *Framework) Preflight(ctx context.Context) error {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := f.Client.Get(ctx, client.ObjectKey{Name: chaosBladeCRDName}, crd); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("CRD %s is not installed: install the operator first (make install-operator)", chaosBladeCRDName)
		}
		return fmt.Errorf("read CRD %s: %w", chaosBladeCRDName, err)
	}
	if crd.Spec.Scope != apiextensionsv1.ClusterScoped {
		return fmt.Errorf("CRD %s is %s-scoped here, but these fixtures build cluster-scoped CRs; "+
			"install the CRD shipped in deploy/helm/chaosblade-operator/crds/crd.yaml "+
			"(deploy/crds/chaosblade.io_chaosblades_crd.yaml still carries a stale Namespaced scope)",
			chaosBladeCRDName, crd.Spec.Scope)
	}
	return nil
}

// OwnsNamespace reports whether this run created the target namespace.
func (f *Framework) OwnsNamespace() bool {
	return f.ownsNamespace
}

// EnsureNamespace creates the namespace for this run's target workloads and
// records ownership. An already existing namespace is adopted for the run but
// is never deleted afterwards.
func (f *Framework) EnsureNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   f.Namespace,
			Labels: map[string]string{TestLabel: "true"},
		},
	}
	err := f.Client.Create(ctx, ns)
	switch {
	case err == nil:
		f.ownsNamespace = true
	case apierrors.IsAlreadyExists(err):
	default:
		return fmt.Errorf("create namespace %s: %w", f.Namespace, err)
	}
	return nil
}

// CleanupNamespace deletes the namespace only when this run created it, and
// waits until it is gone.
func (f *Framework) CleanupNamespace(ctx context.Context) error {
	if !f.ownsNamespace {
		return nil
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: f.Namespace}}
	if err := f.Client.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete namespace %s: %w", f.Namespace, err)
	}
	err := wait.PollUntilContextTimeout(ctx, PollInterval, namespaceGoneTimeout, true,
		func(ctx context.Context) (bool, error) {
			err := f.Client.Get(ctx, client.ObjectKey{Name: f.Namespace}, &corev1.Namespace{})
			switch {
			case apierrors.IsNotFound(err):
				return true, nil
			case err != nil:
				return false, err
			}
			return false, nil
		})
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("namespace %s still terminating after %s", f.Namespace, namespaceGoneTimeout)
	}
	return err
}

// CreatePod creates a sleep pod with a short termination grace period so that
// deletion assertions finish quickly. The pod tolerates all NoSchedule and
// NoExecute taints, matching the chaosblade-tool DaemonSet in
// pkg/controller/chaosblade/daemonset.go, so resource pool taints on shared
// clusters and node pressure eviction cannot make the target vanish mid-test.
func (f *Framework) CreatePod(ctx context.Context, name string, labels map[string]string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "main",
				Image:   imageFlag,
				Command: []string{"sleep", "3600"},
			}},
			Tolerations: []corev1.Toleration{
				{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
				{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
			},
			TerminationGracePeriodSeconds: ptr.To[int64](1),
		},
	}
	if err := f.Client.Create(ctx, pod); err != nil {
		return fmt.Errorf("create pod %s/%s (image %s): %w", f.Namespace, name, imageFlag, err)
	}
	return nil
}

// WaitForPodRunning waits until the pod is Running with a running container,
// which the operator's container matching requires. A timeout reports why the
// pod is stuck, e.g. unschedulable taints or image pull failures.
func (f *Framework) WaitForPodRunning(ctx context.Context, name string) error {
	var lastState string
	err := wait.PollUntilContextTimeout(ctx, PollInterval, podRunningTimeout, true,
		func(ctx context.Context) (bool, error) {
			pod := &corev1.Pod{}
			if err := f.Client.Get(ctx, f.podKey(name), pod); err != nil {
				if apierrors.IsNotFound(err) {
					return false, fmt.Errorf("pod %s/%s disappeared before becoming Running", f.Namespace, name)
				}
				return false, err
			}
			if podRunning(pod) {
				return true, nil
			}
			lastState = podDiagnostics(pod)
			return false, nil
		})
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("pod %s/%s (image %s) not Running after %s: %s",
			f.Namespace, name, imageFlag, podRunningTimeout, lastState)
	}
	return err
}

// WaitForPodDeleted waits until the pod is removed from the API server.
func (f *Framework) WaitForPodDeleted(ctx context.Context, name string) error {
	var lastState string
	err := wait.PollUntilContextTimeout(ctx, PollInterval, podDeletedTimeout, true,
		func(ctx context.Context) (bool, error) {
			pod := &corev1.Pod{}
			err := f.Client.Get(ctx, f.podKey(name), pod)
			switch {
			case apierrors.IsNotFound(err):
				return true, nil
			case err != nil:
				return false, err
			}
			lastState = fmt.Sprintf("phase=%s, deletionTimestamp=%v", pod.Status.Phase, pod.DeletionTimestamp)
			return false, nil
		})
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("pod %s/%s still present after %s: %s", f.Namespace, name, podDeletedTimeout, lastState)
	}
	return err
}

// CreateChaosBlade creates a ChaosBlade CR. The CRD is cluster-scoped, so the
// CR carries no namespace; target workloads are selected via matchers.
func (f *Framework) CreateChaosBlade(ctx context.Context, cb *v1alpha1.ChaosBlade) error {
	return f.Client.Create(ctx, cb)
}

// DeleteChaosBlade deletes the ChaosBlade CR, tolerating NotFound so it stays
// usable in cleanup paths after a failed setup.
func (f *Framework) DeleteChaosBlade(ctx context.Context, name string) error {
	cb := &v1alpha1.ChaosBlade{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := f.Client.Delete(ctx, cb); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// GetChaosBlade fetches the ChaosBlade CR by name.
func (f *Framework) GetChaosBlade(ctx context.Context, name string) (*v1alpha1.ChaosBlade, error) {
	cb := &v1alpha1.ChaosBlade{}
	if err := f.Client.Get(ctx, client.ObjectKey{Name: name}, cb); err != nil {
		return nil, err
	}
	return cb, nil
}

// WaitForChaosBladeSuccess waits until the CR reports success: Phase Running
// and the first experiment status marked Success. It aborts early on an Error
// phase and on any API failure such as missing RBAC.
func (f *Framework) WaitForChaosBladeSuccess(ctx context.Context, name string) (*v1alpha1.ChaosBlade, error) {
	var last *v1alpha1.ChaosBlade
	err := wait.PollUntilContextTimeout(ctx, PollInterval, crExecutedTimeout, true,
		func(ctx context.Context) (bool, error) {
			cb, err := f.GetChaosBlade(ctx, name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return false, fmt.Errorf("chaosblade %s disappeared before reporting success", name)
				}
				return false, err
			}
			last = cb
			if cb.Status.Phase == v1alpha1.ClusterPhaseError {
				return false, fmt.Errorf("chaosblade %s phase Error: %s", name, describeExpStatuses(cb))
			}
			if cb.Status.Phase == v1alpha1.ClusterPhaseRunning &&
				len(cb.Status.ExpStatuses) > 0 && cb.Status.ExpStatuses[0].Success {
				return true, nil
			}
			return false, nil
		})
	if err == nil {
		return last, nil
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return last, err
	}
	state := "no status observed"
	if last != nil {
		state = fmt.Sprintf("phase=%s, expStatuses: %s", last.Status.Phase, describeExpStatuses(last))
	}
	hint := ""
	if last != nil && last.Status.Phase == v1alpha1.ClusterPhaseInitial {
		hint = "; the CR was never reconciled, is chaosblade-operator running?"
	}
	return last, fmt.Errorf("chaosblade %s not successful after %s: %s%s", name, crExecutedTimeout, state, hint)
}

// WaitForChaosBladeGone waits until the CR is removed from the API server,
// proving the finalizer released the object after destroy. A timeout reports
// the finalizers still blocking removal.
func (f *Framework) WaitForChaosBladeGone(ctx context.Context, name string) error {
	var last *v1alpha1.ChaosBlade
	err := wait.PollUntilContextTimeout(ctx, PollInterval, crDeletedTimeout, true,
		func(ctx context.Context) (bool, error) {
			cb, err := f.GetChaosBlade(ctx, name)
			switch {
			case apierrors.IsNotFound(err):
				return true, nil
			case err != nil:
				return false, err
			}
			last = cb
			return false, nil
		})
	if errors.Is(err, context.DeadlineExceeded) {
		state := "no status observed"
		if last != nil {
			state = fmt.Sprintf("phase=%s, finalizers=%v", last.Status.Phase, last.Finalizers)
		}
		return fmt.Errorf("chaosblade %s still present after %s: %s", name, crDeletedTimeout, state)
	}
	return err
}

func (f *Framework) podKey(name string) client.ObjectKey {
	return client.ObjectKey{Namespace: f.Namespace, Name: name}
}

func podRunning(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Running != nil {
			return true
		}
	}
	return false
}

// podDiagnostics renders why a pod is not Running yet, covering the two
// failures that otherwise only show up as a bare timeout: scheduling
// rejections (taints) and image pull problems.
func podDiagnostics(pod *corev1.Pod) string {
	parts := []string{"phase=" + string(pod.Status.Phase)}
	for _, c := range pod.Status.Conditions {
		if c.Status != corev1.ConditionTrue && c.Message != "" {
			parts = append(parts, fmt.Sprintf("%s=%s(%s)", c.Type, c.Status, c.Message))
		}
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			parts = append(parts, fmt.Sprintf("container %s waiting: %s(%s)",
				cs.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message))
		}
	}
	return strings.Join(parts, "; ")
}

func describeExpStatuses(cb *v1alpha1.ChaosBlade) string {
	parts := make([]string, 0, len(cb.Status.ExpStatuses))
	for _, st := range cb.Status.ExpStatuses {
		parts = append(parts, fmt.Sprintf("{scope=%s,target=%s,action=%s,success=%t,state=%s,error=%s}",
			st.Scope, st.Target, st.Action, st.Success, st.State, st.Error))
	}
	return strings.Join(parts, ", ")
}
