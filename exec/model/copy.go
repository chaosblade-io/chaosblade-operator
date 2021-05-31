/*
 * Copyright 2016 The Kubernetes Authors.
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

package model

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"

	"github.com/chaosblade-io/chaosblade-operator/channel"
)

type CopyOptions struct {
	Container string
	Namespace string
	PodName   string
	client    *channel.Client
}

// CheckFileExists return nil if dest file exists
func (o *CopyOptions) CheckFileExists(dest string) error {
	options := &channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			ErrDecoder: func(bytes []byte) interface{} {
				return fmt.Errorf(string(bytes))
			},
			OutDecoder: func(bytes []byte) interface{} {
				return nil
			},
		},
		PodNamespace:  o.Namespace,
		PodName:       o.PodName,
		ContainerName: o.Container,
		Command:       []string{"test", "-e", dest},
		IgnoreOutput:  true,
	}
	if err := o.client.Exec(options); err != nil {
		return err.(error)
	}
	return nil
}

func makeTar(srcPath, destPath string, writer io.Writer) error {
	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()

	srcPath = path.Clean(srcPath)
	destPath = path.Clean(destPath)
	err := recursiveTar(path.Dir(srcPath), path.Base(srcPath), path.Dir(destPath), path.Base(destPath), tarWriter)
	return err
}

func recursiveTar(srcBase, srcFile, destBase, destFile string, tw *tar.Writer) error {
	logrus.WithFields(logrus.Fields{
		"srcBase":  srcBase,
		"srcFile":  srcFile,
		"destBase": destBase,
		"destFile": destFile,
	}).Debugln("recursiveTar")
	srcPath := path.Join(srcBase, srcFile)
	matchedPaths, err := filepath.Glob(srcPath)
	if err != nil {
		return err
	}
	for _, fpath := range matchedPaths {
		stat, err := os.Lstat(fpath)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			files, err := ioutil.ReadDir(fpath)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				//case empty directory
				hdr, _ := tar.FileInfoHeader(stat, fpath)
				hdr.Name = destFile
				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}
			}
			for _, f := range files {
				if err := recursiveTar(srcBase, path.Join(srcFile, f.Name()), destBase, path.Join(destFile, f.Name()), tw); err != nil {
					return err
				}
			}
			return nil
		} else if stat.Mode()&os.ModeSymlink != 0 {
			//case soft link
			hdr, _ := tar.FileInfoHeader(stat, fpath)
			target, err := os.Readlink(fpath)
			if err != nil {
				return err
			}

			hdr.Linkname = target
			hdr.Name = destFile
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
		} else {
			//case regular file or other file type like pipe
			hdr, err := tar.FileInfoHeader(stat, fpath)
			if err != nil {
				return err
			}
			hdr.Name = destFile

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
			return f.Close()
		}
	}
	return nil
}

// CopyToPod copies src file or directory to specify container
func (o *CopyOptions) CopyToPod(experimentId, src, dest string) error {
	if len(src) == 0 || len(dest) == 0 {
		return errors.New("filepath can not be empty")
	}
	reader, writer := io.Pipe()

	// strip trailing slash (if any)
	if dest != "/" && strings.HasSuffix(string(dest[len(dest)-1]), "/") {
		dest = dest[:len(dest)-1]
	}

	go func() error {
		defer writer.Close()
		err := makeTar(src, dest, writer)
		if err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "makeTar", err.Error()))
			return spec.ResponseFailWaitResult(spec.K8sExecFailed, fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].Err, experimentId),
				fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "makeTar", err.Error()))
		}
		return nil
	}()
	cmdArr := []string{"tar", "--no-same-permissions", "--no-same-owner", "-xmf", "-"}
	destDir := path.Dir(dest)
	if len(destDir) > 0 {
		cmdArr = append(cmdArr, "-C", destDir)
	}
	options := &channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			IOStreams: channel.IOStreams{
				In: reader,
			},
			Stdin: true,
			ErrDecoder: func(bytes []byte) interface{} {
				return fmt.Errorf(string(bytes))
			},
			OutDecoder: func(bytes []byte) interface{} {
				return nil
			},
		},
		PodName:       o.PodName,
		PodNamespace:  o.Namespace,
		ContainerName: o.Container,
		Command:       cmdArr,
		IgnoreOutput:  true,
	}
	return o.execute(options)
}

func (o *CopyOptions) execute(options *channel.ExecOptions) error {
	if len(options.PodNamespace) == 0 {
		options.PodNamespace = o.Namespace
	}

	if len(o.Container) > 0 {
		options.ContainerName = o.Container
	}
	if err := o.client.Exec(options); err != nil {
		return err.(error)
	}
	return nil
}

func (o *CopyOptions) CreateDir(dir string) error {
	if len(dir) == 0 {
		return fmt.Errorf("illegal directory name")
	}
	options := &channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			ErrDecoder: func(bytes []byte) interface{} {
				return fmt.Errorf(string(bytes))
			},
			OutDecoder: func(bytes []byte) interface{} {
				return nil
			},
		},
		PodName:       o.PodName,
		PodNamespace:  o.Namespace,
		ContainerName: o.Container,
		Command:       []string{"mkdir", "-p", dir},
		IgnoreOutput:  true,
	}
	if err := o.client.Exec(options); err != nil {
		return err.(error)
	}
	return nil
}
