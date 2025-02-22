// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	webauthnProtocol "github.com/go-webauthn/webauthn/protocol"
	webauthnLib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
)

const ceremonyTimeout time.Duration = 10 * time.Minute

func newWebauthnEngine(guiCfg config.GUIConfiguration, deviceName string) (*webauthnLib.WebAuthn, error) {
	displayName := "Syncthing"
	if deviceName != "" {
		displayName = "Syncthing @ " + deviceName
	}

	timeoutConfig := webauthnLib.TimeoutConfig{
		// go-webauthn can enforce timeouts, but we also add our own enforcement
		// layer on top to clean up stale state and to make it testable
		Enforce: true,
		// These properties also set the timeout option sent to the browser in the front-end
		Timeout:    ceremonyTimeout,
		TimeoutUVD: ceremonyTimeout,
	}
	return webauthnLib.New(&webauthnLib.Config{
		RPDisplayName: displayName,
		RPID:          guiCfg.WebauthnRpId,
		RPOrigins:     guiCfg.WebauthnOrigins,
		Timeouts: webauthnLib.TimeoutsConfig{
			Login:        timeoutConfig,
			Registration: timeoutConfig,
		},
	})
}

type timedSessionData struct {
	startTime   time.Time
	sessionData webauthnLib.SessionData
}

func (s *webauthnService) startTimedSessionData(sessionData *webauthnLib.SessionData) timedSessionData {
	return timedSessionData{
		startTime:   s.timeNow().UTC(),
		sessionData: *sessionData,
	}
}

func (s *webauthnService) expired(t *timedSessionData) bool {
	return s.timeNow().After(t.startTime.Add(ceremonyTimeout))
}

type webauthnService struct {
	miscDB                         *db.NamespacedKV
	miscDBKey                      string
	engine                         *webauthnLib.WebAuthn
	evLogger                       events.Logger
	userHandle                     []byte
	registrationStates             map[string]timedSessionData
	authenticationStates           map[string]timedSessionData
	credentialsPendingRegistration []config.WebauthnCredential
	deviceName                     string
	timeNow                        func() time.Time // can be overridden for testing
}

// State that changes often but also is not security-critical,
// and therefore can be wiped and overwritten without much consequence if needed.
type WebauthnVolatileState struct {
	Credentials map[string]WebauthnCredentialVolatileState `json:"credentials,omitempty"` // Keys are base64.RawURLEncoding.EncodeToString(credential ID)
}

type WebauthnCredentialVolatileState struct {
	SignCount   uint32    `json:"signCount,omitempty"`
	LastUseTime time.Time `json:"lastUseTime,omitempty"`
}

func newWebauthnService(guiCfg config.GUIConfiguration, deviceName string, evLogger events.Logger, miscDB *db.NamespacedKV, miscDBKey string) (webauthnService, error) {
	engine, err := newWebauthnEngine(guiCfg, deviceName)
	if err != nil {
		return webauthnService{}, err
	}

	return webauthnService{
		miscDB:               miscDB,
		miscDBKey:            miscDBKey,
		engine:               engine,
		evLogger:             evLogger,
		userHandle:           guiCfg.WebauthnUserId,
		deviceName:           deviceName,
		registrationStates:   make(map[string]timedSessionData, 1),
		authenticationStates: make(map[string]timedSessionData, 1),
		timeNow:              time.Now,
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
	if u.guiCfg.User != "" {
		return u.guiCfg.User
	}
	if u.service.deviceName != "" {
		return "Syncthing @ " + u.service.deviceName
	}
	return "Syncthing"
}
func (u webauthnLibUser) WebAuthnDisplayName() string {
	return u.WebAuthnName()
}
func (webauthnLibUser) WebAuthnIcon() string {
	return ""
}
func (u webauthnLibUser) WebAuthnCredentials() []webauthnLib.Credential {
	var result []webauthnLib.Credential
	eligibleCredentials := u.guiCfg.WebauthnState.EligibleWebAuthnCredentials(u.guiCfg)
	credentialVolState := u.service.loadVolatileState()

	for _, cred := range eligibleCredentials {
		id, err := base64.RawURLEncoding.DecodeString(cred.ID)
		if err != nil {
			l.Warnf("Failed to base64url-decode ID of WebAuthn credential %q: %s", cred.Nickname, cred.ID, err)
			continue
		}

		pubkey, err := base64.RawURLEncoding.DecodeString(cred.PublicKeyCose)
		if err != nil {
			l.Warnf("Failed to base64url-decode public key of WebAuthn credential %q (%s)", cred.Nickname, cred.ID, err)
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
				SignCount: credentialVolState.Credentials[cred.ID].SignCount,
			},
			Transport: transports,
		})
	}
	return result
}

type startWebauthnRegistrationResponse struct {
	RequestID string                              `json:"requestId"`
	Options   webauthnProtocol.CredentialCreation `json:"options"`
}

type finishWebauthnRegistrationRequest struct {
	RequestID  string                                      `json:"requestId"`
	Credential webauthnProtocol.CredentialCreationResponse `json:"credential"`
}

type startWebauthnAuthenticationResponse struct {
	RequestID string                               `json:"requestId"`
	Options   webauthnProtocol.CredentialAssertion `json:"options"`
}

type finishWebauthnAuthenticationRequest struct {
	StayLoggedIn bool                                         `json:"stayLoggedIn"`
	RequestID    string                                       `json:"requestId"`
	Credential   webauthnProtocol.CredentialAssertionResponse `json:"credential"`
}

func (s *webauthnService) startWebauthnRegistration(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		options, sessionData, err := s.engine.BeginRegistration(s.user(guiCfg))
		if err != nil {
			l.Warnln("Failed to initiate WebAuthn registration:", err)
			internalServerError(w)
			return
		}

		var req startWebauthnRegistrationResponse
		req.Options = *options
		req.RequestID = uuid.New().String()
		s.registrationStates[req.RequestID] = s.startTimedSessionData(sessionData)

		sendJSON(w, req)
	}
}

func (s *webauthnService) finishWebauthnRegistration(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer s.deleteOldStates()

		var req finishWebauthnRegistrationRequest
		if err := unmarshalTo(r.Body, &req); err != nil {
			l.Infoln("Failed to parse WebAuthn response:", err)
			http.Error(w, "Failed to parse WebAuthn response.", http.StatusBadRequest)
			return
		}

		state, ok := s.registrationStates[req.RequestID]
		if !ok {
			l.Debugf("Unknown request ID: %s", req.RequestID)
			badRequest(w)
			return
		}
		delete(s.registrationStates, req.RequestID) // Allow only one attempt per challenge

		if s.expired(&state) {
			l.Debugln("WebAuthn registration timed out: %v", state)
			http.Error(w, "Request Timeout", http.StatusRequestTimeout)
			return
		}

		parsedResponse, err := req.Credential.Parse()
		if err != nil {
			l.Infoln("Failed to parse WebAuthn registration response", err)
			badRequest(w)
			return
		}

		credential, err := s.engine.CreateCredential(s.user(guiCfg), state.sessionData, parsedResponse)
		if err != nil {
			l.Infoln("Failed to register WebAuthn credential:", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for _, existingCred := range guiCfg.WebauthnState.Credentials {
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

		now := s.timeNow().Truncate(time.Second).UTC()
		configCred := config.WebauthnCredential{
			ID:            base64.RawURLEncoding.EncodeToString(credential.ID),
			RpId:          s.engine.Config.RPID,
			PublicKeyCose: base64.RawURLEncoding.EncodeToString(credential.PublicKey),
			Transports:    transports,
			CreateTime:    now,
		}
		s.credentialsPendingRegistration = append(s.credentialsPendingRegistration, configCred)

		credentialVolState := s.loadVolatileState()
		credentialVolState.Credentials[configCred.ID] = WebauthnCredentialVolatileState{
			SignCount:   credential.Authenticator.SignCount,
			LastUseTime: now,
		}
		err = s.storeVolatileState(credentialVolState)
		if err != nil {
			l.Warnln("Failed to save WebAuthn dynamic state", err)
		}

		sendJSON(w, configCred)
	}
}

func (s *webauthnService) startWebauthnAuthentication(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		allRequireUv := true
		someRequiresUv := false
		for _, cred := range guiCfg.WebauthnState.Credentials {
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

		var req startWebauthnAuthenticationResponse
		req.Options = *options
		req.RequestID = uuid.New().String()
		s.authenticationStates[req.RequestID] = s.startTimedSessionData(sessionData)

		sendJSON(w, req)
	}
}

func (s *webauthnService) finishWebauthnAuthentication(tokenCookieManager *tokenCookieManager, guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer s.deleteOldStates()

		var req finishWebauthnAuthenticationRequest

		if err := unmarshalTo(r.Body, &req); err != nil {
			l.Infoln("Failed to parse WebAuthn response:", err)
			http.Error(w, "Failed to parse WebAuthn response.", http.StatusBadRequest)
			return
		}

		state, ok := s.authenticationStates[req.RequestID]
		if !ok {
			l.Debugf("Unknown request ID: %s", req.RequestID)
			badRequest(w)
			return
		}
		delete(s.authenticationStates, req.RequestID) // Allow only one attempt per challenge

		if s.expired(&state) {
			l.Debugln("WebAuthn authentication timed out: %v", state)
			http.Error(w, "Request Timeout", http.StatusRequestTimeout)
			return
		}

		parsedResponse, err := req.Credential.Parse()
		if err != nil {
			l.Infoln("Failed to parse WebAuthn authentication response", err)
			badRequest(w)
			return
		}

		updatedCred, err := s.engine.ValidateLogin(s.user(guiCfg), state.sessionData, parsedResponse)
		if err != nil {
			l.Infoln("WebAuthn authentication failed", err)

			if state.sessionData.UserVerification == webauthnProtocol.VerificationRequired {
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

		volState := s.loadVolatileState()
		dynCredState, ok := volState.Credentials[authenticatedCredId]
		if !ok {
			dynCredState = WebauthnCredentialVolatileState{}
		}
		signCountBefore = dynCredState.SignCount
		dynCredState.SignCount = updatedCred.Authenticator.SignCount
		dynCredState.LastUseTime = s.timeNow().Truncate(time.Second).UTC()
		volState.Credentials[authenticatedCredId] = dynCredState
		err = s.storeVolatileState(volState)
		if err != nil {
			l.Warnln("Failed to update authenticated WebAuthn credential", err)
		}

		if updatedCred.Authenticator.CloneWarning && signCountBefore != 0 {
			l.Warnf("Invalid WebAuthn signature count for credential %q: expected > %d, was: %d. The credential may have been cloned.", authenticatedCredId, signCountBefore, parsedResponse.Response.AuthenticatorData.Counter)
		}

		tokenCookieManager.createSession(guiCfg.User, req.StayLoggedIn, w, r)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *webauthnService) deleteOldStates() {
	for requestId, state := range s.registrationStates {
		if s.expired(&state) {
			l.Debugln("WebAuthn registration expired: %v", state)
			delete(s.registrationStates, requestId)
		}
	}
	for requestId, state := range s.authenticationStates {
		if s.expired(&state) {
			l.Debugln("WebAuthn authentication expired: %v", state)
			delete(s.authenticationStates, requestId)
		}
	}
}

func emptyVolState() *WebauthnVolatileState {
	s := WebauthnVolatileState{}
	s.init()
	return &s
}

func (s *WebauthnVolatileState) init() {
	if s.Credentials == nil {
		s.Credentials = make(map[string]WebauthnCredentialVolatileState, 1)
	}
}

func (s *webauthnService) loadVolatileState() *WebauthnVolatileState {
	stateBytes, ok, err := s.miscDB.Bytes(s.miscDBKey)
	if err != nil {
		l.Warnln("Failed to load WebAuthn dynamic state:", err)
		return emptyVolState()
	}
	if !ok {
		return emptyVolState()
	}

	var state WebauthnVolatileState
	err = json.Unmarshal(stateBytes, &state)
	if err != nil {
		l.Warnln("Failed to unmarshal WebAuthn dynamic state:", err)
		return emptyVolState()
	}
	state.init()
	return &state
}

func (s *webauthnService) storeVolatileState(state *WebauthnVolatileState) error {
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return s.miscDB.PutBytes(s.miscDBKey, stateBytes)
}

func (s *webauthnService) getVolatileState(w http.ResponseWriter, _ *http.Request) {
	st := s.loadVolatileState()
	w.WriteHeader(http.StatusOK)
	sendJSON(w, st)
}
