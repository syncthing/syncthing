// Copyright 2012 Jesse van den Kieboom. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flags

import (
	"errors"
	"strings"
)

// ErrNotPointerToStruct indicates that a provided data container is not
// a pointer to a struct. Only pointers to structs are valid data containers
// for options.
var ErrNotPointerToStruct = errors.New("provided data is not a pointer to struct")

// Group represents an option group. Option groups can be used to logically
// group options together under a description. Groups are only used to provide
// more structure to options both for the user (as displayed in the help message)
// and for you, since groups can be nested.
type Group struct {
	// A short description of the group. The
	// short description is primarily used in the builtin generated help
	// message
	ShortDescription string

	// A long description of the group. The long
	// description is primarily used to present information on commands
	// (Command embeds Group) in the builtin generated help and man pages.
	LongDescription string

	// All the options in the group
	options []*Option

	// All the subgroups
	groups []*Group

	data interface{}
}

// AddGroup adds a new group to the command with the given name and data. The
// data needs to be a pointer to a struct from which the fields indicate which
// options are in the group.
func (g *Group) AddGroup(shortDescription string, longDescription string, data interface{}) (*Group, error) {
	group := newGroup(shortDescription, longDescription, data)

	if err := group.scan(); err != nil {
		return nil, err
	}

	g.groups = append(g.groups, group)
	return group, nil
}

// Groups returns the list of groups embedded in this group.
func (g *Group) Groups() []*Group {
	return g.groups
}

// Options returns the list of options in this group.
func (g *Group) Options() []*Option {
	return g.options
}

// Find locates the subgroup with the given short description and returns it.
// If no such group can be found Find will return nil. Note that the description
// is matched case insensitively.
func (g *Group) Find(shortDescription string) *Group {
	lshortDescription := strings.ToLower(shortDescription)

	var ret *Group

	g.eachGroup(func(gg *Group) {
		if gg != g && strings.ToLower(gg.ShortDescription) == lshortDescription {
			ret = gg
		}
	})

	return ret
}
