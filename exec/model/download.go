/*
 * Copyright 2025 The ChaosBlade Authors
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

package model

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
)

/*
.
├── bin
├── blade
├── lib.tar.gz
└── yaml.tar.gz
*/

const (
	bin   = "bin"
	blade = "blade"
	lib   = "lib.tar.gz"
	yaml  = "yaml.tar.gz"
)

type DownloadOptions struct {
	DeployOptions
	url string
}

func (d *DownloadOptions) DeployToPod(experimentId, src, dest string) error {
	if len(src) == 0 {
		return errors.New("the chaosblade downloaded address is empty")
	}
	url := d.getUrl(src)
	// code=$( curl -s -L -w %{http_code} -o /opt/yaml.tar.gz https://xxx/temp/yaml.tar.gz ) && [ $code = 200 ] && tar -zxf /opt/yaml.tar.gz -C /opt && echo $code || echo $code
	var command []string
	isTarFile := strings.HasSuffix(url, "tar.gz")
	if isTarFile {
		dest = fmt.Sprintf("%s.%s", dest, "tar.gz")
	}
	command = []string{"sh", "-c", "curl -s -L -w %{http_code} " + fmt.Sprintf("-o %s %s && chmod 755 %s", dest, url, dest)}
	options := &channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			ErrDecoder: func(bytes []byte) interface{} {
				return string(bytes)
			},
			OutDecoder: func(bytes []byte) interface{} {
				return string(bytes)
			},
		},
		PodNamespace:  d.Namespace,
		PodName:       d.PodName,
		ContainerName: d.Container,
		Command:       command,
		IgnoreOutput:  false,
	}
	statusCode := d.client.Exec(options).(string)
	logrus.WithFields(
		logrus.Fields{
			"experimentId": experimentId,
			"pod":          d.PodName,
			"container":    d.Container,
			"command":      command,
			"result":       statusCode,
		}).Infof("download to the target container")
	code, err := strconv.Atoi(strings.TrimSpace(statusCode))
	if err != nil {
		return errors.New(statusCode)
	}
	if code != 200 {
		return fmt.Errorf("response code is %d", code)
	}
	if isTarFile {
		return d.uncompress(experimentId, dest)
	}
	return nil
}

func (d *DownloadOptions) uncompress(experimentId, file string) error {
	dir := path.Dir(file)
	command := []string{"/bin/sh", "-c", fmt.Sprintf("tar -zxf %s -C %s && chmod -R 755 %s", file, dir, dir)}
	options := &channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			ErrDecoder: func(bytes []byte) interface{} {
				return string(bytes)
			},
			OutDecoder: func(bytes []byte) interface{} {
				return string(bytes)
			},
		},
		PodNamespace:  d.Namespace,
		PodName:       d.PodName,
		ContainerName: d.Container,
		Command:       command,
		IgnoreOutput:  true,
	}
	error := d.client.Exec(options)
	logrus.WithFields(
		logrus.Fields{
			"experimentId": experimentId,
			"pod":          d.PodName,
			"container":    d.Container,
			"command":      command,
			"result":       error,
		}).Infof("uncompress in the target container")
	if error == nil {
		return nil
	}
	return errors.New(error.(string))
}

func (d *DownloadOptions) getUrl(srcFile string) string {
	obj := srcFile
	switch srcFile {
	case chaosblade.OperatorChaosBladeBlade:
		obj = blade
		break
	case chaosblade.OperatorChaosBladeYaml:
		obj = yaml
		break
	case chaosblade.OperatorChaosBladeLib:
		obj = lib
		break
	default:
		obj = strings.TrimPrefix(srcFile, chaosblade.OperatorChaosBladePath+"/")
	}
	return fmt.Sprintf("%s/%s", d.url, obj)
}
