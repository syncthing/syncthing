// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
)

var locationLocal *time.Location
var errNotAFile = fmt.Errorf("not a file")
var errNotFound = fmt.Errorf("version not found")
var errFileAlreadyExists = fmt.Errorf("file already exists")

func init() {
	var err error
	locationLocal, err = time.LoadLocation("Local")
	if err != nil {
		panic(err.Error())
	}
}

// Inserts ~tag just before the extension of the filename.
func TagFilename(name, tag string) string {
	dir, file := filepath.Dir(name), filepath.Base(name)
	ext := filepath.Ext(file)
	withoutExt := file[:len(file)-len(ext)]
	return filepath.Join(dir, withoutExt+"~"+tag+ext)
}

var tagExp = regexp.MustCompile(`.*~([^~.]+)(?:\.[^.]+)?$`)

// Returns the tag from a filename, whether at the end or middle.
func ExtractTag(path string) string {
	match := tagExp.FindStringSubmatch(path)
	// match is []string{"whole match", "submatch"} when successful

	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func UntagFilename(path string) (string, string) {
	ext := filepath.Ext(path)
	versionTag := ExtractTag(path)

	// Files tagged with old style tags cannot be untagged.
	if versionTag == "" {
		return "", ""
	}

	// Old style tag
	if strings.HasSuffix(ext, versionTag) {
		return strings.TrimSuffix(path, "~"+versionTag), versionTag
	}

	withoutExt := path[:len(path)-len(ext)-len(versionTag)-1]
	name := withoutExt + ext
	return name, versionTag
}

func retrieveVersions(fileSystem fs.Filesystem, versionsDir string) (map[string][]FileVersion, error) {
	files := make(map[string][]FileVersion)

	err := fileSystem.Walk(versionsDir, func(path string, f fs.FileInfo, err error) error {
		// Skip root (which is ok to be a symlink)
		if path == versionsDir {
			return nil
		}

		// Skip walking if we cannot walk...
		if err != nil {
			return err
		}

		// Ignore symlinks
		if f.IsSymlink() {
			return fs.SkipDir
		}

		// No records for directories
		if f.IsDir() {
			return nil
		}

		// Strip prefix.
		path = strings.TrimPrefix(path, versionsDir+string(fs.PathSeparator))
		path = osutil.NormalizedFilename(path)

		name, tag := UntagFilename(path)
		// Something invalid, assume it's an untagged file
		if name == "" || tag == "" {
			versionTime := f.ModTime().Truncate(time.Second)
			files[path] = append(files[path], FileVersion{
				VersionTime: versionTime,
				ModTime:     versionTime,
				Size:        f.Size(),
			})
			return nil
		}

		versionTime, err := time.ParseInLocation(TimeFormat, tag, locationLocal)
		if err != nil {
			// Can't parse it, welp, continue
			return nil
		}

		if err == nil {
			files[name] = append(files[name], FileVersion{
				VersionTime: versionTime.Truncate(time.Second),
				ModTime:     f.ModTime().Truncate(time.Second),
				Size:        f.Size(),
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

func archiveFile(srcFs, dstFs fs.Filesystem, versionsDir, filePath string) error {
	filePath = osutil.NativeFilename(filePath)
	info, err := srcFs.Lstat(filePath)
	if fs.IsNotExist(err) {
		l.Debugln("not archiving nonexistent file", filePath)
		return nil
	} else if err != nil {
		return err
	}
	if info.IsSymlink() {
		panic("bug: attempting to version a symlink")
	}

	_, err = dstFs.Stat(versionsDir)
	if err != nil {
		if fs.IsNotExist(err) {
			l.Debugln("creating versions dir " + versionsDir)
			err := dstFs.Mkdir(versionsDir, 0755)
			if err != nil {
				return err
			}
			_ = dstFs.Hide(versionsDir)
		} else {
			return err
		}
	}

	l.Debugln("archiving", filePath)

	file := filepath.Base(filePath)
	inFolderPath := filepath.Dir(filePath)

	dir := filepath.Join(versionsDir, inFolderPath)
	err = dstFs.MkdirAll(dir, 0755)
	if err != nil && !fs.IsExist(err) {
		return err
	}

	ver := TagFilename(file, info.ModTime().Format(TimeFormat))
	dst := filepath.Join(dir, ver)
	l.Debugln("moving to", dst)
	err = osutil.RenameOrCopy(srcFs, dstFs, filePath, dst)

	// Set the mtime to the time the file was deleted. This can be used by the
	// cleanout routine. If this fails things won't work optimally but there's
	// not much we can do about it so we ignore the error.
	_ = dstFs.Chtimes(dst, time.Now(), time.Now())

	return err
}

func restoreFile(src, dst fs.Filesystem, versionsDir, filePath string, versionTime time.Time) error {
	// If the something already exists where we are restoring to, archive existing file for versioning
	// Or fail if it's not a file
	if info, err := dst.Lstat(filePath); err == nil {
		if !info.IsRegular() {
			return errors.Wrap(errNotAFile, "archiving existing file")
		} else if err := archiveFile(dst, src, versionsDir, filePath); err != nil {
			return errors.Wrap(err, "archiving existing file")
		}
	} else if !fs.IsNotExist(err) {
		return err
	}

	filePath = osutil.NativeFilename(filePath)
	tag := versionTime.In(locationLocal).Truncate(time.Second).Format(TimeFormat)

	taggedFilename := filepath.Join(versionsDir, TagFilename(filePath, tag))
	oldTaggedFilename := filepath.Join(versionsDir, filePath+tag)
	untaggedFileName := filepath.Join(versionsDir, filePath)

	// Check that the thing we've been asked to restore is actually a file
	// and that it exists.
	sourceFile := ""
	for _, candidate := range []string{taggedFilename, oldTaggedFilename, untaggedFileName} {
		if info, err := src.Lstat(candidate); fs.IsNotExist(err) || !info.IsRegular() {
			continue
		} else if err != nil {
			// All other errors are fatal
			return err
		} else if candidate == untaggedFileName && !info.ModTime().Truncate(time.Second).Equal(versionTime) {
			// No error, and untagged file, but mtime does not match, skip
			continue
		}

		sourceFile = candidate
		break
	}

	if sourceFile == "" {
		return errNotFound
	}

	// Check that the target location of where we are supposed to restore does not exist.
	// This should have been taken care of by the first few lines of this function.
	if _, err := dst.Lstat(filePath); err == nil {
		return errFileAlreadyExists
	} else if !fs.IsNotExist(err) {
		return err
	}

	_ = dst.MkdirAll(filepath.Dir(filePath), 0755)
	return osutil.RenameOrCopy(src, dst, sourceFile, filePath)
}
