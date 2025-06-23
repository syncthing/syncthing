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
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	webauthnProtocol "github.com/go-webauthn/webauthn/protocol"
	webauthnLib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/gen/apiproto"
	"github.com/syncthing/syncthing/lib/config"
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
	miscDB    *db.Typed
	miscDBKey string
	miscDBMut sync.RWMutex

	engine                         *webauthnLib.WebAuthn
	evLogger                       events.Logger
	userHandle                     []byte
	registrationStates             map[string]timedSessionData
	authenticationStates           map[string]timedSessionData
	credentialsPendingRegistration []config.WebauthnCredential
	deviceName                     string
	timeNow                        func() time.Time // can be overridden for testing
}

func newWebauthnService(guiCfg config.GUIConfiguration, deviceName string, evLogger events.Logger, miscDB *db.Typed, miscDBKey string) (webauthnService, error) {
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
	eligibleCredentials := u.guiCfg.EligibleWebAuthnCredentials(u.guiCfg)
	webauthnVolState := u.service.loadVolatileState()

	for _, cred := range eligibleCredentials {
		id, err := base64.RawURLEncoding.DecodeString(cred.ID)
		if err != nil {
			l.Warnf("Failed to base64url-decode ID of WebAuthn credential %q (%s): %v", cred.Nickname, cred.ID, err)
			continue
		}

		pubkey, err := base64.RawURLEncoding.DecodeString(cred.PublicKeyCose)
		if err != nil {
			l.Warnf("Failed to base64url-decode public key of WebAuthn credential %q (%s): %v", cred.Nickname, cred.ID, err)
			continue
		}

		transports := make([]webauthnProtocol.AuthenticatorTransport, len(cred.Transports))
		for i, t := range cred.Transports {
			transports[i] = webauthnProtocol.AuthenticatorTransport(t)
		}

		credentialVolState, ok := webauthnVolState.Credentials[cred.ID]
		if !ok || credentialVolState == nil {
			credentialVolState = &apiproto.WebauthnCredentialVolatileState{}
		}

		result = append(result, webauthnLib.Credential{
			ID:        id,
			PublicKey: pubkey,
			Authenticator: webauthnLib.Authenticator{
				SignCount: credentialVolState.SignCount,
			},
			Transport: transports,
		})
	}
	return result
}

func (s *webauthnService) startWebauthnRegistration(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		options, sessionData, err := s.engine.BeginRegistration(s.user(guiCfg))
		if err != nil {
			l.Warnf("Failed to initiate WebAuthn registration: %v", err)
			internalServerError(w)
			return
		}

		requestID := uuid.New().String()
		s.registrationStates[requestID] = s.startTimedSessionData(sessionData)
		sendJSON(w, map[string]any{"requestId": requestID, "options": *options})
	}
}

func (s *webauthnService) finishWebauthnRegistration(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer s.cleanupExpiredRequests()
		requestID := r.URL.Query().Get("requestId")
		var credResp webauthnProtocol.CredentialCreationResponse
		if err := unmarshalTo(r.Body, &credResp); err != nil {
			l.Infof("Failed to parse WebAuthn response: %v", err)
			http.Error(w, "Failed to parse WebAuthn response.", http.StatusBadRequest)
			return
		}

		state, ok := s.registrationStates[requestID]
		if !ok {
			l.Debugf("Unknown request ID: %s", requestID)
			badRequest(w)
			return
		}
		delete(s.registrationStates, requestID) // Allow only one attempt per challenge

		if s.expired(&state) {
			l.Debugf("WebAuthn registration timed out: %v", state)
			http.Error(w, "Request Timeout", http.StatusRequestTimeout)
			return
		}

		parsedResponse, err := credResp.Parse()
		if err != nil {
			l.Infof("Failed to parse WebAuthn registration response: %v", err)
			badRequest(w)
			return
		}

		credential, err := s.engine.CreateCredential(s.user(guiCfg), state.sessionData, parsedResponse)
		if err != nil {
			l.Infof("Failed to register WebAuthn credential: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for _, existingCred := range slices.Concat(guiCfg.WebauthnCredentials, s.credentialsPendingRegistration) {
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
		s.updateCredentialVolatileState(configCred.ID, credential)

		sendJSON(w, configCred)
	}
}

func (s *webauthnService) startWebauthnAuthentication(guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		allRequireUv := true
		someRequiresUv := false
		for _, cred := range guiCfg.WebauthnCredentials {
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
				l.Warnf("Failed to initialize WebAuthn login: %v", err)
			}
			return
		}

		requestID := uuid.New().String()
		s.authenticationStates[requestID] = s.startTimedSessionData(sessionData)
		sendJSON(w, map[string]any{"requestId": requestID, "options": *options})
	}
}

func (s *webauthnService) finishWebauthnAuthentication(tokenCookieManager *tokenCookieManager, guiCfg config.GUIConfiguration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer s.cleanupExpiredRequests()
		requestID := r.URL.Query().Get("requestId")
		stayLoggedIn := r.URL.Query().Get("stayLoggedIn") == "true"

		var credResp webauthnProtocol.CredentialAssertionResponse
		if err := unmarshalTo(r.Body, &credResp); err != nil {
			l.Infof("Failed to parse WebAuthn response: %v", err)
			http.Error(w, "Failed to parse WebAuthn response.", http.StatusBadRequest)
			return
		}

		state, ok := s.authenticationStates[requestID]
		if !ok {
			l.Debugf("Unknown request ID: %s", requestID)
			badRequest(w)
			return
		}
		delete(s.authenticationStates, requestID) // Allow only one attempt per challenge

		if s.expired(&state) {
			l.Debugf("WebAuthn authentication timed out: %v", state)
			http.Error(w, "Request Timeout", http.StatusRequestTimeout)
			return
		}

		parsedResponse, err := credResp.Parse()
		if err != nil {
			l.Infof("Failed to parse WebAuthn authentication response: %v", err)
			badRequest(w)
			return
		}

		authenticatedCredId := parsedResponse.ID
		for _, cred := range guiCfg.WebauthnCredentials {
			if cred.ID == authenticatedCredId {
				if cred.RequireUv {
					// engine.ValidateLogin requires UV only if sessionData.UserVerification is set to "required",
					// and startWebauthnAuthentication sets it to "required" only if ALL credentials require UV.
					// Check which credential was used and set sessionData.UserVerification to "required" if this credential requires UV.
					state.sessionData.UserVerification = webauthnProtocol.VerificationRequired
				}
				break
			}
		}

		updatedCred, err := s.engine.ValidateLogin(s.user(guiCfg), state.sessionData, parsedResponse)
		if err != nil {
			l.Infof("WebAuthn authentication failed: %v", err)

			if state.sessionData.UserVerification == webauthnProtocol.VerificationRequired && !parsedResponse.Response.AuthenticatorData.Flags.UserVerified() {
				antiBruteForceSleep()
				http.Error(w, "Conflict", http.StatusConflict)
				return
			}

			forbidden(w)
			return
		}

		s.updateCredentialVolatileState(authenticatedCredId, updatedCred)
		tokenCookieManager.createSession(guiCfg.User, stayLoggedIn, w, r)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *webauthnService) cleanupExpiredRequests() {
	for requestId, state := range s.registrationStates {
		if s.expired(&state) {
			l.Debugf("WebAuthn registration expired: %v", state)
			delete(s.registrationStates, requestId)
		}
	}
	for requestId, state := range s.authenticationStates {
		if s.expired(&state) {
			l.Debugf("WebAuthn authentication expired: %v", state)
			delete(s.authenticationStates, requestId)
		}
	}
}

func newVolState() *apiproto.WebauthnVolatileState {
	s := apiproto.WebauthnVolatileState{}
	if s.Credentials == nil {
		s.Credentials = make(map[string]*apiproto.WebauthnCredentialVolatileState, 1)
	}
	return &s
}

// Load volatile WebAuthn state with a read lock during loading.
func (s *webauthnService) loadVolatileState() *apiproto.WebauthnVolatileState {
	s.miscDBMut.RLock()
	defer s.miscDBMut.RUnlock()
	return s.loadVolatileStateRLocked()
}

// Load volatile WebAuthn state without acquiring a read lock.
func (s *webauthnService) loadVolatileStateRLocked() *apiproto.WebauthnVolatileState {
	stateBytes, ok, err := s.miscDB.Bytes(s.miscDBKey)
	if err != nil {
		l.Warnf("Failed to load WebAuthn dynamic state: %v", err)
		return newVolState()
	}
	if !ok {
		return newVolState()
	}

	state := newVolState()
	err = proto.Unmarshal(stateBytes, state)
	if err != nil {
		l.Warnf("Failed to unmarshal WebAuthn dynamic state: %v", err)
		return newVolState()
	}
	if state.Credentials == nil {
		state.Credentials = newVolState().Credentials
	}
	return state
}

func (s *webauthnService) updateVolatileState(update func(state *apiproto.WebauthnVolatileState)) error {
	s.miscDBMut.Lock()
	defer s.miscDBMut.Unlock()

	state := s.loadVolatileStateRLocked()
	update(state)
	stateBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}

	return s.miscDB.PutBytes(s.miscDBKey, stateBytes)
}

func (s *webauthnService) updateCredentialVolatileState(credId string, updatedCred *webauthnLib.Credential) {
	var signCountBefore uint32
	err := s.updateVolatileState(func(volState *apiproto.WebauthnVolatileState) {
		dynCredState, ok := volState.Credentials[credId]
		if !ok {
			dynCredState = &apiproto.WebauthnCredentialVolatileState{}
		}
		signCountBefore = dynCredState.SignCount
		dynCredState.SignCount = updatedCred.Authenticator.SignCount
		dynCredState.LastUseTime = timestamppb.New(s.timeNow().Truncate(time.Second).UTC())
		volState.Credentials[credId] = dynCredState
	})
	if err != nil {
		l.Warnf("Failed to update authenticated WebAuthn credential: %v", err)
	}

	if updatedCred.Authenticator.CloneWarning && signCountBefore != 0 {
		l.Warnf("Invalid WebAuthn signature count for credential %q: expected > %d, was: %d. The credential may have been cloned.", credId, signCountBefore, updatedCred.Authenticator.SignCount)
	}
}

func (s *webauthnService) getVolatileState(w http.ResponseWriter, _ *http.Request) {
	st := s.loadVolatileState()
	w.WriteHeader(http.StatusOK)
	sendProtobufJSON(w, st)
}
