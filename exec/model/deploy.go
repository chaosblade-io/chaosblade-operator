package model

import (
	"fmt"

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

func (o *DeployOptions) CreateDir(dir string) error {
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
