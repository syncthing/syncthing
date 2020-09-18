// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

const configBase = "/rest/config/"

type configHandler struct {
	*http.ServeMux
	id  protocol.DeviceID
	cfg config.Wrapper
	mut sync.Mutex
}

func newConfigHandler(id protocol.DeviceID, cfg config.Wrapper) http.Handler {
	c := &configHandler{
		ServeMux: http.NewServeMux(),
		id:       id,
		cfg:      cfg,
		mut:      sync.NewMutex(),
	}

	c.HandleFunc(configBase, c.handleConfig)
	c.HandleFunc(configBase+"insync", c.handleConfigInsync)
	c.HandleFunc(configBase+"folders", c.handleFolders)
	c.HandleFunc(configBase+"folders/", c.handleFolder)
	c.HandleFunc(configBase+"devices", c.handleDevices)
	c.HandleFunc(configBase+"devices/", c.handleDevice)
	c.HandleFunc(configBase+"options", c.handleOptions)
	c.HandleFunc(configBase+"ldap", c.handleLDAP)
	c.HandleFunc(configBase+"gui", c.handleGUI)

	// Legacy
	c.HandleFunc("/rest/system/config", c.handleConfig)
	c.HandleFunc("/rest/system/config/insync", c.handleConfigInsync)

	return c
}

func (c *configHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		sendJSON(w, c.cfg.RawCopy())
		return
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	cfg, err := config.ReadJSON(r.Body, c.id)
	r.Body.Close()
	if err != nil {
		l.Warnln("Decoding posted config:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cfg.GUI.Password, err = checkGUIPassword(c.cfg.GUI(), cfg.GUI); err != nil {
		l.Warnln("bcrypting password:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	waiter, err := c.cfg.Replace(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configHandler) handleConfigInsync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sendJSON(w, map[string]bool{"configInSync": !c.cfg.RequiresRestart()})
}

func (c *configHandler) handleFolders(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		sendJSON(w, c.cfg.FolderList())
		return
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	folders := make([]config.FolderConfiguration, 0)
	if err := unmarshalTo(r.Body, &folders); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	waiter, err := c.cfg.SetFolders(folders)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configHandler) handleFolder(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPatch, http.MethodDelete) {
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, configBase+"folders/")
	var folder config.FolderConfiguration
	switch r.Method {
	case http.MethodGet, http.MethodPatch:
		var ok bool
		folder, ok = c.cfg.Folder(suffix)
		if !ok {
			http.Error(w, "No folder with given ID", http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet {
			sendJSON(w, folder)
			return
		}
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	var err error
	var waiter config.Waiter
	if r.Method == http.MethodDelete {
		waiter, err = c.cfg.RemoveFolder(suffix)
	} else if err := unmarshalTo(r.Body, &folder); err == nil {
		waiter, err = c.cfg.SetFolder(folder)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configHandler) handleDevices(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r) {
		return
	}
	if r.Method == http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodGet {
		sendJSON(w, c.cfg.DeviceList())
		return
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	devices := make([]config.DeviceConfiguration, 0)
	if err := unmarshalTo(r.Body, &devices); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	waiter, err := c.cfg.SetDevices(devices)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configHandler) handleDevice(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPatch, http.MethodDelete) {
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, configBase+"devices/")
	var device config.DeviceConfiguration
	var err error
	var id protocol.DeviceID
	switch r.Method {
	case http.MethodGet, http.MethodPatch, http.MethodDelete:
		id, err = protocol.DeviceIDFromString(suffix)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodDelete {
			break
		}
		var ok bool
		device, ok = c.cfg.Device(id)
		if !ok {
			http.Error(w, "No device with given ID", http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet {
			sendJSON(w, device)
			return
		}
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	var waiter config.Waiter
	if r.Method == http.MethodDelete {
		waiter, err = c.cfg.RemoveDevice(id)
	} else if err := unmarshalTo(r.Body, &device); err == nil {
		waiter, err = c.cfg.SetDevice(device)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configHandler) handleOptions(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPatch) {
		return
	}
	if r.Method == http.MethodGet {
		sendJSON(w, c.cfg.Options())
		return
	}
	c.mut.Lock()
	defer c.mut.Unlock()

	var opts config.OptionsConfiguration
	if r.Method == http.MethodPatch {
		opts = c.cfg.Options()
	}
	if err := unmarshalTo(r.Body, &opts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	waiter, err := c.cfg.SetOptions(opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configHandler) handleGUI(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPatch) {
		return
	}
	if r.Method == http.MethodGet {
		sendJSON(w, c.cfg.GUI())
		return
	}
	c.mut.Lock()
	defer c.mut.Unlock()

	oldGUI := c.cfg.GUI()
	var newGUI config.GUIConfiguration
	if r.Method == http.MethodPatch {
		newGUI = oldGUI.Copy()
	}
	if err := unmarshalTo(r.Body, &newGUI); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var err error
	if newGUI.Password, err = checkGUIPassword(oldGUI, newGUI); err != nil {
		l.Warnln("bcrypting password:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	waiter, err := c.cfg.SetGUI(newGUI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configHandler) handleLDAP(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPatch) {
		return
	}
	if r.Method == http.MethodGet {
		sendJSON(w, c.cfg.LDAP())
		return
	}
	c.mut.Lock()
	defer c.mut.Unlock()

	var ldap config.LDAPConfiguration
	if r.Method == http.MethodPatch {
		ldap = c.cfg.LDAP()
	}
	if err := unmarshalTo(r.Body, &ldap); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	waiter, err := c.cfg.SetLDAP(ldap)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func checkMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range append(methods, http.MethodGet, http.MethodPost, http.MethodPut) {
		if r.Method == m {
			return true
		}
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	return false
}

// Unmarshals the content of the given body and stores it in to (i.e. to must be a pointer).
func unmarshalTo(body io.ReadCloser, to interface{}) error {
	bs, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, to)
}

func checkGUIPassword(from, to config.GUIConfiguration) (string, error) {
	if to.Password == from.Password {
		return to.Password, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(to.Password), 0)
	return string(hash), err
}

func (c *configHandler) finish(w http.ResponseWriter, waiter config.Waiter) {
	waiter.Wait()
	if err := c.cfg.Save(); err != nil {
		l.Warnln("Saving config:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
