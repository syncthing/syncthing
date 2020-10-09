// Copyright (C) 2020 The Syncthing Authors.
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

	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type configMuxBuilder struct {
	*httprouter.Router
	id  protocol.DeviceID
	cfg config.Wrapper
	mut sync.Mutex
}

func (c *configMuxBuilder) registerConfig(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		sendJSON(w, c.cfg.RawCopy())
	})
	c.PUT(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.adjustConfig(w, r)
	})
}

func (c *configMuxBuilder) registerConfigDeprecated(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		sendJSON(w, c.cfg.RawCopy())
	})
	c.POST(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.adjustConfig(w, r)
	})
}

func (c *configMuxBuilder) registerConfigInsync(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		sendJSON(w, map[string]bool{"configInSync": !c.cfg.RequiresRestart()})
	})
}

func (c *configMuxBuilder) registerFolders(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		sendJSON(w, c.cfg.FolderList())
	})
	c.PUT(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.mut.Lock()
		defer c.mut.Unlock()
		var waiter config.Waiter
		folders := make([]config.FolderConfiguration, 0)
		err := unmarshalTo(r.Body, &folders)
		if err == nil {
			waiter, err = c.cfg.SetFolders(folders)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.finish(w, waiter)
	})
}

func (c *configMuxBuilder) registerDevices(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		sendJSON(w, c.cfg.DeviceList())
	})
	c.PUT(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.mut.Lock()
		defer c.mut.Unlock()
		var waiter config.Waiter
		devices := make([]config.DeviceConfiguration, 0)
		err := unmarshalTo(r.Body, &devices)
		if err == nil {
			waiter, err = c.cfg.SetDevices(devices)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.finish(w, waiter)
	})
}

func (c *configMuxBuilder) registerFolder(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		folder, ok := c.cfg.Folder(p.ByName("id"))
		if !ok {
			http.Error(w, "No folder with given ID", http.StatusNotFound)
			return
		}
		sendJSON(w, folder)
	})
	c.PUT(path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c.adjustFolder(w, r, config.FolderConfiguration{})
	})

	c.PATCH(path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		folder, ok := c.cfg.Folder(p.ByName("id"))
		if !ok {
			http.Error(w, "No folder with given ID", http.StatusNotFound)
			return
		}
		c.adjustFolder(w, r, folder)
	})
	c.DELETE(path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		waiter, err := c.cfg.RemoveFolder(p.ByName("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.finish(w, waiter)
	})
}

func (c *configMuxBuilder) registerDevice(path string) {
	deviceFromParams := func(w http.ResponseWriter, p httprouter.Params) (config.DeviceConfiguration, bool) {
		id, err := protocol.DeviceIDFromString(p.ByName("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return config.DeviceConfiguration{}, false
		}
		device, ok := c.cfg.Device(id)
		if !ok {
			http.Error(w, "No device with given ID", http.StatusNotFound)
			return config.DeviceConfiguration{}, false
		}
		return device, true
	}

	c.GET(path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		if device, ok := deviceFromParams(w, p); ok {
			sendJSON(w, device)
		}
	})
	c.PUT(path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c.adjustDevice(w, r, config.DeviceConfiguration{})
	})
	c.PATCH(path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if device, ok := deviceFromParams(w, p); ok {
			c.adjustDevice(w, r, device)
		}
	})
	c.DELETE(path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		var waiter config.Waiter
		id, err := protocol.DeviceIDFromString(p.ByName("id"))
		if err == nil {
			waiter, err = c.cfg.RemoveDevice(id)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.finish(w, waiter)
	})
}

func (c *configMuxBuilder) registerOptions(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		sendJSON(w, c.cfg.Options())
	})
	c.PUT(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.adjustOptions(w, r, config.OptionsConfiguration{})
	})
	c.PATCH(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.adjustOptions(w, r, c.cfg.Options())
	})
}

func (c *configMuxBuilder) registerLDAP(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		sendJSON(w, c.cfg.LDAP())
	})
	c.PUT(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.adjustLDAP(w, r, config.LDAPConfiguration{})
	})
	c.PATCH(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.adjustLDAP(w, r, c.cfg.LDAP())
	})
}

func (c *configMuxBuilder) registerGUI(path string) {
	c.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		sendJSON(w, c.cfg.GUI())
	})
	c.PUT(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.adjustGUI(w, r, config.GUIConfiguration{})
	})
	c.PATCH(path, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.adjustGUI(w, r, c.cfg.GUI())
	})
}

func (c *configMuxBuilder) adjustConfig(w http.ResponseWriter, r *http.Request) {
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

func (c *configMuxBuilder) adjustFolder(w http.ResponseWriter, r *http.Request, folder config.FolderConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	var waiter config.Waiter
	err := unmarshalTo(r.Body, &folder)
	if err == nil {
		waiter, err = c.cfg.SetFolder(folder)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustDevice(w http.ResponseWriter, r *http.Request, device config.DeviceConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	var waiter config.Waiter
	err := unmarshalTo(r.Body, &device)
	if err == nil {
		waiter, err = c.cfg.SetDevice(device)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustOptions(w http.ResponseWriter, r *http.Request, opts config.OptionsConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	var waiter config.Waiter
	err := unmarshalTo(r.Body, &opts)
	if err == nil {
		waiter, err = c.cfg.SetOptions(opts)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustGUI(w http.ResponseWriter, r *http.Request, newGUI config.GUIConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	oldGUI := c.cfg.GUI()
	err := unmarshalTo(r.Body, &newGUI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

func (c *configMuxBuilder) adjustLDAP(w http.ResponseWriter, r *http.Request, ldap config.LDAPConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	var waiter config.Waiter
	err := unmarshalTo(r.Body, &ldap)
	if err == nil {
		waiter, err = c.cfg.SetLDAP(ldap)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
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

func (c *configMuxBuilder) finish(w http.ResponseWriter, waiter config.Waiter) {
	waiter.Wait()
	if err := c.cfg.Save(); err != nil {
		l.Warnln("Saving config:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
