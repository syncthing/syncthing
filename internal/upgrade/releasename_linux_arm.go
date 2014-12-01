// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package upgrade

import (
	"fmt"
	"strings"
	"syscall"
)

func releaseName(tag string) string {
	return fmt.Sprintf("syncthing-linux-armv%s-%s.", goARM(), tag)
}

// Get the current ARM architecture version for upgrade purposes. If we can't
// figure it out from the uname, default to ARMv6 (same as Go distribution).
func goARM() string {
	var name syscall.Utsname
	syscall.Uname(&name)
	machine := string(name.Machine[:5])
	if strings.HasPrefix(machine, "armv") {
		return machine[4:]
	}
	return "6"
}
