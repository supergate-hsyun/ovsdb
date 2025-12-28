// Copyright 2018 Paul Greenberg (greenpau@outlook.com)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ovsdb

import (
	"os"
	"testing"
)

func TestParseOvsVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "OVS 3.5.1 format",
			input:    "ovs-vswitchd (Open vSwitch) 3.5.1",
			expected: "3.5.1",
		},
		{
			name:     "OVS 2.17.0 format",
			input:    "ovs-vswitchd (Open vSwitch) 2.17.0",
			expected: "2.17.0",
		},
		{
			name:     "OVS with patch version",
			input:    "ovs-vswitchd (Open vSwitch) 2.17.1",
			expected: "2.17.1",
		},
		{
			name:     "Malformed version string",
			input:    "some random text",
			expected: "some random text",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Version with extra whitespace",
			input:    "ovs-vswitchd (Open vSwitch)  3.5.1  ",
			expected: "3.5.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseOvsVersion(tt.input)
			if result != tt.expected {
				t.Errorf("parseOvsVersion(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetSystemInfoFromOS(t *testing.T) {
	// This test checks if the function can read /etc/os-release
	// The actual values will vary depending on the system
	systemType, systemVersion := getSystemInfoFromOS()

	// On a system with /etc/os-release, we should get non-empty values
	if _, err := os.Stat("/etc/os-release"); err == nil {
		if systemType == "" {
			t.Error("Expected non-empty system type when /etc/os-release exists")
		}
		if systemVersion == "" {
			t.Error("Expected non-empty system version when /etc/os-release exists")
		}
		t.Logf("Detected system: %s %s", systemType, systemVersion)
	} else {
		// If /etc/os-release doesn't exist, both should be empty
		if systemType != "" || systemVersion != "" {
			t.Error("Expected empty values when /etc/os-release doesn't exist")
		}
	}
}

func TestPopulateVersionFromAppctl(t *testing.T) {
	tests := []struct {
		name         string
		systemInfo   map[string]string
		schema       *Schema
		expectOvsVer bool
		expectDbVer  bool
		expectSysTyp bool
		expectSysVer bool
	}{
		{
			name: "All fields present in systemInfo",
			systemInfo: map[string]string{
				"ovs_version":    "2.17.0",
				"db_version":     "7.16.1",
				"system_type":    "ubuntu",
				"system_version": "20.04",
			},
			schema:       &Schema{Version: "7.16.1"},
			expectOvsVer: false, // Should not query when already present
			expectDbVer:  false,
			expectSysTyp: false,
			expectSysVer: false,
		},
		{
			name:       "All fields missing",
			systemInfo: map[string]string{},
			schema:     &Schema{Version: "7.16.1"},
			expectOvsVer: true, // Should populate with "unknown" or queried value
			expectDbVer:  true, // Should populate from schema
			expectSysTyp: true, // Should populate from OS
			expectSysVer: true,
		},
		{
			name: "Only ovs_version missing",
			systemInfo: map[string]string{
				"db_version":     "7.16.1",
				"system_type":    "ubuntu",
				"system_version": "20.04",
			},
			schema:       &Schema{Version: "7.16.1"},
			expectOvsVer: true,
			expectDbVer:  false,
			expectSysTyp: false,
			expectSysVer: false,
		},
		{
			name: "Empty values treated as missing",
			systemInfo: map[string]string{
				"ovs_version":    "",
				"db_version":     "",
				"system_type":    "",
				"system_version": "",
			},
			schema:       &Schema{Version: "7.16.1"},
			expectOvsVer: true,
			expectDbVer:  true,
			expectSysTyp: true,
			expectSysVer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: We can't test the actual appctl query without a running OVS
			// but we can test the logic with a fake socket that will fail
			populateVersionFromAppctl(tt.systemInfo, "/nonexistent/socket", 1, tt.schema)

			// Check that fields were populated (either with real values or "unknown")
			if tt.expectOvsVer {
				if _, exists := tt.systemInfo["ovs_version"]; !exists {
					t.Error("Expected ovs_version to be populated")
				}
			}
			if tt.expectDbVer {
				if val, exists := tt.systemInfo["db_version"]; !exists {
					t.Error("Expected db_version to be populated")
				} else if tt.schema != nil && tt.schema.Version != "" && val != tt.schema.Version {
					t.Errorf("Expected db_version to be %q from schema, got %q", tt.schema.Version, val)
				}
			}
			if tt.expectSysTyp {
				if _, exists := tt.systemInfo["system_type"]; !exists {
					t.Error("Expected system_type to be populated")
				}
			}
			if tt.expectSysVer {
				if _, exists := tt.systemInfo["system_version"]; !exists {
					t.Error("Expected system_version to be populated")
				}
			}

			t.Logf("systemInfo after populate: %+v", tt.systemInfo)
		})
	}
}

func TestPopulateVersionFromAppctlWithSchema(t *testing.T) {
	systemInfo := map[string]string{}
	schema := &Schema{Version: "7.16.1"}

	// Populate with a fake socket (will fail to connect, but should still populate from schema)
	populateVersionFromAppctl(systemInfo, "/nonexistent/socket", 1, schema)

	// db_version should be populated from schema
	if dbVersion, exists := systemInfo["db_version"]; !exists {
		t.Error("Expected db_version to be populated")
	} else if dbVersion != "7.16.1" {
		t.Errorf("Expected db_version to be '7.16.1', got %q", dbVersion)
	}

	// ovs_version should be "unknown" since socket doesn't exist
	if ovsVersion, exists := systemInfo["ovs_version"]; !exists {
		t.Error("Expected ovs_version to be populated")
	} else if ovsVersion != "unknown" {
		t.Errorf("Expected ovs_version to be 'unknown' when socket fails, got %q", ovsVersion)
	}

	// system_type and system_version should be populated from OS or "unknown"
	if _, exists := systemInfo["system_type"]; !exists {
		t.Error("Expected system_type to be populated")
	}
	if _, exists := systemInfo["system_version"]; !exists {
		t.Error("Expected system_version to be populated")
	}
}
