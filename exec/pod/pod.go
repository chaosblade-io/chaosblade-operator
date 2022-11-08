/*
 * Copyright 1999-2019 Alibaba Group Holding Ltd.
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

package pod

import (
	"fmt"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/cpu"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/disk"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/file"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/mem"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/network"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/network/tc"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/script"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"strings"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
)

type ResourceModelSpec struct {
	model.BaseResourceExpModelSpec
}

func NewResourceModelSpec(client *channel.Client) model.ResourceExpModelSpec {
	modelSpec := &ResourceModelSpec{
		model.NewBaseResourceExpModelSpec("pod", client),
	}
	// os experiment models
	osExpModels := model.NewOSSubResourceModelSpec().ExpModels()
	spec.AddExecutorToModelSpec(&model.CommonExecutor{Client: client}, osExpModels...)
	// pod-self experiment models
	expModels := append(osExpModels, NewSelfExpModelCommandSpec(client))

	spec.AddFlagsToModelSpec(getResourceFlags, expModels...)
	modelSpec.RegisterExpModels(expModels...)
	addActionExamples(modelSpec)
	return modelSpec
}

func addActionExamples(modelSpec *ResourceModelSpec) {
	for _, expModelSpec := range modelSpec.ExpModelSpecs {
		for _, action := range expModelSpec.Actions() {
			v := interface{}(action)
			switch v.(type) {
			case *disk.FillActionSpec:
				action.SetLongDesc("The disk fill scenario experiment in the pod")
				action.SetExample(
					`
# Fill the /home directory with 40G of disk space in the pod
blade create k8s pod-disk fill --path /home --size 40000 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Fill the /home directory with 80% of the disk space in the pod and retains the file handle that populates the disk
blade create k8s pod-disk fill --path /home --percent 80 --retain-handle --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Perform a fixed-size experimental scenario in the pod
blade c k8s pod-disk fill --path /home --reserve 1024 --names nginx-app --kubeconfig ~/.kube/config --namespace default
`)
			case *disk.BurnActionSpec:
				action.SetLongDesc("Disk read and write IO load experiment in the pod")
				action.SetExample(
					`# The data of rkB/s, wkB/s and % Util were mainly observed. Perform disk read IO high-load scenarios
blade create k8s pod-disk burn --read --path /home --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Perform disk write IO high-load scenarios
blade create k8s pod-disk burn --write --path /home --names nginx-app --kubeconfig ~/.kube/config --namespace default8

# Read and write IO load scenarios are performed at the same time. Path is not specified. The default is /
blade create k8s pod-disk burn --read --write --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *mem.MemLoadActionCommand:
				action.SetLongDesc("The memory fill experiment scenario in the pod")
				action.SetExample(
					`# The execution memory footprint is 50%
blade create k8s pod-mem load --mode ram --mem-percent 50 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# The execution memory footprint is 50%, cache model
blade create k8s pod-mem load --mode cache --mem-percent 50 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# The execution memory footprint is 50%, usage contains buffer/cache
blade create k8s pod-mem load --mode ram --mem-percent 50 --include-buffer-cache --names nginx-app --kubeconfig ~/.kube/config --namespace default

# The execution memory footprint is 50% for 200 seconds
blade create k8s pod-mem load --mode ram --mem-percent 50 --timeout 200 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# 200M memory is reserved
blade create k8s pod-mem load --mode ram --reserve 200 --rate 100 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *file.FileAppendActionSpec:
				action.SetLongDesc("The file append experiment scenario in the pod")
				action.SetExample(
					`# Appends the content "HELLO WORLD" to the /home/logs/nginx.log file
blade create k8s pod-file append --filepath=/home/logs/nginx.log --content="HELL WORLD" --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Appends the content "HELLO WORLD" to the /home/logs/nginx.log file, interval 10 seconds
blade create k8s pod-file append --filepath=/home/logs/nginx.log --content="HELL WORLD" --interval 10 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Appends the content "HELLO WORLD" to the /home/logs/nginx.log file, enable base64 encoding
blade create k8s pod-file append --filepath=/home/logs/nginx.log --content=SEVMTE8gV09STEQ= --names nginx-app --kubeconfig ~/.kube/config --namespace default

# mock interface timeout exception
blade create k8s pod-file append --filepath=/home/logs/nginx.log --content="@{DATE:+%Y-%m-%d %H:%M:%S} ERROR invoke getUser timeout [@{RANDOM:100-200}]ms abc  mock exception" --names nginx-app --kubeconfig ~/.kube/config --namespace default
`)
			case *file.FileAddActionSpec:
				action.SetLongDesc("The file add experiment scenario in the pod")
				action.SetExample(
					`# Create a file named nginx.log in the /home directory
blade create k8s pod-file add --filepath /home/nginx.log --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Create a file named nginx.log in the /home directory with the contents of HELLO WORLD
blade create k8s pod-file add --filepath /home/nginx.log --content "HELLO WORLD" --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Create a file named nginx.log in the /temp directory and automatically create directories that don't exist
blade create k8s pod-file add --filepath /temp/nginx.log --auto-create-dir --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Create a directory named /nginx in the /temp directory and automatically create directories that don't exist
blade create k8s pod-file add --directory --filepath /temp/nginx --auto-create-dir --names nginx-app --kubeconfig ~/.kube/config --namespace default
`)

			case *file.FileChmodActionSpec:
				action.SetLongDesc("The file permission modification scenario in the pod")
				action.SetExample(`# Modify /home/logs/nginx.log file permissions to 777
blade create k8s pod-file chmod --filepath /home/logs/nginx.log --mark=777 --names nginx-app --kubeconfig ~/.kube/config --namespace default
`)
			case *file.FileDeleteActionSpec:
				action.SetLongDesc("The file delete scenario in the pod")
				action.SetExample(
					`# Delete the file /home/logs/nginx.log
blade create k8s pod-file delete --filepath /home/logs/nginx.log --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Force delete the file /home/logs/nginx.log unrecoverable
blade create k8s pod-file delete --filepath /home/logs/nginx.log --force --names nginx-app --kubeconfig ~/.kube/config --namespace default
`)
			case *file.FileMoveActionSpec:
				action.SetExample("The file move scenario in the pod")
				action.SetExample(`# Move the file /home/logs/nginx.log to /tmp
blade create k8s pod-file move --filepath /home/logs/nginx.log --target /tmp --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Force Move the file /home/logs/nginx.log to /temp
blade create k8s pod-file move --filepath /home/logs/nginx.log --target /tmp --force --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Move the file /home/logs/nginx.log to /temp/ and automatically create directories that don't exist
blade create k8s pod-file move --filepath /home/logs/nginx.log --target /temp --auto-create-dir --names nginx-app --kubeconfig ~/.kube/config --namespace default
`)
			case *tc.DelayActionSpec:
				action.SetExample(
					`# Access to native 8080 and 8081 ports is delayed by 3 seconds, and the delay time fluctuates by 1 second
blade create k8s pod-network delay --time 3000 --offset 1000 --interface eth0 --local-port 8080,8081 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Local access to external 14.215.177.39 machine (ping www.baidu.com obtained IP) port 80 delay of 3 seconds
blade create k8s pod-network delay --time 3000 --interface eth0 --remote-port 80 --destination-ip 14.215.177.39 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Do a 5 second delay for the entire network card eth0, excluding ports 22 and 8000 to 8080
blade create k8s pod-network delay --time 5000 --interface eth0 --exclude-port 22,8000-8080 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *network.DropActionSpec:
				action.SetExample(
					`# Experimental scenario of network shielding
blade create k8s pod-network drop --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *network.DnsActionSpec:
				action.SetExample(
					`# The domain name www.baidu.com is not accessible
blade create k8s pod-network dns --domain www.baidu.com --ip 10.0.0.0 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *tc.LossActionSpec:
				action.SetExample(`# Access to native 8080 and 8081 ports lost 70% of packets
blade create k8s pod-network loss --percent 70 --interface eth0 --local-port 8080,8081 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# The machine accesses external 14.215.177.39 machine (ping www.baidu.com) 80 port packet loss rate 100%
blade create k8s pod-network loss --percent 100 --interface eth0 --remote-port 80 --destination-ip 14.215.177.39 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Do 60% packet loss for the entire network card Eth0, excluding ports 22 and 8000 to 8080
blade create k8s pod-network loss --percent 60 --interface eth0 --exclude-port 22,8000-8080 --names nginx-app --kubeconfig ~/.kube/config --namespace default

# Realize the whole network card is not accessible, not accessible time 20 seconds. After executing the following command, the current network is disconnected and restored in 20 seconds. Remember!! Don't forget -timeout parameter
blade create k8s pod-network loss --percent 100 --interface eth0 --timeout 20 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *tc.DuplicateActionSpec:
				action.SetExample(`# Specify the network card eth0 and repeat the packet by 10%
blade create k8s pod-network duplicate --percent=10 --interface=eth0 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *tc.CorruptActionSpec:
				action.SetExample(`# Access to the specified IP request packet is corrupted, 80% of the time
blade create k8s pod-network corrupt --percent 80 --destination-ip 180.101.49.12 --interface eth0 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *tc.ReorderActionSpec:
				action.SetExample(`# Access the specified IP request packet disorder
blade create k8s pod-network reorder --correlation 80 --percent 50 --gap 2 --time 500 --interface eth0 --destination-ip 180.101.49.12 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *network.OccupyActionSpec:
				action.SetExample(`#Specify port 8080 occupancy
blade create k8s pod-network occupy --port 8080 --force --names nginx-app --kubeconfig ~/.kube/config --namespace default

# The machine accesses external 14.215.177.39 machine (ping www.baidu.com) 80 port packet loss rate 100%
blade create k8s pod-network loss --percent 100 --interface eth0 --remote-port 80 --destination-ip 14.215.177.39 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *cpu.FullLoadActionCommand:
				action.SetExample(`
# Create a CPU full load experiment
blade create k8s pod-cpu load --names nginx-app --kubeconfig ~/.kube/config --namespace default

#Specifies two random kernel's full load
blade create k8s pod-cpu load --names nginx-app --kubeconfig ~/.kube/config --namespace default --cpu-percent 60 --cpu-count 2

# Specifies that the kernel is full load with index 0, 3, and that the kernel's index starts at 0
blade create k8s pod-cpu load --names nginx-app --kubeconfig ~/.kube/config --namespace default --cpu-list 0,3

# Specify the kernel full load of indexes 1-3
blade create k8s pod-cpu load --names nginx-app --kubeconfig ~/.kube/config --namespace default --cpu-list 1-3

# Specified percentage load
blade create k8s pod-cpu load --names nginx-app --kubeconfig ~/.kube/config --namespace default --cpu-percent 60`)
			case *script.ScriptDelayActionCommand:
				action.SetExample(`
# Add commands to the script "start0() { sleep 10.000000 ...}"
blade create k8s pod-script delay --time 10000 --file test.sh --function-name start0 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			case *script.ScriptExitActionCommand:
				action.SetExample(`
# Add commands to the script "start0() { echo this-is-error-message; exit 1; ... }"
blade create k8s pod-script exit --exit-code 1 --exit-message this-is-error-message --file test.sh --function-name start0 --names nginx-app --kubeconfig ~/.kube/config --namespace default`)
			default:
				action.SetExample(strings.Replace(action.Example(),
					fmt.Sprintf("blade create %s %s", expModelSpec.Name(), action.Name()),
					fmt.Sprintf("blade create k8s pod-%s %s --names nginx-app --kubeconfig ~/.kube/config --namespace default", expModelSpec.Name(), action.Name()),
					-1,
				))
				action.SetExample(strings.Replace(action.Example(),
					fmt.Sprintf("blade c %s %s", expModelSpec.Name(), action.Name()),
					fmt.Sprintf("blade c k8s pod-%s %s --names nginx-app --kubeconfig ~/.kube/config --namespace default", expModelSpec.Name(), action.Name()),
					-1,
				))
			}

		}
	}
}

func getResourceFlags() []spec.ExpFlagSpec {
	coverageFlags := model.GetResourceCoverageFlags()
	commonFlags := model.GetResourceCommonFlags()
	chaosbladeFlags := model.GetChaosBladeFlags()
	return append(append(coverageFlags, commonFlags...), chaosbladeFlags...)
}

type SelfExpModelCommandSpec struct {
	spec.BaseExpModelCommandSpec
}

func NewSelfExpModelCommandSpec(client *channel.Client) spec.ExpModelCommandSpec {
	return &SelfExpModelCommandSpec{
		spec.BaseExpModelCommandSpec{
			ExpFlags: []spec.ExpFlagSpec{},
			ExpActions: []spec.ExpActionCommandSpec{
				NewDeletePodActionSpec(client),
				NewPodIOActionSpec(client),
				NewFailPodActionSpec(client),
			},
		},
	}
}

func (*SelfExpModelCommandSpec) Name() string {
	return "pod"
}

func (*SelfExpModelCommandSpec) ShortDesc() string {
	return "Pod experiments"
}

func (*SelfExpModelCommandSpec) LongDesc() string {
	return "Pod experiments"
}

func (*SelfExpModelCommandSpec) Example() string {
	return "blade c k8s pod-pod delete --names redis-slave-674d68586-n5s4q --namespace default --kubeconfig ~/.kube/config"
}
