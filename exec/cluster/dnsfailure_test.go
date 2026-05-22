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

package cluster

import (
	"context"
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

// newTestScheme builds a runtime.Scheme registered with the api groups the
// cluster DNS action touches. We avoid pulling in the full kubernetes scheme
// to keep this test package's dependency surface small.
func newTestScheme(t *testing.T) *runtime.Scheme {
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

// newTestClient wraps a controller-runtime fake client into the operator's
// channel.Client. The kubernetes.Interface field stays nil because the cluster
// DNS action only uses the controller-runtime Client surface (Get/List/Update).
func newTestClient(t *testing.T, objs ...sigsclient.Object) *channel.Client {
	t.Helper()
	fake := sigsfake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objs...).
		Build()
	return &channel.Client{Client: fake}
}

func int32Ptr(i int32) *int32 { return &i }

// dnsService builds a kube-dns-shaped Service object pointing at the given
// selector labels.
func dnsService(name, namespace string, selector map[string]string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1.ServiceSpec{
			Selector: selector,
		},
	}
}

// dnsDeployment builds a Deployment whose pod template carries the given
// labels and replica count.
func dnsDeployment(name, namespace string, templateLabels map[string]string, replicas *int32, annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: templateLabels},
			},
		},
	}
}

func TestReadOriginalReplicas(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int32
		wantErr bool
	}{
		{name: "empty defaults to 1", raw: "", want: 1},
		{name: "raw integer string", raw: "3", want: 3},
		{name: "raw zero", raw: "0", want: 0},
		{name: "negative integer accepted", raw: "-1", want: -1},
		{name: "json-encoded integer", raw: "2", want: 2},
		{name: "invalid value returns error", raw: "not-a-number", wantErr: true},
		{name: "invalid json object returns error", raw: `{"x":1}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readOriginalReplicas(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readOriginalReplicas(%q) expected error, got nil", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("readOriginalReplicas(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("readOriginalReplicas(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestResolveDnsServiceFlags(t *testing.T) {
	tests := []struct {
		name          string
		flags         map[string]string
		wantService   string
		wantNamespace string
	}{
		{
			name:          "defaults when flags missing",
			flags:         map[string]string{},
			wantService:   DefaultDnsServiceName,
			wantNamespace: DefaultDnsServiceNamespace,
		},
		{
			name:          "defaults when flags empty",
			flags:         map[string]string{DnsServiceFlag: "", DnsServiceNamespaceFlag: ""},
			wantService:   DefaultDnsServiceName,
			wantNamespace: DefaultDnsServiceNamespace,
		},
		{
			name: "custom service and namespace honored",
			flags: map[string]string{
				DnsServiceFlag:          "coredns",
				DnsServiceNamespaceFlag: "dns-system",
			},
			wantService:   "coredns",
			wantNamespace: "dns-system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, ns := resolveDnsServiceFlags(&spec.ExpModel{ActionFlags: tt.flags})
			if svc != tt.wantService {
				t.Errorf("service = %q, want %q", svc, tt.wantService)
			}
			if ns != tt.wantNamespace {
				t.Errorf("namespace = %q, want %q", ns, tt.wantNamespace)
			}
		})
	}
}

func TestFindDnsDeployment(t *testing.T) {
	const ns = "kube-system"

	t.Run("service not found returns descriptive error", func(t *testing.T) {
		c := newTestClient(t)
		exec := &ClusterDnsFailureActionExecutor{client: c}
		_, err := exec.findDnsDeployment(context.Background(), ns, "kube-dns")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found error, got: %v", err)
		}
	})

	t.Run("service without selector cannot be reverse-resolved", func(t *testing.T) {
		c := newTestClient(t, dnsService("kube-dns", ns, nil))
		exec := &ClusterDnsFailureActionExecutor{client: c}
		_, err := exec.findDnsDeployment(context.Background(), ns, "kube-dns")
		if err == nil || !strings.Contains(err.Error(), "no selector") {
			t.Fatalf("expected no-selector error, got: %v", err)
		}
	})

	t.Run("matches deployment by pod template labels", func(t *testing.T) {
		svc := dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})
		matching := dnsDeployment("coredns", ns,
			map[string]string{"k8s-app": "kube-dns", "tier": "control-plane"},
			int32Ptr(2), nil)
		other := dnsDeployment("nginx", ns,
			map[string]string{"app": "nginx"}, int32Ptr(3), nil)

		c := newTestClient(t, svc, matching, other)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		got, err := exec.findDnsDeployment(context.Background(), ns, "kube-dns")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "coredns" {
			t.Errorf("got deployment %q, want %q", got.Name, "coredns")
		}
	})

	t.Run("ignores deployments whose template labels do not match", func(t *testing.T) {
		// Deployment has matching top-level labels but pod template labels do not.
		// The reverse-resolution must rely on pod template labels (what the
		// Service selector actually filters on).
		svc := dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: ns,
				Labels:    map[string]string{"k8s-app": "kube-dns"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "other"}},
				},
			},
		}

		c := newTestClient(t, svc, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		_, err := exec.findDnsDeployment(context.Background(), ns, "kube-dns")
		if err == nil || !strings.Contains(err.Error(), "no Deployment") {
			t.Fatalf("expected no-deployment error, got: %v", err)
		}
	})

	t.Run("ignores deployments in other namespaces", func(t *testing.T) {
		svc := dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})
		other := dnsDeployment("coredns", "other-ns",
			map[string]string{"k8s-app": "kube-dns"}, int32Ptr(2), nil)

		c := newTestClient(t, svc, other)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		_, err := exec.findDnsDeployment(context.Background(), ns, "kube-dns")
		if err == nil || !strings.Contains(err.Error(), "no Deployment") {
			t.Fatalf("expected no-deployment error for cross-namespace match, got: %v", err)
		}
	})

	t.Run("multiple matches refuse to choose and list candidates", func(t *testing.T) {
		// This is the exact footgun the code review flagged: two Deployments
		// (e.g. CoreDNS + node-local-dns) share the kube-dns Service selector.
		// Picking either one silently would risk a cluster-wide DNS outage.
		svc := dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})
		coredns := dnsDeployment("coredns", ns,
			map[string]string{"k8s-app": "kube-dns", "name": "coredns"}, int32Ptr(2), nil)
		nodeLocal := dnsDeployment("node-local-dns", ns,
			map[string]string{"k8s-app": "kube-dns", "name": "node-local-dns"}, int32Ptr(2), nil)

		c := newTestClient(t, svc, coredns, nodeLocal)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		_, err := exec.findDnsDeployment(context.Background(), ns, "kube-dns")
		if err == nil {
			t.Fatal("expected ambiguous-match error, got nil")
		}
		msg := err.Error()
		for _, want := range []string{"coredns", "node-local-dns", "matches 2 Deployments", DnsDeploymentFlag} {
			if !strings.Contains(msg, want) {
				t.Errorf("ambiguous-match error %q must mention %q", msg, want)
			}
		}
	})

	t.Run("ambiguous-match candidate list is sorted for stable output", func(t *testing.T) {
		// Build the conflicting deployments in non-alphabetical order to make
		// sure sorting (not insertion order) drives the printed list. This
		// keeps the error message stable across List() iteration ordering.
		svc := dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})
		z := dnsDeployment("z-dns", ns, map[string]string{"k8s-app": "kube-dns"}, int32Ptr(1), nil)
		a := dnsDeployment("a-dns", ns, map[string]string{"k8s-app": "kube-dns"}, int32Ptr(1), nil)
		m := dnsDeployment("m-dns", ns, map[string]string{"k8s-app": "kube-dns"}, int32Ptr(1), nil)

		c := newTestClient(t, svc, z, a, m)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		_, err := exec.findDnsDeployment(context.Background(), ns, "kube-dns")
		if err == nil {
			t.Fatal("expected ambiguous-match error, got nil")
		}
		// Whichever order the fake client returns, the error string must
		// contain the candidates in lexicographic order.
		if !strings.Contains(err.Error(), "[a-dns, m-dns, z-dns]") {
			t.Errorf("expected sorted candidate list in error, got: %s", err.Error())
		}
	})
}

func TestResolveTargetDnsDeployment(t *testing.T) {
	const ns = "kube-system"

	t.Run("explicit deployment bypasses reverse-resolution", func(t *testing.T) {
		// Service has an ambiguous selector that would normally fail. The
		// explicit --dns-deployment flag must short-circuit the selector logic
		// entirely so the user can recover from the ambiguity.
		svc := dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})
		coredns := dnsDeployment("coredns", ns,
			map[string]string{"k8s-app": "kube-dns"}, int32Ptr(2), nil)
		nodeLocal := dnsDeployment("node-local-dns", ns,
			map[string]string{"k8s-app": "kube-dns"}, int32Ptr(2), nil)

		c := newTestClient(t, svc, coredns, nodeLocal)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		got, err := exec.resolveTargetDnsDeployment(context.Background(), ns, "kube-dns", "coredns")
		if err != nil {
			t.Fatalf("explicit deployment lookup failed: %v", err)
		}
		if got.Name != "coredns" {
			t.Errorf("got deployment %q, want %q", got.Name, "coredns")
		}
	})

	t.Run("explicit deployment ignores Service selector entirely", func(t *testing.T) {
		// Even when the named Deployment does NOT match the Service selector,
		// we trust the operator's explicit choice. This is the escape hatch
		// for clusters where the Service's selector is unrelated to the
		// Deployment's pod template labels.
		svc := dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})
		dep := dnsDeployment("custom-dns", ns,
			map[string]string{"app": "custom"}, int32Ptr(1), nil)

		c := newTestClient(t, svc, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		got, err := exec.resolveTargetDnsDeployment(context.Background(), ns, "kube-dns", "custom-dns")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "custom-dns" {
			t.Errorf("got %q, want %q", got.Name, "custom-dns")
		}
	})

	t.Run("explicit deployment that does not exist surfaces a clear error", func(t *testing.T) {
		c := newTestClient(t)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		_, err := exec.resolveTargetDnsDeployment(context.Background(), ns, "kube-dns", "ghost")
		if err == nil || !strings.Contains(err.Error(), "DNS deployment kube-system/ghost not found") {
			t.Fatalf("expected not-found error for explicit deployment, got: %v", err)
		}
	})

	t.Run("empty explicit deployment falls back to reverse-resolution", func(t *testing.T) {
		svc := dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})
		dep := dnsDeployment("coredns", ns,
			map[string]string{"k8s-app": "kube-dns"}, int32Ptr(2), nil)

		c := newTestClient(t, svc, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		got, err := exec.resolveTargetDnsDeployment(context.Background(), ns, "kube-dns", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "coredns" {
			t.Errorf("got %q, want %q", got.Name, "coredns")
		}
	})
}

func TestErrorsHintAtDisambiguationFlag(t *testing.T) {
	// All reverse-resolution failure paths must point the operator at the
	// --dns-deployment escape hatch so they are not stuck.
	const ns = "kube-system"

	cases := []struct {
		name string
		objs []sigsclient.Object
	}{
		{
			name: "service not found",
			objs: nil,
		},
		{
			name: "service without selector",
			objs: []sigsclient.Object{dnsService("kube-dns", ns, nil)},
		},
		{
			name: "no matching deployment",
			objs: []sigsclient.Object{dnsService("kube-dns", ns, map[string]string{"k8s-app": "kube-dns"})},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, tt.objs...)
			exec := &ClusterDnsFailureActionExecutor{client: c}
			_, err := exec.findDnsDeployment(context.Background(), ns, "kube-dns")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), DnsDeploymentFlag) {
				t.Errorf("error %q should mention --%s as the escape hatch", err.Error(), DnsDeploymentFlag)
			}
		})
	}
}

func TestResolveDnsFlags(t *testing.T) {
	t.Run("explicit deployment value is trimmed and returned", func(t *testing.T) {
		svc, ns, dep := resolveDnsFlags(&spec.ExpModel{ActionFlags: map[string]string{
			DnsServiceFlag:          "coredns",
			DnsServiceNamespaceFlag: "kube-system",
			DnsDeploymentFlag:       "  my-dns  ",
		}})
		if svc != "coredns" || ns != "kube-system" {
			t.Errorf("service/ns = %q/%q, want coredns/kube-system", svc, ns)
		}
		if dep != "my-dns" {
			t.Errorf("deployment = %q, want %q (whitespace must be trimmed)", dep, "my-dns")
		}
	})

	t.Run("missing explicit deployment yields empty string", func(t *testing.T) {
		_, _, dep := resolveDnsFlags(&spec.ExpModel{ActionFlags: map[string]string{}})
		if dep != "" {
			t.Errorf("deployment = %q, want empty", dep)
		}
	})
}

func TestDnsIdentifierHelpers(t *testing.T) {
	const ns = "kube-system"

	tests := []struct {
		name        string
		svc         string
		explicit    string
		resolved    string
		wantInitial string
		wantFinal   string
	}{
		{
			name:        "service-based identification carries svc/dep in resolved form",
			svc:         "kube-dns",
			explicit:    "",
			resolved:    "coredns",
			wantInitial: "kube-system/kube-dns",
			wantFinal:   "kube-system/kube-dns/coredns",
		},
		{
			name:        "explicit-deployment path omits the service segment",
			svc:         "kube-dns",
			explicit:    "node-local-dns",
			resolved:    "node-local-dns",
			wantInitial: "kube-system/node-local-dns",
			wantFinal:   "kube-system/node-local-dns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := initialDnsIdentifier(ns, tt.svc, tt.explicit); got != tt.wantInitial {
				t.Errorf("initial identifier = %q, want %q", got, tt.wantInitial)
			}
			if got := resolvedDnsIdentifier(ns, tt.svc, tt.explicit, tt.resolved); got != tt.wantFinal {
				t.Errorf("resolved identifier = %q, want %q", got, tt.wantFinal)
			}
		})
	}
}

func TestInjectClusterDnsFailure(t *testing.T) {
	const ns = "kube-system"
	templateLabels := map[string]string{"k8s-app": "kube-dns"}

	t.Run("backs up replicas, scales to zero and writes annotations", func(t *testing.T) {
		dep := dnsDeployment("coredns", ns, templateLabels, int32Ptr(2), nil)
		c := newTestClient(t, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		if err := exec.injectClusterDnsFailure(context.Background(), dep, "exp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := &appsv1.Deployment{}
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, got); err != nil {
			t.Fatalf("get after inject failed: %v", err)
		}
		if got.Spec.Replicas == nil || *got.Spec.Replicas != 0 {
			t.Errorf("replicas after inject = %v, want 0", got.Spec.Replicas)
		}
		if got.Annotations[ChaosBladeOriginalReplicasAnnotation] != "2" {
			t.Errorf("original replicas annotation = %q, want %q",
				got.Annotations[ChaosBladeOriginalReplicasAnnotation], "2")
		}
		if got.Annotations[ChaosBladeClusterDnsAnnotation] != ChaosBladeClusterDnsAction {
			t.Errorf("dns action annotation = %q, want %q",
				got.Annotations[ChaosBladeClusterDnsAnnotation], ChaosBladeClusterDnsAction)
		}
		if got.Annotations[ChaosBladeExperimentAnnotation] != "exp-1" {
			t.Errorf("experiment annotation = %q, want %q",
				got.Annotations[ChaosBladeExperimentAnnotation], "exp-1")
		}
	})

	t.Run("nil replicas treated as 1", func(t *testing.T) {
		dep := dnsDeployment("coredns", ns, templateLabels, nil, nil)
		c := newTestClient(t, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		if err := exec.injectClusterDnsFailure(context.Background(), dep, "exp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := &appsv1.Deployment{}
		_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, got)
		if got.Annotations[ChaosBladeOriginalReplicasAnnotation] != "1" {
			t.Errorf("original replicas annotation = %q, want %q",
				got.Annotations[ChaosBladeOriginalReplicasAnnotation], "1")
		}
	})

	t.Run("idempotent: same experiment id is a no-op", func(t *testing.T) {
		// Deployment already marked by the same experiment with replicas==0.
		// Re-running the injection must not overwrite the backed-up replicas
		// (which would cause the destroy flow to "restore" to 0).
		dep := dnsDeployment("coredns", ns, templateLabels, int32Ptr(0), map[string]string{
			ChaosBladeOriginalReplicasAnnotation: "2",
			ChaosBladeClusterDnsAnnotation:       ChaosBladeClusterDnsAction,
			ChaosBladeExperimentAnnotation:       "exp-1",
		})
		c := newTestClient(t, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		if err := exec.injectClusterDnsFailure(context.Background(), dep, "exp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := &appsv1.Deployment{}
		_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, got)
		if got.Annotations[ChaosBladeOriginalReplicasAnnotation] != "2" {
			t.Errorf("idempotent re-injection clobbered backup, annotation = %q",
				got.Annotations[ChaosBladeOriginalReplicasAnnotation])
		}
	})

	t.Run("conflict: another experiment owns the deployment", func(t *testing.T) {
		dep := dnsDeployment("coredns", ns, templateLabels, int32Ptr(0), map[string]string{
			ChaosBladeOriginalReplicasAnnotation: "2",
			ChaosBladeClusterDnsAnnotation:       ChaosBladeClusterDnsAction,
			ChaosBladeExperimentAnnotation:       "exp-other",
		})
		c := newTestClient(t, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		err := exec.injectClusterDnsFailure(context.Background(), dep, "exp-1")
		if err == nil {
			t.Fatalf("expected conflict error, got nil")
		}
		if !strings.Contains(err.Error(), "already modified by another chaosblade experiment") {
			t.Errorf("unexpected error message: %v", err)
		}

		// Backup must remain untouched so the original owner can still restore.
		got := &appsv1.Deployment{}
		_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, got)
		if got.Annotations[ChaosBladeOriginalReplicasAnnotation] != "2" {
			t.Errorf("conflict path mutated backup annotation: %q",
				got.Annotations[ChaosBladeOriginalReplicasAnnotation])
		}
		if got.Annotations[ChaosBladeExperimentAnnotation] != "exp-other" {
			t.Errorf("conflict path mutated experiment owner: %q",
				got.Annotations[ChaosBladeExperimentAnnotation])
		}
	})
}

func TestRestoreClusterDnsDeployment(t *testing.T) {
	const ns = "kube-system"
	templateLabels := map[string]string{"k8s-app": "kube-dns"}

	t.Run("restores replicas and clears annotations", func(t *testing.T) {
		dep := dnsDeployment("coredns", ns, templateLabels, int32Ptr(0), map[string]string{
			ChaosBladeOriginalReplicasAnnotation: "2",
			ChaosBladeClusterDnsAnnotation:       ChaosBladeClusterDnsAction,
			ChaosBladeExperimentAnnotation:       "exp-1",
			"other-annotation":                   "keep-me",
		})
		c := newTestClient(t, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		if err := exec.restoreClusterDnsDeployment(context.Background(), dep, "exp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := &appsv1.Deployment{}
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, got); err != nil {
			t.Fatalf("get after restore failed: %v", err)
		}
		if got.Spec.Replicas == nil || *got.Spec.Replicas != 2 {
			t.Errorf("replicas after restore = %v, want 2", got.Spec.Replicas)
		}
		if _, exists := got.Annotations[ChaosBladeOriginalReplicasAnnotation]; exists {
			t.Errorf("original replicas annotation should be cleared")
		}
		if _, exists := got.Annotations[ChaosBladeClusterDnsAnnotation]; exists {
			t.Errorf("dns action annotation should be cleared")
		}
		if _, exists := got.Annotations[ChaosBladeExperimentAnnotation]; exists {
			t.Errorf("experiment annotation should be cleared")
		}
		if got.Annotations["other-annotation"] != "keep-me" {
			t.Errorf("unrelated annotation must be preserved")
		}
	})

	t.Run("deployment without annotations is a safe no-op", func(t *testing.T) {
		dep := dnsDeployment("coredns", ns, templateLabels, int32Ptr(3), nil)
		c := newTestClient(t, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		if err := exec.restoreClusterDnsDeployment(context.Background(), dep, "exp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := &appsv1.Deployment{}
		_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, got)
		if got.Spec.Replicas == nil || *got.Spec.Replicas != 3 {
			t.Errorf("replicas changed unexpectedly: %v", got.Spec.Replicas)
		}
	})

	t.Run("foreign experiment id is a safe no-op", func(t *testing.T) {
		// Critical safety property: a destroy from a different experiment must
		// not restore (and thereby leak) somebody else's backup, nor change
		// the current replica count.
		dep := dnsDeployment("coredns", ns, templateLabels, int32Ptr(0), map[string]string{
			ChaosBladeOriginalReplicasAnnotation: "2",
			ChaosBladeClusterDnsAnnotation:       ChaosBladeClusterDnsAction,
			ChaosBladeExperimentAnnotation:       "exp-other",
		})
		c := newTestClient(t, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		if err := exec.restoreClusterDnsDeployment(context.Background(), dep, "exp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := &appsv1.Deployment{}
		_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, got)
		if got.Spec.Replicas == nil || *got.Spec.Replicas != 0 {
			t.Errorf("foreign-id restore mutated replicas: %v", got.Spec.Replicas)
		}
		if got.Annotations[ChaosBladeExperimentAnnotation] != "exp-other" {
			t.Errorf("foreign-id restore cleared other owner's annotation")
		}
	})

	t.Run("malformed backup annotation surfaces decode error", func(t *testing.T) {
		dep := dnsDeployment("coredns", ns, templateLabels, int32Ptr(0), map[string]string{
			ChaosBladeOriginalReplicasAnnotation: "not-a-number",
			ChaosBladeClusterDnsAnnotation:       ChaosBladeClusterDnsAction,
			ChaosBladeExperimentAnnotation:       "exp-1",
		})
		c := newTestClient(t, dep)
		exec := &ClusterDnsFailureActionExecutor{client: c}

		err := exec.restoreClusterDnsDeployment(context.Background(), dep, "exp-1")
		if err == nil {
			t.Fatalf("expected decode error, got nil")
		}
		if !strings.Contains(err.Error(), "decode original replicas") {
			t.Errorf("unexpected error message: %v", err)
		}

		// The deployment must remain untouched so a human (or a follow-up
		// destroy with a corrected annotation) can recover it.
		got := &appsv1.Deployment{}
		_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, got)
		if got.Spec.Replicas == nil || *got.Spec.Replicas != 0 {
			t.Errorf("error path mutated replicas: %v", got.Spec.Replicas)
		}
	})
}

// TestExec_CreateThenDestroy_RoundTrip is the end-to-end safety net the code
// review asked for: it stitches the real Exec entry point against a fake
// client and verifies that destroying the experiment scales the DNS workload
// back to its original replica count.
func TestExec_CreateThenDestroy_RoundTrip(t *testing.T) {
	const ns = "kube-system"
	selector := map[string]string{"k8s-app": "kube-dns"}

	svc := dnsService("kube-dns", ns, selector)
	dep := dnsDeployment("coredns", ns, selector, int32Ptr(2), nil)

	c := newTestClient(t, svc, dep)
	executor := &ClusterDnsFailureActionExecutor{client: c}

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			DnsServiceFlag:          "kube-dns",
			DnsServiceNamespaceFlag: ns,
		},
	}

	createCtx := model.SetExperimentIdToContext(context.Background(), "exp-roundtrip")
	createResp := executor.Exec("uid-1", createCtx, expModel)
	if createResp == nil {
		t.Fatal("create response is nil")
	}
	if status, ok := createResp.Result.(v1alpha1.ExperimentStatus); ok && !status.Success {
		t.Fatalf("create failed: %s", status.Error)
	}

	// Mid-experiment state: replicas==0 and annotations populated.
	mid := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, mid); err != nil {
		t.Fatalf("get mid-state failed: %v", err)
	}
	if mid.Spec.Replicas == nil || *mid.Spec.Replicas != 0 {
		t.Fatalf("mid-state replicas = %v, want 0", mid.Spec.Replicas)
	}
	if mid.Annotations[ChaosBladeExperimentAnnotation] != "exp-roundtrip" {
		t.Fatalf("mid-state experiment annotation = %q, want %q",
			mid.Annotations[ChaosBladeExperimentAnnotation], "exp-roundtrip")
	}

	// Destroy phase.
	destroyCtx := spec.SetDestroyFlag(createCtx, "uid-1")
	destroyResp := executor.Exec("uid-1", destroyCtx, expModel)
	if destroyResp == nil {
		t.Fatal("destroy response is nil")
	}

	// Post-destroy state: replicas restored, annotations cleared.
	post := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, post); err != nil {
		t.Fatalf("get post-destroy state failed: %v", err)
	}
	if post.Spec.Replicas == nil || *post.Spec.Replicas != 2 {
		t.Errorf("post-destroy replicas = %v, want 2 — DNS workload would be left scaled to %v",
			post.Spec.Replicas, post.Spec.Replicas)
	}
	for _, key := range []string{
		ChaosBladeOriginalReplicasAnnotation,
		ChaosBladeClusterDnsAnnotation,
		ChaosBladeExperimentAnnotation,
	} {
		if _, exists := post.Annotations[key]; exists {
			t.Errorf("annotation %q should have been cleared after destroy", key)
		}
	}
}

// TestExec_ExplicitDeployment_RoundTrip covers the disambiguation path: two
// Deployments share the kube-dns Service selector, so the reverse-resolution
// would refuse to choose. The operator escapes via --dns-deployment, and the
// action must still inject and restore the explicitly named workload while
// leaving the other one completely untouched.
func TestExec_ExplicitDeployment_RoundTrip(t *testing.T) {
	const ns = "kube-system"
	selector := map[string]string{"k8s-app": "kube-dns"}

	svc := dnsService("kube-dns", ns, selector)
	coredns := dnsDeployment("coredns", ns, selector, int32Ptr(2), nil)
	nodeLocal := dnsDeployment("node-local-dns", ns, selector, int32Ptr(3), nil)

	c := newTestClient(t, svc, coredns, nodeLocal)
	executor := &ClusterDnsFailureActionExecutor{client: c}

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			DnsServiceFlag:          "kube-dns",
			DnsServiceNamespaceFlag: ns,
			DnsDeploymentFlag:       "coredns",
		},
	}

	createCtx := model.SetExperimentIdToContext(context.Background(), "exp-explicit")
	if createResp := executor.Exec("uid-1", createCtx, expModel); createResp == nil {
		t.Fatal("create response is nil")
	}

	mid := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, mid)
	if mid.Spec.Replicas == nil || *mid.Spec.Replicas != 0 {
		t.Errorf("explicit-target replicas after inject = %v, want 0", mid.Spec.Replicas)
	}
	if mid.Annotations[ChaosBladeExperimentAnnotation] != "exp-explicit" {
		t.Errorf("explicit-target experiment annotation = %q, want exp-explicit",
			mid.Annotations[ChaosBladeExperimentAnnotation])
	}

	// The non-targeted workload must be completely untouched — this is the
	// whole point of the disambiguation fix.
	bystander := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "node-local-dns"}, bystander)
	if bystander.Spec.Replicas == nil || *bystander.Spec.Replicas != 3 {
		t.Errorf("bystander replicas mutated to %v — disambiguation failed", bystander.Spec.Replicas)
	}
	if _, exists := bystander.Annotations[ChaosBladeExperimentAnnotation]; exists {
		t.Error("bystander annotation set — disambiguation failed")
	}

	// Destroy phase: must also operate on the explicit target only.
	destroyCtx := spec.SetDestroyFlag(createCtx, "uid-1")
	if destroyResp := executor.Exec("uid-1", destroyCtx, expModel); destroyResp == nil {
		t.Fatal("destroy response is nil")
	}

	post := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "coredns"}, post)
	if post.Spec.Replicas == nil || *post.Spec.Replicas != 2 {
		t.Errorf("post-destroy replicas = %v, want 2", post.Spec.Replicas)
	}

	bystanderAfter := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "node-local-dns"}, bystanderAfter)
	if bystanderAfter.Spec.Replicas == nil || *bystanderAfter.Spec.Replicas != 3 {
		t.Errorf("bystander replicas mutated during destroy: %v", bystanderAfter.Spec.Replicas)
	}
}

// TestExec_AmbiguousSelector_FailsLoudly verifies the create path surfaces a
// resource-level failure (rather than silently picking one) when multiple
// Deployments match the DNS Service selector and no --dns-deployment is set.
func TestExec_AmbiguousSelector_FailsLoudly(t *testing.T) {
	const ns = "kube-system"
	selector := map[string]string{"k8s-app": "kube-dns"}

	svc := dnsService("kube-dns", ns, selector)
	coredns := dnsDeployment("coredns", ns, selector, int32Ptr(2), nil)
	nodeLocal := dnsDeployment("node-local-dns", ns, selector, int32Ptr(3), nil)

	c := newTestClient(t, svc, coredns, nodeLocal)
	executor := &ClusterDnsFailureActionExecutor{client: c}

	expModel := &spec.ExpModel{
		ActionFlags: map[string]string{
			DnsServiceFlag:          "kube-dns",
			DnsServiceNamespaceFlag: ns,
			// Intentionally no DnsDeploymentFlag.
		},
	}

	ctx := model.SetExperimentIdToContext(context.Background(), "exp-ambiguous")
	resp := executor.Exec("uid-1", ctx, expModel)
	if resp == nil {
		t.Fatal("response is nil")
	}

	// Neither Deployment should have been scaled — the ambiguity must be
	// surfaced before any mutation. This is the regression the reviewer's
	// concern would have allowed.
	for _, name := range []string{"coredns", "node-local-dns"} {
		got := &appsv1.Deployment{}
		_ = c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, got)
		if got.Spec.Replicas == nil {
			t.Errorf("%s replicas became nil", name)
			continue
		}
		// coredns started at 2, node-local-dns at 3 — both must be unchanged.
		want := int32(2)
		if name == "node-local-dns" {
			want = 3
		}
		if *got.Spec.Replicas != want {
			t.Errorf("%s replicas = %d, want %d (ambiguous-selector path must not mutate)",
				name, *got.Spec.Replicas, want)
		}
		if _, exists := got.Annotations[ChaosBladeExperimentAnnotation]; exists {
			t.Errorf("%s was annotated by ambiguous-selector path", name)
		}
	}
}
