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

func (db *Lowlevel) AddOrUpdateCandidateLink(folder, label string, device, introducer protocol.DeviceID, certName, name string, addresses []string) error {
	key, err := db.keyer.GenerateCandidateLinkKey(nil, introducer[:], []byte(folder), device[:])
	if err != nil {
		return err
	}
	link := ObservedCandidateLink{
		Time:            time.Now().Round(time.Second),
		IntroducerLabel: label,
		CertName:        certName,
		IntroducerName:  name,
		Addresses:       addresses,
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

// RemoveCandidateLinksBeforeTime removes entries from a specific introducer device which
// are older than a given timestamp or invalid.  It returns only the valid removed entries.
func (db *Lowlevel) RemoveCandidateLinksBeforeTime(introducer protocol.DeviceID, oldest time.Time) ([]CandidateLink, error) {
	prefixKey, err := db.keyer.GenerateCandidateLinkKey(nil, introducer[:], nil, nil)
	if err != nil {
		return nil, err
	}
	iter, err := db.NewPrefixIterator(prefixKey)
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	oldest = oldest.Round(time.Second)
	var res []CandidateLink
	for iter.Next() {
		var ocl ObservedCandidateLink
		ocl, candidateID, introducerID, folderID, ok, err := db.readCandidateLink(iter)
		if err != nil {
			// Fatal error, not just invalid (and already discarded) entry
			return nil, err
		} else if !ok {
			// Skip in the returned list, as likely invalid values anyway
			continue
		} else if ocl.Time.Before(oldest) {
			l.Infof("Removing stale candidate link (device %v has folder %s) from introducer %s, last seen %v",
				candidateID, folderID, introducerID.Short(), ocl.Time)
		} else {
			// Keep entries younger or equal to the given timestamp
			continue
		}
		if err := db.Delete(iter.Key()); err != nil {
			l.Warnf("Failed to remove candidate link entry: %v", err)
		}
		res = append(res, CandidateLink{
			Introducer: introducer,
			Folder:     folderID,
			Candidate:  candidateID,
		})
	}
	return res, nil
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
	l.Infof("Invalid candidate link entry (%s / %v), deleting from database: %x",
		deleteCause, err, iter.Key())
	err = db.Delete(iter.Key())
	return
}

// Consolidated information about a candidate device, enough to add a connection to it
type CandidateDevice struct {
	CertName     string                                           `json:"certName,omitempty"`
	Addresses    []string                                         `json:"addresses,omitempty"`
	IntroducedBy map[protocol.DeviceID]candidateDeviceAttribution `json:"introducedBy"`
}

// Details which an introducer told us about a candidate device
type candidateDeviceAttribution struct {
	Time          time.Time         `json:"time"`
	CommonFolders map[string]string `json:"commonFolders"`
	SuggestedName string            `json:"suggestedName,omitempty"`
}

func (db *Lowlevel) CandidateDevices() (map[protocol.DeviceID]CandidateDevice, error) {
	return db.CandidateDevicesForFolder("")
}

// CandidateDevicesForFolder returns the same information as CandidateLinks, but
// aggregated by candidate device.  Given a non-empty folder ID, the results are filtered
// to only include candidate devices already sharing that specific folder indirectly.
// Invalid entries are dropped from the database after a warning log message, as a
// side-effect.
func (db *Lowlevel) CandidateDevicesForFolder(folder string) (map[protocol.DeviceID]CandidateDevice, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypeCandidateLink})
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[protocol.DeviceID]CandidateDevice)
	for iter.Next() {
		ocl, candidateID, introducerID, folderID, ok, err := db.readCandidateLink(iter)
		if err != nil {
			// Fatal error, not just invalid (and already discarded) entry
			return nil, err
		} else if !ok {
			continue
		}
		if len(folder) > 0 && folderID != folder {
			// Filter out links through folders not of interest
			continue
		}
		cd, ok := res[candidateID]
		if !ok {
			cd = CandidateDevice{
				Addresses:    []string{},
				IntroducedBy: map[protocol.DeviceID]candidateDeviceAttribution{},
			}
		}
		cd.mergeCandidateLink(ocl, folderID, introducerID)
		res[candidateID] = cd
	}
	return res, nil
}

// mergeCandidateLink aggregates one recorded link's information onto the existing structure
func (cd *CandidateDevice) mergeCandidateLink(observed ObservedCandidateLink, folder string, introducer protocol.DeviceID) {
	attrib, ok := cd.IntroducedBy[introducer]
	if !ok {
		attrib = candidateDeviceAttribution{
			CommonFolders: map[string]string{},
		}
	}
	attrib.Time = observed.Time
	attrib.CommonFolders[folder] = observed.IntroducerLabel
	if cd.CertName != observed.CertName {
		//FIXME warn?
		cd.CertName = observed.CertName
	}
	cd.collectAddresses(observed.Addresses)
	attrib.SuggestedName = observed.IntroducerName
	cd.IntroducedBy[introducer] = attrib
}

// collectAddresses deduplicates addresses to try for contacting a candidate device later
func (d *CandidateDevice) collectAddresses(addresses []string) {
	if len(addresses) == 0 {
		return
	}
	// Sort addresses into a map for deduplication
	addressMap := make(map[string]struct{}, len(d.Addresses))
	for _, s := range d.Addresses {
		addressMap[s] = struct{}{}
	}
	for _, s := range addresses {
		addressMap[s] = struct{}{}
	}
	d.Addresses = make([]string, 0, len(addressMap))
	for a := range addressMap {
		d.Addresses = append(d.Addresses, a)
	}
}

// Consolidated information about candidate devices linked through a certain common folder
type CandidateFolder map[protocol.DeviceID]candidateFolderDevice

// Description of one candidate device, mainly who announced the link to our folder
type candidateFolderDevice struct {
	IntroducedBy map[protocol.DeviceID]candidateFolderAttribution `json:"introducedBy"`
}

// Details which an introducer told us about a candidate device
type candidateFolderAttribution struct {
	Time  time.Time `json:"time"`
	Label string    `json:"label"`
}

func (db *Lowlevel) CandidateFolders() (map[string]CandidateFolder, error) {
	return db.CandidateFoldersForDevice(protocol.EmptyDeviceID)
}

// CandidateFoldersForDevice returns the same information as CandidateLinks, but
// aggregated by common folder.  The results are filtered to include only folders which
// the given device is a candidate for, unless it is EmptyDeviceID.  Invalid entries are
// dropped from the database after a warning log message, as a side-effect.
func (db *Lowlevel) CandidateFoldersForDevice(device protocol.DeviceID) (map[string]CandidateFolder, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypeCandidateLink})
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[string]CandidateFolder)
	for iter.Next() {
		ocl, candidateID, introducerID, folderID, ok, err := db.readCandidateLink(iter)
		if err != nil {
			// Fatal error, not just invalid (and already discarded) entry
			return nil, err
		} else if !ok {
			continue
		}
		if device != protocol.EmptyDeviceID && candidateID != device {
			// Filter out links where the given device is no candidate
			continue
		}
		cf, ok := res[folderID]
		if !ok {
			cf = CandidateFolder{}
		}
		cf.mergeCandidateLink(ocl, candidateID, introducerID)
		res[folderID] = cf
	}
	return res, nil
}

// mergeCandidateLink aggregates one recorded link's information onto the existing structure
func (cf *CandidateFolder) mergeCandidateLink(observed ObservedCandidateLink, candidate, introducer protocol.DeviceID) {
	device, ok := (*cf)[candidate]
	if !ok {
		device = candidateFolderDevice{
			IntroducedBy: map[protocol.DeviceID]candidateFolderAttribution{},
		}
	}
	device.IntroducedBy[introducer] = candidateFolderAttribution{
		Time:  observed.Time,
		Label: observed.IntroducerLabel,
	}
	(*cf)[candidate] = device
}
