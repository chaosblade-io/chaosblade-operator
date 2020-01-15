# Chaosblade-operator: 云原生平台混沌实验执行器
![license](https://img.shields.io/github/license/chaosblade-io/chaosblade.svg)

## 介绍
Chaosblade-operator 项目是针对 Kubernetes 平台所实现的混沌实验注入工具，遵循上述混沌实验模型规范化实验场景，把实验定义为 Kubernetes CRD 资源，将实验模型中的四部分映射为 Kubernetes 资源属性，很友好的将混沌实验模型与 Kubernetes 声明式设计结合在一起，依靠混沌实验模型便捷开发场景的同时，又可以很好的结合 Kubernetes 设计理念，通过 kubectl 或者编写代码直接调用 Kubernetes API 来创建、更新、删除混沌实验，而且资源状态可以非常清晰的表示实验的执行状态，标准化实现 Kubernetes 故障注入。除了使用上述方式执行实验外，还可以使用 chaosblade cli 方式非常方便的执行 kubernetes 实验场景，查询实验状态等。遵循混沌实验模型实现的 chaosblade operator 除上述优势之外，还可以实现基础资源、应用服务、Docker 容器等场景复用，大大方便了 Kubernetes 场景的扩展，所以在符合 Kubernetes 标准化实现场景方式之上，结合混沌实验模型可以更有效、更清晰、更方便的实现、使用混沌实验场景。

## 使用
安装 chaosblade operator 后即可通过 kubectl 进行故障注入，也可以下载 [chaosblade](https://github.com/chaosblade-io/chaosblade/releases) 工具执行。
chaosblade operator 可通过 kubectl 或者 helm 进行安装，安装方式如下：

注意：以下的 `VERSION` 请使用最新的版本号替代
### Helm v2 安装
* 在 [Release](https://github.com/chaosblade-io/chaosblade-operator/releases) 地址下载最新的 `chaosblade-operator-VERSION.tag` 包
* 使用 `helm install --namespace kube-system --name chaosblade-operator chaosblade-operator-VERSION.tgz` 命令安装
* 使用 `kubectl get pod -l part-of=chaosblade -n kube-system` 查看 Pod 的安装状态，如果都是 running 状态，说明安装成功

### Helm v3 安装
* 在 [Release](https://github.com/chaosblade-io/chaosblade-operator/releases) 地址下载最新的 `chaosblade-operator-VERSION-v3.tag` 包
* 使用 `helm install chaosblade-operator chaosblade-operator-VERSION-v3.tgz --namespace kube-system` 命令安装
* 使用 `kubectl get pod -l part-of=chaosblade -n kube-system` 查看 Pod 的安装状态，如果都是 running 状态，说明安装成功

### Kubectl 安装
* 在 [Release](https://github.com/chaosblade-io/chaosblade-operator/releases) 地址下载最新的 `chaosblade-operator-yaml-VERSION.tar.gz` 包
* 解压后执行 `kubectl -f chaosblade-operator-yaml-VERSION/` 安装
* 使用 `kubectl get pod -l part-of=chaosblade -n kube-system` 查看 Pod 的安装状态，如果都是 running 状态，说明安装成功

### 执行故障注入
请参考 [Examples](https://github.com/chaosblade-io/chaosblade-operator/tree/master/examples) 案例或者查看详细的中文文档：https://chaosblade-io.gitbook.io/chaosblade-help-zh-cn/blade-create-k8s

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
