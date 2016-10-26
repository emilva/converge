// Copyright © 2016 Asteris, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package user

import (
	"fmt"
	"math"

	"github.com/asteris-llc/converge/load/registry"
	"github.com/asteris-llc/converge/resource"
)

// Preparer for User
//
// User renders user data
type Preparer struct {
	// Username is the user login name.
	Username string `hcl:"username" required:"true"`

	// NewUsername is used when modifying a user.
	// Username will be changed to NewUsername. No changes to the home directory
	// name or location of the contents will be made. This can be done using
	// HomeDir and MoveDir options.
	NewUsername string `hcl:"new_username"`

	// UID is the user ID.
	UID *uint32 `hcl:"uid"`

	// GroupName is the primary group for user and must already exist.
	// Only one of GID or Groupname may be indicated.
	GroupName string `hcl:"groupname" mutually_exclusive:"gid,groupname"`

	// Gid is the primary group ID for user and must refer to an existing group.
	// Only one of GID or Groupname may be indicated.
	GID *uint32 `hcl:"gid" mutually_exclusive:"gid,groupname"`

	// Name is the user description.
	// This field can be indicated when adding or modifying a user.
	Name string `hcl:"name"`

	// HomeDir is the user's login directory. By default, the login
	// name is appended to the home directory.
	// This field can be indicated when adding or modifying a user.
	HomeDir string `hcl:"home_dir"`

	// MoveDir is used to move the contents of HomeDir when modifying a user.
	// HomeDir must also be indicated if MoveDir is set to true.
	MoveDir bool `hcl:"move_dir"`

	// State is whether the user should be present.
	// The default value is present.
	State State `hcl:"state" valid_values:"present,absent"`
}

// Prepare a new task
func (p *Preparer) Prepare(render resource.Renderer) (resource.Task, error) {
	if p.UID != nil && *p.UID == math.MaxUint32 {
		// the maximum uid on linux is MaxUint32 - 1
		return nil, fmt.Errorf("user \"uid\" parameter out of range")
	}

	if p.GID != nil && *p.GID == math.MaxUint32 {
		// the maximum gid on linux is MaxUint32 - 1
		return nil, fmt.Errorf("user \"gid\" parameter out of range")
	}

	if p.MoveDir && p.HomeDir == "" {
		return nil, fmt.Errorf("user \"home_dir\" parameter required with \"move_dir\" parameter")
	}

	if p.State == "" {
		p.State = StatePresent
	}

	usr := NewUser(new(System))
	usr.Username = p.Username
	usr.NewUsername = p.NewUsername
	usr.GroupName = p.GroupName
	usr.Name = p.Name
	usr.HomeDir = p.HomeDir
	usr.MoveDir = p.MoveDir
	usr.State = p.State

	if p.UID != nil {
		usr.UID = fmt.Sprintf("%v", *p.UID)
	}

	if p.GID != nil {
		usr.GID = fmt.Sprintf("%v", *p.GID)
	}

	return usr, nil
}

func init() {
	registry.Register("user.user", (*Preparer)(nil), (*User)(nil))
}
