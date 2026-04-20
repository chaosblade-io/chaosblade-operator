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

package pod

import (
	"testing"
)

func TestParseImage(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantTag  string
	}{
		{"nginx", "nginx", ""},
		{"nginx:latest", "nginx", "latest"},
		{"nginx:1.19.0", "nginx", "1.19.0"},
		{"registry.example.com/nginx", "registry.example.com/nginx", ""},
		{"registry.example.com/nginx:v1.0", "registry.example.com/nginx", "v1.0"},
		{"registry.example.com:5000/nginx", "registry.example.com:5000/nginx", ""},
		{"registry.example.com:5000/nginx:v1.0", "registry.example.com:5000/nginx", "v1.0"},
		{"my-registry.io:5000/my-org/my-image:sha-abc123", "my-registry.io:5000/my-org/my-image", "sha-abc123"},
		{"localhost:5000/test", "localhost:5000/test", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotName, gotTag := parseImage(tt.input)
			if gotName != tt.wantName {
				t.Errorf("parseImage(%q) name = %q, want %q", tt.input, gotName, tt.wantName)
			}
			if gotTag != tt.wantTag {
				t.Errorf("parseImage(%q) tag = %q, want %q", tt.input, gotTag, tt.wantTag)
			}
		})
	}
}

func TestBuildNewImage(t *testing.T) {
	tests := []struct {
		name          string
		originalImage string
		imageName     string
		imageTag      string
		want          string
	}{
		// Default behavior: both params empty -> append "-image-config-error"
		{
			name:          "default behavior - simple image",
			originalImage: "nginx",
			imageName:     "",
			imageTag:      "",
			want:          "nginx-image-config-error",
		},
		{
			name:          "default behavior - image with tag",
			originalImage: "nginx:latest",
			imageName:     "",
			imageTag:      "",
			want:          "nginx:latest-image-config-error",
		},
		{
			name:          "default behavior - full registry path",
			originalImage: "registry.example.com/nginx:v1.0",
			imageName:     "",
			imageTag:      "",
			want:          "registry.example.com/nginx:v1.0-image-config-error",
		},

		// Only imageName provided
		{
			name:          "replace image name only - simple image",
			originalImage: "nginx",
			imageName:     "nginx-not-exist",
			imageTag:      "",
			want:          "nginx-not-exist",
		},
		{
			name:          "replace image name only - image with tag",
			originalImage: "nginx:latest",
			imageName:     "nginx-not-exist",
			imageTag:      "",
			want:          "nginx-not-exist:latest",
		},
		{
			name:          "replace image name only - full registry path",
			originalImage: "registry.example.com/nginx:v1.0",
			imageName:     "my-bad-image",
			imageTag:      "",
			want:          "my-bad-image:v1.0",
		},

		// Only imageTag provided
		{
			name:          "replace tag only - simple image without tag",
			originalImage: "nginx",
			imageName:     "",
			imageTag:      "non-existent-tag",
			want:          "nginx:non-existent-tag",
		},
		{
			name:          "replace tag only - image with tag",
			originalImage: "nginx:latest",
			imageName:     "",
			imageTag:      "non-existent-tag",
			want:          "nginx:non-existent-tag",
		},
		{
			name:          "replace tag only - full registry path",
			originalImage: "registry.example.com/nginx:v1.0",
			imageName:     "",
			imageTag:      "broken-tag",
			want:          "registry.example.com/nginx:broken-tag",
		},

		// Both imageName and imageTag provided
		{
			name:          "replace both name and tag",
			originalImage: "nginx:latest",
			imageName:     "bad-image",
			imageTag:      "bad-tag",
			want:          "bad-image:bad-tag",
		},
		{
			name:          "replace both - full registry path",
			originalImage: "registry.example.com/nginx:v1.0",
			imageName:     "totally-wrong",
			imageTag:      "no-such-tag",
			want:          "totally-wrong:no-such-tag",
		},

		// Edge cases with registry port
		{
			name:          "registry with port - replace tag only",
			originalImage: "registry.example.com:5000/nginx:v1.0",
			imageName:     "",
			imageTag:      "broken",
			want:          "registry.example.com:5000/nginx:broken",
		},
		{
			name:          "registry with port no tag - replace tag",
			originalImage: "registry.example.com:5000/nginx",
			imageName:     "",
			imageTag:      "broken",
			want:          "registry.example.com:5000/nginx:broken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildNewImage(tt.originalImage, tt.imageName, tt.imageTag)
			if got != tt.want {
				t.Errorf("buildNewImage(%q, %q, %q) = %q, want %q",
					tt.originalImage, tt.imageName, tt.imageTag, got, tt.want)
			}
		})
	}
}

func TestIsImageConfigAnnotationExist(t *testing.T) {
	tests := []struct {
		name       string
		annotation map[string]string
		key        string
		want       bool
	}{
		{
			name:       "annotation exists",
			annotation: map[string]string{"imageConfig-nginx": "nginx:latest"},
			key:        "imageConfig-nginx",
			want:       true,
		},
		{
			name:       "annotation does not exist",
			annotation: map[string]string{"imageConfig-nginx": "nginx:latest"},
			key:        "imageConfig-redis",
			want:       false,
		},
		{
			name:       "empty annotations",
			annotation: map[string]string{},
			key:        "imageConfig-nginx",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isImageConfigAnnotationExist(tt.annotation, tt.key)
			if got != tt.want {
				t.Errorf("isImageConfigAnnotationExist() = %v, want %v", got, tt.want)
			}
		})
	}
}
