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

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/ur"
)

type indexHandler struct {
	conn                     protocol.Connection
	downloads                *deviceDownloadState
	folder                   string
	folderIsReceiveEncrypted bool
	evLogger                 events.Logger

	// We track the latest / highest sequence number in two ways for two
	// different reasons. Initially they are the same -- the highest seen
	// sequence number reported by the other side (or zero).
	//
	// One is the highest number we've seen when iterating the database,
	// which we track for database iteration purposes. When we loop, we
	// start looking at that number plus one in the next loop. Our index
	// numbering may have holes which this will skip over.
	//
	// The other is the highest sequence we previously sent to the other
	// side, used by them for correctness checks. This one must not skip
	// holes. That is, if we iterate and find a hole, this is not
	// incremented because nothing was sent to the other side.
	localPrevSequence int64 // the highest sequence number we've seen in our FileInfos
	sentPrevSequence  int64 // the highest sequence number we've sent to the peer

	cond   *sync.Cond
	paused bool
	fset   *db.FileSet
	runner service
}

func newIndexHandler(conn protocol.Connection, downloads *deviceDownloadState, folder config.FolderConfiguration, fset *db.FileSet, runner service, startInfo *clusterConfigDeviceInfo, evLogger events.Logger) *indexHandler {
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
			l.Infof("Device %v folder %s is delta index compatible, but seems out of sync with reality", conn.DeviceID().Short(), folder.Description())
			startSequence = 0
		} else {
			l.Debugf("Device %v folder %s is delta index compatible (mlv=%d)", conn.DeviceID().Short(), folder.Description(), startInfo.local.MaxSequence)
			startSequence = startInfo.local.MaxSequence
		}
	} else if startInfo.local.IndexID != 0 {
		// They say they've seen an index ID from us, but it's
		// not the right one. Either they are confused or we
		// must have reset our database since last talking to
		// them. We'll start with a full index transfer.
		l.Infof("Device %v folder %s has mismatching index ID for us (%v != %v)", conn.DeviceID().Short(), folder.Description(), startInfo.local.IndexID, myIndexID)
		startSequence = 0
	} else {
		l.Debugf("Device %v folder %s has no index ID for us", conn.DeviceID().Short(), folder.Description())
	}

	// This is the other side's description of themselves. We
	// check to see that it matches the IndexID we have on file,
	// otherwise we drop our old index data and expect to get a
	// completely new set.

	theirIndexID := fset.IndexID(conn.DeviceID())
	if startInfo.remote.IndexID == 0 {
		// They're not announcing an index ID. This means they
		// do not support delta indexes and we should clear any
		// information we have from them before accepting their
		// index, which will presumably be a full index.
		l.Debugf("Device %v folder %s does not announce an index ID", conn.DeviceID().Short(), folder.Description())
		fset.Drop(conn.DeviceID())
	} else if startInfo.remote.IndexID != theirIndexID {
		// The index ID we have on file is not what they're
		// announcing. They must have reset their database and
		// will probably send us a full index. We drop any
		// information we have and remember this new index ID
		// instead.
		l.Infof("Device %v folder %s has a new index ID (%v)", conn.DeviceID().Short(), folder.Description(), startInfo.remote.IndexID)
		fset.Drop(conn.DeviceID())
		fset.SetIndexID(conn.DeviceID(), startInfo.remote.IndexID)
	}

	return &indexHandler{
		conn:                     conn,
		downloads:                downloads,
		folder:                   folder.ID,
		folderIsReceiveEncrypted: folder.Type == config.FolderTypeReceiveEncrypted,
		localPrevSequence:        startSequence,
		sentPrevSequence:         startSequence,
		evLogger:                 evLogger,

		fset:   fset,
		runner: runner,
		cond:   sync.NewCond(new(sync.Mutex)),
	}
}

// waitForFileset waits for the handler to resume and fetches the current fileset.
func (s *indexHandler) waitForFileset(ctx context.Context) (*db.FileSet, error) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	for s.paused {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			s.cond.Wait()
		}
	}

	return s.fset, nil
}

func (s *indexHandler) Serve(ctx context.Context) (err error) {
	l.Debugf("Starting index handler for %s to %s at %s (localPrevSequence=%d)", s.folder, s.conn.DeviceID().Short(), s.conn, s.localPrevSequence)
	stop := make(chan struct{})

	defer func() {
		err = svcutil.NoRestartErr(err)
		l.Debugf("Exiting index handler for %s to %s at %s: %v", s.folder, s.conn.DeviceID().Short(), s.conn, err)
		close(stop)
	}()

	// Broadcast the pause cond when the context quits
	go func() {
		select {
		case <-ctx.Done():
			s.cond.Broadcast()
		case <-stop:
		}
	}()

	// We need to send one index, regardless of whether there is something to send or not
	fset, err := s.waitForFileset(ctx)
	if err != nil {
		return err
	}
	err = s.sendIndexTo(ctx, fset)

	// Subscribe to LocalIndexUpdated (we have new information to send) and
	// DeviceDisconnected (it might be us who disconnected, so we should
	// exit).
	sub := s.evLogger.Subscribe(events.LocalIndexUpdated | events.DeviceDisconnected)
	defer sub.Unsubscribe()

	evChan := sub.C()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for err == nil {
		fset, err = s.waitForFileset(ctx)
		if err != nil {
			return err
		}

		// While we have sent a sequence at least equal to the one
		// currently in the database, wait for the local index to update. The
		// local index may update for other folders than the one we are
		// sending for.
		if fset.Sequence(protocol.LocalDeviceID) <= s.localPrevSequence {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-evChan:
			case <-ticker.C:
			}
			continue
		}

		err = s.sendIndexTo(ctx, fset)

		// Wait a short amount of time before entering the next loop. If there
		// are continuous changes happening to the local index, this gives us
		// time to batch them up a little.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}

	return err
}

// resume might be called because the folder was actually resumed, or just
// because the folder config changed (and thus the runner and potentially fset).
func (s *indexHandler) resume(fset *db.FileSet, runner service) {
	s.cond.L.Lock()
	s.paused = false
	s.fset = fset
	s.runner = runner
	s.cond.Broadcast()
	s.cond.L.Unlock()
}

func (s *indexHandler) pause() {
	s.cond.L.Lock()
	if s.paused {
		s.evLogger.Log(events.Failure, "index handler got paused while already paused")
	}
	s.paused = true
	s.fset = nil
	s.runner = nil
	s.cond.Broadcast()
	s.cond.L.Unlock()
}

// sendIndexTo sends file infos with a sequence number higher than prevSequence and
// returns the highest sent sequence number.
func (s *indexHandler) sendIndexTo(ctx context.Context, fset *db.FileSet) error {
	initial := s.localPrevSequence == 0
	batch := db.NewFileInfoBatch(nil)
	var batchError error
	batch.SetFlushFunc(func(fs []protocol.FileInfo) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if len(fs) == 0 {
			// can't happen, flush is not called with an empty batch
			panic("bug: flush called with empty batch (race condition?)")
		}
		if batchError != nil {
			// can't happen, once an error is returned the index sender exits
			panic(fmt.Sprintf("bug: once failed it should stay failed (%v)", batchError))
		}
		l.Debugf("%v: Sending %d files (<%d bytes)", s, len(fs), batch.Size())

		lastSequence := fs[len(fs)-1].Sequence
		var err error
		if initial {
			initial = false
			err = s.conn.Index(ctx, &protocol.Index{
				Folder:       s.folder,
				Files:        fs,
				LastSequence: lastSequence,
			})
		} else {
			err = s.conn.IndexUpdate(ctx, &protocol.IndexUpdate{
				Folder:       s.folder,
				Files:        fs,
				PrevSequence: s.sentPrevSequence,
				LastSequence: lastSequence,
			})
		}
		if err != nil {
			batchError = err
			return err
		}
		s.sentPrevSequence = lastSequence
		return nil
	})

	var err error
	var f protocol.FileInfo
	snap, err := fset.Snapshot()
	if err != nil {
		return svcutil.AsFatalErr(err, svcutil.ExitError)
	}
	defer snap.Release()
	previousWasDelete := false
	snap.WithHaveSequence(s.localPrevSequence+1, func(fi protocol.FileInfo) bool {
		// This is to make sure that renames (which is an add followed by a delete) land in the same batch.
		// Even if the batch is full, we allow a last delete to slip in, we do this by making sure that
		// the batch ends with a non-delete, or that the last item in the batch is already a delete
		if batch.Full() && (!fi.IsDeleted() || previousWasDelete) {
			if err = batch.Flush(); err != nil {
				return false
			}
		}

		if fi.SequenceNo() < s.localPrevSequence+1 {
			s.logSequenceAnomaly("database returned sequence lower than requested", map[string]any{
				"sequence": fi.SequenceNo(),
				"start":    s.localPrevSequence + 1,
			})
		}

		if f.Sequence > 0 && fi.SequenceNo() <= f.Sequence {
			s.logSequenceAnomaly("database returned non-increasing sequence", map[string]any{
				"sequence": fi.SequenceNo(),
				"start":    s.localPrevSequence + 1,
				"previous": f.Sequence,
			})
			// Abort this round of index sending - the next one will pick
			// up from the last successful one with the repeaired db.
			defer func() {
				if fixed, dbErr := fset.RepairSequence(); dbErr != nil {
					l.Warnln("Failed repairing sequence entries:", dbErr)
					panic("Failed repairing sequence entries")
				} else {
					s.evLogger.Log(events.Failure, "detected and repaired non-increasing sequence")
					l.Infof("Repaired %v sequence entries in database", fixed)
				}
			}()
			return false
		}

		f = fi

		// If this is a folder receiving encrypted files only, we
		// mustn't ever send locally changed file infos. Those aren't
		// encrypted and thus would be a protocol error at the remote.
		if s.folderIsReceiveEncrypted && fi.IsReceiveOnlyChanged() {
			return true
		}

		f = prepareFileInfoForIndex(f)

		previousWasDelete = f.IsDeleted()

		batch.Append(f)
		return true
	})
	if err != nil {
		return err
	}

	if err := batch.Flush(); err != nil {
		return err
	}

	// Use the sequence of the snapshot we iterated as a starting point for the
	// next run. Previously we used the sequence of the last file we sent,
	// however it's possible that a higher sequence exists, just doesn't need to
	// be sent (e.g. in a receive-only folder, when a local change was
	// reverted). No point trying to send nothing again.
	s.localPrevSequence = snap.Sequence(protocol.LocalDeviceID)

	return nil
}

func (s *indexHandler) receive(fs []protocol.FileInfo, update bool, op string, prevSequence, lastSequence int64) error {
	deviceID := s.conn.DeviceID()

	s.cond.L.Lock()
	paused := s.paused
	fset := s.fset
	runner := s.runner
	s.cond.L.Unlock()

	if paused {
		l.Infof("%v for paused folder %q", op, s.folder)
		return fmt.Errorf("%v: %w", s.folder, ErrFolderPaused)
	}

	defer runner.SchedulePull()

	s.downloads.Update(s.folder, makeForgetUpdate(fs))

	if !update {
		fset.Drop(deviceID)
	}

	l.Debugf("Received %d files for %s from %s, prevSeq=%d, lastSeq=%d", len(fs), s.folder, deviceID.Short(), prevSequence, lastSequence)

	// Verify that the previous sequence number matches what we expected
	if exp := fset.Sequence(deviceID); prevSequence > 0 && prevSequence != exp {
		s.logSequenceAnomaly("index update with unexpected sequence", map[string]any{
			"prevSeq":      prevSequence,
			"lastSeq":      lastSequence,
			"batch":        len(fs),
			"expectedPrev": exp,
		})
	}

	for i := range fs {
		// Verify index in relation to the claimed sequence boundaries
		if fs[i].Sequence < prevSequence {
			s.logSequenceAnomaly("file with sequence before prevSequence", map[string]any{
				"prevSeq": prevSequence,
				"lastSeq": lastSequence,
				"batch":   len(fs),
				"seenSeq": fs[i].Sequence,
				"atIndex": i,
			})
		}
		if lastSequence > 0 && fs[i].Sequence > lastSequence {
			s.logSequenceAnomaly("file with sequence after lastSequence", map[string]any{
				"prevSeq": prevSequence,
				"lastSeq": lastSequence,
				"batch":   len(fs),
				"seenSeq": fs[i].Sequence,
				"atIndex": i,
			})
		}
		if i > 0 && fs[i].Sequence <= fs[i-1].Sequence {
			s.logSequenceAnomaly("index update with non-increasing sequence", map[string]any{
				"prevSeq":      prevSequence,
				"lastSeq":      lastSequence,
				"batch":        len(fs),
				"seenSeq":      fs[i].Sequence,
				"atIndex":      i,
				"precedingSeq": fs[i-1].Sequence,
			})
		}

		// The local attributes should never be transmitted over the wire.
		// Make sure they look like they weren't.
		fs[i].LocalFlags = 0
		fs[i].VersionHash = nil
	}

	// Verify the claimed last sequence number
	if lastSequence > 0 && len(fs) > 0 && lastSequence != fs[len(fs)-1].Sequence {
		s.logSequenceAnomaly("index update with unexpected last sequence", map[string]any{
			"prevSeq": prevSequence,
			"lastSeq": lastSequence,
			"batch":   len(fs),
			"seenSeq": fs[len(fs)-1].Sequence,
		})
	}

	fset.Update(deviceID, fs)
	seq := fset.Sequence(deviceID)

	// Check that the sequence we get back is what we put in...
	if lastSequence > 0 && len(fs) > 0 && seq != lastSequence {
		s.logSequenceAnomaly("unexpected sequence after update", map[string]any{
			"prevSeq":     prevSequence,
			"lastSeq":     lastSequence,
			"batch":       len(fs),
			"seenSeq":     fs[len(fs)-1].Sequence,
			"returnedSeq": seq,
		})
	}

	s.evLogger.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"device":   deviceID.String(),
		"folder":   s.folder,
		"items":    len(fs),
		"sequence": seq,
		"version":  seq, // legacy for sequence
	})

	return nil
}

func (s *indexHandler) logSequenceAnomaly(msg string, extra map[string]any) {
	extraStrs := make(map[string]string, len(extra))
	for k, v := range extra {
		extraStrs[k] = fmt.Sprint(v)
	}

	s.evLogger.Log(events.Failure, ur.FailureData{
		Description: msg,
		Extra:       extraStrs,
	})
}

func prepareFileInfoForIndex(f protocol.FileInfo) protocol.FileInfo {
	// Mark the file as invalid if any of the local bad stuff flags are set.
	f.RawInvalid = f.IsInvalid()
	// If the file is marked LocalReceive (i.e., changed locally on a
	// receive only folder) we do not want it to ever become the
	// globally best version, invalid or not.
	if f.IsReceiveOnlyChanged() {
		f.Version = protocol.Vector{}
	}
	// The trailer with the encrypted fileinfo is device local, don't send info
	// about that to remotes
	f.Size -= int64(f.EncryptionTrailerSize)
	f.EncryptionTrailerSize = 0
	// never sent externally
	f.LocalFlags = 0
	f.VersionHash = nil
	f.InodeChangeNs = 0
	return f
}

func (s *indexHandler) String() string {
	return fmt.Sprintf("indexHandler@%p for %s to %s at %s", s, s.folder, s.conn.DeviceID().Short(), s.conn)
}

type indexHandlerRegistry struct {
	evLogger      events.Logger
	conn          protocol.Connection
	downloads     *deviceDownloadState
	indexHandlers *serviceMap[string, *indexHandler]
	startInfos    map[string]*clusterConfigDeviceInfo
	folderStates  map[string]*indexHandlerFolderState
	mut           sync.Mutex
}

type indexHandlerFolderState struct {
	cfg    config.FolderConfiguration
	fset   *db.FileSet
	runner service
}

func newIndexHandlerRegistry(conn protocol.Connection, downloads *deviceDownloadState, evLogger events.Logger) *indexHandlerRegistry {
	r := &indexHandlerRegistry{
		evLogger:      evLogger,
		conn:          conn,
		downloads:     downloads,
		indexHandlers: newServiceMap[string, *indexHandler](evLogger),
		startInfos:    make(map[string]*clusterConfigDeviceInfo),
		folderStates:  make(map[string]*indexHandlerFolderState),
		mut:           sync.Mutex{},
	}
	return r
}

func (r *indexHandlerRegistry) String() string {
	return fmt.Sprintf("indexHandlerRegistry/%v", r.conn.DeviceID().Short())
}

func (r *indexHandlerRegistry) Serve(ctx context.Context) error {
	// Running the index handler registry means running the individual index
	// handler children.
	return r.indexHandlers.Serve(ctx)
}

func (r *indexHandlerRegistry) startLocked(folder config.FolderConfiguration, fset *db.FileSet, runner service, startInfo *clusterConfigDeviceInfo) {
	r.indexHandlers.RemoveAndWait(folder.ID, 0)
	delete(r.startInfos, folder.ID)

	is := newIndexHandler(r.conn, r.downloads, folder, fset, runner, startInfo, r.evLogger)
	r.indexHandlers.Add(folder.ID, is)

	// This new connection might help us get in sync.
	runner.SchedulePull()
}

// AddIndexInfo starts an index handler for given folder, unless it is paused.
// If it is paused, the given startInfo is stored to start the sender once the
// folder is resumed.
// If an index handler is already running, it will be stopped first.
func (r *indexHandlerRegistry) AddIndexInfo(folder string, startInfo *clusterConfigDeviceInfo) {
	r.mut.Lock()
	defer r.mut.Unlock()

	if r.indexHandlers.RemoveAndWait(folder, 0) == nil {
		l.Debugf("Removed index sender for device %v and folder %v due to added pending", r.conn.DeviceID().Short(), folder)
	}
	folderState, ok := r.folderStates[folder]
	if !ok {
		l.Debugf("Pending index handler for device %v and folder %v", r.conn.DeviceID().Short(), folder)
		r.startInfos[folder] = startInfo
		return
	}
	r.startLocked(folderState.cfg, folderState.fset, folderState.runner, startInfo)
}

// Remove stops a running index handler or removes one pending to be started.
// It is a noop if the folder isn't known.
func (r *indexHandlerRegistry) Remove(folder string) {
	r.mut.Lock()
	defer r.mut.Unlock()

	l.Debugf("Removing index handler for device %v and folder %v", r.conn.DeviceID().Short(), folder)
	r.indexHandlers.RemoveAndWait(folder, 0)
	delete(r.startInfos, folder)
	l.Debugf("Removed index handler for device %v and folder %v", r.conn.DeviceID().Short(), folder)
}

// RemoveAllExcept stops all running index handlers and removes those pending to be started,
// except mentioned ones.
// It is a noop if the folder isn't known.
func (r *indexHandlerRegistry) RemoveAllExcept(except map[string]remoteFolderState) {
	r.mut.Lock()
	defer r.mut.Unlock()

	r.indexHandlers.Each(func(folder string, is *indexHandler) error {
		if _, ok := except[folder]; !ok {
			r.indexHandlers.RemoveAndWait(folder, 0)
			l.Debugf("Removed index handler for device %v and folder %v (removeAllExcept)", r.conn.DeviceID().Short(), folder)
		}
		return nil
	})
	for folder := range r.startInfos {
		if _, ok := except[folder]; !ok {
			delete(r.startInfos, folder)
			l.Debugf("Removed pending index handler for device %v and folder %v (removeAllExcept)", r.conn.DeviceID().Short(), folder)
		}
	}
}

// RegisterFolderState must be called whenever something about the folder
// changes. The exception being if the folder is removed entirely, then call
// Remove. The fset and runner arguments may be nil, if given folder is paused.
func (r *indexHandlerRegistry) RegisterFolderState(folder config.FolderConfiguration, fset *db.FileSet, runner service) {
	if !folder.SharedWith(r.conn.DeviceID()) {
		r.Remove(folder.ID)
		return
	}

	r.mut.Lock()
	if folder.Paused {
		r.folderPausedLocked(folder.ID)
	} else {
		r.folderRunningLocked(folder, fset, runner)
	}
	r.mut.Unlock()
}

// folderPausedLocked stops a running index handler.
// It is a noop if the folder isn't known or has not been started yet.
func (r *indexHandlerRegistry) folderPausedLocked(folder string) {
	l.Debugf("Pausing index handler for device %v and folder %v", r.conn.DeviceID().Short(), folder)
	delete(r.folderStates, folder)
	if is, ok := r.indexHandlers.Get(folder); ok {
		is.pause()
		l.Debugf("Paused index handler for device %v and folder %v", r.conn.DeviceID().Short(), folder)
	} else {
		l.Debugf("No index handler for device %v and folder %v to pause", r.conn.DeviceID().Short(), folder)
	}
}

// folderRunningLocked resumes an already running index handler or starts it, if it
// was added while paused.
// It is a noop if the folder isn't known.
func (r *indexHandlerRegistry) folderRunningLocked(folder config.FolderConfiguration, fset *db.FileSet, runner service) {
	r.folderStates[folder.ID] = &indexHandlerFolderState{
		cfg:    folder,
		fset:   fset,
		runner: runner,
	}

	is, isOk := r.indexHandlers.Get(folder.ID)
	if info, ok := r.startInfos[folder.ID]; ok {
		if isOk {
			r.indexHandlers.RemoveAndWait(folder.ID, 0)
			l.Debugf("Removed index handler for device %v and folder %v in resume", r.conn.DeviceID().Short(), folder.ID)
		}
		r.startLocked(folder, fset, runner, info)
		delete(r.startInfos, folder.ID)
		l.Debugf("Started index handler for device %v and folder %v in resume", r.conn.DeviceID().Short(), folder.ID)
	} else if isOk {
		l.Debugf("Resuming index handler for device %v and folder %v", r.conn.DeviceID().Short(), folder)
		is.resume(fset, runner)
	} else {
		l.Debugf("Not resuming index handler for device %v and folder %v as none is paused and there is no start info", r.conn.DeviceID().Short(), folder.ID)
	}
}

func (r *indexHandlerRegistry) ReceiveIndex(folder string, fs []protocol.FileInfo, update bool, op string, prevSequence, lastSequence int64) error {
	r.mut.Lock()
	defer r.mut.Unlock()
	is, isOk := r.indexHandlers.Get(folder)
	if !isOk {
		l.Infof("%v for nonexistent or paused folder %q", op, folder)
		return fmt.Errorf("%s: %w", folder, ErrFolderMissing)
	}
	return is.receive(fs, update, op, prevSequence, lastSequence)
}

// makeForgetUpdate takes an index update and constructs a download progress update
// causing to forget any progress for files which we've just been sent.
func makeForgetUpdate(files []protocol.FileInfo) []protocol.FileDownloadProgressUpdate {
	updates := make([]protocol.FileDownloadProgressUpdate, 0, len(files))
	for _, file := range files {
		if file.IsSymlink() || file.IsDirectory() || file.IsDeleted() {
			continue
		}
		updates = append(updates, protocol.FileDownloadProgressUpdate{
			Name:       file.Name,
			Version:    file.Version,
			UpdateType: protocol.FileDownloadProgressUpdateTypeForget,
		})
	}
	return updates
}
