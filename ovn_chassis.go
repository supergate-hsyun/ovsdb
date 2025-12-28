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
	"fmt"
	"net"
)

// OvnChassis represent an OVN chassis.
type OvnChassis struct {
	UUID      string
	Name      string
	IPAddress net.IP
	Encaps    struct {
		UUID  string
		Proto string
	}
	NbCfg          int64 // Configuration sequence number from Chassis_Private table (0 if not present)
	NbCfgTimestamp int64 // Timestamp from Chassis_Private table (0 if not present)
	Ports          []string
	Switches       []string
}

// GetChassis returns a list of OVN chassis.
func (cli *OvnClient) GetChassis() ([]*OvnChassis, error) {
	chassis := []*OvnChassis{}
	// First, get the names and UUIDs of chassis.
	query := "SELECT _uuid, name, encaps FROM Chassis"
	result, err := cli.Database.Southbound.Client.Transact(cli.Database.Southbound.Name, query)
	if err != nil {
		return nil, fmt.Errorf("%s: '%s' table error: %s", cli.Database.Southbound.Name, "Chassis", err)
	}
	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("%s: no chassis found", cli.Database.Southbound.Name)
	}
	for _, row := range result.Rows {
		c := &OvnChassis{}
		c.Ports = []string{}
		c.Switches = []string{}
		if r, dt, err := row.GetColumnValue("_uuid", result.Columns); err != nil {
			continue
		} else {
			if dt != "string" {
				continue
			}
			c.UUID = r.(string)
		}
		if r, dt, err := row.GetColumnValue("name", result.Columns); err != nil {
			continue
		} else {
			if dt != "string" {
				continue
			}
			c.Name = r.(string)
		}
		if r, dt, err := row.GetColumnValue("encaps", result.Columns); err != nil {
			continue
		} else {
			if dt != "string" {
				continue
			}
			c.Encaps.UUID = r.(string)
		}
		chassis = append(chassis, c)
	}

	// Second, get the IP addresses of the chassis
	query = "SELECT _uuid, chassis_name, ip, type FROM Encap"
	result, err = cli.Database.Southbound.Client.Transact(cli.Database.Southbound.Name, query)
	if err != nil {
		return nil, fmt.Errorf("%s: '%s' table error: %s", cli.Database.Southbound.Name, "Encap", err)
	}
	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("%s: no chassis found", cli.Database.Southbound.Name)
	}
	for _, row := range result.Rows {
		var encapUUID string
		var encapProto string
		var chassisName string
		var chassisIPAddress string
		if r, dt, err := row.GetColumnValue("_uuid", result.Columns); err != nil {
			continue
		} else {
			if dt != "string" {
				continue
			}
			encapUUID = r.(string)
		}
		if r, dt, err := row.GetColumnValue("type", result.Columns); err != nil {
			continue
		} else {
			if dt != "string" {
				continue
			}
			encapProto = r.(string)
		}
		if r, dt, err := row.GetColumnValue("chassis_name", result.Columns); err != nil {
			continue
		} else {
			if dt != "string" {
				continue
			}
			chassisName = r.(string)
		}
		if r, dt, err := row.GetColumnValue("ip", result.Columns); err != nil {
			continue
		} else {
			if dt != "string" {
				continue
			}
			chassisIPAddress = r.(string)
		}
		for _, c := range chassis {
			if c.Encaps.UUID != encapUUID {
				continue
			}
			if c.Name != chassisName {
				continue
			}
			c.IPAddress = net.ParseIP(chassisIPAddress)
			c.Encaps.Proto = encapProto
			break
		}
	}

	query = "SELECT chassis, name, nb_cfg, nb_cfg_timestamp FROM Chassis_Private"
	result, err = cli.Database.Southbound.Client.Transact(cli.Database.Southbound.Name, query)
	if err != nil {
		return chassis, nil
	}

	// Create maps for chassis nb_cfg and nb_cfg_timestamp
	chassisNbCfgMap := make(map[string]int64)
	chassisTimestampMap := make(map[string]int64)
	if len(result.Rows) > 0 {
		for _, row := range result.Rows {
			var chassisUUID string
			var chassisName string
			var nbCfg int64
			var nbCfgTimestamp int64

			// Get chassis UUID (reference to Chassis table)
			if r, dt, err := row.GetColumnValue("chassis", result.Columns); err == nil && dt == "string" {
				chassisUUID = r.(string)
			}

			// Get chassis name
			if r, dt, err := row.GetColumnValue("name", result.Columns); err == nil && dt == "string" {
				chassisName = r.(string)
			}

			// Get the nb_cfg
			if r, dt, err := row.GetColumnValue("nb_cfg", result.Columns); err == nil {
				switch dt {
				case "int64":
					nbCfg = r.(int64)
				case "integer":
					// GetColumnValue returns "integer" for float64 values after converting to int64
					nbCfg = r.(int64)
				case "float64":
					nbCfg = int64(r.(float64))
				case "int":
					nbCfg = int64(r.(int))
				}
			}

			// Get the nb_cfg_timestamp
			if r, dt, err := row.GetColumnValue("nb_cfg_timestamp", result.Columns); err == nil {
				switch dt {
				case "int64":
					nbCfgTimestamp = r.(int64)
				case "integer":
					// GetColumnValue returns "integer" for float64 values after converting to int64
					nbCfgTimestamp = r.(int64)
				case "float64":
					nbCfgTimestamp = int64(r.(float64))
				case "int":
					nbCfgTimestamp = int64(r.(int))
				}
			}

			// Store values by both UUID and name
			if chassisUUID != "" {
				chassisNbCfgMap[chassisUUID] = nbCfg
				chassisTimestampMap[chassisUUID] = nbCfgTimestamp
			}
			if chassisName != "" {
				chassisNbCfgMap[chassisName] = nbCfg
				chassisTimestampMap[chassisName] = nbCfgTimestamp
			}
		}
	}

	// Set the NbCfg and NbCfgTimestamp fields for each chassis
	// Will be 0 if chassis has no entry in Chassis_Private
	for _, c := range chassis {
		if nbCfg, exists := chassisNbCfgMap[c.UUID]; exists {
			c.NbCfg = nbCfg
		} else if nbCfg, exists := chassisNbCfgMap[c.Name]; exists {
			c.NbCfg = nbCfg
		}

		if timestamp, exists := chassisTimestampMap[c.UUID]; exists {
			c.NbCfgTimestamp = timestamp
		} else if timestamp, exists := chassisTimestampMap[c.Name]; exists {
			c.NbCfgTimestamp = timestamp
		}
		// If no entry found, NbCfg and NbCfgTimestamp remain 0 (default)
	}

	return chassis, nil
}

// MapPortToChassis updates logical switch ports with the entries from the
// chassis associated with the ports.
func (cli *OvnClient) MapPortToChassis(vteps []*OvnChassis, logicalSwitchPorts []*OvnLogicalSwitchPort) {
	portMap := make(map[string]*OvnChassis)
	switchMap := make(map[string]bool)
	for _, vtep := range vteps {
		portMap[vtep.UUID] = vtep
	}
	for _, logicalSwitchPort := range logicalSwitchPorts {
		if _, exists := portMap[logicalSwitchPort.ChassisUUID]; !exists {
			continue
		}
		logicalSwitchPort.Encapsulation = portMap[logicalSwitchPort.ChassisUUID].Encaps.Proto
		logicalSwitchPort.ChassisIPAddress = portMap[logicalSwitchPort.ChassisUUID].IPAddress
		portMap[logicalSwitchPort.ChassisUUID].Ports = append(portMap[logicalSwitchPort.ChassisUUID].Ports, logicalSwitchPort.UUID)
		if _, exists := switchMap[logicalSwitchPort.LogicalSwitchUUID]; !exists {
			switchMap[logicalSwitchPort.LogicalSwitchUUID] = true
			portMap[logicalSwitchPort.ChassisUUID].Switches = append(portMap[logicalSwitchPort.ChassisUUID].Switches, logicalSwitchPort.LogicalSwitchUUID)
		}
	}
}
