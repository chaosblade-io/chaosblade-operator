# ChaosBlade Operator E2E Tests

中文版 [README](README_CN.md)

End-to-end test module: verifies the full "CR created → fault takes effect → recovery"
chain against a real Kubernetes cluster.

## Layout

```
e2e-test/                  # standalone Go module (replace points at the main repo's CRD types)
├── e2e/                   # test cases, one package per scope, each with its own Ginkgo bootstrap
│   └── pod/               # Pod scope (phase 1: pod-delete)
└── pkg/fixture/           # Framework (Kubernetes helpers) + ChaosBladeBuilder
```

Each scope subdirectory is its own Go package holding its own `RunSpecs` bootstrap
(Ginkgo's "auto-discovery" only holds within a single package); shared flag and client
logic lives in `pkg/fixture`.

## Prerequisites

1. A local kubeconfig that reaches the target cluster (`export KUBECONFIG=/path/to/kubeconfig`,
   or the default `~/.kube/config`).
2. chaosblade-operator installed in the cluster, and the `chaosblades.chaosblade.io` CRD is
   **cluster-scoped** (both the helm chart and `deploy/oss/crd.yaml` are; `deploy/crds/*.yaml`
   is a stale namespaced variant). The suite preflight checks both and fails immediately with
   the reason instead of letting the specs time out.

   From scratch: `make install-operator` (helm installs `../deploy/helm/chaosblade-operator`
   into the `chaosblade` namespace).

## Running

```
make e2e-run                                  # all suites
make e2e-run PKG=./e2e/pod/...                # the pod scope suite only
make e2e-run PKG=./e2e/pod/... LABEL_FILTER=pod-delete   # the pod-delete spec only
make e2e-run IMAGE=busybox:1.36               # pick an image when the cluster can reach a public registry
make e2e-run NAMESPACE=my-ns                  # reuse an existing namespace
```

### Namespace semantics

- Default (`NAMESPACE` empty): every run creates a unique `chaosblade-e2e-<suffix>` namespace
  and deletes it when the suite finishes, so two concurrent runs cannot interfere.
- Explicit `NAMESPACE`: that namespace is reused and **never deleted by the suite**, so it
  cannot destroy other people's resources (including a mistaken `NAMESPACE=default`). Remove
  leftovers with `make e2e-cleanup`, which selects by label.
- CRs, target pods and namespaces created by the framework all carry the `chaosblade-e2e=true`
  label, and `e2e-cleanup` deletes by that label only (pods across all namespaces, to cover the
  reused-namespace case).

### Image and scheduling

The target pod image defaults to `ghcr.io/chaosblade-io/chaosblade-tool:1.8.0` (the framework
constant `fixture.DefaultImage`; leave the Makefile's `IMAGE` empty to use it): clusters inside
a VPC cannot reach docker.io, while this image is already cached on every node by the tool
DaemonSet. **Arm64 clusters must override it** with
`IMAGE=ghcr.io/chaosblade-io/chaosblade-tool-arm64:1.8.0` (name taken from
`deploy/helm/chaosblade-operator-arm64/values.yaml`). Target pods tolerate all NoSchedule and
NoExecute taints, like the chaosblade-tool DaemonSet, so they still schedule on shared clusters
whose nodes carry resource pool taints and are not evicted by node pressure mid-test. When
scheduling or image pulling fails, the timeout error carries the root cause (image name,
`phase`, `PodScheduled` condition, container waiting reason).

### What the specs assert

Assertions land on both the physical effect (the target pod is deleted) and the CR status
(`Status.Phase=Running`, `ExpStatuses[].Success=true`, non-empty `ResStatuses`); after the CR is
deleted the specs verify that the finalizer released it (the CR object disappears). Note that the
CRD is cluster-scoped, so the CR itself carries no namespace — the target pod's namespace is
expressed through the experiment's `namespace` matcher.

## CI

`.github/workflows/e2e_test.yml` runs on PR / push (main, master) and on manual dispatch;
documentation-only changes (`**.md`, `docs/`, `examples/`) are skipped. Three jobs:

1. **build-operator-image**: builds the operator image **from this revision** with docker buildx,
   tagged `e2e-<sha8>` (distinct from the published `1.8.0`), then passes it downstream as a
   `docker save` artifact. Built once.
2. **e2e**: minikube (K8s v1.29.4, docker driver) → `minikube image load` →
   `helm install --set operator.repository/version=...` → `kubectl rollout status` waits for the
   operator to become Ready → records tool DaemonSet readiness → pre-creates the
   `chaosblade-e2e` namespace → `make -C e2e-test e2e-run`. The matrix is
   `include: [{focus, pkg}]` with `fail-fast: false`. The job limit is **45 minutes**, and the
   worst-case budget is: cluster setup ≤10min + DaemonSet probe ≤2min + inner
   `go test -timeout 30m` = ≤42min < 45min. That margin exists so the **inner Go timeout fires
   first** and prints its goroutine dump, instead of the job being killed outright (on a kill
   `if: failure()` does not run and the diagnostics would be lost, which is why the diagnostics
   condition is `failure() || cancelled()`). **Recompute this arithmetic when adding steps.**
   On failure or cancellation it uploads the `e2e-diagnostics-<focus>` artifact (operator logs,
   `chaosblade` CR yaml, pods/describe/events for the operator and target namespaces, DaemonSet
   describe, minikube logs).
3. **summary**: aggregates the upstream results and fails unless all of them succeeded.

CI **pre-creates** the target namespace so the suite takes the "adopt" path
(`OwnsNamespace()=false`) and AfterSuite does not delete it — that is what keeps the target pods
and events available to the diagnostics step after a failure. Each matrix job owns its own
minikube, so a fixed name cannot collide.

The `chaosblade-tool` DaemonSet readiness is only **recorded, never blocking** (the wait is
capped at 120s and a timeout only emits a warning): pod-delete goes through the Kubernetes API
end to end (`exec/pod/delete.go` calls `client.Delete`) and never touches the tool or the exec
channel, so blocking on it would add a failure mode without removing a flake. Phase 2's pod
cpu/network and node scope specs do depend on the tool; that step must then become a blocking
`kubectl rollout status daemonset/chaosblade-tool`, and the 45 minute budget above must be
recomputed.

**When adding a spec**: append one `{focus, pkg}` entry to `matrix.include` in `e2e_test.yml` and
put a matching `Label` on the spec's `Describe`; the job structure does not change.

## Troubleshooting

The operator's namespace depends on how it was installed: `chaosblade` for helm, `default` in
this repo's development cluster. Below, `OPERATOR_NS` stands for whichever applies.

```
kubectl get deploy chaosblade-operator -n ${OPERATOR_NS}
kubectl logs deploy/chaosblade-operator -n ${OPERATOR_NS}
kubectl get chaosblade <name> -o yaml            # no -n: the CR is cluster-scoped
kubectl get events -n <target-namespace> --sort-by=.lastTimestamp
make e2e-cleanup                                 # remove leftover CRs and namespaces by framework label
```
