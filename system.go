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
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// OvsDataFile stores information about the files related to OVS
// operations, e.g. log files, database files, etc.
type OvsDataFile struct {
	Path      string
	Component string
	Info      os.FileInfo
	Reader    struct {
		Offset int64
	}
}

// OvsDaemon stores information about a process or database, together
// with associated log and process id files.
type OvsDaemon struct {
	File struct {
		Log OvsDataFile
		Pid OvsDataFile
	}
	Process OvsProcess
	Socket  struct {
		Control string
	}
}

// GetSystemID TODO
func (cli *OvsClient) GetSystemID() error {
	systemID, err := getSystemID(cli.Database.Vswitch.Client, cli.Database.Vswitch.Name, cli.Database.Vswitch.File.SystemID.Path)
	if err != nil {
		return err
	}
	cli.System.ID = systemID
	return nil
}

func getSystemID(client *Client, dbName string, filepath string) (string, error) {
	var systemID string
	var dbErr error

	// First, try to query database if client is provided
	if client != nil && dbName != "" {
		query := fmt.Sprintf("SELECT external_ids FROM %s", dbName)
		result, err := client.Transact(dbName, query)
		if err == nil && len(result.Rows) > 0 {
			col := "external_ids"
			rowData, dataType, err := result.Rows[0].GetColumnValue(col, result.Columns)
			if err == nil && dataType == "map[string]string" {
				externalIDs := rowData.(map[string]string)
				if dbSystemID, exists := externalIDs["system-id"]; exists && dbSystemID != "" {
					systemID = dbSystemID
					if len(systemID) > 253 {
						return systemID, fmt.Errorf("system-id is greater than what the exporter currently allows: %d vs 253", len(systemID))
					}
					return systemID, nil
				}
			}
		} else if err != nil {
			dbErr = err
		}
	}

	// Fallback to reading from file
	file, err := os.Open(filepath)
	if err != nil {
		// If we also had a database error, return both
		if dbErr != nil {
			return "", fmt.Errorf("failed to get system-id from database (%s) and file (%s)", dbErr, err)
		}
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		systemID = scanner.Text()
		break
	}
	if err := scanner.Err(); err != nil {
		if dbErr != nil {
			return "", fmt.Errorf("failed to get system-id from database (%s) and file (%s)", dbErr, err)
		}
		return "", err
	}
	// vswitch.ovsschema does not limit system IDs to a particular length and a common
	// ID to use is UUID (36 bytes). However, some tools use FQDNs for system-ids which
	// are limited to 253 octets per RFC1035. Hence the current limit checked by the
	// exporter is 253 bytes to avoid arbitrary length for system IDs and to have a sane limit.
	if len(systemID) > 253 {
		return systemID, fmt.Errorf("system-id is greater than what the exporter currently allows: %d vs 253", len(systemID))
	}
	return systemID, nil
}

func getVersionViaAppctl(sock string, timeout int) (string, error) {
	cmd := "version"
	app, err := NewClient(sock, timeout)
	if err != nil {
		return "", fmt.Errorf("failed to connect to socket %s: %s", sock, err)
	}
	r, err := app.query(cmd, nil)
	app.Close()
	if err != nil {
		return "", fmt.Errorf("the '%s' command failed: %s", cmd, err)
	}
	response := r.String()
	if response == "" {
		return "", fmt.Errorf("the '%s' command returned no data", cmd)
	}
	return response, nil
}

func parseOvsVersion(versionStr string) string {
	// Parse version from string like "ovs-vswitchd (Open vSwitch) 3.5.1"
	re := regexp.MustCompile(`\(Open vSwitch\)\s+([\d.]+)`)
	matches := re.FindStringSubmatch(versionStr)
	if len(matches) > 1 {
		return matches[1]
	}
	// Fallback: return the whole string
	return strings.TrimSpace(versionStr)
}

func getSystemInfoFromOS() (string, string) {
	// Read /etc/os-release to get system type and version
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "", ""
	}
	defer file.Close()

	var systemType, systemVersion string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			systemType = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		} else if strings.HasPrefix(line, "VERSION_ID=") {
			systemVersion = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}
	return systemType, systemVersion
}

func populateVersionFromAppctl(systemInfo map[string]string, sock string, timeout int, schema *Schema) {
	// Get OVS version via ovs-appctl if missing from DB
	if val, exists := systemInfo["ovs_version"]; !exists || val == "" {
		versionStr, err := getVersionViaAppctl(sock, timeout)
		if err == nil {
			systemInfo["ovs_version"] = parseOvsVersion(versionStr)
		} else {
			systemInfo["ovs_version"] = "unknown"
		}
	}

	// Get DB version from schema if missing from DB
	if val, exists := systemInfo["db_version"]; !exists || val == "" {
		if schema != nil && schema.Version != "" {
			systemInfo["db_version"] = schema.Version
		} else {
			systemInfo["db_version"] = "unknown"
		}
	}

	// Get system type and version from /etc/os-release if missing from DB
	if val, exists := systemInfo["system_type"]; !exists || val == "" {
		systemType, systemVersion := getSystemInfoFromOS()
		if systemType != "" {
			systemInfo["system_type"] = systemType
		} else {
			systemInfo["system_type"] = "unknown"
		}
		if systemVersion != "" {
			systemInfo["system_version"] = systemVersion
		} else {
			systemInfo["system_version"] = "unknown"
		}
	} else if val, exists := systemInfo["system_version"]; !exists || val == "" {
		_, systemVersion := getSystemInfoFromOS()
		if systemVersion != "" {
			systemInfo["system_version"] = systemVersion
		} else {
			systemInfo["system_version"] = "unknown"
		}
	}
}

func parseSystemInfo(systemID string, result Result) (map[string]string, error) {
	systemInfo := make(map[string]string)
	for _, row := range result.Rows {
		col := "external_ids"
		rowData, dataType, err := row.GetColumnValue(col, result.Columns)
		if err != nil {
			return systemInfo, fmt.Errorf("parsing '%s' failed: %s", col, err)
		}
		if dataType != "map[string]string" {
			return systemInfo, fmt.Errorf("data type '%s' for '%s' column is unexpected in this context", dataType, col)
		}
		systemInfo = rowData.(map[string]string)
		columns := []string{"ovs_version", "db_version", "system_type", "system_version"}
		for _, col := range columns {
			rowData, dataType, err = row.GetColumnValue(col, result.Columns)
			if err != nil {
				return systemInfo, fmt.Errorf("parsing '%s' failed: %s", col, err)
			}
			switch dataType {
			case "string":
				systemInfo[col] = rowData.(string)
			case "[]string":
				arr := rowData.([]string)
				if len(arr) > 0 {
					systemInfo[col] = arr[0]
				} else {
					systemInfo[col] = ""
				}
			default:
				return systemInfo, fmt.Errorf("data type '%s' for '%s' column is unexpected in this context", dataType, col)
			}
		}
		break //nolint:staticcheck
	}
	if dbSystemID, exists := systemInfo["system-id"]; exists {
		if dbSystemID != systemID {
			return systemInfo, fmt.Errorf("found 'system-id' mismatch %s (db) vs. %s (config)", dbSystemID, systemID)
		}
	} else {
		return systemInfo, fmt.Errorf("no 'system-id' found")
	}
	// Set defaults for optional keys that may not be in external_ids
	if _, exists := systemInfo["rundir"]; !exists {
		systemInfo["rundir"] = "/var/run/openvswitch"
	}
	// Only hostname is truly required
	requiredKeys := []string{"hostname"}
	for _, key := range requiredKeys {
		if _, exists := systemInfo[key]; !exists {
			return systemInfo, fmt.Errorf("no mandatory '%s' found", key)
		}
	}
	return systemInfo, nil
}

// GetSystemInfo returns a hash containing system information, e.g. `system_id`
// associated with the Open_vSwitch database.
func (cli *OvsClient) GetSystemInfo() error {
	// Get system-id (tries database first, falls back to file)
	systemID, err := getSystemID(cli.Database.Vswitch.Client, cli.Database.Vswitch.Name, cli.Database.Vswitch.File.SystemID.Path)
	if err != nil {
		return err
	}

	query := fmt.Sprintf("SELECT ovs_version, db_version, system_type, system_version, external_ids FROM %s", cli.Database.Vswitch.Name)
	result, err := cli.Database.Vswitch.Client.Transact(cli.Database.Vswitch.Name, query)
	if err != nil {
		return fmt.Errorf("The '%s' query failed: %s", query, err)
	}
	if len(result.Rows) == 0 {
		return fmt.Errorf("The '%s' query did not return any rows", query)
	}
	systemInfo, err := parseSystemInfo(systemID, result)
	if err != nil {
		return fmt.Errorf("The '%s' query returned results but erred: %s", query, err)
	}
	// Get schema for db_version
	schema, _ := cli.Database.Vswitch.Client.GetSchema(cli.Database.Vswitch.Name)
	// Ensure PID is read and socket path is updated before using control socket
	if cli.Database.Vswitch.Process.ID == 0 {
		p, pidErr := getProcessInfoFromFile(cli.Database.Vswitch.File.Pid.Path)
		if pidErr == nil {
			cli.Database.Vswitch.Process = p
		}
	}
	cli.updateRefs()
	// Query version information via ovs-appctl for fields not in DB (OVS 3.x+)
	populateVersionFromAppctl(systemInfo, cli.Database.Vswitch.Socket.Control, cli.Timeout, &schema)
	cli.System.ID = systemInfo["system-id"]
	cli.System.RunDir = systemInfo["rundir"]
	cli.System.Hostname = systemInfo["hostname"]
	cli.System.Type = systemInfo["system_type"]
	cli.System.Version = systemInfo["system_version"]
	cli.Database.Vswitch.Version = systemInfo["ovs_version"]
	cli.Database.Vswitch.Schema.Version = systemInfo["db_version"]
	return nil
}
