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

	"github.com/chaosblade-io/chaosblade-operator/channel"
)

type DeployMode interface {
	DeployToPod(experimentId, src, dest string) error
}

type DeployOptions struct {
	Container string
	Namespace string
	PodName   string
	client    *channel.Client
}

// CheckFileExists return nil if dest file exists
func (o *DeployOptions) CheckFileExists(dest string) error {
	options := &channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			ErrDecoder: func(bytes []byte) interface{} {
				return errors.New(string(bytes))
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

func (o *DeployOptions) CreateDir(dir string) error {
	if len(dir) == 0 {
		return errors.New("illegal directory name")
	}
	options := &channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			ErrDecoder: func(bytes []byte) interface{} {
				return errors.New(string(bytes))
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
