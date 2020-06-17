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

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
)

// TODO: Waiting to be implemented.
type CordonActionCommandSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewCordonActionCommandSpec() spec.ExpActionCommandSpec {
	return &CordonActionCommandSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags:    []spec.ExpFlagSpec{},
			ActionExecutor: &CordonExecutor{},
		},
	}
}

func (*CordonActionCommandSpec) Name() string {
	return "cordon"
}

func (*CordonActionCommandSpec) Aliases() []string {
	return []string{}
}

func (*CordonActionCommandSpec) ShortDesc() string {
	return "Cordon node"
}

func (*CordonActionCommandSpec) LongDesc() string {
	return "Cordon node"
}

type CordonExecutor struct {
}

func (*CordonExecutor) Exec(uid string, ctx context.Context, model *spec.ExpModel) *spec.Response {
	panic("implement me")
}

func (*CordonExecutor) Name() string {
	panic("implement me")
}

func (*CordonExecutor) SetChannel(channel spec.Channel) {
	panic("implement me")
}
