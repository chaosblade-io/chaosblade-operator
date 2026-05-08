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
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	CniBinPathFlag  = "cni-bin-path"
	CniErrorMsgFlag = "error-msg"
)

func NewCniExpModelCommandSpec(client *channel.Client) spec.ExpModelCommandSpec {
	return &CniExpModelCommandSpec{
		spec.BaseExpModelCommandSpec{
			ExpActions: []spec.ExpActionCommandSpec{
				NewCniAddFaultActionSpec(client),
				NewCniDelFaultActionSpec(client),
			},
			ExpFlags: []spec.ExpFlagSpec{},
		},
	}
}

type CniExpModelCommandSpec struct {
	spec.BaseExpModelCommandSpec
}

func (*CniExpModelCommandSpec) Name() string {
	return "cni"
}

func (*CniExpModelCommandSpec) ShortDesc() string {
	return "CNI fault experiment"
}

func (*CniExpModelCommandSpec) LongDesc() string {
	return "CNI fault experiment, simulate CNI plugin failures on the node"
}

func (*CniExpModelCommandSpec) Example() string {
	return `# Auto-discover CNI binary
blade create k8s node-cni add_fault --names cn-hangzhou.192.168.0.205 --kubeconfig ~/.kube/config
# Or specify explicitly
blade create k8s node-cni add_fault --cni-bin-path /opt/cni/bin/calico --names cn-hangzhou.192.168.0.205 --kubeconfig ~/.kube/config`
}

// CniAddFaultActionSpec

func NewCniAddFaultActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &CniAddFaultActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: CniBinPathFlag,
					Desc: "The full path of the CNI plugin binary. If not specified, auto-discovered from kubelet CNI config",
				},
			},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: CniErrorMsgFlag,
					Desc: "Custom error message returned by the CNI plugin failure",
				},
			},
			ActionExecutor:   &CniFaultExecutor{client: client, cniCommand: "ADD"},
			ActionCategories: []string{model.CategorySystemContainer},
			ActionExample: `# Simulate CNI ADD failure with auto-discovered CNI binary
blade create k8s node-cni add_fault --names cn-hangzhou.192.168.0.205 --kubeconfig ~/.kube/config
# Or specify the CNI binary path explicitly
blade create k8s node-cni add_fault --cni-bin-path /opt/cni/bin/calico --names cn-hangzhou.192.168.0.205 --kubeconfig ~/.kube/config
# With custom error message
blade create k8s node-cni add_fault --cni-bin-path /opt/cni/bin/calico --error-msg "network unavailable" --names cn-hangzhou.192.168.0.205 --kubeconfig ~/.kube/config`,
		},
	}
}

type CniAddFaultActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func (*CniAddFaultActionSpec) Name() string {
	return "add_fault"
}

func (*CniAddFaultActionSpec) Aliases() []string {
	return []string{}
}

func (*CniAddFaultActionSpec) ShortDesc() string {
	return "Simulate CNI ADD failure"
}

func (*CniAddFaultActionSpec) LongDesc() string {
	return "Simulate CNI ADD failure, new pods will be stuck in ContainerCreating"
}

// CniDelFaultActionSpec

func NewCniDelFaultActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &CniDelFaultActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: CniBinPathFlag,
					Desc: "The full path of the CNI plugin binary. If not specified, auto-discovered from kubelet CNI config",
				},
			},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: CniErrorMsgFlag,
					Desc: "Custom error message returned by the CNI plugin failure",
				},
			},
			ActionExecutor:   &CniFaultExecutor{client: client, cniCommand: "DEL"},
			ActionCategories: []string{model.CategorySystemContainer},
			ActionExample: `# Simulate CNI DEL failure with auto-discovered CNI binary
blade create k8s node-cni del_fault --names cn-hangzhou.192.168.0.205 --kubeconfig ~/.kube/config
# Or specify the CNI binary path explicitly
blade create k8s node-cni del_fault --cni-bin-path /opt/cni/bin/calico --names cn-hangzhou.192.168.0.205 --kubeconfig ~/.kube/config`,
		},
	}
}

type CniDelFaultActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func (*CniDelFaultActionSpec) Name() string {
	return "del_fault"
}

func (*CniDelFaultActionSpec) Aliases() []string {
	return []string{}
}

func (*CniDelFaultActionSpec) ShortDesc() string {
	return "Simulate CNI DEL failure"
}

func (*CniDelFaultActionSpec) LongDesc() string {
	return "Simulate CNI DEL failure, terminating pods will be stuck"
}

// CniFaultExecutor

type CniFaultExecutor struct {
	client     *channel.Client
	cniCommand string // "ADD" or "DEL"
}

func (e *CniFaultExecutor) Name() string {
	return "cni_fault"
}

func (e *CniFaultExecutor) SetChannel(channel spec.Channel) {
}

func (e *CniFaultExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return e.destroy(uid, ctx, expModel)
	}
	return e.create(uid, ctx, expModel)
}

func (e *CniFaultExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx))
	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(err.Error(), []v1alpha1.ResourceStatus{}))
	}

	cniBinPath := expModel.ActionFlags[CniBinPathFlag]

	errorMsg := expModel.ActionFlags[CniErrorMsgFlag]
	if errorMsg == "" {
		errorMsg = fmt.Sprintf("chaosblade: simulated CNI %s failure", e.cniCommand)
	}

	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := true
	updateLock := &sync.Mutex{}

	execFunc := func(i int) {
		meta := containerObjectMetaList[i]
		status := v1alpha1.ResourceStatus{
			Kind:       "node",
			Identifier: meta.GetIdentifier(),
			Id:         uid,
		}

		daemonsetPodName, err := model.GetChaosBladeDaemonsetPodName(meta.NodeName, e.client)
		if err != nil {
			logrusField.Errorf("get chaosblade daemonset pod on node %s failed: %v", meta.NodeName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
			updateLock.Lock()
			statuses = append(statuses, status)
			success = false
			updateLock.Unlock()
			return
		}
		if daemonsetPodName == "" {
			errMsg := fmt.Sprintf("chaosblade daemonset pod not found on node %s", meta.NodeName)
			logrusField.Error(errMsg)
			status = status.CreateFailResourceStatus(errMsg, spec.K8sExecFailed.Code)
			updateLock.Lock()
			statuses = append(statuses, status)
			success = false
			updateLock.Unlock()
			return
		}

		resolvedPath := cniBinPath
		if resolvedPath == "" {
			discovered, discoverErr := discoverCniBinPath(e.client, daemonsetPodName)
			if discoverErr != nil {
				errMsg := fmt.Sprintf("auto-discover CNI binary on node %s failed: %v", meta.NodeName, discoverErr)
				logrusField.Error(errMsg)
				status = status.CreateFailResourceStatus(errMsg, spec.K8sExecFailed.Code)
				updateLock.Lock()
				statuses = append(statuses, status)
				success = false
				updateLock.Unlock()
				return
			}
			logrusField.Infof("auto-discovered CNI binary: %s on node %s", discovered, meta.NodeName)
			resolvedPath = discovered
		}

		script := generateCniCreateScript(resolvedPath, e.cniCommand, errorMsg)
		resp := execScriptInDaemonsetPod(e.client, daemonsetPodName, script)
		if resp.Success {
			status = status.CreateSuccessResourceStatus()
		} else {
			status = status.CreateFailResourceStatus(resp.Err, spec.K8sExecFailed.Code)
			success = false
		}
		updateLock.Lock()
		statuses = append(statuses, status)
		updateLock.Unlock()
	}

	model.ParallelizeExec(len(containerObjectMetaList), execFunc)
	logrusField.Infof("cni %s fault create result, success: %t, statuses: %+v", e.cniCommand, success, statuses)

	if success {
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateSuccessExperimentStatus(statuses))
	}
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses))
}

func (e *CniFaultExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx))
	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(err.Error(), []v1alpha1.ResourceStatus{}))
	}

	cniBinPath := expModel.ActionFlags[CniBinPathFlag]

	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := true
	updateLock := &sync.Mutex{}

	execFunc := func(i int) {
		meta := containerObjectMetaList[i]
		status := v1alpha1.ResourceStatus{
			Kind:       "node",
			Identifier: meta.GetIdentifier(),
			Id:         meta.Id,
			State:      v1alpha1.DestroyedState,
		}

		daemonsetPodName, err := model.GetChaosBladeDaemonsetPodName(meta.NodeName, e.client)
		if err != nil {
			logrusField.Errorf("get chaosblade daemonset pod on node %s failed: %v", meta.NodeName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
			updateLock.Lock()
			statuses = append(statuses, status)
			success = false
			updateLock.Unlock()
			return
		}
		if daemonsetPodName == "" {
			errMsg := fmt.Sprintf("chaosblade daemonset pod not found on node %s", meta.NodeName)
			logrusField.Error(errMsg)
			status = status.CreateFailResourceStatus(errMsg, spec.K8sExecFailed.Code)
			updateLock.Lock()
			statuses = append(statuses, status)
			success = false
			updateLock.Unlock()
			return
		}

		resolvedPath := cniBinPath
		if resolvedPath == "" {
			discovered, discoverErr := discoverCniBinPath(e.client, daemonsetPodName)
			if discoverErr != nil {
				errMsg := fmt.Sprintf("auto-discover CNI binary on node %s failed: %v", meta.NodeName, discoverErr)
				logrusField.Error(errMsg)
				status = status.CreateFailResourceStatus(errMsg, spec.K8sExecFailed.Code)
				updateLock.Lock()
				statuses = append(statuses, status)
				success = false
				updateLock.Unlock()
				return
			}
			logrusField.Infof("auto-discovered CNI binary: %s on node %s", discovered, meta.NodeName)
			resolvedPath = discovered
		}

		script := generateCniDestroyScript(resolvedPath)
		resp := execScriptInDaemonsetPod(e.client, daemonsetPodName, script)
		if resp.Success {
			status.Success = true
		} else {
			status = status.CreateFailResourceStatus(resp.Err, spec.K8sExecFailed.Code)
			success = false
		}
		updateLock.Lock()
		statuses = append(statuses, status)
		updateLock.Unlock()
	}

	model.ParallelizeExec(len(containerObjectMetaList), execFunc)
	logrusField.Infof("cni fault destroy result, success: %t, statuses: %+v", success, statuses)

	if success {
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus(statuses))
	}
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses))
}

func generateCniCreateScript(cniBinPath string, cniCommand string, errorMsg string) string {
	backupPath := cniBinPath + ".chaosblade.bak"
	// The wrapper script: fail for targeted CNI_COMMAND, passthrough for all others
	// Per CNI spec: error result MUST be written to stdout (not stderr), and exit non-zero
	// We read stdin to extract cniVersion from network config for spec compliance
	wrapperContent := fmt.Sprintf(`#!/bin/sh
if [ "$CNI_COMMAND" = "%s" ]; then
  cni_input=$(cat)
  cni_ver=$(echo "$cni_input" | grep -o '"cniVersion" *: *"[^"]*"' | head -1 | grep -o '[0-9][0-9.]*')
  [ -z "$cni_ver" ] && cni_ver="0.3.1"
  echo "{\"cniVersion\":\"$cni_ver\",\"code\":100,\"msg\":\"%s\"}"
  exit 1
fi
exec %s "$@"
`, cniCommand, errorMsg, backupPath)

	// Use base64 encoding to safely transport the wrapper content through shell
	// This avoids all complex single-quote escaping issues
	b64Content := base64.StdEncoding.EncodeToString([]byte(wrapperContent))

	script := fmt.Sprintf(`BIN_PATH='%s'
BACKUP_PATH='%s'
B64_CONTENT='%s'
if [ ! -f "$BIN_PATH" ]; then
  echo '{"code":404,"success":false,"error":"CNI binary not found: '"$BIN_PATH"'"}'
  exit 0
fi
if [ -f "$BACKUP_PATH" ]; then
  echo '{"code":409,"success":false,"error":"CNI fault already injected, backup exists: '"$BACKUP_PATH"'"}'
  exit 0
fi
mv "$BIN_PATH" "$BACKUP_PATH" 2>/dev/null
if [ $? -ne 0 ]; then
  echo '{"code":500,"success":false,"error":"failed to backup CNI binary, permission denied or read-only filesystem"}'
  exit 0
fi
echo "$B64_CONTENT" | base64 -d > "$BIN_PATH"
if [ $? -ne 0 ]; then
  mv "$BACKUP_PATH" "$BIN_PATH" 2>/dev/null
  echo '{"code":500,"success":false,"error":"failed to write wrapper script, rolling back"}'
  exit 0
fi
chmod +x "$BIN_PATH"
echo '{"code":200,"success":true}'
`, cniBinPath, backupPath, b64Content)

	return script
}

func generateCniDiscoverScript() string {
	return `# Find kubelet PID
KUBELET_PID=$(pgrep -x kubelet 2>/dev/null)
if [ -z "$KUBELET_PID" ]; then
  for p in /proc/[0-9]*/comm; do
    if [ -f "$p" ] && grep -qx kubelet "$p" 2>/dev/null; then
      KUBELET_PID=$(echo "$p" | cut -d/ -f3)
      break
    fi
  done
fi
if [ -z "$KUBELET_PID" ]; then
  echo '{"code":500,"success":false,"error":"kubelet process not found"}'
  exit 0
fi

# Parse kubelet cmdline
CMDLINE=$(cat /proc/$KUBELET_PID/cmdline 2>/dev/null | tr '\0' ' ')
if [ -z "$CMDLINE" ]; then
  echo '{"code":500,"success":false,"error":"failed to read kubelet cmdline"}'
  exit 0
fi

# Extract --cni-bin-dir (default /opt/cni/bin)
CNI_BIN_DIR=$(echo "$CMDLINE" | grep -o '\-\-cni-bin-dir=[^ ]*' | head -1 | cut -d= -f2)
[ -z "$CNI_BIN_DIR" ] && CNI_BIN_DIR="/opt/cni/bin"

# Extract --cni-conf-dir (default /etc/cni/net.d)
CNI_CONF_DIR=$(echo "$CMDLINE" | grep -o '\-\-cni-conf-dir=[^ ]*' | head -1 | cut -d= -f2)
[ -z "$CNI_CONF_DIR" ] && CNI_CONF_DIR="/etc/cni/net.d"

# Find first conflist or conf file (alphabetically, same as kubelet)
CONF_FILE=$(ls -1 "$CNI_CONF_DIR"/*.conflist 2>/dev/null | sort | head -1)
if [ -z "$CONF_FILE" ]; then
  CONF_FILE=$(ls -1 "$CNI_CONF_DIR"/*.conf 2>/dev/null | sort | head -1)
fi
if [ -z "$CONF_FILE" ]; then
  echo '{"code":500,"success":false,"error":"no CNI config files found in '"$CNI_CONF_DIR"'"}'
  exit 0
fi

# Extract "type" field from config
CNI_TYPE=$(grep -o '"type" *: *"[^"]*"' "$CONF_FILE" | head -1 | grep -o '"[^"]*"$' | tr -d '"')
if [ -z "$CNI_TYPE" ]; then
  echo '{"code":500,"success":false,"error":"cannot extract CNI type from '"$CONF_FILE"'"}'
  exit 0
fi

# Compose full binary path and verify
CNI_BIN_PATH="${CNI_BIN_DIR}/${CNI_TYPE}"
if [ ! -f "$CNI_BIN_PATH" ]; then
  echo '{"code":500,"success":false,"error":"CNI binary not found at '"$CNI_BIN_PATH"'"}'
  exit 0
fi

echo '{"code":200,"success":true,"result":"'"$CNI_BIN_PATH"'"}'
`
}

func discoverCniBinPath(client *channel.Client, daemonsetPodName string) (string, error) {
	script := generateCniDiscoverScript()
	resp := execScriptInDaemonsetPod(client, daemonsetPodName, script)
	if !resp.Success {
		return "", fmt.Errorf("%s", resp.Err)
	}
	path, ok := resp.Result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected discovery result type: %T", resp.Result)
	}
	return path, nil
}

func generateCniDestroyScript(cniBinPath string) string {
	backupPath := cniBinPath + ".chaosblade.bak"
	script := fmt.Sprintf(`BIN_PATH='%s'
BACKUP_PATH='%s'
if [ ! -f "$BACKUP_PATH" ]; then
  echo '{"code":200,"success":true}'
  exit 0
fi
rm -f "$BIN_PATH" 2>/dev/null
mv "$BACKUP_PATH" "$BIN_PATH" 2>/dev/null
if [ $? -ne 0 ]; then
  echo '{"code":500,"success":false,"error":"failed to restore CNI binary"}'
  exit 0
fi
echo '{"code":200,"success":true}'
`, cniBinPath, backupPath)

	return script
}
