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
	"time"

	webauthnProtocol "github.com/go-webauthn/webauthn/protocol"
	webauthnLib "github.com/go-webauthn/webauthn/webauthn"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
)

type webauthnService struct {
	registrationState              webauthnLib.SessionData
	authenticationState            webauthnLib.SessionData
	cfg                            config.Wrapper
	cookieName                     string
	evLogger                       events.Logger
	credentialsPendingRegistration []config.WebauthnCredential
}

func newWebauthnService(cfg config.Wrapper, cookieName string, evLogger events.Logger) webauthnService {
	return webauthnService{
		cfg:        cfg,
		cookieName: cookieName,
		evLogger:   evLogger,
	}
}

func (s *webauthnService) startWebauthnRegistration(w http.ResponseWriter, r *http.Request) {
	webauthn, err := config.NewWebauthnHandle(s.cfg)
	if err != nil {
		l.Warnln("Failed to instantiate WebAuthn engine:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	options, sessionData, err := webauthn.BeginRegistration(s.cfg.GUI())
	if err != nil {
		l.Warnln("Failed to initiate WebAuthn registration:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.registrationState = *sessionData

	sendJSON(w, options)
}

func (s *webauthnService) finishWebauthnRegistration(w http.ResponseWriter, r *http.Request) {
	webauthn, err := config.NewWebauthnHandle(s.cfg)
	if err != nil {
		l.Warnln("Failed to instantiate WebAuthn engine:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	state := s.registrationState
	s.registrationState = webauthnLib.SessionData{} // Allow only one attempt per challenge

	credential, err := webauthn.FinishRegistration(s.cfg.GUI(), state, r)
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
	webauthn, err := config.NewWebauthnHandle(s.cfg)
	if err != nil {
		l.Warnln("Failed to initialize WebAuthn handle", err)
		internalServerError(w)
		return
	}

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

	options, sessionData, err := webauthn.BeginLogin(s.cfg.GUI(), webauthnLib.WithUserVerification(uv))
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
	webauthn, err := config.NewWebauthnHandle(s.cfg)
	if err != nil {
		l.Warnln("Failed to initialize WebAuthn handle", err)
		internalServerError(w)
		return
	}

	state := s.authenticationState
	s.authenticationState = webauthnLib.SessionData{} // Allow only one attempt per challenge

	parsedResponse, err := webauthnProtocol.ParseCredentialRequestResponse(r)
	if err != nil {
		l.Debugln("Failed to parse WebAuthn authentication response", err)
		badRequest(w)
		return
	}

	guiCfg := s.cfg.GUI()
	updatedCred, err := webauthn.ValidateLogin(guiCfg, state, parsedResponse)
	if err != nil {
		l.Infoln("WebAuthn authentication failed", err)

		if state.UserVerification == webauthnProtocol.VerificationRequired && !parsedResponse.Response.AuthenticatorData.Flags.HasUserVerified() {
			antiBruteForceSleep()
			http.Error(w, "Conflict", http.StatusConflict)
			return
		}

		forbidden(w)
		return
	}

	authenticatedCredId := base64.URLEncoding.EncodeToString(updatedCred.ID)
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

	createSession(s.cookieName, guiCfg.User, guiCfg, s.evLogger, w, r)
	w.WriteHeader(http.StatusNoContent)
}
