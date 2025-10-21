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

package main

import (
	"log"
	"os"

	"github.com/chaosblade-io/chaosblade-operator/exec/container"
	"github.com/chaosblade-io/chaosblade-operator/exec/node"
	"github.com/chaosblade-io/chaosblade-operator/exec/pod"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
)

// main creates the yaml file of the experiments about kubernetes
func main() {
	if len(os.Args) < 2 {
		log.Panicln("less yaml file path")
	}
	if len(os.Args) == 3 {
		container.JvmSpecPathForYaml = os.Args[2]
	}
	err := util.CreateYamlFile(getModels(), os.Args[1])
	if err != nil {
		log.Panicf("create yaml file error, %v", err)
	}
}

func getModels() *spec.Models {
	models := make([]*spec.Models, 0)
	nodeResourceModelSpec := node.NewResourceModelSpec(nil)
	for _, modelSpec := range nodeResourceModelSpec.ExpModels() {
		model := util.ConvertSpecToModels(modelSpec, spec.ExpPrepareModel{}, nodeResourceModelSpec.Scope())
		models = append(models, model)
	}
	podResourceModelSpec := pod.NewResourceModelSpec(nil)
	for _, modelSpec := range podResourceModelSpec.ExpModels() {
		model := util.ConvertSpecToModels(modelSpec, spec.ExpPrepareModel{}, podResourceModelSpec.Scope())
		models = append(models, model)
	}
	containerResourceModelSpec := container.NewResourceModelSpec(nil)
	for _, modelSpec := range containerResourceModelSpec.ExpModels() {
		model := util.ConvertSpecToModels(modelSpec, spec.ExpPrepareModel{}, containerResourceModelSpec.Scope())
		models = append(models, model)
	}
	return util.MergeModels(models...)
}
