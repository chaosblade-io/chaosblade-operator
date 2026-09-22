# Copyright 2025 The ChaosBlade Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# NAMESPACE empty means each run creates its own chaosblade-e2e-<suffix>
# namespace and cleans it up afterwards. Setting it adopts an existing
# namespace, which the suite then never deletes.
NAMESPACE ?=
# IMAGE empty uses the fixture default (fixture.DefaultImage: the amd64
# chaosblade-tool image, cached on every node of a cluster running the
# operator). Override it for arm64 clusters or clusters that can reach a
# public registry:
#   make e2e-run IMAGE=ghcr.io/chaosblade-io/chaosblade-tool-arm64:1.8.0
#   make e2e-run IMAGE=busybox:1.36
IMAGE ?=
PKG ?= ./e2e/...
LABEL_FILTER ?=
# Keep in sync with fixture.TestLabel in pkg/fixture/framework.go.
TEST_LABEL ?= chaosblade-e2e

# GINKGO_ARGS carries the optional flags so that empty values fall back to the
# fixture defaults instead of overriding them with "".
GINKGO_ARGS = $(if $(NAMESPACE),-namespace=$(NAMESPACE)) $(if $(IMAGE),-image=$(IMAGE))

.PHONY: e2e-build e2e-run e2e-all e2e-cleanup install-operator help

# e2e-build compiles and vets the whole e2e module, including test packages.
e2e-build:
	go vet ./...

# e2e-run runs the e2e tests against the cluster of the local kubeconfig.
# Examples:
#   make e2e-run                                  # all suites
#   make e2e-run PKG=./e2e/pod/...                # only the pod scope suite
#   make e2e-run PKG=./e2e/pod/... LABEL_FILTER=pod-delete
e2e-run:
	go test $(PKG) -v -timeout 30m -args $(GINKGO_ARGS) --ginkgo.label-filter="$(LABEL_FILTER)"

# e2e-all is a shortcut for running every scope suite.
e2e-all:
	go test ./e2e/... -v -timeout 30m -args $(GINKGO_ARGS)

# e2e-cleanup removes leftovers of interrupted runs. Every selector matches
# only resources this framework labeled, so unrelated workloads are never
# touched. Pods are deleted across namespaces because a run may have adopted
# an existing namespace it does not delete.
e2e-cleanup:
	kubectl delete chaosblade -l $(TEST_LABEL)=true --ignore-not-found
	kubectl delete pods -A -l $(TEST_LABEL)=true --ignore-not-found
	kubectl delete namespace -l $(TEST_LABEL)=true --ignore-not-found

# install-operator installs the operator with helm for a from-scratch cluster.
# A regular dev cluster already runs it, so this is not needed for e2e-run.
install-operator:
	helm install chaosblade ../deploy/helm/chaosblade-operator --namespace chaosblade --create-namespace

help:
	@echo "Targets:"
	@echo "  e2e-build        vet/compile the e2e module"
	@echo "  e2e-run          run e2e suites (vars: PKG, LABEL_FILTER, NAMESPACE, IMAGE)"
	@echo "  e2e-all          run all scope suites"
	@echo "  e2e-cleanup      delete CRs, pods and namespaces labeled $(TEST_LABEL)=true"
	@echo "  install-operator helm install the operator from ../deploy/helm"
