/*
 * Copyright 2020 Yubico AB
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"os"
	"reflect"
	"testing"
)

func TestGetFullPullURL(t *testing.T) {
	tests := []struct {
		pullURL    string
		pullSecret string
		expected   string
	}{
		{"", "", ""},
		{"https://www.some.com/policy", "", "https://www.some.com/policy"},
		{"gcs::https://www.some.com/policy", `{"test": "test"}`, "gcs::https://www.some.com/policy"},
		{"s3::https://www.some.com/policy", "aws_access_key_id=KEYID&aws_access_key_secret=SECRETKEY", "s3::https://www.some.com/policy?aws_access_key_id=KEYID&aws_access_key_secret=SECRETKEY"},
		{"https://www.some.com/policy", "user:pass", "https://user:pass@www.some.com/policy"},
	}

	for _, test := range tests {
		os.Setenv("PULL_URL", test.pullURL)
		os.Setenv("PULL_SECRET", test.pullSecret)

		out, err := getFullPullURL()
		if err != nil {
			t.Fatal(err)
		}

		if out != test.expected {
			t.Errorf("output %v did not match expected %v", out, test.expected)
		}
	}
}

func TestGetFlagFromEnv(t *testing.T) {
	tests := []struct {
		env      string
		expected string
	}{
		{"ALL_NAMESPACES", "--all-namespaces"},
		{"COMBINE", "--combine"},
		{"data", "--data"},
	}

	for _, test := range tests {
		out := getFlagFromEnv(test.env)
		if out != test.expected {
			t.Errorf("output %v did not match expected %v", out, test.expected)
		}
	}
}

func TestGetFlagsFromEnv(t *testing.T) {
	tests := []struct {
		envs     map[string]string
		expected []string
	}{
		{
			envs: map[string]string{
				"COMBINE": "true",
			},
			expected: []string{"--combine"},
		},
		{
			envs: map[string]string{
				"COMBINE":        "true",
				"ALL_NAMESPACES": "false",
			},
			expected: []string{"--combine"},
		},
		{
			envs: map[string]string{
				"COMBINE": "true",
				"POLICY":  "some/path",
			},
			expected: []string{"--combine", "--policy", "some/path"},
		},
		{
			envs: map[string]string{
				"COMBINE":    "true",
				"IRRELEVANT": "true",
				"DATA":       "path2",
			},
			expected: []string{"--combine", "--data", "path2"},
		},
		{
			envs: map[string]string{
				"IRRELEVANT": "true",
			},
			expected: nil,
		},
		{
			envs:     nil,
			expected: nil,
		},
	}

	for _, test := range tests {
		for _, v := range conftestFlags {
			os.Unsetenv(v)
		}

		for k, v := range test.envs {
			os.Setenv(k, v)
		}

		out := getFlagsFromEnv()
		if !reflect.DeepEqual(out, test.expected) {
			t.Errorf("output %v did not match expected %v", out, test.expected)
		}
	}
}
