// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"io"
	"sync"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"

	"github.com/thejerf/suture"
)

const (
	bepProtocolName      = "bep/1.0"
	tlsDefaultCommonName = "syncthing"
	maxSystemErrors      = 5
	initialSystemLog     = 10
	maxSystemLog         = 250
)

type ExitStatus int

const (
	ExitSuccess ExitStatus = 0
	ExitError   ExitStatus = 1
	ExitRestart ExitStatus = 3
)

type Options struct {
	AssetDir         string
	AuditWriter      io.Writer
	DeadlockTimeoutS int
	NoUpgrade        bool
	ProfilerURL      string
	ResetDeltaIdxs   bool
	Verbose          bool
}

type App struct {
	myID        protocol.DeviceID
	mainService *suture.Supervisor
	ll          *db.Lowlevel
	opts        Options
	cfg         config.Wrapper
	exitStatus  ExitStatus
	startOnce   sync.Once
	stop        chan struct{}
	stopped     chan struct{}
}

func New(cfg config.Wrapper, opts Options) *App {
	return &App{
		opts:    opts,
		cfg:     cfg,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (a *App) Run() ExitStatus {
	a.Start()
	return a.Wait()
}

func (a *App) Start() {
	a.startOnce.Do(func() {
		if err := a.startup(); err != nil {
			close(a.stop)
			a.exitStatus = ExitError
			close(a.stopped)
			return
		}
		go a.run()
	})
}

func (a *App) Wait() ExitStatus {
	<-a.stopped
	return a.exitStatus
}

func (a *App) Stop(stopReason ExitStatus) ExitStatus {
	select {
	case <-a.stopped:
	case <-a.stop:
	default:
		close(a.stop)
	}
	<-a.stopped
	// If there was already an exit status set internally, ignore the reason
	// given to Stop.
	if a.exitStatus == ExitSuccess {
		a.exitStatus = stopReason
	}
	return a.exitStatus
}
