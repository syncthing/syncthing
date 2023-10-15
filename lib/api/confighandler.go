// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/structutil"
)

type configMuxBuilder struct {
	*httprouter.Router
	id              protocol.DeviceID
	cfg             config.Wrapper
	webauthnService *webauthnService
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

func (c *configMuxBuilder) registerConfigRequiresRestart(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, map[string]bool{"requiresRestart": c.cfg.RequiresRestart()})
	})
}

func (c *configMuxBuilder) registerFolders(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.FolderList())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		data, err := unmarshalToRawMessages(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		folders := make([]config.FolderConfiguration, len(data))
		defaultFolder := c.cfg.DefaultFolder()
		for i, bs := range data {
			folders[i] = defaultFolder.Copy()
			if err := json.Unmarshal(bs, &folders[i]); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
			cfg.SetFolders(folders)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.finish(w, waiter)
	})

	c.HandlerFunc(http.MethodPost, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustFolder(w, r, c.cfg.DefaultFolder(), false)
	})
}

func (c *configMuxBuilder) registerDevices(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.DeviceList())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		data, err := unmarshalToRawMessages(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		devices := make([]config.DeviceConfiguration, len(data))
		defaultDevice := c.cfg.DefaultDevice()
		for i, bs := range data {
			devices[i] = defaultDevice.Copy()
			if err := json.Unmarshal(bs, &devices[i]); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
			cfg.SetDevices(devices)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.finish(w, waiter)
	})

	c.HandlerFunc(http.MethodPost, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustDevice(w, r, c.cfg.DefaultDevice(), false)
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
		c.adjustFolder(w, r, c.cfg.DefaultFolder(), false)
	})

	c.Handle(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		folder, ok := c.cfg.Folder(p.ByName("id"))
		if !ok {
			http.Error(w, "No folder with given ID", http.StatusNotFound)
			return
		}
		c.adjustFolder(w, r, folder, false)
	})

	c.Handle(http.MethodDelete, path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
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
		c.adjustDevice(w, r, c.cfg.DefaultDevice(), false)
	})

	c.Handle(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if device, ok := deviceFromParams(w, p); ok {
			c.adjustDevice(w, r, device, false)
		}
	})

	c.Handle(http.MethodDelete, path, func(w http.ResponseWriter, _ *http.Request, p httprouter.Params) {
		id, err := protocol.DeviceIDFromString(p.ByName("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		waiter, err := c.cfg.RemoveDevice(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.finish(w, waiter)
	})
}

func (c *configMuxBuilder) registerDefaultFolder(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.DefaultFolder())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		var cfg config.FolderConfiguration
		structutil.SetDefaults(&cfg)
		c.adjustFolder(w, r, cfg, true)
	})

	c.HandlerFunc(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustFolder(w, r, c.cfg.DefaultFolder(), true)
	})
}

func (c *configMuxBuilder) registerDefaultDevice(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.DefaultDevice())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		var cfg config.DeviceConfiguration
		structutil.SetDefaults(&cfg)
		c.adjustDevice(w, r, cfg, true)
	})

	c.HandlerFunc(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustDevice(w, r, c.cfg.DefaultDevice(), true)
	})
}

func (c *configMuxBuilder) registerDefaultIgnores(path string) {
	c.HandlerFunc(http.MethodGet, path, func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, c.cfg.DefaultIgnores())
	})

	c.HandlerFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
		var ignores config.Ignores
		if err := unmarshalTo(r.Body, &ignores); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
			cfg.Defaults.Ignores = ignores
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		var cfg config.OptionsConfiguration
		structutil.SetDefaults(&cfg)
		c.adjustOptions(w, r, cfg)
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
		var cfg config.LDAPConfiguration
		structutil.SetDefaults(&cfg)
		c.adjustLDAP(w, r, cfg)
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
		var cfg config.GUIConfiguration
		structutil.SetDefaults(&cfg)
		c.adjustGUI(w, r, cfg)
	})

	c.HandlerFunc(http.MethodPatch, path, func(w http.ResponseWriter, r *http.Request) {
		c.adjustGUI(w, r, c.cfg.GUI())
	})
}

func (c *configMuxBuilder) registerWebauthnConfig(path string) {
	c.HandlerFunc(http.MethodPost, path+"/register-start", c.webauthnService.startWebauthnRegistration)
	c.HandlerFunc(http.MethodPost, path+"/register-finish", c.webauthnService.finishWebauthnRegistration)
}

func (c *configMuxBuilder) adjustConfig(w http.ResponseWriter, r *http.Request) {
	to, err := config.ReadJSON(r.Body, c.id)
	r.Body.Close()
	if err != nil {
		l.Warnln("Decoding posted config:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var errMsg string
	var status int
	waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
		if to.GUI.Password != cfg.GUI.Password {
			if err := to.GUI.SetPassword(to.GUI.Password); err != nil {
				l.Warnln("hashing password:", err)
				errMsg = err.Error()
				status = http.StatusInternalServerError
				return
			}
		}

		// Don't allow adding new WebAuthn credentials without passing a registration challenge,
		// and only allow updating the Nickname and RequireUv fields
		existingCredentials := make(map[string]config.WebauthnCredential)
		for _, cred := range cfg.GUI.WebauthnCredentials {
			existingCredentials[cred.ID] = cred
		}
		for _, cred := range c.webauthnService.credentialsPendingRegistration {
			existingCredentials[cred.ID] = cred
		}

		var updatedCredentials []config.WebauthnCredential
		for _, newCred := range to.GUI.WebauthnCredentials {
			if exCred, ok := existingCredentials[newCred.ID]; ok {
				exCred.Nickname = newCred.Nickname
				exCred.RequireUv = newCred.RequireUv
				updatedCredentials = append(updatedCredentials, exCred)
			}
		}
		to.GUI.WebauthnCredentials = updatedCredentials

		*cfg = to
	})
	if errMsg != "" {
		http.Error(w, errMsg, status)
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if c.finish(w, waiter) {
		c.webauthnService.credentialsPendingRegistration = make([]config.WebauthnCredential, 0)
	}
}

func (c *configMuxBuilder) adjustFolder(w http.ResponseWriter, r *http.Request, folder config.FolderConfiguration, defaults bool) {
	if err := unmarshalTo(r.Body, &folder); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
		if defaults {
			cfg.Defaults.Folder = folder
		} else {
			cfg.SetFolder(folder)
		}
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustDevice(w http.ResponseWriter, r *http.Request, device config.DeviceConfiguration, defaults bool) {
	if err := unmarshalTo(r.Body, &device); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
		if defaults {
			cfg.Defaults.Device = device
		} else {
			cfg.SetDevice(device)
		}
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustOptions(w http.ResponseWriter, r *http.Request, opts config.OptionsConfiguration) {
	if err := unmarshalTo(r.Body, &opts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
		cfg.Options = opts
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustGUI(w http.ResponseWriter, r *http.Request, gui config.GUIConfiguration) {
	oldPassword := gui.Password
	err := unmarshalTo(r.Body, &gui)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var errMsg string
	var status int
	waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
		if gui.Password != oldPassword {
			if err := gui.SetPassword(gui.Password); err != nil {
				l.Warnln("hashing password:", err)
				errMsg = err.Error()
				status = http.StatusInternalServerError
				return
			}
		}
		cfg.GUI = gui
	})
	if errMsg != "" {
		http.Error(w, errMsg, status)
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

func (c *configMuxBuilder) adjustLDAP(w http.ResponseWriter, r *http.Request, ldap config.LDAPConfiguration) {
	if err := unmarshalTo(r.Body, &ldap); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	waiter, err := c.cfg.Modify(func(cfg *config.Configuration) {
		cfg.LDAP = ldap
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.finish(w, waiter)
}

// Unmarshals the content of the given body and stores it in to (i.e. to must be a pointer).
func unmarshalTo(body io.ReadCloser, to interface{}) error {
	bs, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, to)
}

func unmarshalToRawMessages(body io.ReadCloser) ([]json.RawMessage, error) {
	var data []json.RawMessage
	err := unmarshalTo(body, &data)
	return data, err
}

func awaitSaveConfig(w http.ResponseWriter, wrapper config.Wrapper, waiter config.Waiter) bool {
	waiter.Wait()
	if err := wrapper.Save(); err != nil {
		l.Warnln("Failed to save config:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	return true
}

func (c *configMuxBuilder) finish(w http.ResponseWriter, waiter config.Waiter) bool {
	return awaitSaveConfig(w, c.cfg, waiter)
}
