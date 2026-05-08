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
	"bytes"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/sirupsen/logrus"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
)

// execScriptInDaemonsetPod executes a shell script inside the chaosblade daemonset pod
// using nsenter to enter the host mount namespace.
func execScriptInDaemonsetPod(client *channel.Client, podName string, script string) *spec.Response {
	// Prepend exec 2>/dev/null to suppress stderr from script commands (mv, chmod, etc.)
	// This is critical because client.Exec prioritizes stderr over stdout -
	// any stderr output would cause the JSON response from stdout to be ignored.
	// Note: nsenter's own errors (e.g., command not found, permission denied) happen
	// BEFORE the script runs, so those still properly surface as errors.
	fullScript := "exec 2>/dev/null\n" + script
	cmd := []string{"nsenter", "-t", "1", "-m", "--", "sh", "-c", fullScript}
	logrus.Infof("exec in daemonset pod %s/%s, container: %s", chaosblade.DaemonsetPodNamespace, podName, chaosblade.DaemonsetPodName)
	response := client.Exec(&channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			IOStreams: channel.IOStreams{
				Out:    bytes.NewBuffer([]byte{}),
				ErrOut: bytes.NewBuffer([]byte{}),
			},
			ErrDecoder: func(bytes []byte) interface{} {
				content := string(bytes)
				return spec.Decode(content, spec.ResponseFailWithFlags(spec.K8sExecFailed, "pods/exec", content))
			},
			OutDecoder: func(bytes []byte) interface{} {
				content := string(bytes)
				return spec.Decode(content, spec.ResponseFailWithFlags(spec.K8sExecFailed, "pods/exec", content))
			},
		},
		PodName:       podName,
		PodNamespace:  chaosblade.DaemonsetPodNamespace,
		ContainerName: chaosblade.DaemonsetPodName,
		Command:       cmd,
	}).(*spec.Response)
	return response
}
