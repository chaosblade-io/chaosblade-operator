# Chaosblade Operator: 面向云原生的混沌工程执行工具
![license](https://img.shields.io/github/license/chaosblade-io/chaosblade.svg)

## 介绍
Chaosblade Operator 是混沌工程实验工具 ChaosBlade 下的一款面向云原生领域的混沌实验注入工具，可单独部署使用。通过定义 Kubernetes CRD 来管理混沌实验，每个实验都有非常明确的执行状态。工具具有部署简单、执行便捷、标准化实现、场景丰富等特点。将 ChaosBlade 混沌实验模型与 Kubernetes CRD 很好的结合在一起，可以实现基础资源、应用服务、容器等场景在 Kubernetes 平台上场景复用，方便了 Kubernetes 下资源场景的扩展，而且可通过 chaosblade cli 统一执行调用。

## 支持的场景(持续新增中...)
目前实验场景涉及到资源包含 Node、Pod、Container，具体支持的场景如下：
* Node：
    * CPU: 指定 CPU 使用率
    * 网络: 指定网卡、端口、IP 等包延迟、丢包、包阻塞、包重复、包乱序、包损坏等
    * 进程：指定进程 Hang、强杀指定进程等
    * 磁盘：指定目录磁盘填充、磁盘 IO 读写负载等
    * 内存：指定内存使用率
* Pod：
    * 网络：指定网卡、端口、IP 等包延迟、丢包、包阻塞、包重复、包乱序、包损坏等
    * 磁盘：指定目录磁盘填充、磁盘 IO 读写负载等
    * 内存：指定内存使用率
    * Pod：杀 Pod
* Container：
    * CPU: 指定 CPU 使用率
    * 网络: 指定网卡、端口、IP 等包延迟、丢包、包阻塞、包重复、包乱序、包损坏等
    * 进程：指定进程 Hang、强杀指定进程等
    * 磁盘：指定目录磁盘填充、磁盘 IO 读写负载等
    * 内存：指定内存使用率
    * Container: 杀 Container

## 安装&卸载
支持的 Kubernetes 最小版本是 v1.12，chaosblade operator 可通过 kubectl 或者 helm 进行安装，安装方式如下：
注意：以下的 `VERSION` 请使用最新的版本号替代
### Helm v2
* 在 [Release](https://github.com/chaosblade-io/chaosblade-operator/releases) 地址下载最新的 `chaosblade-operator-VERSION-v2.tgz` 包
* 使用 `helm install --namespace kube-system --name chaosblade-operator chaosblade-operator-VERSION-v2.tgz` 命令安装
* 使用 `kubectl get pod -l part-of=chaosblade -n kube-system` 查看 Pod 的安装状态，如果都是 running 状态，说明安装成功
* 使用以下命令进行卸载，注意执行顺序：
```shell script
kubectl delete crd chaosblades.chaosblade.io
helm del --purge chaosblade-operator
```
### Helm v3
* 在 [Release](https://github.com/chaosblade-io/chaosblade-operator/releases) 地址下载最新的 `chaosblade-operator-VERSION-v3.tgz` 包
* 使用 `helm install chaosblade-operator chaosblade-operator-VERSION-v3.tgz --namespace kube-system` 命令安装
* 使用 `kubectl get pod -l part-of=chaosblade -n kube-system` 查看 Pod 的安装状态，如果都是 running 状态，说明安装成功
* 使用以下命令卸载，注意执行顺序:
```shell script
kubectl delete crd chaosblades.chaosblade.io
helm uninstall chaosblade-operator -n kube-system
```

### Kubectl
* 在 [Release](https://github.com/chaosblade-io/chaosblade-operator/releases) 地址下载最新的 `chaosblade-operator-yaml-VERSION.tar.gz` 包
* 解压后执行 `kubectl apply -f chaosblade-operator-yaml-VERSION/` 安装
* 使用 `kubectl get pod -l part-of=chaosblade -n kube-system` 查看 Pod 的安装状态，如果都是 running 状态，说明安装成功
* 使用以下命令卸载，注意执行顺序：
```shell script
kubectl delete crd chaosblades.chaosblade.io
kubectl delete -f chaosblade-operator-yaml-VERSION/
```

## 使用
安装 chaosblade operator 后即可执行混沌实验，执行方式有以下三种：
* 通过配置 yaml 方式，使用 kubectl 执行
* 使用 chaosblade cli 工具执行
* 通过编写代码调用 Kubernetes API 执行

下面通过一个具体的案例来说明 chaosblade-operator 的使用：模拟 cn-hangzhou.192.168.0.205 节点本地端口 40690 60% 的网络丢包。

### 通过配置 yaml 方式，使用 kubectl 执行
```
apiVersion: chaosblade.io/v1alpha1
kind: ChaosBlade
metadata:
  name: loss-node-network-by-names
spec:
  experiments:
  - scope: node
    target: network
    action: loss
    desc: "node network loss"
    matchers:
    - name: names
      value: ["cn-hangzhou.192.168.0.205"]
    - name: percent
      value: ["60"]
    - name: interface
      value: ["eth0"]
    - name: local-port
      value: ["40690"]
```
执行实验：
```
kubectl apply -f loss-node-network-by-names.yaml
```
查询实验状态，返回信息如下（省略了 spec 等内容）：
```
~ » kubectl get blade loss-node-network-by-names -o json                                                            
{
    "apiVersion": "chaosblade.io/v1alpha1",
    "kind": "ChaosBlade",
    "metadata": {
        "creationTimestamp": "2019-11-04T09:56:36Z",
        "finalizers": [
            "finalizer.chaosblade.io"
        ],
        "generation": 1,
        "name": "loss-node-network-by-names",
        "resourceVersion": "9262302",
        "selfLink": "/apis/chaosblade.io/v1alpha1/chaosblades/loss-node-network-by-names",
        "uid": "63a926dd-fee9-11e9-b3be-00163e136d88"
    },
        "status": {
        "expStatuses": [
            {
                "action": "loss",
                "resStatuses": [
                    {
                        "id": "057acaa47ae69363",
                        "kind": "node",
                        "name": "cn-hangzhou.192.168.0.205",
                        "nodeName": "cn-hangzhou.192.168.0.205",
                        "state": "Success",
                        "success": true,
                        "uid": "e179b30d-df77-11e9-b3be-00163e136d88"
                    }
                ],
                "scope": "node",
                "state": "Success",
                "success": true,
                "target": "network"
            }
        ],
        "phase": "Running"
    }
}
```
通过以上内容可以很清晰的看出混沌实验的运行状态，执行以下命令停止实验：
```
kubectl delete -f loss-node-network-by-names.yaml
```
或者直接删除此 blade 资源
```
kubectl delete blade loss-node-network-by-names
```
还可以编辑 yaml 文件，更新实验内容执行，chaosblade operator 会完成实验的更新操作。更多案例请查看 [Examples](https://github.com/chaosblade-io/chaosblade-operator/tree/master/examples)

### 使用 chaosblade cli 工具执行
```
blade create k8s node-network loss --percent 60 --interface eth0 --local-port 40690 --names cn-hangzhou.192.168.0.205 --kubeconfig config
```
如果执行失败，会返回详细的错误信息；如果执行成功，会返回实验的 UID：
```
{"code":200,"success":true,"result":"e647064f5f20953c"}
```
可通过以下命令查询实验状态：
```
blade query k8s create e647064f5f20953c --kubeconfig config

{
  "code": 200,
  "success": true,
  "result": {
    "uid": "e647064f5f20953c",
    "success": true,
    "error": "",
    "statuses": [
      {
        "id": "fa471a6285ec45f5",
        "uid": "e179b30d-df77-11e9-b3be-00163e136d88",
        "name": "cn-hangzhou.192.168.0.205",
        "state": "Success",
        "kind": "node",
        "success": true,
        "nodeName": "cn-hangzhou.192.168.0.205"
      }
    ]
  }
}
```
销毁实验：
```
blade destroy e647064f5f20953c
```
除了上述两种方式调用外，还可以使用 kubernetes client-go 方式执行，具体可参考：[executor.go](https://github.com/chaosblade-io/chaosblade/blob/master/exec/kubernetes/executor.go) 代码实现。

[中文使用文档](https://chaosblade-io.gitbook.io/chaosblade-help-zh-cn/blade-create-k8s)

## 问题&建议
如果在安装使用过程中遇到问题，或者建议和新功能，所有项目（包含其他项目）的问题都可以提交到[Github Issues](https://github.com/chaosblade-io/chaosblade/issues) 

你也可以通过以下方式联系我们：
* 钉钉群（推荐）：23177705
* Gitter room: [chaosblade community](https://gitter.im/chaosblade-io/community)
* 邮箱：chaosblade.io.01@gmail.com
* Twitter: [chaosblade.io](https://twitter.com/ChaosbladeI)

## 参与贡献
我们非常欢迎每个 Issue 和 PR，即使一个标点符号，如何参加贡献请阅读 [CONTRIBUTING](CONTRIBUTING.md) 文档，或者通过上述的方式联系我们。

## 开源许可证
Chaosblade-operator 遵循 Apache 2.0 许可证，详细内容请阅读 [LICENSE](LICENSE)
