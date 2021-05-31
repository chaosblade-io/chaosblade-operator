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

package channel

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
)

// Client contains the kubernetes client, operator client and kubeconfig
type Client struct {
	kubernetes.Interface
	client.Client
	Config *rest.Config
}

// NewClientFunc returns the controller client
func NewClientFunc() manager.NewClientFunc {
	return func(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error) {
		// Create the Client for Write operations.
		c, err := client.New(config, options)
		if err != nil {
			return nil, err
		}
		cli := &Client{}
		cli.Interface = kubernetes.NewForConfigOrDie(config)
		cli.Client = &client.DelegatingClient{
			Reader: &client.DelegatingReader{
				CacheReader:  cache,
				ClientReader: c,
			},
			Writer:       c,
			StatusClient: c,
		}
		cli.Config = config
		return cli, nil
	}
}

type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

type StreamOptions struct {
	IOStreams
	Stdin      bool
	TTY        bool
	OutDecoder func(bytes []byte) interface{}
	ErrDecoder func(bytes []byte) interface{}
}

type ExecOptions struct {
	StreamOptions
	PodName       string
	PodNamespace  string
	ContainerName string
	Command       []string
	IgnoreOutput  bool
}

// Exec command in pod
func (c *Client) Exec(options *ExecOptions) interface{} {
	logFields := logrus.WithFields(logrus.Fields{
		"command":      options.Command,
		"podName":      options.PodName,
		"podNamespace": options.PodNamespace,
		"container":    options.ContainerName,
	})
	logFields.Infof("Exec command in pod")
	request := c.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(options.PodName).
		Namespace(options.PodNamespace).
		SubResource("exec").
		VersionedParams(
			&corev1.PodExecOptions{
				Container: options.ContainerName,
				Command:   options.Command,
				Stdin:     options.Stdin,
				Stdout:    true,
				Stderr:    true,
				TTY:       options.TTY,
			}, scheme.ParameterCodec)
	output := bytes.NewBuffer([]byte{})
	options.Out = output
	errput := bytes.NewBuffer([]byte{})
	options.ErrOut = errput

	err := execute("POST", request.URL(), c.Config, options)
	errMsg := strings.TrimSpace(errput.String())
	outMsg := strings.TrimSpace(output.String())
	execLog := logFields.WithFields(logrus.Fields{
		"err": errMsg,
		"out": outMsg,
	})
	if errMsg != "" {
		execLog.Infof("get err message")
		return options.ErrDecoder(errput.Bytes())
	}
	if err != nil {
		execLog.WithError(err).Errorln("Invoke exec command error")
		return spec.ResponseFailWaitResult(spec.K8sExecFailed, fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec cmd", err.Error()),
			fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec cmd", err.Error()))
	}
	if outMsg != "" {
		execLog.Infof("get output message")
		return options.OutDecoder(output.Bytes())
	}
	if options.IgnoreOutput {
		return nil
	}
	return spec.ReturnFail(spec.Code[spec.K8sInvokeError],
		fmt.Sprintf("cannot get output of pods/%s/exec, maybe kubelet cannot be accessed or container not found",
			options.PodName))
}

// "172.21.1.11:8080/api/v1/namespaces/default/pods/my-nginx-3855515330-l1uqk/exec
// ?container=my-nginx&stdin=1&stdout=1&stderr=1&tty=1&command=%2Fbin%2Fbash"
func execute(method string, url *url.URL, config *rest.Config, options *ExecOptions) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}
	return exec.Stream(remotecommand.StreamOptions{
		Stdin:  options.StreamOptions.In,
		Stdout: options.StreamOptions.Out,
		Stderr: options.StreamOptions.ErrOut,
		Tty:    options.StreamOptions.TTY,
	})
}
