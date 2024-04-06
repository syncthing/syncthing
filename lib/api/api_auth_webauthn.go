// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"reflect"
	"time"

	webauthnProtocol "github.com/go-webauthn/webauthn/protocol"
	webauthnLib "github.com/go-webauthn/webauthn/webauthn"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
)

func newWebauthnEngine(cfg config.Wrapper) (*webauthnLib.WebAuthn, error) {
	guiCfg := cfg.GUI()

	displayName := "Syncthing"
	if dev, ok := cfg.Device(cfg.MyID()); ok && dev.Name != "" {
		displayName = "Syncthing @ " + dev.Name
	}

	rpId := guiCfg.WebauthnRpId
	if rpId == "" {
		guiCfgStruct := reflect.TypeOf(guiCfg)
		field, found := guiCfgStruct.FieldByName("WebauthnRpId")
		if !found {
			return nil, fmt.Errorf(`Field "WebauthnRpId" not found in struct GUIConfiguration`)
		}
		rpId = field.Tag.Get("default")
		if rpId == "" {
			return nil, fmt.Errorf(`Default tag not found on field "WebauthnRpId" in struct GUIConfiguration`)
		}
	}

	origin := guiCfg.WebauthnOrigin
	if origin == "" {
		guiCfgStruct := reflect.TypeOf(guiCfg)
		field, found := guiCfgStruct.FieldByName("WebauthnOrigin")
		if !found {
			return nil, fmt.Errorf(`Field "WebauthnOrigin" not found in struct GUIConfiguration`)
		}
		origin = field.Tag.Get("default")
		if origin == "" {
			return nil, fmt.Errorf(`Default tag not found on field "WebauthnOrigin" in struct GUIConfiguration`)
		}
	}

	return webauthnLib.New(&webauthnLib.Config{
		RPDisplayName: displayName,
		RPID:          rpId,
		RPOrigins:     []string{origin},
	})
}

type webauthnService struct {
	tokenCookieManager             *tokenCookieManager
	engine                         *webauthnLib.WebAuthn
	cfg                            config.Wrapper
	evLogger                       events.Logger
	userHandle                     []byte
	registrationState              webauthnLib.SessionData
	authenticationState            webauthnLib.SessionData
	credentialsPendingRegistration []config.WebauthnCredential
}

func newWebauthnService(tokenCookieManager *tokenCookieManager, cfg config.Wrapper, evLogger events.Logger) (webauthnService, error) {
	engine, err := newWebauthnEngine(cfg)
	if err != nil {
		return webauthnService{}, err
	}

	userHandle, err := base64.URLEncoding.DecodeString(cfg.GUI().WebauthnUserId)
	if err != nil {
		return webauthnService{}, err
	}

	return webauthnService{
		tokenCookieManager: tokenCookieManager,
		engine:             engine,
		cfg:                cfg,
		evLogger:           evLogger,
		userHandle:         userHandle,
	}, nil
}

func (s *webauthnService) WebAuthnID() []byte {
	return s.userHandle
}

func (s *webauthnService) WebAuthnName() string {
	return s.cfg.GUI().User
}

func (s *webauthnService) WebAuthnDisplayName() string {
	return s.cfg.GUI().User
}

func (s *webauthnService) WebAuthnIcon() string {
	return ""
}

func (s *webauthnService) WebAuthnCredentials() []webauthnLib.Credential {
	var result []webauthnLib.Credential
	for _, cred := range s.cfg.GUI().EligibleWebAuthnCredentials() {
		id, err := base64.URLEncoding.DecodeString(cred.ID)
		if err != nil {
			l.Warnln(fmt.Sprintf("Failed to base64url-decode ID of WebAuthn credential \"%s\": %s", cred.Nickname, cred.ID), err)
			continue
		}

		pubkey, err := base64.URLEncoding.DecodeString(cred.PublicKeyCose)
		if err != nil {
			l.Warnln(fmt.Sprintf("Failed to base64url-decode public key of WebAuthn credential \"%s\" (%s)", cred.Nickname, cred.ID), err)
			continue
		}

		transports := make([]webauthnProtocol.AuthenticatorTransport, len(cred.Transports))
		for i, t := range cred.Transports {
			transports[i] = webauthnProtocol.AuthenticatorTransport(t)
		}

		result = append(result, webauthnLib.Credential{
			ID:        id,
			PublicKey: pubkey,
			Authenticator: webauthnLib.Authenticator{
				SignCount: cred.SignCount,
			},
			Transport: transports,
		})
	}
	return result
}

func (s *webauthnService) startWebauthnRegistration(w http.ResponseWriter, r *http.Request) {
	options, sessionData, err := s.engine.BeginRegistration(s)
	if err != nil {
		l.Warnln("Failed to initiate WebAuthn registration:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.registrationState = *sessionData

	sendJSON(w, options)
}

func (s *webauthnService) finishWebauthnRegistration(w http.ResponseWriter, r *http.Request) {
	state := s.registrationState
	s.registrationState = webauthnLib.SessionData{} // Allow only one attempt per challenge

	credential, err := s.engine.FinishRegistration(s, state, r)
	if err != nil {
		l.Infoln("Failed to register WebAuthn credential:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	transports := make([]string, len(credential.Transport))
	for i, t := range credential.Transport {
		transports[i] = string(t)
	}

	now := time.Now().Truncate(time.Second)
	configCred := config.WebauthnCredential{
		ID:            base64.URLEncoding.EncodeToString(credential.ID),
		RpId:          s.engine.Config.RPID,
		PublicKeyCose: base64.URLEncoding.EncodeToString(credential.PublicKey),
		SignCount:     credential.Authenticator.SignCount,
		Transports:    transports,
		CreateTime:    now,
		LastUseTime:   now,
	}
	s.credentialsPendingRegistration = append(s.credentialsPendingRegistration, configCred)

	sendJSON(w, configCred)
}

func (s *webauthnService) startWebauthnAuthentication(w http.ResponseWriter, r *http.Request) {
	allRequireUv := true
	someRequiresUv := false
	for _, cred := range s.cfg.GUI().WebauthnCredentials {
		if cred.RequireUv {
			someRequiresUv = true
		} else {
			allRequireUv = false
		}
	}
	uv := webauthnProtocol.VerificationDiscouraged
	if allRequireUv {
		uv = webauthnProtocol.VerificationRequired
	} else if someRequiresUv {
		uv = webauthnProtocol.VerificationPreferred
	}

	options, sessionData, err := s.engine.BeginLogin(s, webauthnLib.WithUserVerification(uv))
	if err != nil {
		badRequest, ok := err.(*webauthnProtocol.Error)
		if ok && badRequest.Type == "invalid_request" && badRequest.Details == "Found no credentials for user" {
			sendJSON(w, make(map[string]string))
		} else {
			l.Warnln("Failed to initialize WebAuthn login", err)
		}
		return
	}

	s.authenticationState = *sessionData

	sendJSON(w, options)
}

func (s *webauthnService) finishWebauthnAuthentication(w http.ResponseWriter, r *http.Request) {
	state := s.authenticationState
	s.authenticationState = webauthnLib.SessionData{} // Allow only one attempt per challenge

	var req struct {
		StayLoggedIn bool
		Credential   webauthnProtocol.CredentialAssertionResponse
	}

	if err := unmarshalTo(r.Body, &req); err != nil {
		l.Debugln("Failed to parse response:", err)
		http.Error(w, "Failed to parse response.", http.StatusBadRequest)
		return
	}

	parsedResponse, err := req.Credential.Parse()
	if err != nil {
		l.Debugln("Failed to parse WebAuthn authentication response", err)
		badRequest(w)
		return
	}

	updatedCred, err := s.engine.ValidateLogin(s, state, parsedResponse)
	if err != nil {
		l.Infoln("WebAuthn authentication failed", err)

		if state.UserVerification == webauthnProtocol.VerificationRequired && !updatedCred.Flags.UserVerified {
			antiBruteForceSleep()
			http.Error(w, "Conflict", http.StatusConflict)
			return
		}

		forbidden(w)
		return
	}

	authenticatedCredId := base64.URLEncoding.EncodeToString(updatedCred.ID)

	for _, cred := range s.cfg.GUI().WebauthnCredentials {
		if cred.ID == authenticatedCredId {
			if cred.RequireUv && !updatedCred.Flags.UserVerified {
				antiBruteForceSleep()
				http.Error(w, "Conflict", http.StatusConflict)
				return
			}
			break
		}
	}

	authenticatedCredName := authenticatedCredId
	var signCountBefore uint32 = 0
	waiter, err := s.cfg.Modify(func(cfg *config.Configuration) {
		for i, cred := range cfg.GUI.WebauthnCredentials {
			if cred.ID == authenticatedCredId {
				signCountBefore = cfg.GUI.WebauthnCredentials[i].SignCount
				authenticatedCredName = cfg.GUI.WebauthnCredentials[i].NicknameOrID()
				cfg.GUI.WebauthnCredentials[i].SignCount = updatedCred.Authenticator.SignCount
				cfg.GUI.WebauthnCredentials[i].LastUseTime = time.Now().Truncate(time.Second)
				break
			}
		}
	})
	awaitSaveConfig(w, s.cfg, waiter)

	if updatedCred.Authenticator.CloneWarning && signCountBefore != 0 {
		l.Warnln(fmt.Sprintf("Invalid WebAuthn signature count for credential \"%s\": expected > %d, was: %d. The credential may have been cloned.", authenticatedCredName, signCountBefore, parsedResponse.Response.AuthenticatorData.Counter))
	}

	guiCfg := s.cfg.GUI()
	s.tokenCookieManager.createSession(guiCfg.User, req.StayLoggedIn, w, r)
	w.WriteHeader(http.StatusNoContent)
}
