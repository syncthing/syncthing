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
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.RawCopy())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustConfig(w, r)
	})
}

func (c *configMuxBuilder) registerConfigDeprecated(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.RawCopy())
	})

	c.HandlerFunc(http.MethodPost, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustConfig(w, r)
	})
}

func (c *configMuxBuilder) registerConfigInsync(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, map[string]bool{"configInSync": !c.cfg.RequiresRestart()})
	})
}

func (c *configMuxBuilder) registerFolders(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.FolderList())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		c.mut.Lock()
		defer c.mut.Unlock()
		var folders []config.FolderConfiguration
		if err := unmarshalTo(r.Body, &folders); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		waiter, err := c.cfg.SetFolders(folders)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.finish(w, waiter)
	})

	c.HandlerFunc(http.MethodPost, path, func(w http.ResponseWriter, r *http.Request) {
		c.mut.Lock()
		defer c.mut.Unlock()
		var folder config.FolderConfiguration
		if err := unmarshalTo(r.Body, &folder); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		waiter, err := c.cfg.SetFolder(folder)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.finish(w, waiter)
	})
}

func (c *configMuxBuilder) registerDevices(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.DeviceList())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		c.mut.Lock()
		defer c.mut.Unlock()
		var devices []config.DeviceConfiguration
		if err := unmarshalTo(r.Body, &devices); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		waiter, err := c.cfg.SetDevices(devices)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.finish(w, waiter)
	})

	c.HandlerFunc(http.MethodPost, path, func(w http.ResponseWriter, r *http.Request) {
		c.mut.Lock()
		defer c.mut.Unlock()
		var device config.DeviceConfiguration
		if err := unmarshalTo(r.Body, &device); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		waiter, err := c.cfg.SetDevice(device)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.finish(w, waiter)
	})
}

func (c *configMuxBuilder) registerFolder(path string) {
	c.Handle(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		folder, ok := c.cfg.Folder(p.ByName("id"))
		if !ok {
			http.Error(w, "No folder with given ID", http.StatusNotFound)
			return
		}
		sendJSON(w, folder)
	})

	c.Handle(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c.adjustFolder(w, r, config.FolderConfiguration{})
	})

	c.Handle(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		folder, ok := c.cfg.Folder(p.ByName("id"))
		if !ok {
			http.Error(w, "No folder with given ID", http.StatusNotFound)
			return
		}
		c.adjustFolder(w, r, folder)
	})

	c.Handle(http.MethodDelete, path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		c.mut.Lock()
		defer c.mut.Unlock()
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

	c.Handle(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		if device, ok := deviceFromParams(w, p); ok {
			sendJSON(w, device)
		}
	})

	c.Handle(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c.adjustDevice(w, r, config.DeviceConfiguration{})
	})

	c.Handle(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if device, ok := deviceFromParams(w, p); ok {
			c.adjustDevice(w, r, device)
		}
	})

	c.Handle(http.MethodDelete, path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		c.mut.Lock()
		defer c.mut.Unlock()
		id, err := protocol.DeviceIDFromString(p.ByName("id"))
		waiter, err := c.cfg.RemoveDevice(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.finish(w, waiter)
	})
}

func (c *configMuxBuilder) registerOptions(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.Options())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustOptions(w, r, config.OptionsConfiguration{})
	})

	c.HandlerFunc(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustOptions(w, r, c.cfg.Options())
	})
}

func (c *configMuxBuilder) registerLDAP(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.LDAP())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustLDAP(w, r, config.LDAPConfiguration{})
	})

	c.HandlerFunc(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustLDAP(w, r, c.cfg.LDAP())
	})
}

func (c *configMuxBuilder) registerGUI(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.GUI())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustGUI(w, r, config.GUIConfiguration{})
	})

	c.HandlerFunc(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request) {
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
	if cfg.GUI.Password, err = checkGUIPassword(c.cfg.GUI().Password, cfg.GUI.Password); err != nil {
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
	if err := unmarshalTo(r.Body, &folder); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	waiter, err := c.cfg.SetFolder(folder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustDevice(w http.ResponseWriter, r *http.Request, device config.DeviceConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if err := unmarshalTo(r.Body, &device); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	waiter, err := c.cfg.SetDevice(device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustOptions(w http.ResponseWriter, r *http.Request, opts config.OptionsConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if err := unmarshalTo(r.Body, &opts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	waiter, err := c.cfg.SetOptions(opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustGUI(w http.ResponseWriter, r *http.Request, gui config.GUIConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	oldPassword := gui.Password
	err := unmarshalTo(r.Body, &gui)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if gui.Password, err = checkGUIPassword(oldPassword, gui.Password); err != nil {
		l.Warnln("bcrypting password:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	waiter, err := c.cfg.SetGUI(gui)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustLDAP(w http.ResponseWriter, r *http.Request, ldap config.LDAPConfiguration) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if err := unmarshalTo(r.Body, &ldap); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	waiter, err := c.cfg.SetLDAP(ldap)
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

func checkGUIPassword(oldPassword, newPassword string) (string, error) {
	if newPassword == oldPassword {
		return newPassword, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 0)
	return string(hash), err
}

func (c *configMuxBuilder) finish(w http.ResponseWriter, waiter config.Waiter) {
	waiter.Wait()
	if err := c.cfg.Save(); err != nil {
		l.Warnln("Saving config:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
