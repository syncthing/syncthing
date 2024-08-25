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
	"slices"
	"time"

	webauthnProtocol "github.com/go-webauthn/webauthn/protocol"
	webauthnLib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/sliceutil"
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
	credentialsPendingRegistration []WebauthnCredential
}

func newWebauthnService(guiCfg config.GUIConfiguration, deviceName string, evLogger events.Logger, miscDB *db.NamespacedKV, miscDBKey string) (webauthnService, error) {
	engine, err := newWebauthnEngine(guiCfg, deviceName)
	if err != nil {
		return webauthnService{}, err
	}

	userHandle, err := base64.URLEncoding.DecodeString(guiCfg.WebauthnUserId)
	if err != nil {
		return webauthnService{}, err
	}

	return webauthnService{
		miscDB:     miscDB,
		miscDBKey:  miscDBKey,
		engine:     engine,
		evLogger:   evLogger,
		userHandle: userHandle,
	}, nil
}

func (s *webauthnService) loadState() (WebauthnState, error) {
	stateBytes, ok, err := s.miscDB.Bytes(s.miscDBKey)
	if err != nil {
		return WebauthnState{}, err
	}
	if !ok {
		return WebauthnState{}, nil
	}

	var state WebauthnState
	err = state.Unmarshal(stateBytes)
	if err != nil {
		return WebauthnState{}, err
	}

	return state, nil
}

func (s *webauthnService) storeState(state WebauthnState) error {
	stateBytes, err := state.Marshal()
	if err != nil {
		return err
	}

	return s.miscDB.PutBytes(s.miscDBKey, stateBytes)
}

func (s *WebauthnState) Copy() WebauthnState {
	c := *s
	c.Credentials = make([]WebauthnCredential, len(s.Credentials))
	for i := range s.Credentials {
		c.Credentials[i] = s.Credentials[i].Copy()
	}
	return c
}

func (g *WebauthnCredential) Copy() WebauthnCredential {
	c := *g
	if c.Transports != nil {
		c.Transports = make([]string, len(c.Transports))
		copy(c.Transports, g.Transports)
	}
	return c
}

func (c *WebauthnCredential) NicknameOrID() string {
	if c.Nickname != "" {
		return c.Nickname
	} else {
		return c.ID
	}
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
	eligibleCredentials, err := u.service.EligibleWebAuthnCredentials(u.guiCfg)
	if err != nil {
		return make([]webauthnLib.Credential, 0)
	}

	for _, cred := range eligibleCredentials {
		id, err := base64.URLEncoding.DecodeString(cred.ID)
		if err != nil {
			l.Warnln(fmt.Sprintf("Failed to base64url-decode ID of WebAuthn credential %q: %s", cred.Nickname, cred.ID), err)
			continue
		}

		pubkey, err := base64.URLEncoding.DecodeString(cred.PublicKeyCose)
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
				SignCount: cred.SignCount,
			},
			Transport: transports,
		})
	}
	return result
}

func (s *webauthnService) IsAuthReady(guiCfg config.GUIConfiguration) (bool, error) {
	eligibleCredentials, err := s.EligibleWebAuthnCredentials(guiCfg)
	if err != nil {
		return false, err
	}
	return guiCfg.UseTLS() && len(eligibleCredentials) > 0, nil
}

func (s *webauthnService) EligibleWebAuthnCredentials(guiCfg config.GUIConfiguration) ([]WebauthnCredential, error) {
	state, err := s.loadState()
	if err != nil {
		return nil, err
	}

	var result []WebauthnCredential
	for _, cred := range state.Credentials {
		if cred.RpId == guiCfg.WebauthnRpId {
			result = append(result, cred)
		}
	}
	return result, nil
}

func (s *webauthnService) startWebauthnRegistration(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		options, sessionData, err := s.engine.BeginRegistration(s.user(guiCfg))
		if err != nil {
			l.Warnln("Failed to initiate WebAuthn registration:", err)
			internalServerError(w)
			return
		}

		s.registrationState = *sessionData

		sendJSON(w, options)
	}
}

func (s *webauthnService) finishWebauthnRegistration(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := s.registrationState
		s.registrationState = webauthnLib.SessionData{} // Allow only one attempt per challenge

		credential, err := s.engine.FinishRegistration(s.user(guiCfg), state, r)
		if err != nil {
			l.Infoln("Failed to register WebAuthn credential:", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		persistentState, err := s.loadState()
		if err != nil {
			l.Warnln("Failed to load persistent WebAuthn state", err)
			http.Error(w, "Failed to load persistent WebAuthn state", http.StatusInternalServerError)
			return
		}

		for _, existingCred := range persistentState.Credentials {
			existId, err := base64.URLEncoding.DecodeString(existingCred.ID)
			if err == nil && bytes.Equal(credential.ID, existId) {
				l.Infof("Cannot register WebAuthn credential with duplicate credential ID: %s", existingCred.ID)
				http.Error(w, fmt.Sprintf("Cannot register WebAuthn credential with duplicate credential ID: %s", existingCred.ID), http.StatusBadRequest)
				return
			}
		}
		for _, existingCred := range s.credentialsPendingRegistration {
			existId, err := base64.URLEncoding.DecodeString(existingCred.ID)
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
		configCred := WebauthnCredential{
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
}

func (s *webauthnService) startWebauthnAuthentication(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		persistentState, err := s.loadState()
		if err != nil {
			l.Warnln("Failed to load persistent WebAuthn state", err)
			http.Error(w, "Failed to load persistent WebAuthn state", http.StatusInternalServerError)
			return
		}

		allRequireUv := true
		someRequiresUv := false
		for _, cred := range persistentState.Credentials {
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

		options, sessionData, err := s.engine.BeginLogin(s.user(guiCfg), webauthnLib.WithUserVerification(uv))
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

func (s *webauthnService) finishWebauthnAuthentication(tokenCookieManager *tokenCookieManager, guiCfg config.GUIConfiguration) http.HandlerFunc {
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

		authenticatedCredId := base64.URLEncoding.EncodeToString(updatedCred.ID)

		persistentState, err := s.loadState()
		if err != nil {
			l.Warnln("Failed to load persistent WebAuthn state", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

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

		authenticatedCredName := authenticatedCredId
		var signCountBefore uint32 = 0

		updateCredIndex := slices.IndexFunc(persistentState.Credentials, func(cred WebauthnCredential) bool { return cred.ID == authenticatedCredId })
		if updateCredIndex != -1 {
			updateCred := &persistentState.Credentials[updateCredIndex]
			signCountBefore = updateCred.SignCount
			authenticatedCredName = updateCred.NicknameOrID()
			updateCred.SignCount = updatedCred.Authenticator.SignCount
			updateCred.LastUseTime = time.Now().Truncate(time.Second).UTC()
			err = s.storeState(persistentState)
			if err != nil {
				l.Warnln("Failed to update authenticated WebAuthn credential", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		if updatedCred.Authenticator.CloneWarning && signCountBefore != 0 {
			l.Warnln(fmt.Sprintf("Invalid WebAuthn signature count for credential %q: expected > %d, was: %d. The credential may have been cloned.", authenticatedCredName, signCountBefore, parsedResponse.Response.AuthenticatorData.Counter))
		}

		tokenCookieManager.createSession(guiCfg.User, req.StayLoggedIn, w, r)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *webauthnService) getConfigLikeState(w http.ResponseWriter, _ *http.Request) {
	persistentState, err := s.loadState()
	if err != nil {
		l.Warnln("Failed to load persistent WebAuthn state", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	sendJSON(w, persistentState)
}

func (s *webauthnService) updateConfigLikeState(w http.ResponseWriter, r *http.Request) {
	// Don't allow adding new WebAuthn credentials without passing a registration challenge,
	// and only allow updating the Nickname and RequireUv fields

	persistentState, err := s.loadState()
	if err != nil {
		l.Warnln("Failed to load persistent WebAuthn state", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var newState WebauthnState
	if err := unmarshalTo(r.Body, &newState); err != nil {
		l.Debugln("Failed to parse response:", err)
		http.Error(w, "Failed to parse response.", http.StatusBadRequest)
		return
	}

	existingCredentials := make(map[string]WebauthnCredential)
	for _, cred := range persistentState.Credentials {
		existingCredentials[cred.ID] = cred
	}
	for _, cred := range s.credentialsPendingRegistration {
		existingCredentials[cred.ID] = cred
	}

	var updatedCredentials []WebauthnCredential
	updatedCredentialsMap := make(map[string]WebauthnCredential)
	for _, newCred := range newState.Credentials {
		if exCred, ok := existingCredentials[newCred.ID]; ok {
			exCred.Nickname = newCred.Nickname
			exCred.RequireUv = newCred.RequireUv
			updatedCredentials = append(updatedCredentials, exCred)
			updatedCredentialsMap[newCred.ID] = exCred
		}
	}

	persistentState.Credentials = updatedCredentials
	err = s.storeState(persistentState)
	if err != nil {
		l.Warnln("Failed to update WebAuthn credentials", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.credentialsPendingRegistration = sliceutil.Filter(
		s.credentialsPendingRegistration,
		func(pendCred *WebauthnCredential) bool {
			_, ok := updatedCredentialsMap[pendCred.ID]
			return !ok
		},
	)

	w.WriteHeader(http.StatusOK)
	sendJSON(w, persistentState)
}
