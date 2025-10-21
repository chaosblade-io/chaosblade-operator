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

package hookfs

var defaultHookPoints = []string{
	"read",
	"write",
	"mkdir",
	"rmdir",
	"opendir",
	"fsync",
	"flush",
	"release",
	"truncate",
	"getattr",
	"chown",
	"utimens",
	"allocate",
	"getlk",
	"setlk",
	"setlkw",
	"statfs",
	"readlink",
	"symlink",
	"create",
	"access",
	"link",
	"mknod",
	"rename",
	"unlink",
	"getxattr",
	"listxattr",
	"removexattr",
	"setxattr",
}

var (
	InjectPath  = "/inject"
	RecoverPath = "/recover"
)
