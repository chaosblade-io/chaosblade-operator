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

.PHONY: build clean

BLADE_VERSION=0.6.0

# Build [OLM](https://github.com/operator-framework/operator-lifecycle-manager)
build:
	operator-sdk olm-catalog gen-csv \
	  --operator-name chaosblade-operator \
	  --csv-version $(BLADE_VERSION) \
	  --update-crds --verbose
	# Change `olm` keyword to `chaosblade-operator`
	sed 's/olm/chaosblade-operator/g' deploy/olm-catalog/olm/olm.package.yaml \
      > deploy/olm-catalog/chaosblade-operator/chaosblade-operator.package.yaml

	rm -rf deploy/olm-catalog/olm

clean:
	rm -rf deploy/olm-catalog