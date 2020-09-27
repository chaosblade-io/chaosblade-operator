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
	"reflect"
	"testing"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func TestClient_Exec(t *testing.T) {
	type fields struct {
		Interface kubernetes.Interface
		Client    client.Client
		Config    *rest.Config
	}
	type args struct {
		options *ExecOptions
	}
	kubeconfig := config.GetConfigOrDie()
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *spec.Response
	}{
		{
			name: "test exec",
			fields: fields{
				Interface: kubernetes.NewForConfigOrDie(kubeconfig),
				Client:    nil,
				Config:    kubeconfig,
			},
			args: args{
				options: &ExecOptions{
					StreamOptions: StreamOptions{
						ErrDecoder: func(bytes []byte) interface{} {
							content := string(bytes)
							return spec.Decode(content, spec.ReturnFail(spec.Code[spec.K8sInvokeError], content))
						},
						OutDecoder: func(bytes []byte) interface{} {
							content := string(bytes)
							return spec.Decode(content, spec.ReturnFail(spec.Code[spec.K8sInvokeError], content))
						},
					},
					PodName:       "frontend-6c887c56c8-4g7sh",
					PodNamespace:  "default",
					ContainerName: "php-redis",
					Command:       []string{"/opt/chaosblade/blade", "create", "cpu", "fullload"},
					IgnoreOutput:  false,
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{
				Interface: tt.fields.Interface,
				Client:    tt.fields.Client,
				Config:    tt.fields.Config,
			}
			if got := c.Exec(tt.args.options); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Exec() = %v, want %v", got, tt.want)
			}
		})
	}
}
