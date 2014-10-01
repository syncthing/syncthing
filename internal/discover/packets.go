// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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

package discover

const (
	AnnouncementMagic = 0x9D79BC39
	QueryMagic        = 0x2CA856F5
)

type Query struct {
	Magic    uint32
	DeviceID []byte // max:32
}

type Announce struct {
	Magic uint32
	This  Device
	Extra []Device // max:16
}

type Device struct {
	ID        []byte    // max:32
	Addresses []Address // max:16
}

type Address struct {
	IP   []byte // max:16
	Port uint16
}
