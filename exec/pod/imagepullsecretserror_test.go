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
	"encoding/base64"
	"encoding/json"
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestCorruptDockerConfigJSON(t *testing.T) {
	// Valid dockerconfigjson with multiple registries
	input := map[string]interface{}{
		"auths": map[string]interface{}{
			"registry.example.com": map[string]interface{}{
				"username": "real-user",
				"password": "real-pass",
				"auth":     base64.StdEncoding.EncodeToString([]byte("real-user:real-pass")),
			},
			"docker.io": map[string]interface{}{
				"username":      "docker-user",
				"password":      "docker-pass",
				"auth":          base64.StdEncoding.EncodeToString([]byte("docker-user:docker-pass")),
				"identitytoken": "some-token",
			},
		},
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	result, err := corruptDockerConfigJSON(inputBytes)
	if err != nil {
		t.Fatalf("corruptDockerConfigJSON failed: %v", err)
	}

	// Parse result
	var output map[string]interface{}
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	auths, ok := output["auths"].(map[string]interface{})
	if !ok {
		t.Fatal("result has no 'auths' map")
	}

	// Verify both registries are corrupted
	expectedAuth := base64.StdEncoding.EncodeToString([]byte("chaosblade-invalid-user:chaosblade-invalid-pass"))

	for registry, creds := range auths {
		credsMap, ok := creds.(map[string]interface{})
		if !ok {
			t.Fatalf("credentials for %s is not a map", registry)
		}

		if credsMap["username"] != "chaosblade-invalid-user" {
			t.Errorf("registry %s: expected username 'chaosblade-invalid-user', got '%v'", registry, credsMap["username"])
		}
		if credsMap["password"] != "chaosblade-invalid-pass" {
			t.Errorf("registry %s: expected password 'chaosblade-invalid-pass', got '%v'", registry, credsMap["password"])
		}
		if credsMap["auth"] != expectedAuth {
			t.Errorf("registry %s: expected auth '%s', got '%v'", registry, expectedAuth, credsMap["auth"])
		}
		if _, exists := credsMap["identitytoken"]; exists {
			t.Errorf("registry %s: identitytoken should be removed", registry)
		}
		if _, exists := credsMap["registrytoken"]; exists {
			t.Errorf("registry %s: registrytoken should be removed", registry)
		}
	}
}

func TestCorruptDockerConfigJSON_EmptyAuths(t *testing.T) {
	input := map[string]interface{}{
		"auths": map[string]interface{}{},
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	_, err = corruptDockerConfigJSON(inputBytes)
	if err == nil {
		t.Fatal("expected error for empty auths, got nil")
	}
}

func TestCorruptDockerConfigJSON_NoAuthsKey(t *testing.T) {
	input := map[string]interface{}{
		"someOtherKey": "value",
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	_, err = corruptDockerConfigJSON(inputBytes)
	if err == nil {
		t.Fatal("expected error for missing auths key, got nil")
	}
}

func TestCorruptDockerConfigJSON_InvalidJSON(t *testing.T) {
	_, err := corruptDockerConfigJSON([]byte("not valid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestCorruptDockerCfg(t *testing.T) {
	// .dockercfg format: top-level keys are registries
	input := map[string]interface{}{
		"https://index.docker.io/v1/": map[string]interface{}{
			"username": "myuser",
			"password": "mypass",
			"email":    "user@example.com",
			"auth":     base64.StdEncoding.EncodeToString([]byte("myuser:mypass")),
		},
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	result, err := corruptDockerCfg(inputBytes)
	if err != nil {
		t.Fatalf("corruptDockerCfg failed: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	expectedAuth := base64.StdEncoding.EncodeToString([]byte("chaosblade-invalid-user:chaosblade-invalid-pass"))
	for registry, creds := range output {
		credsMap, ok := creds.(map[string]interface{})
		if !ok {
			t.Fatalf("credentials for %s is not a map", registry)
		}
		if credsMap["username"] != "chaosblade-invalid-user" {
			t.Errorf("registry %s: expected username 'chaosblade-invalid-user', got '%v'", registry, credsMap["username"])
		}
		if credsMap["password"] != "chaosblade-invalid-pass" {
			t.Errorf("registry %s: expected password 'chaosblade-invalid-pass', got '%v'", registry, credsMap["password"])
		}
		if credsMap["auth"] != expectedAuth {
			t.Errorf("registry %s: expected auth '%s', got '%v'", registry, expectedAuth, credsMap["auth"])
		}
		// email field should be preserved
		if credsMap["email"] != "user@example.com" {
			t.Errorf("registry %s: email field should be preserved, got '%v'", registry, credsMap["email"])
		}
	}
}

func TestCorruptDockerCfg_Empty(t *testing.T) {
	input := map[string]interface{}{}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	_, err = corruptDockerCfg(inputBytes)
	if err == nil {
		t.Fatal("expected error for empty dockercfg, got nil")
	}
}

func TestGenerateBackupSecretName(t *testing.T) {
	tests := []struct {
		name         string
		experimentId string
		namespace    string
		secretName   string
	}{
		{
			name:         "normal case",
			experimentId: "abc12345def67890",
			namespace:    "default",
			secretName:   "my-registry-secret",
		},
		{
			name:         "short experiment id",
			experimentId: "short",
			namespace:    "kube-system",
			secretName:   "docker-secret",
		},
		{
			name:         "long experiment id",
			experimentId: "very-long-experiment-id-that-exceeds-eight-characters",
			namespace:    "production",
			secretName:   "registry-credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateBackupSecretName(tt.experimentId, tt.namespace, tt.secretName)

			// Should start with prefix
			if len(result) < len("chaosblade-ips-") {
				t.Errorf("result too short: %s", result)
			}

			// Should be deterministic
			result2 := generateBackupSecretName(tt.experimentId, tt.namespace, tt.secretName)
			if result != result2 {
				t.Errorf("not deterministic: %s != %s", result, result2)
			}

			// Should be a valid DNS subdomain (max 253 chars, lowercase, alphanumeric/dash)
			if len(result) > 253 {
				t.Errorf("name too long: %d characters", len(result))
			}

			// Different inputs should produce different names
			different := generateBackupSecretName(tt.experimentId, tt.namespace, "other-secret")
			if result == different {
				t.Errorf("different inputs produced same name: %s", result)
			}
		})
	}
}

func TestGenerateBackupSecretName_Deterministic(t *testing.T) {
	// Same inputs should always produce the same output
	name1 := generateBackupSecretName("exp123", "default", "my-secret")
	name2 := generateBackupSecretName("exp123", "default", "my-secret")
	if name1 != name2 {
		t.Errorf("expected deterministic result, got %s and %s", name1, name2)
	}
}

func TestImagePullSecretsErrorActionSpec_Name(t *testing.T) {
	spec := &ImagePullSecretsErrorActionSpec{}
	if spec.Name() != "imagepullsecretserror" {
		t.Errorf("expected name 'imagepullsecretserror', got '%s'", spec.Name())
	}
}

func TestImagePullSecretsErrorActionSpec_Aliases(t *testing.T) {
	spec := &ImagePullSecretsErrorActionSpec{}
	aliases := spec.Aliases()
	if len(aliases) != 0 {
		t.Errorf("expected no aliases, got %v", aliases)
	}
}

func TestImagePullSecretsErrorActionExecutor_Name(t *testing.T) {
	executor := &ImagePullSecretsErrorActionExecutor{}
	if executor.Name() != "imagepullsecretserror" {
		t.Errorf("expected name 'imagepullsecretserror', got '%s'", executor.Name())
	}
}

func TestFilterSecretRefs(t *testing.T) {
	refs := []v1.LocalObjectReference{
		{Name: "secret-a"},
		{Name: "secret-b"},
		{Name: "secret-c"},
	}

	tests := []struct {
		name     string
		filter   string
		expected int
	}{
		{
			name:     "match single",
			filter:   "secret-a",
			expected: 1,
		},
		{
			name:     "no match",
			filter:   "nonexistent",
			expected: 0,
		},
		{
			name:     "match last",
			filter:   "secret-c",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterSecretRefs(refs, tt.filter)
			if len(result) != tt.expected {
				t.Errorf("expected %d results, got %d", tt.expected, len(result))
			}
			if tt.expected > 0 && result[0].Name != tt.filter {
				t.Errorf("expected name %s, got %s", tt.filter, result[0].Name)
			}
		})
	}
}

func TestFilterSecretRefs_EmptyInput(t *testing.T) {
	result := filterSecretRefs(nil, "any")
	if len(result) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(result))
	}

	result = filterSecretRefs([]v1.LocalObjectReference{}, "any")
	if len(result) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(result))
	}
}

func TestFilterSecretRefs_DuplicateNames(t *testing.T) {
	refs := []v1.LocalObjectReference{
		{Name: "my-secret"},
		{Name: "other-secret"},
		{Name: "my-secret"},
	}
	result := filterSecretRefs(refs, "my-secret")
	if len(result) != 2 {
		t.Errorf("expected 2 results for duplicate names, got %d", len(result))
	}
}

func TestCopySecretData(t *testing.T) {
	original := map[string][]byte{
		".dockerconfigjson": []byte(`{"auths":{"registry.io":{"auth":"dXNlcjpwYXNz"}}}`),
	}

	copied := copySecretData(original)

	// Verify content is the same
	if string(copied[".dockerconfigjson"]) != string(original[".dockerconfigjson"]) {
		t.Error("copied content should match original")
	}

	// Verify modifying copy doesn't affect original
	copied[".dockerconfigjson"][0] = 'X'
	if original[".dockerconfigjson"][0] == 'X' {
		t.Error("modifying copy should not affect original")
	}
}

func TestCopySecretData_Nil(t *testing.T) {
	result := copySecretData(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}
