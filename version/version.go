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

package version

import (
	"strconv"
	"strings"
)

const criVersion = "1.4.0"
var (
	Version = "unknown"
	Product = "community"

	// Version#Product
	CombinedVersion = ""
	Delimiter       = ","
)

func init() {
	if CombinedVersion != "" {
		fields := strings.Split(CombinedVersion, Delimiter)
		if len(fields) > 0 {
			Version = fields[0]
		}
		if len(fields) > 1 {
			Product = fields[1]
		}
	}
}

func CheckVerisonHaveCriCommand() bool {
	verisonA := strings.Split(Version, ".")
	criA := strings.Split(criVersion, ".")
	if len(verisonA) != 3 {
		return false
	}

	for k, v := range verisonA {
		vi, err := strconv.Atoi(v)
		if err != nil {
			return false
		}

		ci, _ := strconv.Atoi(criA[k])

		if ci == vi {
			continue
		}

		if vi < ci {
			return false
		}
		return true
	}
	return true
}