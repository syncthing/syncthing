// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
)

var errSkip = errors.New("notify: skip")

type walkPathFunc func(nd node, isbase bool) error

type walkFunc func(node) error

func errnotexist(name string) error {
	return &os.PathError{
		Op:   "Node",
		Path: name,
		Err:  os.ErrNotExist,
	}
}

type node struct {
	Name  string
	Watch watchpoint
	Child map[string]node
}

func newnode(name string) node {
	return node{
		Name:  name,
		Watch: make(watchpoint),
		Child: make(map[string]node),
	}
}

func (nd node) addchild(name, base string) node {
	child, ok := nd.Child[base]
	if !ok {
		child = newnode(name)
		nd.Child[base] = child
	}
	return child
}

func (nd node) Add(name string) node {
	i := indexbase(nd.Name, name)
	if i == -1 {
		return node{}
	}
	for j := indexSep(name[i:]); j != -1; j = indexSep(name[i:]) {
		nd = nd.addchild(name[:i+j], name[i:i+j])
		i += j + 1
	}
	return nd.addchild(name, name[i:])
}

func (nd node) AddDir(fn walkFunc) error {
	stack := []node{nd}
Traverse:
	for n := len(stack); n != 0; n = len(stack) {
		nd, stack = stack[n-1], stack[:n-1]
		switch err := fn(nd); err {
		case nil:
		case errSkip:
			continue Traverse
		default:
			return &os.PathError{
				Op:   "error while traversing",
				Path: nd.Name,
				Err:  err,
			}
		}
		// TODO(rjeczalik): tolerate open failures - add failed names to
		// AddDirError and notify users which names are not added to the tree.
		fi, err := ioutil.ReadDir(nd.Name)
		if err != nil {
			return err
		}
		for _, fi := range fi {
			if fi.Mode()&(os.ModeSymlink|os.ModeDir) == os.ModeDir {
				name := filepath.Join(nd.Name, fi.Name())
				stack = append(stack, nd.addchild(name, name[len(nd.Name)+1:]))
			}
		}
	}
	return nil
}

func (nd node) Get(name string) (node, error) {
	i := indexbase(nd.Name, name)
	if i == -1 {
		return node{}, errnotexist(name)
	}
	ok := false
	for j := indexSep(name[i:]); j != -1; j = indexSep(name[i:]) {
		if nd, ok = nd.Child[name[i:i+j]]; !ok {
			return node{}, errnotexist(name)
		}
		i += j + 1
	}
	if nd, ok = nd.Child[name[i:]]; !ok {
		return node{}, errnotexist(name)
	}
	return nd, nil
}

func (nd node) Del(name string) error {
	i := indexbase(nd.Name, name)
	if i == -1 {
		return errnotexist(name)
	}
	stack := []node{nd}
	ok := false
	for j := indexSep(name[i:]); j != -1; j = indexSep(name[i:]) {
		if nd, ok = nd.Child[name[i:i+j]]; !ok {
			return errnotexist(name[:i+j])
		}
		stack = append(stack, nd)
	}
	if nd, ok = nd.Child[name[i:]]; !ok {
		return errnotexist(name)
	}
	nd.Child = nil
	nd.Watch = nil
	for name, i = base(nd.Name), len(stack); i != 0; name, i = base(nd.Name), i-1 {
		nd = stack[i-1]
		if nd := nd.Child[name]; len(nd.Watch) > 1 || len(nd.Child) != 0 {
			break
		} else {
			nd.Child = nil
			nd.Watch = nil
		}
		delete(nd.Child, name)
	}
	return nil
}

func (nd node) Walk(fn walkFunc) error {
	stack := []node{nd}
Traverse:
	for n := len(stack); n != 0; n = len(stack) {
		nd, stack = stack[n-1], stack[:n-1]
		switch err := fn(nd); err {
		case nil:
		case errSkip:
			continue Traverse
		default:
			return err
		}
		for name, nd := range nd.Child {
			if name == "" {
				// Node storing inactive watchpoints has empty name, skip it
				// form traversing. Root node has also an empty name, but it
				// never has a parent node.
				continue
			}
			stack = append(stack, nd)
		}
	}
	return nil
}

func (nd node) WalkPath(name string, fn walkPathFunc) error {
	i := indexbase(nd.Name, name)
	if i == -1 {
		return errnotexist(name)
	}
	ok := false
	for j := indexSep(name[i:]); j != -1; j = indexSep(name[i:]) {
		switch err := fn(nd, false); err {
		case nil:
		case errSkip:
			return nil
		default:
			return err
		}
		if nd, ok = nd.Child[name[i:i+j]]; !ok {
			return errnotexist(name[:i+j])
		}
		i += j + 1
	}
	switch err := fn(nd, false); err {
	case nil:
	case errSkip:
		return nil
	default:
		return err
	}
	if nd, ok = nd.Child[name[i:]]; !ok {
		return errnotexist(name)
	}
	switch err := fn(nd, true); err {
	case nil, errSkip:
		return nil
	default:
		return err
	}
}

type root struct {
	nd node
}

func (r root) addroot(name string) node {
	if vol := filepath.VolumeName(name); vol != "" {
		root, ok := r.nd.Child[vol]
		if !ok {
			root = r.nd.addchild(vol, vol)
		}
		return root
	}
	return r.nd
}

func (r root) root(name string) (node, error) {
	if vol := filepath.VolumeName(name); vol != "" {
		nd, ok := r.nd.Child[vol]
		if !ok {
			return node{}, errnotexist(name)
		}
		return nd, nil
	}
	return r.nd, nil
}

func (r root) Add(name string) node {
	return r.addroot(name).Add(name)
}

func (r root) AddDir(dir string, fn walkFunc) error {
	return r.Add(dir).AddDir(fn)
}

func (r root) Del(name string) error {
	nd, err := r.root(name)
	if err != nil {
		return err
	}
	return nd.Del(name)
}

func (r root) Get(name string) (node, error) {
	nd, err := r.root(name)
	if err != nil {
		return node{}, err
	}
	if nd.Name != name {
		if nd, err = nd.Get(name); err != nil {
			return node{}, err
		}
	}
	return nd, nil
}

func (r root) Walk(name string, fn walkFunc) error {
	nd, err := r.Get(name)
	if err != nil {
		return err
	}
	return nd.Walk(fn)
}

func (r root) WalkPath(name string, fn walkPathFunc) error {
	nd, err := r.root(name)
	if err != nil {
		return err
	}
	return nd.WalkPath(name, fn)
}
