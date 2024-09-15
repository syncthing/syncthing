// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	webauthnProtocol "github.com/go-webauthn/webauthn/protocol"
	webauthnLib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
)

func newWebauthnEngine(guiCfg config.GUIConfiguration, deviceName string) (*webauthnLib.WebAuthn, error) {
	displayName := "Syncthing"
	if deviceName != "" {
		displayName = "Syncthing @ " + deviceName
	}

	origins, err := guiCfg.WebauthnOrigins()
	if err != nil {
		return nil, err
	}

	return webauthnLib.New(&webauthnLib.Config{
		RPDisplayName: displayName,
		RPID:          guiCfg.WebauthnRpId,
		RPOrigins:     origins,
	})
}

type webauthnService struct {
	miscDB                         *db.NamespacedKV
	miscDBKey                      string
	engine                         *webauthnLib.WebAuthn
	evLogger                       events.Logger
	userHandle                     []byte
	registrationState              webauthnLib.SessionData
	authenticationState            webauthnLib.SessionData
	credentialsPendingRegistration []config.WebauthnCredential
}

func newWebauthnService(guiCfg config.GUIConfiguration, deviceName string, evLogger events.Logger, miscDB *db.NamespacedKV, miscDBKey string) (webauthnService, error) {
	engine, err := newWebauthnEngine(guiCfg, deviceName)
	if err != nil {
		return webauthnService{}, err
	}

	return webauthnService{
		miscDB:     miscDB,
		miscDBKey:  miscDBKey,
		engine:     engine,
		evLogger:   evLogger,
		userHandle: guiCfg.WebauthnUserId,
	}, nil
}

func (s *webauthnService) user(guiCfg config.GUIConfiguration) webauthnLibUser {
	return webauthnLibUser{
		service: s,
		guiCfg:  guiCfg,
	}
}

type webauthnLibUser struct {
	service *webauthnService
	guiCfg  config.GUIConfiguration
}

func (u webauthnLibUser) WebAuthnID() []byte {
	return u.service.userHandle
}
func (u webauthnLibUser) WebAuthnName() string {
	return u.guiCfg.User
}
func (u webauthnLibUser) WebAuthnDisplayName() string {
	return u.guiCfg.User
}
func (webauthnLibUser) WebAuthnIcon() string {
	return ""
}
func (u webauthnLibUser) WebAuthnCredentials() []webauthnLib.Credential {
	var result []webauthnLib.Credential
	eligibleCredentials := u.guiCfg.WebauthnState.EligibleWebAuthnCredentials(u.guiCfg)
	credentialDynState := u.service.loadDynamicState()

	for _, cred := range eligibleCredentials {
		id, err := base64.RawURLEncoding.DecodeString(cred.ID)
		if err != nil {
			l.Warnln(fmt.Sprintf("Failed to base64url-decode ID of WebAuthn credential %q: %s", cred.Nickname, cred.ID), err)
			continue
		}

		pubkey, err := base64.RawURLEncoding.DecodeString(cred.PublicKeyCose)
		if err != nil {
			l.Warnln(fmt.Sprintf("Failed to base64url-decode public key of WebAuthn credential %q (%s)", cred.Nickname, cred.ID), err)
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
				SignCount: credentialDynState.Credentials[cred.ID].SignCount,
			},
			Transport: transports,
		})
	}
	return result
}

func (s *webauthnService) startWebauthnRegistration(cfg config.Wrapper, guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		options, sessionData, err := s.engine.BeginRegistration(s.user(cfg.GUI()))
		if err != nil {
			l.Warnln("Failed to initiate WebAuthn registration:", err)
			internalServerError(w)
			return
		}

		s.registrationState = *sessionData

		sendJSON(w, options)
	}
}

func (s *webauthnService) finishWebauthnRegistration(cfg config.Wrapper) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := s.registrationState
		s.registrationState = webauthnLib.SessionData{} // Allow only one attempt per challenge

		credential, err := s.engine.FinishRegistration(s.user(cfg.GUI()), state, r)
		if err != nil {
			l.Infoln("Failed to register WebAuthn credential:", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for _, existingCred := range cfg.GUI().WebauthnState.Credentials {
			existId, err := base64.RawURLEncoding.DecodeString(existingCred.ID)
			if err == nil && bytes.Equal(credential.ID, existId) {
				l.Infof("Cannot register WebAuthn credential with duplicate credential ID: %s", existingCred.ID)
				http.Error(w, fmt.Sprintf("Cannot register WebAuthn credential with duplicate credential ID: %s", existingCred.ID), http.StatusBadRequest)
				return
			}
		}
		for _, existingCred := range s.credentialsPendingRegistration {
			existId, err := base64.RawURLEncoding.DecodeString(existingCred.ID)
			if err == nil && bytes.Equal(credential.ID, existId) {
				l.Infof("Cannot register WebAuthn credential with duplicate credential ID: %s", existingCred.ID)
				http.Error(w, fmt.Sprintf("Cannot register WebAuthn credential with duplicate credential ID: %s", existingCred.ID), http.StatusBadRequest)
				return
			}
		}

		transports := make([]string, len(credential.Transport))
		for i, t := range credential.Transport {
			transports[i] = string(t)
		}

		now := time.Now().Truncate(time.Second).UTC()
		configCred := config.WebauthnCredential{
			ID:            base64.RawURLEncoding.EncodeToString(credential.ID),
			RpId:          s.engine.Config.RPID,
			PublicKeyCose: base64.RawURLEncoding.EncodeToString(credential.PublicKey),
			Transports:    transports,
			CreateTime:    now,
		}
		s.credentialsPendingRegistration = append(s.credentialsPendingRegistration, configCred)

		credentialDynState := s.loadDynamicState()
		credentialDynState.Credentials[configCred.ID] = WebauthnCredentialDynState{
			SignCount:   credential.Authenticator.SignCount,
			LastUseTime: now,
		}
		err = s.storeDynamicState(credentialDynState)
		if err != nil {
			l.Warnln("Failed to save WebAuthn dynamic state", err)
		}

		sendJSON(w, configCred)
	}
}

func (s *webauthnService) startWebauthnAuthentication(cfg config.Wrapper) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		allRequireUv := true
		someRequiresUv := false
		for _, cred := range cfg.GUI().WebauthnState.Credentials {
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

		options, sessionData, err := s.engine.BeginLogin(s.user(cfg.GUI()), webauthnLib.WithUserVerification(uv))
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
}

func (s *webauthnService) finishWebauthnAuthentication(tokenCookieManager *tokenCookieManager, cfg config.Wrapper) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		guiCfg := cfg.GUI()
		updatedCred, err := s.engine.ValidateLogin(s.user(guiCfg), state, parsedResponse)
		if err != nil {
			l.Infoln("WebAuthn authentication failed", err)

			if state.UserVerification == webauthnProtocol.VerificationRequired {
				antiBruteForceSleep()
				http.Error(w, "Conflict", http.StatusConflict)
				return
			}

			forbidden(w)
			return
		}

		authenticatedCredId := base64.RawURLEncoding.EncodeToString(updatedCred.ID)
		persistentState := guiCfg.WebauthnState

		for _, cred := range persistentState.Credentials {
			if cred.ID == authenticatedCredId {
				if cred.RequireUv && !updatedCred.Flags.UserVerified {
					antiBruteForceSleep()
					http.Error(w, "Conflict", http.StatusConflict)
					return
				}
				break
			}
		}

		var signCountBefore uint32 = 0

		dynState := s.loadDynamicState()
		dynCredState, ok := dynState.Credentials[authenticatedCredId]
		if !ok {
			dynCredState = WebauthnCredentialDynState{}
		}
		signCountBefore = dynCredState.SignCount
		dynCredState.SignCount = updatedCred.Authenticator.SignCount
		dynCredState.LastUseTime = time.Now().Truncate(time.Second).UTC()
		dynState.Credentials[authenticatedCredId] = dynCredState
		err = s.storeDynamicState(dynState)
		if err != nil {
			l.Warnln("Failed to update authenticated WebAuthn credential", err)
		}

		if updatedCred.Authenticator.CloneWarning && signCountBefore != 0 {
			l.Warnln(fmt.Sprintf("Invalid WebAuthn signature count for credential %q: expected > %d, was: %d. The credential may have been cloned.", authenticatedCredId, signCountBefore, parsedResponse.Response.AuthenticatorData.Counter))
		}

		tokenCookieManager.createSession(guiCfg.User, req.StayLoggedIn, w, r)
		w.WriteHeader(http.StatusNoContent)
	}
}

func emptyDynState() WebauthnDynState {
	dynState := WebauthnDynState{}
	dynState.init()
	return dynState
}

func (s *WebauthnDynState) init() {
	if s.Credentials == nil {
		s.Credentials = make(map[string]WebauthnCredentialDynState, 1)
	}
}

func (s *webauthnService) loadDynamicState() WebauthnDynState {
	stateBytes, ok, err := s.miscDB.Bytes(s.miscDBKey)
	if err != nil {
		l.Warnln("Failed to load WebAuthn dynamic state", err)
		return emptyDynState()
	}
	if !ok {
		return emptyDynState()
	}

	var state WebauthnDynState
	err = state.Unmarshal(stateBytes)
	if err != nil {
		l.Warnln("Failed to unmarshal WebAuthn dynamic state", err)
		return emptyDynState()
	}
	state.init()
	return state
}

func (s *webauthnService) storeDynamicState(state WebauthnDynState) error {
	stateBytes, err := state.Marshal()
	if err != nil {
		return err
	}

	return s.miscDB.PutBytes(s.miscDBKey, stateBytes)
}

func (s *webauthnService) getDynamicState(w http.ResponseWriter, _ *http.Request) {
	dynState := s.loadDynamicState()
	w.WriteHeader(http.StatusOK)
	sendJSON(w, dynState)
}
