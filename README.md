# Chaosblade-operator: A Chaos Engineering Tool for Cloud-native 
![license](https://img.shields.io/github/license/chaosblade-io/chaosblade.svg)

中文版 [README](README_CN.md)
## Introduction
Chaosblade Operator is a chaos experiments injection tool for cloud-native on kubernetes platform. By defining Kubernetes CRD to manage chaos experiments, each experiment has a very clear execution status. The tool has the characteristics of simple deployment, convenient execution, standardized implementation, and rich experiments. The chaos experimental model in chaosblade is well integrated with Kubernetes, which can realize the reuse of experiments such as basic resources, application services, and containers on the Kubernetes platform, which facilitates the expansion of resource experiments under Kubernetes, and can be executed uniformly through chaosblade cli tool.

## Supported experiments (continuously adding ...)
The current experimental scenarios involve resources including Node, Pod, and Container. The specific supported experimental scenarios are as follows:
* Node:
    * CPU: specify CPU usage
    * Network: specify network card, port, IP, etc. packet delay, packet loss, packet blocking, packet duplication, packet re-ordering, packet corruption, etc.
    * Process: specify process Hang, kill process, etc.
    * Disk: specify the directory disk occupation, disk IO read and write load, etc.
    * Memory: specify memory usage
* Pod:
    * Network: specify network card, port, IP, etc. packet delay, packet loss, packet blocking, packet duplication, packet re-ordering, packet corruption, etc.
    * Disk: specify the directory disk occupation, disk IO read and write load, etc.
    * Memory: specify memory usage
    * Pod: kill pod
    * IO: specify the file system io exception. Supports 31 file operations and 11 exception scenarios, such as "Too many open files", "Device or resource busy" and so on.
* Container:
    * CPU: specify CPU usage
    * Network: specify network card, port, IP, etc. packet delay, packet loss, packet blocking, packet duplication, packet re-ordering, packet corruption, etc.
    * Process: specify process Hang, kill process, etc.
    * Disk: specify the directory disk occupation, disk IO read and write load, etc.
    * Memory: specify memory usage
    * Container: remove container

## 

## Install and uninstall
The lowest version of kubernetes supported is 1.12. Chaosblade operator can be installed through kubectl or helm, the installation method is as follows:

Note: For the following `VERSION`, please use the latest version number instead

### Helm v2
* Download the latest `chaosblade-operator-VERSION-v2.tgz` package at [Release](https://github.com/chaosblade-io/chaosblade-operator/releases)
* Install using `helm install --namespace chaosblade --name chaosblade-operator chaosblade-operator-VERSION-v2.tgz`
* Use `kubectl get pod -l part-of=chaosblade -n chaosblade` to check the installation status of the Pod. If both are running, the installation was successful
* Use the following command to uninstall, pay attention to the execution order:
```shell script
kubectl delete crd chaosblades.chaosblade.io
helm del --purge chaosblade-operator
```
### Helm v3
* Download the latest `chaosblade-operator-VERSION-v3.tgz` package at [Release](https://github.com/chaosblade-io/chaosblade-operator/releases)
* Use `helm install chaosblade-operator chaosblade-operator-VERSION-v3.tgz --namespace chaosblade` command to install
* Use `kubectl get pod -l part-of=chaosblade -n chaosblade` to check the installation status of the Pod. If both are running, the installation was successful
* Use the following command to uninstall, pay attention to the execution order:
```shell script
kubectl delete crd chaosblades.chaosblade.io
helm uninstall chaosblade-operator -n chaosblade
```
### Kubectl
* Download the latest `chaosblade-operator-yaml-VERSION.tar.gz` package at [Release](https://github.com/chaosblade-io/chaosblade-operator/releases)
* After decompression, execute `kubectl apply -f chaosblade-operator-yaml-VERSION/` installation
* Use `kubectl get pod -l part-of=chaosblade -n chaosblade` to check the installation status of the Pod. If both are running, the installation was successful
* Use the following command to uninstall, pay attention to the execution order:
```shell script
kubectl delete crd chaosblades.chaosblade.io
kubectl delete -f chaosblade-operator-yaml-VERSION/
```

## How to use
You can run chaos experiments after installing the chaosblade operator. There are three ways to execute chaos experiments:
* By configuring yaml file, use kubectl to execute
* Executed using chaosblade cli tool
* Use Kubernetes API to execute by writing code

The following uses a specific case to illustrate the use of chaosblade-operator: simulate cn-hangzhou.192.168.0.205 node local port 40690 60% network packet loss.

### By configuring the yaml file, use kubectl to execute
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
Execute experiment：
```
kubectl apply -f loss-node-network-by-names.yaml
```
Query the experimental status, the returned information is as follows (spec and other contents are omitted):
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
From the above, you can clearly see the running status of the chaos experiment. Run the following command to stop the experiment:
```
kubectl delete -f loss-node-network-by-names.yaml
```
Or delete this blade resource directly:
```
kubectl delete blade loss-node-network-by-names
```
You can also edit the yaml file to update the content of the experiment and the chaosblade operator will complete the update of the experiment. See more examples: [Examples](https://github.com/chaosblade-io/chaosblade-operator/tree/master/examples)

### Execute with chaosblade cli tool
```
blade create k8s node-network loss --percent 60 --interface eth0 --local-port 40690 --names cn-hangzhou.192.168.0.205 --kubeconfig config
```
If the execution fails, a detailed error message is returned; if the execution is successful, the experiment UID is returned:
```
{"code":200,"success":true,"result":"e647064f5f20953c"}
```
You can query the status of the experiment with the following command:
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
Destroy experiment:
```
blade destroy e647064f5f20953c
```
In addition to the above two methods, you can also use the kubernetes client-go api for execution. For details, please refer to: [executor.go](https://github.com/chaosblade-io/chaosblade/blob/master/exec/kubernetes/executor.go) code implementation.

[Chinese documentation](https://chaosblade-io.gitbook.io/chaosblade-help-zh-cn/blade-create-k8s)

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
