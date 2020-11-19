// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"time"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/protocol"
)

func (db *Lowlevel) AddOrUpdatePendingDevice(device protocol.DeviceID, name, address string) error {
	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	od := ObservedDevice{
		Time:    time.Now().Round(time.Second),
		Name:    name,
		Address: address,
	}
	bs, err := od.Marshal()
	if err != nil {
		return err
	}
	return db.Put(key, bs)
}

func (db *Lowlevel) RemovePendingDevice(device protocol.DeviceID) {
	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	if err := db.Delete(key); err != nil {
		l.Warnf("Failed to remove pending device entry: %v", err)
	}
}

// PendingDevices enumerates all entries.  Invalid ones are dropped from the database
// after a warning log message, as a side-effect.
func (db *Lowlevel) PendingDevices() (map[protocol.DeviceID]ObservedDevice, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingDevice})
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[protocol.DeviceID]ObservedDevice)
	for iter.Next() {
		keyDev := db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		var od ObservedDevice
		if err != nil {
			goto deleteKey
		}
		if err = od.Unmarshal(iter.Value()); err != nil {
			goto deleteKey
		}
		res[deviceID] = od
		continue
	deleteKey:
		// Deleting invalid entries is the only possible "repair" measure and
		// appropriate for the importance of pending entries.  They will come back
		// soon if still relevant.
		l.Infof("Invalid pending device entry, deleting from database: %x", iter.Key())
		if err := db.Delete(iter.Key()); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (db *Lowlevel) AddOrUpdatePendingFolder(id, label string, device protocol.DeviceID) error {
	key, err := db.keyer.GeneratePendingFolderKey(nil, device[:], []byte(id))
	if err != nil {
		return err
	}
	of := ObservedFolder{
		Time:  time.Now().Round(time.Second),
		Label: label,
	}
	bs, err := of.Marshal()
	if err != nil {
		return err
	}
	return db.Put(key, bs)
}

// RemovePendingFolderForDevice removes entries for specific folder / device combinations.
func (db *Lowlevel) RemovePendingFolderForDevice(id string, device protocol.DeviceID) {
	key, err := db.keyer.GeneratePendingFolderKey(nil, device[:], []byte(id))
	if err != nil {
		return
	}
	if err := db.Delete(key); err != nil {
		l.Warnf("Failed to remove pending folder entry: %v", err)
	}
}

// RemovePendingFolder removes all entries matching a specific folder ID.
func (db *Lowlevel) RemovePendingFolder(id string) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingFolder})
	if err != nil {
		l.Infof("Could not iterate through pending folder entries: %v", err)
		return
	}
	defer iter.Release()
	for iter.Next() {
		if id != string(db.keyer.FolderFromPendingFolderKey(iter.Key())) {
			continue
		}
		if err := db.Delete(iter.Key()); err != nil {
			l.Warnf("Failed to remove pending folder entry: %v", err)
		}
	}
}

// Consolidated information about a pending folder
type PendingFolder struct {
	OfferedBy map[protocol.DeviceID]ObservedFolder `json:"offeredBy"`
}

func (db *Lowlevel) PendingFolders() (map[string]PendingFolder, error) {
	return db.PendingFoldersForDevice(protocol.EmptyDeviceID)
}

// PendingFoldersForDevice enumerates only entries matching the given device ID, unless it
// is EmptyDeviceID.  Invalid ones are dropped from the database after a warning log
// message, as a side-effect.
func (db *Lowlevel) PendingFoldersForDevice(device protocol.DeviceID) (map[string]PendingFolder, error) {
	var err error
	prefixKey := []byte{KeyTypePendingFolder}
	if device != protocol.EmptyDeviceID {
		prefixKey, err = db.keyer.GeneratePendingFolderKey(nil, device[:], nil)
		if err != nil {
			return nil, err
		}
	}
	iter, err := db.NewPrefixIterator(prefixKey)
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[string]PendingFolder)
	for iter.Next() {
		keyDev, ok := db.keyer.DeviceFromPendingFolderKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		var of ObservedFolder
		var folderID string
		if !ok || err != nil {
			goto deleteKey
		}
		if folderID = string(db.keyer.FolderFromPendingFolderKey(iter.Key())); len(folderID) < 1 {
			goto deleteKey
		}
		if err = of.Unmarshal(iter.Value()); err != nil {
			goto deleteKey
		}
		if _, ok := res[folderID]; !ok {
			res[folderID] = PendingFolder{
				OfferedBy: map[protocol.DeviceID]ObservedFolder{},
			}
		}
		res[folderID].OfferedBy[deviceID] = of
		continue
	deleteKey:
		// Deleting invalid entries is the only possible "repair" measure and
		// appropriate for the importance of pending entries.  They will come back
		// soon if still relevant.
		l.Infof("Invalid pending folder entry, deleting from database: %x", iter.Key())
		if err := db.Delete(iter.Key()); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (db *Lowlevel) AddOrUpdateCandidateLink(folder, label string, device, introducer protocol.DeviceID, meta *IntroducedDeviceDetails) error {
	key, err := db.keyer.GenerateCandidateLinkKey(nil, introducer[:], []byte(folder), device[:])
	if err != nil {
		return err
	}
	link := ObservedCandidateLink{
		Time:            time.Now().Round(time.Second),
		IntroducerLabel: label,
		CandidateMeta:   meta,
	}
	bs, err := link.Marshal()
	if err != nil {
		return err
	}
	return db.Put(key, bs)
}

// Details of a candidate device introduced through a specific folder:
// "Introducer says Folder exists on device Candidate"
type CandidateLink struct {
	Introducer protocol.DeviceID
	Folder     string
	Candidate  protocol.DeviceID

	// No embedded ObservedCandidateDevice needed, as this is sufficient for cleanup
}

// RemoveCandidateLink deletes a single entry, ideally retreived through CandidateLinks()
func (db *Lowlevel) RemoveCandidateLink(cl CandidateLink) {
	key, err := db.keyer.GenerateCandidateLinkKey(nil, cl.Introducer[:], []byte(cl.Folder), cl.Candidate[:])
	if err != nil {
		return
	}
	if err := db.Delete(key); err != nil {
		l.Warnf("Failed to remove candidate link entry: %v", err)
	}
}

// RemoveCandidateLinksForDevice deletes all entries related to a certain introducer and
// common folder ID.
func (db *Lowlevel) RemoveCandidateLinksForDevice(introducer protocol.DeviceID, folder string) {
	//FIXME Method currently unused!  This would be useful for an introducer being
	//FIXME completely removed, or a folder no longer shared with it.
	prefixKey, err := db.keyer.GenerateCandidateLinkKey(nil, introducer[:], nil, nil)
	if err != nil {
		return
	}
	iter, err := db.NewPrefixIterator(prefixKey)
	if err != nil {
		l.Infof("Could not iterate through candidate link entries: %v", err)
		return
	}
	defer iter.Release()
	for iter.Next() {
		if len(folder) > 0 {
			keyFolder, ok := db.keyer.FolderFromCandidateLinkKey(iter.Key())
			if ok && string(keyFolder) != folder {
				// Skip if given folder ID does not match
				continue
			}
		}
		if err := db.Delete(iter.Key()); err != nil {
			l.Warnf("Failed to remove candidate link entry: %v", err)
		}
	}
}

// CandidateLinks enumerates all entries as a flat list.  Invalid ones are dropped from
// the database after a warning log message, as a side-effect.
func (db *Lowlevel) CandidateLinks() ([]CandidateLink, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypeCandidateLink})
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	var res []CandidateLink
	for iter.Next() {
		_, candidateID, introducerID, folderID, ok, err := db.readCandidateLink(iter)
		if err != nil {
			// Fatal error, not just invalid (and already discarded) entry
			return nil, err
		} else if !ok {
			continue
		}
		res = append(res, CandidateLink{
			Introducer: introducerID,
			Folder:     folderID,
			Candidate:  candidateID,
		})
	}
	return res, nil
}

// readCandidateLink drops any invalid entries from the database after a warning log
// message, as a side-effect.  That's the only possible "repair" measure and appropriate
// for the importance of such entries.  They will come back soon if still relevant.  For
// such invalid entries, "valid" is returned false and the error value from the deletion
// is passed on.
func (db *Lowlevel) readCandidateLink(iter backend.Iterator) (ocl ObservedCandidateLink, candidateID, introducerID protocol.DeviceID, folderID string, valid bool, err error) {
	var deleteCause string
	keyDev, ok := db.keyer.IntroducerFromCandidateLinkKey(iter.Key())
	introducerID, err = protocol.DeviceIDFromBytes(keyDev)
	if !ok || err != nil {
		deleteCause = "invalid introducer device ID"
		goto deleteKey
	}
	if keyFolder, ok := db.keyer.FolderFromCandidateLinkKey(iter.Key()); !ok || len(keyFolder) < 1 {
		deleteCause = "invalid folder ID"
		goto deleteKey
	} else {
		folderID = string(keyFolder)
	}
	keyDev = db.keyer.DeviceFromCandidateLinkKey(iter.Key())
	candidateID, err = protocol.DeviceIDFromBytes(keyDev)
	if err != nil {
		deleteCause = "invalid candidate device ID"
		goto deleteKey
	}
	if err = ocl.Unmarshal(iter.Value()); err != nil {
		deleteCause = "DB Unmarshal failed"
		goto deleteKey
	}
	valid = true
	return

deleteKey:
	l.Infof("Invalid candidate link entry (%v / %v), deleting from database: %x",
		deleteCause, err, iter.Key())
	err = db.Delete(iter.Key())
	return
}
