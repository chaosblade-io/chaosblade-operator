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
	"time"

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
	config *rest.Config
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
		cli.config = config
		return cli, nil
	}
}

// Exec command in pod
func (c *Client) Exec(pod *corev1.Pod, containerName string, command string, timeout time.Duration) *spec.Response {
	logrus.Infof("exec command in pod, command: %s, container: %s", command, containerName)
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		error := fmt.Sprintf("cannot exec into a container in a completed pod; current phase is %s", pod.Status.Phase)
		return spec.ReturnFail(spec.Code[spec.IllegalParameters], error)
	}

	const TTY = false
	request := c.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(
			&corev1.PodExecOptions{
				Container: containerName,
				Command:   []string{"/bin/sh", "-c", command},
				Stdin:     false,
				Stdout:    true,
				Stderr:    true,
				TTY:       TTY,
			}, scheme.ParameterCodec)

	var stdout, stderr bytes.Buffer

	err := execute("POST", request.URL(), c.config, &stdout, &stderr, TTY)
	if err != nil {
		logrus.Warningf("invoke exec command err, %v", err)
	}
	errMsg := strings.TrimSpace(stderr.String())
	outMsg := strings.TrimSpace(stdout.String())
	logrus.Infof("err: %s; out: %s", errMsg, outMsg)
	if errMsg != "" {
		return spec.Decode(errMsg, spec.ReturnFail(spec.Code[spec.K8sInvokeError], errMsg))
	}
	if outMsg != "" {
		return spec.Decode(outMsg, spec.ReturnFail(spec.Code[spec.K8sInvokeError], outMsg))
	}
	return spec.ReturnFail(spec.Code[spec.K8sInvokeError],
		fmt.Sprintf("cannot get output of pods/%s/exec, maybe kubelet cannot be accessed", pod.Name))
}

// "172.21.1.11:8080/api/v1/namespaces/default/pods/my-nginx-3855515330-l1uqk/exec
// ?container=my-nginx&stdin=1&stdout=1&stderr=1&tty=1&command=%2Fbin%2Fbash"
func execute(method string, url *url.URL, config *rest.Config, stdout, stderr io.Writer, tty bool) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}
	return exec.Stream(remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	})
}
