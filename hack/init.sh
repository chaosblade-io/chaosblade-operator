#!/usr/bin/env bash
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


# Ensure Go-installed tool binaries (goimports, gofumpt, ...) are reachable.
#
# The update-/verify- hack scripts run `go install <tool>@<version>` and then
# invoke the binary by its bare name. `go install` writes to $GOBIN if set,
# otherwise to $(go env GOPATH)/bin. On many developer machines that
# directory is NOT on $PATH by default (notably default macOS shells with
# Homebrew Go), which makes `make format` / `make verify` fail with
#   "<tool>: command not found".
#
# Prepending the install dir here means every script that sources init.sh
# picks up freshly installed tools without requiring developers to fiddle
# with their shell rc files.
__chaosblade_go_tool_dir="$(go env GOBIN 2>/dev/null || true)"
if [[ -z "${__chaosblade_go_tool_dir}" ]]; then
    __chaosblade_go_tool_dir="$(go env GOPATH 2>/dev/null || true)/bin"
fi
if [[ -n "${__chaosblade_go_tool_dir}" && "${__chaosblade_go_tool_dir}" != "/bin" ]]; then
    case ":${PATH:-}:" in
        *":${__chaosblade_go_tool_dir}:"*) ;;
        *) export PATH="${__chaosblade_go_tool_dir}:${PATH:-}" ;;
    esac
fi
unset __chaosblade_go_tool_dir

function git_find() {
    # Similar to find but faster and easier to understand.  We want to include
    # modified and untracked files because this might be running against code
    # which is not tracked by git yet.
    git ls-files -cmo --exclude-standard \
        ':!:vendor/*'        `# catches vendor/...` \
        ':!:*/vendor/*'      `# catches any subdir/vendor/...` \
        ':!:third_party/*'   `# catches third_party/...` \
        ':!:*/third_party/*' `# catches third_party/...` \
        ':!:*/testdata/*'    `# catches any subdir/testdata/...` \
        ':(glob)**/*.go' \
        "$@"
}