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

package model

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type ExpController interface {
	// controller Name
	Name() string
	// Create
	Create(bladeName string, expSpec v1alpha1.ExperimentSpec) v1alpha1.ExperimentStatus
	// Destroy
	Destroy(bladeName string, expSpec v1alpha1.ExperimentSpec, oldExpStatus v1alpha1.ExperimentStatus) v1alpha1.ExperimentStatus
}

type ExperimentController interface {
	// controller Name
	Name() string
	// Create
	Create(ctx context.Context, expSpec v1alpha1.ExperimentSpec) *spec.Response
	// Destroy
	Destroy(ctx context.Context, expSpec v1alpha1.ExperimentSpec, oldExpStatus v1alpha1.ExperimentStatus) *spec.Response
}

type BaseExperimentController struct {
	Client            *channel.Client
	ResourceModelSpec ResourceExpModelSpec
}

func (b *BaseExperimentController) Destroy(ctx context.Context, expSpec v1alpha1.ExperimentSpec,
	oldExpStatus v1alpha1.ExperimentStatus) *spec.Response {
	expModel := ExtractExpModelFromExperimentStatus(oldExpStatus)
	return b.Exec(ctx, expModel)
}

func (b *BaseExperimentController) Exec(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrus.Infof("start exec, target: %s, action: %s", expModel.Target, expModel.ActionName)
	// get action spec
	actionSpec := b.ResourceModelSpec.GetExpActionModelSpec(expModel.Target, expModel.ActionName)
	if actionSpec == nil {
		errMsg := "can not find the action handler"
		return spec.ReturnFailWitResult(spec.Code[spec.HandlerNotFound], errMsg,
			v1alpha1.CreateFailExperimentStatus(errMsg, nil))
	}
	// invoke action executor
	response := actionSpec.Executor().Exec("", ctx, expModel)
	return response
}

func ExtractExpModelFromExperimentSpec(experimentSpec v1alpha1.ExperimentSpec) *spec.ExpModel {
	expModel := &spec.ExpModel{
		Target:      experimentSpec.Target,
		Scope:       experimentSpec.Scope,
		ActionName:  experimentSpec.Action,
		ActionFlags: make(map[string]string, 0),
	}
	if experimentSpec.Matchers != nil {
		for _, flag := range experimentSpec.Matchers {
			expModel.ActionFlags[flag.Name] = strings.Join(flag.Value, ",")
		}
	}
	return expModel
}

func ExtractExpModelFromExperimentStatus(experimentStatus v1alpha1.ExperimentStatus) *spec.ExpModel {
	return &spec.ExpModel{
		Target:     experimentStatus.Target,
		Scope:      experimentStatus.Scope,
		ActionName: experimentStatus.Action,
		// unuseful for destroy operation
		ActionFlags: make(map[string]string),
	}
}

func GetResourceCount(resourceCount int, flags map[string]string) (int, error) {
	count := math.MaxInt32
	percent := 100
	var err error
	countValue := flags[ResourceCountFlag.Name]
	if countValue != "" {
		count, err = strconv.Atoi(countValue)
		if err != nil {
			return 0, err
		}
	}

	percentValue := flags[ResourcePercentFlag.Name]
	if percentValue != "" {
		percent, err = strconv.Atoi(percentValue)
		if err != nil {
			return 0, err
		}
	}

	percentCount := int(math.Round(float64(percent) / 100.0 * float64(resourceCount)))
	if count > percentCount {
		count = percentCount
	}
	if count > resourceCount {
		return resourceCount, nil
	}
	return count, nil
}

// CreateDestroyedStatus returns the ExperimentStatus with destroyed state
func CreateDestroyedStatus(oldExpStatus v1alpha1.ExperimentStatus) v1alpha1.ExperimentStatus {
	statuses := make([]v1alpha1.ResourceStatus, 0)
	if oldExpStatus.ResStatuses != nil {
		for _, status := range oldExpStatus.ResStatuses {
			statuses = append(statuses, v1alpha1.ResourceStatus{
				// experiment uid in chaosblade
				Id: status.Id,
				// resource uid
				Uid: status.Uid,
				// resource name
				Name: status.Name,
				Kind: status.Kind,
				// experiment state
				State:    v1alpha1.DestroyedState,
				Success:  true,
				NodeName: status.NodeName,
			})
		}
	}
	return v1alpha1.CreateDestroyedExperimentStatus(statuses)
}

func CreateErrorStatus(errorMsgs []string, oldExpStatus v1alpha1.ExperimentStatus) v1alpha1.ExperimentStatus {
	statuses := make([]v1alpha1.ResourceStatus, 0)
	if oldExpStatus.ResStatuses != nil {
		for idx, status := range oldExpStatus.ResStatuses {
			statuses = append(statuses, v1alpha1.ResourceStatus{
				// experiment uid in chaosblade
				Id: status.Id,
				// resource uid
				Uid: status.Uid,
				// resource name
				Name: status.Name,
				Kind: status.Kind,
				// experiment state
				State:   v1alpha1.ErrorState,
				Error:   errorMsgs[idx],
				Success: false,
			})
		}
	}
	return v1alpha1.CreateFailExperimentStatus("see resStatus for the error details", statuses)
}
