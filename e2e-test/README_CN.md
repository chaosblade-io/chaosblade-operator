# ChaosBlade Operator E2E Tests

端到端测试 module：在真实 Kubernetes 集群上验证「CR 创建 → 故障生效 → 恢复」完整链路。

## 结构

```
e2e-test/                  # 独立 Go module（replace 指向主仓 CRD 类型）
├── e2e/                   # 测试用例，按 scope 分包，每个包自带 Ginkgo bootstrap
│   └── pod/               # Pod Scope（一期：pod-delete）
└── pkg/fixture/           # Framework（K8s 操作封装）+ ChaosBladeBuilder
```

每个 scope 子目录是独立 Go package、各自持有 `RunSpecs` bootstrap（Ginkgo 的
「自动发现」只在同一 package 内成立），共享 flag/客户端逻辑在 `pkg/fixture`。

## 前置条件

1. 本地 kubeconfig 可连目标集群（`export KUBECONFIG=/path/to/kubeconfig` 或默认 `~/.kube/config`）。
2. 集群已安装 chaosblade-operator，且 `chaosblades.chaosblade.io` CRD 为 **Cluster 作用域**
   （helm chart 与 `deploy/oss/crd.yaml` 均是；`deploy/crds/*.yaml` 是过期的 Namespaced 版本）。
   套件启动时的 preflight 会校验这两点，不满足会立即失败并给出原因，而不是让用例超时。

   从零搭建：`make install-operator`（helm 安装 `../deploy/helm/chaosblade-operator` 到 `chaosblade` namespace）。

## 运行

```
make e2e-run                                  # 跑全部套件
make e2e-run PKG=./e2e/pod/...                # 只跑 Pod Scope 套件
make e2e-run PKG=./e2e/pod/... LABEL_FILTER=pod-delete   # 只跑 pod-delete 用例
make e2e-run IMAGE=busybox:1.36               # 集群可访问公共 registry 时指定镜像
make e2e-run NAMESPACE=my-ns                  # 复用已有 namespace
```

### 命名空间语义

- 默认（`NAMESPACE` 为空）：每次运行创建唯一的 `chaosblade-e2e-<suffix>` namespace，
  套件结束时自动删除；两次并行运行互不干扰。
- 显式指定 `NAMESPACE`：复用该 namespace，**套件绝不删除它**（避免误删他人资源，
  包括 `NAMESPACE=default` 这类误用）。残留资源用 `make e2e-cleanup` 按 label 清理。
- 框架创建的 CR、被测 Pod、namespace 都带 `chaosblade-e2e=true` label；`e2e-cleanup`
  只按该 label 删除（Pod 跨 namespace 删，以覆盖复用已有 namespace 的场景）。

### 镜像与调度

被测 Pod 镜像默认 `ghcr.io/chaosblade-io/chaosblade-tool:1.8.0`（框架常量
`fixture.DefaultImage`，Makefile 的 `IMAGE` 留空即用它）：VPC 内集群访问不了 docker.io，
而该镜像由 tool DaemonSet 缓存在每个节点上。**arm64 集群需覆盖**为
`IMAGE=ghcr.io/chaosblade-io/chaosblade-tool-arm64:1.8.0`（镜像名取自
`deploy/helm/chaosblade-operator-arm64/values.yaml`）。被测 Pod 自动容忍所有
NoSchedule 与 NoExecute taint（与 chaosblade-tool DaemonSet 一致），以适配带资源池
taint 的共享集群、并避免节点压力驱逐导致用例闪断。调度或拉镜像失败时，超时报错会带出
根因（镜像名、`phase`、`PodScheduled` 条件、容器 waiting reason）。

### 断言口径

用例断言同时落在物理量（目标 Pod 被删除）与 CR 状态（`Status.Phase=Running`、
`ExpStatuses[].Success=true`、`ResStatuses` 非空）上；删除 CR 后验证 finalizer 释放
（CR 对象消失）。注意 CRD 是 Cluster 作用域，CR 本身不带 namespace，被测 Pod 的
归属通过实验 matcher `namespace` 表达。

## CI

`.github/workflows/e2e_test.yml` 在 PR / push（main、master）与手动触发时运行，
文档类改动（`**.md`、`docs/`、`examples/`）跳过。三个 job：

1. **build-operator-image**：用 docker buildx 构建**本次提交**的 operator 镜像，tag 为 `e2e-<sha8>`（与已发布的
   `1.8.0` 区分），`docker save` 后作为 artifact 传给下游，只构建一次。
2. **e2e**：minikube（K8s v1.29.4，docker driver）→ `minikube image load` →
   `helm install --set operator.repository/version=...` → `kubectl rollout status` 等
   operator Ready → 记录 tool DaemonSet 就绪状态 → 预建 `chaosblade-e2e` namespace →
   `make -C e2e-test e2e-run`。matrix 为 `include: [{focus, pkg}]`，`fail-fast: false`。
   job 上限 **45 分钟**，预算算式（最坏路径）：集群准备 ≤10min + DS 探测 ≤2min +
   内层 `go test -timeout 30m` = ≤42min < 45min。留这段余量是为了让**内层 Go 超时先触发**、
   打印 goroutine dump，而不是被 job 级硬杀（硬杀时 `if: failure()` 也不执行，诊断会丢，
   故诊断步骤条件是 `failure() || cancelled()`）。**新增步骤必须重新核算这个算式。**
   失败或取消时上传 `e2e-diagnostics-<focus>` artifact（operator 日志、`chaosblade` CR yaml、
   operator 与目标 namespace 的 pods/describe/events、DaemonSet describe、minikube 日志）。
3. **summary**：汇总上游 result，非全 success 则失败。

CI 里**预建**目标 namespace，使套件走「借用」路径（`OwnsNamespace()=false`），
AfterSuite 不删除它，失败后诊断步骤才查得到目标 Pod 与 events；每个 matrix job
独占一个 minikube，固定名字不会互相干扰。

`chaosblade-tool` DaemonSet 的就绪状态只做**非阻塞记录**（等待上限 120s，超时只发 warning）：
pod-delete 全程走 K8s API（`exec/pod/delete.go` 调 `client.Delete`），不经过 tool 与 exec
通道，阻塞等待只会凭空增加失败面。二期的 pod cpu/network、node 级用例确实依赖 tool，
届时该步骤必须改成阻塞式 `kubectl rollout status daemonset/chaosblade-tool`，
并同步重算上面的 45 分钟预算。

**新增用例时**：在 `e2e_test.yml` 的 `matrix.include` 追加一条 `{focus, pkg}`，
并在用例的 `Describe` 上打同名 `Label`；无需改 job 结构。

## 排障

operator 所在 namespace 取决于安装方式：helm 安装在 `chaosblade`，本仓联调集群在 `default`。
下面以 `OPERATOR_NS` 代指，按实际情况替换。

```
kubectl get deploy chaosblade-operator -n ${OPERATOR_NS}
kubectl logs deploy/chaosblade-operator -n ${OPERATOR_NS}
kubectl get chaosblade <name> -o yaml            # 不带 -n，CR 为 Cluster 作用域
kubectl get events -n <target-namespace> --sort-by=.lastTimestamp
make e2e-cleanup                                 # 清理框架 label 下的残留 CR 与 namespace
```
