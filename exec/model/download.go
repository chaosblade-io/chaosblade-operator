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

const bin = "bin"
const blade = "blade"
const lib = "lib.tar.gz"
const yaml = "yaml.tar.gz"

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
	command = []string{"curl", "-s", "-L", "-w", "%{http_code}", "-o", dest, url}
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
	var obj = srcFile
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
