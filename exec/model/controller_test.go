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
	"testing"
)

func TestGetResourceCount(t *testing.T) {
	type args struct {
		resourceCount int
		flags         map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{name: "evict-percent=0", args: args{10, map[string]string{"evict-count": "", "evict-percent": "0"}}, want: 0, wantErr: true},
		{name: "evict-percent=10", args: args{10, map[string]string{"evict-count": "", "evict-percent": "10"}}, want: 1, wantErr: false},
		{name: "evict-percent=55", args: args{10, map[string]string{"evict-count": "", "evict-percent": "55"}}, want: 6, wantErr: false},
		{name: "evict-percent=100", args: args{10, map[string]string{"evict-count": "", "evict-percent": "100"}}, want: 10, wantErr: false},
		{name: "evict-count=5,evict-percent==10", args: args{10, map[string]string{"evict-count": "5", "evict-percent": "10"}}, want: 1, wantErr: false},
		{name: "evict-count=20", args: args{10, map[string]string{"evict-count": "20", "evict-percent": ""}}, want: 10, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, resp := GetResourceCount(tt.args.resourceCount, tt.args.flags)
			hasErr := resp != nil && !resp.Success
			if hasErr != tt.wantErr {
				t.Errorf("GetResourceCount() error = %v, wantErr %v", resp, tt.wantErr)
				return
			}
			if !hasErr && got != tt.want {
				t.Errorf("GetResourceCount() got = %v, want %v", got, tt.want)
			}
		})
	}
}
