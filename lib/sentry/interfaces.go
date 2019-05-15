// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sentry

import (
	"time"

	"github.com/getsentry/raven-go"
)

type Threads struct {
	Values []Thread `json:"values"`
}

func (Threads) Class() string {
	return "threads"
}

type Thread struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Crashed    bool              `json:"crashed"`
	Current    bool              `json:"current"`
	Stacktrace *raven.Stacktrace `json:"stacktrace,omitempty"`
}

type ExceptionWithThreadId struct {
	raven.Exception
	ThreadId string `json:"thread_id"`
}

type Breadcrumbs struct {
	Values []Breadcrumb `json:"values"`
}

func (Breadcrumbs) Class() string {
	return "breadcrumbs"
}

type Breadcrumb struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Category  string    `json:"category"`
	Message   string    `json:"message"`
	Level     string    `json:"level"`
}

type SDK struct {
	Name     string    `json:"name"`
	Version  string    `json:"version"`
	Packages []Package `json:"packages"`
}

func (SDK) Class() string {
	return "sdk"
}

type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
