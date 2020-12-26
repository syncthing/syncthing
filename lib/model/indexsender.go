// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
)

type indexSender struct {
	conn                     protocol.Connection
	folder                   string
	folderIsReceiveEncrypted bool
	dev                      string
	fset                     *db.FileSet
	prevSequence             int64
	evLogger                 events.Logger
	connClosed               chan struct{}
	token                    suture.ServiceToken
	pauseChan                chan struct{}
	resumeChan               chan *db.FileSet
}

func (s *indexSender) Serve(ctx context.Context) (err error) {
	l.Debugf("Starting indexSender for %s to %s at %s (slv=%d)", s.folder, s.conn.ID(), s.conn, s.prevSequence)
	defer func() {
		err = svcutil.NoRestartErr(err)
		l.Debugf("Exiting indexSender for %s to %s at %s: %v", s.folder, s.conn.ID(), s.conn, err)
	}()

	// We need to send one index, regardless of whether there is something to send or not
	err = s.sendIndexTo(ctx)

	// Subscribe to LocalIndexUpdated (we have new information to send) and
	// DeviceDisconnected (it might be us who disconnected, so we should
	// exit).
	sub := s.evLogger.Subscribe(events.LocalIndexUpdated | events.DeviceDisconnected)
	defer sub.Unsubscribe()

	paused := false
	evChan := sub.C()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for err == nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.connClosed:
			return nil
		default:
		}

		// While we have sent a sequence at least equal to the one
		// currently in the database, wait for the local index to update. The
		// local index may update for other folders than the one we are
		// sending for.
		if s.fset.Sequence(protocol.LocalDeviceID) <= s.prevSequence {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-s.connClosed:
				return nil
			case <-evChan:
			case <-ticker.C:
			case <-s.pauseChan:
				paused = true
			case s.fset = <-s.resumeChan:
				paused = false
			}

			continue
		}

		if !paused {
			err = s.sendIndexTo(ctx)
		}

		// Wait a short amount of time before entering the next loop. If there
		// are continuous changes happening to the local index, this gives us
		// time to batch them up a little.
		time.Sleep(250 * time.Millisecond)
	}

	return err
}

func (s *indexSender) resume(fset *db.FileSet) {
	select {
	case <-s.connClosed:
	case s.resumeChan <- fset:
	}
}

func (s *indexSender) pause() {
	select {
	case <-s.connClosed:
	case s.pauseChan <- struct{}{}:
	}
}

// sendIndexTo sends file infos with a sequence number higher than prevSequence and
// returns the highest sent sequence number.
func (s *indexSender) sendIndexTo(ctx context.Context) error {
	initial := s.prevSequence == 0
	batch := newFileInfoBatch(nil)
	batch.flushFn = func(fs []protocol.FileInfo) error {
		l.Debugf("%v: Sending %d files (<%d bytes)", s, len(batch.infos), batch.size)
		if initial {
			initial = false
			return s.conn.Index(ctx, s.folder, fs)
		}
		return s.conn.IndexUpdate(ctx, s.folder, fs)
	}

	var err error
	var f protocol.FileInfo
	snap := s.fset.Snapshot()
	defer snap.Release()
	previousWasDelete := false
	snap.WithHaveSequence(s.prevSequence+1, func(fi protocol.FileIntf) bool {
		// This is to make sure that renames (which is an add followed by a delete) land in the same batch.
		// Even if the batch is full, we allow a last delete to slip in, we do this by making sure that
		// the batch ends with a non-delete, or that the last item in the batch is already a delete
		if batch.full() && (!fi.IsDeleted() || previousWasDelete) {
			if err = batch.flush(); err != nil {
				return false
			}
		}

		if shouldDebug() {
			if fi.SequenceNo() < s.prevSequence+1 {
				panic(fmt.Sprintln("sequence lower than requested, got:", fi.SequenceNo(), ", asked to start at:", s.prevSequence+1))
			}
		}

		if f.Sequence > 0 && fi.SequenceNo() <= f.Sequence {
			l.Warnln("Non-increasing sequence detected: Checking and repairing the db...")
			// Abort this round of index sending - the next one will pick
			// up from the last successful one with the repeaired db.
			defer func() {
				if fixed, dbErr := s.fset.RepairSequence(); dbErr != nil {
					l.Warnln("Failed repairing sequence entries:", dbErr)
					panic("Failed repairing sequence entries")
				} else {
					s.evLogger.Log(events.Failure, "detected and repaired non-increasing sequence")
					l.Infof("Repaired %v sequence entries in database", fixed)
				}
			}()
			return false
		}

		f = fi.(protocol.FileInfo)

		// If this is a folder receiving encrypted files only, we
		// mustn't ever send locally changed file infos. Those aren't
		// encrypted and thus would be a protocol error at the remote.
		if s.folderIsReceiveEncrypted && fi.IsReceiveOnlyChanged() {
			return true
		}

		// Mark the file as invalid if any of the local bad stuff flags are set.
		f.RawInvalid = f.IsInvalid()
		// If the file is marked LocalReceive (i.e., changed locally on a
		// receive only folder) we do not want it to ever become the
		// globally best version, invalid or not.
		if f.IsReceiveOnlyChanged() {
			f.Version = protocol.Vector{}
		}

		// never sent externally
		f.LocalFlags = 0
		f.VersionHash = nil

		previousWasDelete = f.IsDeleted()

		batch.append(f)
		return true
	})
	if err != nil {
		return err
	}

	err = batch.flush()

	// True if there was nothing to be sent
	if f.Sequence == 0 {
		return err
	}

	s.prevSequence = f.Sequence
	return err
}

func (s *indexSender) String() string {
	return fmt.Sprintf("indexSender@%p for %s to %s at %s", s, s.folder, s.conn.ID(), s.conn)
}

type indexSenderRegistry struct {
	deviceID     protocol.DeviceID
	sup          *suture.Supervisor
	evLogger     events.Logger
	conn         protocol.Connection
	closed       chan struct{}
	indexSenders map[string]*indexSender
	startInfos   map[string]*indexSenderStartInfo
	mut          sync.Mutex
}

func newIndexSenderRegistry(conn protocol.Connection, closed chan struct{}, sup *suture.Supervisor, evLogger events.Logger) *indexSenderRegistry {
	return &indexSenderRegistry{
		deviceID:     conn.ID(),
		conn:         conn,
		closed:       closed,
		sup:          sup,
		evLogger:     evLogger,
		indexSenders: make(map[string]*indexSender),
		startInfos:   make(map[string]*indexSenderStartInfo),
		mut:          sync.Mutex{},
	}
}

// add starts an index sender for given folder.
// If an index sender is already running, it will be stopped first.
func (r *indexSenderRegistry) add(folder config.FolderConfiguration, fset *db.FileSet, startInfo *indexSenderStartInfo) {
	r.mut.Lock()
	r.addLocked(folder, fset, startInfo)
	r.mut.Unlock()
}

func (r *indexSenderRegistry) addLocked(folder config.FolderConfiguration, fset *db.FileSet, startInfo *indexSenderStartInfo) {
	myIndexID := fset.IndexID(protocol.LocalDeviceID)
	mySequence := fset.Sequence(protocol.LocalDeviceID)
	var startSequence int64

	// This is the other side's description of what it knows
	// about us. Lets check to see if we can start sending index
	// updates directly or need to send the index from start...

	if startInfo.local.IndexID == myIndexID {
		// They say they've seen our index ID before, so we can
		// send a delta update only.

		if startInfo.local.MaxSequence > mySequence {
			// Safety check. They claim to have more or newer
			// index data than we have - either we have lost
			// index data, or reset the index without resetting
			// the IndexID, or something else weird has
			// happened. We send a full index to reset the
			// situation.
			l.Infof("Device %v folder %s is delta index compatible, but seems out of sync with reality", r.deviceID, folder.Description())
			startSequence = 0
		} else {
			l.Debugf("Device %v folder %s is delta index compatible (mlv=%d)", r.deviceID, folder.Description(), startInfo.local.MaxSequence)
			startSequence = startInfo.local.MaxSequence
		}
	} else if startInfo.local.IndexID != 0 {
		// They say they've seen an index ID from us, but it's
		// not the right one. Either they are confused or we
		// must have reset our database since last talking to
		// them. We'll start with a full index transfer.
		l.Infof("Device %v folder %s has mismatching index ID for us (%v != %v)", r.deviceID, folder.Description(), startInfo.local.IndexID, myIndexID)
		startSequence = 0
	} else {
		l.Debugf("Device %v folder %s has no index ID for us", r.deviceID, folder.Description())
	}

	// This is the other side's description of themselves. We
	// check to see that it matches the IndexID we have on file,
	// otherwise we drop our old index data and expect to get a
	// completely new set.

	theirIndexID := fset.IndexID(r.deviceID)
	if startInfo.remote.IndexID == 0 {
		// They're not announcing an index ID. This means they
		// do not support delta indexes and we should clear any
		// information we have from them before accepting their
		// index, which will presumably be a full index.
		l.Debugf("Device %v folder %s does not announce an index ID", r.deviceID, folder.Description())
		fset.Drop(r.deviceID)
	} else if startInfo.remote.IndexID != theirIndexID {
		// The index ID we have on file is not what they're
		// announcing. They must have reset their database and
		// will probably send us a full index. We drop any
		// information we have and remember this new index ID
		// instead.
		l.Infof("Device %v folder %s has a new index ID (%v)", r.deviceID, folder.Description(), startInfo.remote.IndexID)
		fset.Drop(r.deviceID)
		fset.SetIndexID(r.deviceID, startInfo.remote.IndexID)
	}

	if is, ok := r.indexSenders[folder.ID]; ok {
		r.sup.RemoveAndWait(is.token, 0)
		delete(r.indexSenders, folder.ID)
	}
	if _, ok := r.startInfos[folder.ID]; ok {
		delete(r.startInfos, folder.ID)
	}

	is := &indexSender{
		conn:                     r.conn,
		connClosed:               r.closed,
		folder:                   folder.ID,
		folderIsReceiveEncrypted: folder.Type == config.FolderTypeReceiveEncrypted,
		fset:                     fset,
		prevSequence:             startSequence,
		evLogger:                 r.evLogger,
		pauseChan:                make(chan struct{}),
		resumeChan:               make(chan *db.FileSet),
	}
	is.token = r.sup.Add(is)
	r.indexSenders[folder.ID] = is
}

// addPending stores the given info to start an index sender once resume is called
// for this folder.
// If an index sender is already running, it will be stopped.
func (r *indexSenderRegistry) addPending(folder config.FolderConfiguration, startInfo *indexSenderStartInfo) {
	r.mut.Lock()
	defer r.mut.Unlock()

	if is, ok := r.indexSenders[folder.ID]; ok {
		r.sup.RemoveAndWait(is.token, 0)
		delete(r.indexSenders, folder.ID)
	}
	r.startInfos[folder.ID] = startInfo
}

// remove stops a running index sender or removes one pending to be started.
// It is a noop if the folder isn't known.
func (r *indexSenderRegistry) remove(folder string) {
	r.mut.Lock()
	defer r.mut.Unlock()

	if is, ok := r.indexSenders[folder]; ok {
		r.sup.RemoveAndWait(is.token, 0)
		delete(r.indexSenders, folder)
	}
	delete(r.startInfos, folder)
}

// removeAllExcept stops all running index senders and removes those pending to be started,
// except mentioned ones.
// It is a noop if the folder isn't known.
func (r *indexSenderRegistry) removeAllExcept(except map[string]struct{}) {
	r.mut.Lock()
	defer r.mut.Unlock()

	for folder, is := range r.indexSenders {
		if _, ok := except[folder]; !ok {
			r.sup.RemoveAndWait(is.token, 0)
			delete(r.indexSenders, folder)
		}
	}
	for folder := range r.indexSenders {
		if _, ok := except[folder]; !ok {
			delete(r.startInfos, folder)
		}
	}
}

// pause stops a running index sender.
// It is a noop if the folder isn't known or has not been started yet.
func (r *indexSenderRegistry) pause(folder string) {
	r.mut.Lock()
	defer r.mut.Unlock()

	if is, ok := r.indexSenders[folder]; ok {
		is.pause()
	}
}

// resume unpauses an already running index sender or starts it, if it was added
// while paused.
// It is a noop if the folder isn't known.
func (r *indexSenderRegistry) resume(folder config.FolderConfiguration, fset *db.FileSet) {
	r.mut.Lock()
	defer r.mut.Unlock()

	is, isOk := r.indexSenders[folder.ID]
	if info, ok := r.startInfos[folder.ID]; ok {
		if isOk {
			r.sup.RemoveAndWait(is.token, 0)
			delete(r.indexSenders, folder.ID)
		}
		r.addLocked(folder, fset, info)
		delete(r.startInfos, folder.ID)
	} else if isOk {
		is.resume(fset)
	}
}

type indexSenderStartInfo struct {
	local, remote protocol.Device
}
