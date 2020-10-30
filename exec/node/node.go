/*
 * Copyright 1999-2020 Alibaba Group Holding Ltd.
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

package node

import (
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-exec-os/exec"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"

	osModel "github.com/chaosblade-io/chaosblade-exec-os/exec/model"
)

type ResourceModelSpec struct {
	model.BaseResourceExpModelSpec
}

func NewResourceModelSpec(client *channel.Client) model.ResourceExpModelSpec {
	modelSpec := &ResourceModelSpec{
		model.NewBaseResourceExpModelSpec("node", client),
	}
	osModelSpecs := model.NewOSSubResourceModelSpec().ExpModels()
	spec.AddExecutorToModelSpec(&model.ExecCommandInPodExecutor{Client: client}, osModelSpecs...)
	selfModelSpec := NewSelfExpModelCommandSpec()
	expModelSpecs := append(osModelSpecs, selfModelSpec)
	spec.AddFlagsToModelSpec(getResourceFlags, expModelSpecs...)
	spec.AddFlagsToModelSpec(osModel.GetSSHExpFlags, expModelSpecs...)
	modelSpec.RegisterExpModels(osModelSpecs...)
	addActionExamples(modelSpec)
	return modelSpec
}

func addActionExamples(modelSpec *ResourceModelSpec) {
	for _, expModelSpec := range modelSpec.ExpModelSpecs {

		for _, action := range expModelSpec.Actions() {
			v := interface{}(action)
			switch v.(type) {
			case *exec.FullLoadActionCommand:
				action.SetLongDesc("The CPU load experiment scenario for k8s node")
				action.SetExample(
					`# Create a CPU full load experiment in the node
blade create k8s node-cpu load --channel ssh --ssh-host 192.168.1.100 --ssh-user root

#Specifies two random kernel's full load in the node
blade create k8s node-cpu load --cpu-percent 60 --cpu-count 2 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Specifies that the kernel is full load with index 0, 3, and that the kernel's index starts at 0
blade create k8s node-cpu load --cpu-list 0,3 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Specify the kernel full load of indexes 1-3
blade create k8s node-cpu load --cpu-list 1-3 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Specified percentage load in the node
blade create k8s node-cpu load --cpu-percent 60 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.DelayActionSpec:
				action.SetExample(
					`# Access to native 8080 and 8081 ports is delayed by 3 seconds, and the delay time fluctuates by 1 second
blade create k8s node-network delay --time 3000 --offset 1000 --interface eth0 --local-port 8080,8081 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Local access to external 14.215.177.39 machine (ping www.baidu.com obtained IP) port 80 delay of 3 seconds
blade create k8s node-network delay --time 3000 --interface eth0 --remote-port 80 --destination-ip 14.215.177.39 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Do a 5 second delay for the entire network card eth0, excluding ports 22 and 8000 to 8080
blade create k8s node-network delay --time 5000 --interface eth0 --exclude-port 22,8000-8080 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.DropActionSpec:
				action.SetExample(
					`# Experimental scenario of network shielding
blade create k8s node-network drop --source-port 80 --network-traffic in --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.DnsActionSpec:
				action.SetExample(
					`# The domain name www.baidu.com is not accessible
blade create k8s node-network dns --domain www.baidu.com --ip 10.0.0.0 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.LossActionSpec:
				action.SetExample(`# Access to native 8080 and 8081 ports lost 70% of packets
blade create k8s node-network loss --percent 70 --interface eth0 --local-port 8080,8081 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# The machine accesses external 14.215.177.39 machine (ping www.baidu.com) 80 port packet loss rate 100%
blade create k8s node-network loss --percent 100 --interface eth0 --remote-port 80 --destination-ip 14.215.177.39 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Do 60% packet loss for the entire network card Eth0, excluding ports 22 and 8000 to 8080
blade create k8s node-network loss --percent 60 --interface eth0 --exclude-port 22,8000-8080 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Realize the whole network card is not accessible, not accessible time 20 seconds. After executing the following command, the current network is disconnected and restored in 20 seconds. Remember!! Don't forget -timeout parameter
blade create k8s node-network loss --percent 100 --interface eth0 --timeout 20 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.DuplicateActionSpec:
				action.SetExample(`# Specify the network card eth0 and repeat the packet by 10%
blade create k8s node-network duplicate --percent=10 --interface=eth0 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.CorruptActionSpec:
				action.SetExample(`# Access to the specified IP request packet is corrupted, 80% of the time
blade create k8s node-network corrupt --percent 80 --destination-ip 180.101.49.12 --interface eth0 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.ReorderActionSpec:
				action.SetExample(`# Access the specified IP request packet disorder
blade create k8s node-network reorder --correlation 80 --percent 50 --gap 2 --time 500 --interface eth0 --destination-ip 180.101.49.12 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.OccupyActionSpec:
				action.SetExample(`#Specify port 8080 occupancy
blade create k8s node-network occupy --port 8080 --force --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# The machine accesses external 14.215.177.39 machine (ping www.baidu.com) 80 port packet loss rate 100%
blade create k8s node-network loss --percent 100 --interface eth0 --remote-port 80 --destination-ip 14.215.177.39 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.KillProcessActionCommandSpec:
				action.SetLongDesc("The process scenario in container is the same as the basic resource process scenario")
				action.SetExample(
					`
# Kill the nginx process in the node
blade create k8s node-process kill --process nginx --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Use blade CLI
# Specifies the signal and local port to kill the process in the node
blade create k8s node-process kill --local-port 8080 --signal 15 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)

			case *exec.StopProcessActionCommandSpec:
				action.SetLongDesc("The process scenario in container is the same as the basic resource process scenario")
				action.SetExample(
					`
# Pause the process that contains the "nginx" keyword in the node
blade create k8s node-process stop --process nginx --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Pause the Java process in the node
blade create k8s node-process stop --process-cmd java --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.FillActionSpec:
				action.SetLongDesc("The disk fill scenario experiment in the node")
				action.SetExample(
					`
# Fill the /home directory with 40G of disk space in the node
blade create k8s node-disk fill --path /home --size 40000 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Fill the /home directory with 80% of the disk space in the node and retains the file handle that populates the disk
blade create k8s node-disk fill --path /home --percent 80 --retain-handle --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Perform a fixed-size experimental scenario in the node
blade c k8s node-disk fill --path /home --reserve 1024 --channel ssh --ssh-host 192.168.1.100 --ssh-user root
`)
			case *exec.BurnActionSpec:
				action.SetLongDesc("Disk read and write IO load experiment in the node")
				action.SetExample(
					`# The data of rkB/s, wkB/s and % Util were mainly observed. Perform disk read IO high-load scenarios
blade create k8s node-disk burn --read --path /home --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# Perform disk write IO high-load scenarios
blade create k8s node-disk burn --write --path /home --channel ssh --ssh-host 192.168.1.100 --ssh-user root8

# Read and write IO load scenarios are performed at the same time. Path is not specified. The default is
blade create k8s node-disk burn --read --write --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.MemLoadActionCommand:
				action.SetLongDesc("The memory fill experiment scenario in container")
				action.SetExample(
					`# The execution memory footprint is 50%
blade create k8s node-mem load --mode ram --mem-percent 50 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# The execution memory footprint is 50%, cache model
blade create k8s node-mem load --mode cache --mem-percent 50 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# The execution memory footprint is 50%, usage contains buffer/cache
blade create k8s node-mem load --mode ram --mem-percent 50 --include-buffer-cache --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# The execution memory footprint is 50% for 200 seconds
blade create k8s node-mem load --mode ram --mem-percent 50 --timeout 200 --channel ssh --ssh-host 192.168.1.100 --ssh-user root

# 200M memory is reserved
blade create k8s node-mem load --mode ram --reserve 200 --rate 100 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.ScriptDelayActionCommand:
				action.SetExample(`
# Add commands to the script "start0() { sleep 10.000000 ...}"
blade create k8s node-script delay --time 10000 --file test.sh --function-name start0 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			case *exec.ScriptExitActionCommand:
				action.SetExample(`
# Add commands to the script "start0() { echo this-is-error-message; exit 1; ... }"
blade create k8s node-script exit --exit-code 1 --exit-message this-is-error-message --file test.sh --function-name start0 --channel ssh --ssh-host 192.168.1.100 --ssh-user root`)
			default:
				action.SetExample(strings.Replace(action.Example(),
					fmt.Sprintf("blade create %s %s", expModelSpec.Name(), action.Name()),
					fmt.Sprintf("blade create k8s node-%s %s --names nginx-app --channel ssh --ssh-host 192.168.1.100 --ssh-user root", expModelSpec.Name(), action.Name()),
					-1,
				))
				action.SetExample(strings.Replace(action.Example(),
					fmt.Sprintf("blade c %s %s", expModelSpec.Name(), action.Name()),
					fmt.Sprintf("blade c k8s node-%s %s --names nginx-app --channel ssh --ssh-host 192.168.1.100 --ssh-user root", expModelSpec.Name(), action.Name()),
					-1,
				))
			}
		}
	}
}

func getResourceFlags() []spec.ExpFlagSpec {
	coverageFlags := model.GetResourceCoverageFlags()
	return append(coverageFlags, model.ResourceNamesFlag, model.ResourceLabelsFlag)
}

func NewSelfExpModelCommandSpec() spec.ExpModelCommandSpec {
	return &SelfExpModelCommandSpec{
		spec.BaseExpModelCommandSpec{
			ExpFlags: []spec.ExpFlagSpec{},
			ExpActions: []spec.ExpActionCommandSpec{
				// TODO
				//NewCordonActionCommandSpec(),
			},
		},
	}
}

type SelfExpModelCommandSpec struct {
	spec.BaseExpModelCommandSpec
}

func (*SelfExpModelCommandSpec) Name() string {
	return "node"
}

func (*SelfExpModelCommandSpec) ShortDesc() string {
	return "Node resource experiment for itself, for example cpu load"
}

func (*SelfExpModelCommandSpec) LongDesc() string {
	return "Node resource experiment for itself, for example cpu load"
}

func (*SelfExpModelCommandSpec) Example() string {
	return "blade c k8s node-cpu load --evict-count 1 --kubeconfig ~/.kube/config --names cn-hangzhou.192.168.0.205"
}
