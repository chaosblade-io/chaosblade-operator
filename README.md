# Chaosblade-operator: Cloud-native platform chaos experiment executor
![license](https://img.shields.io/github/license/chaosblade-io/chaosblade.svg)

中文版 [README](README_CN.md)
## Introduction
The Chaosblade-operator project is a chaos experiment injection tool implemented for the Kubernetes platform. It follows the chaos experiment model to standardize the experimental scene, defines the experiment as a Kubernetes CRD resource, and maps the four parts of the experiment model to Kubernetes resource attributes. The chaotic experimental model is combined with Kubernetes declarative design. While relying on the chaotic experimental model to conveniently develop the scene, it can also be well combined with the Kubernetes design concept. Through kubectl or writing code, directly call the Kubernetes API to create, update, and delete chaotic experiments. In addition, the resource status can clearly indicate the execution status of the experiment, and standardized Kubernetes fault injection. In addition to using the above methods to perform experiments, you can also use the chaosblade cli method to execute kubernetes experimental scenarios and query the experimental status very conveniently. In addition to the above advantages, the chaosblade operator implemented by following the chaos experimental model can also reuse basic resources, application services, Docker containers, and other scenarios, which greatly facilitates the expansion of Kubernetes scenarios. Therefore, in accordance with Kubernetes standardized implementation scenarios, combining The chaos experimental model can be implemented more effectively, clearly and conveniently, using chaotic experimental scenarios.

## How to use
After installing the chaosblade operator, you can use kubectl for fault injection, or you can download the [chaosblade](https://github.com/chaosblade-io/chaosblade/releases) tool for execution.
Chaosblade operator can be installed through kubectl or helm, the installation method is as follows:

Note: For the following `VERSION`, please use the latest version number instead

### Helm v2 installation
* Download the latest `chaosblade-operator-VERSION.tag` package at [Release](https://github.com/chaosblade-io/chaosblade-operator/releases)
* Install using `helm install --namespace kube-system --name chaosblade-operator chaosblade-operator-VERSION.tgz`
* Use `kubectl get pod -l part-of=chaosblade -n kube-system` to check the installation status of the Pod. If both are running, the installation was successful

### Helm v3 installation
* Download the latest `chaosblade-operator-VERSION-v3.tag` package at [Release](https://github.com/chaosblade-io/chaosblade-operator/releases)
* Use `helm install chaosblade-operator chaosblade-operator-VERSION-v3.tgz --namespace kube-system` command to install
* Use `kubectl get pod -l part-of=chaosblade -n kube-system` to check the installation status of the Pod. If both are running, the installation was successful

### Kubectl installation
* Download the latest `chaosblade-operator-yaml-VERSION.tar.gz` package at [Release](https://github.com/chaosblade-io/chaosblade-operator/releases)
* After decompression, execute `kubectl -f chaosblade-operator-yaml-VERSION/` installation
* Use `kubectl get pod -l part-of=chaosblade -n kube-system` to check the installation status of the Pod. If both are running, the installation was successful

### Perform fault injection
Please refer to the [Examples](https://github.com/chaosblade-io/chaosblade-operator/tree/master/examples) case or view the detailed Chinese documentation: https://chaosblade-io.gitbook.io/chaosblade-help-zh-cn/blade-create-k8s

## Questions & Suggestions
If you encounter problems during installation and use, or suggestions and new features, all projects (including other projects) can be submitted to [Github Issues](https://github.com/chaosblade-io/chaosblade/issues)

You can also contact us via:
* Dingding group: 23177705
* Gitter room: [chaosblade community](https://gitter.im/chaosblade-io/community)
* Email: chaosblade.io.01@gmail.com
* Twitter: [chaosblade.io](https://twitter.com/ChaosbladeI)

## Contributions
We welcome every issue and PR. Even a punctuation mark, how to participate in the contribution please read the [CONTRIBUTING](CONTRIBUTING.md) document, or contact us through the above method.

## Open source license
Chaosblade-operator is licensed under the Apache 2.0 license. For details, please read [LICENSE](LICENSE)
