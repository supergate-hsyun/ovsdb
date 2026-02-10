// Copyright 2020 Paul Greenberg greenpau@outlook.com
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
	"testing"
)

func TestOvnClientUpdateRefs(t *testing.T) {
	client := NewOvnClient()

	client.Service.Northd.Process.ID = 202
	client.Service.Northd.File.Pid.Path = "/tmp/random-path/ovn-northd.pid"

	client.updateRefs()

	expectedNorthdCtrl := "unix:/tmp/random-path/ovn-northd.202.ctl"
	if client.Service.Northd.Socket.Control != expectedNorthdCtrl {
		t.Errorf("UpdateRefs fail. Expected: %s Ctrl: %s", expectedNorthdCtrl, client.Service.Northd.Socket.Control)
	}
}
