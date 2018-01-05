// Package goversioninfo creates a syso file which contains Microsoft Version Information and an optional icon.
package goversioninfo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strconv"

	"github.com/akavel/rsrc/binutil"
	"github.com/akavel/rsrc/coff"
)

// *****************************************************************************
// JSON and Config
// *****************************************************************************

// ParseJSON parses the given bytes as a VersionInfo JSON.
func (vi *VersionInfo) ParseJSON(jsonBytes []byte) error {
	return json.Unmarshal([]byte(jsonBytes), &vi)
}

// VersionInfo data container
type VersionInfo struct {
	FixedFileInfo  `json:"FixedFileInfo"`
	StringFileInfo `json:"StringFileInfo"`
	VarFileInfo    `json:"VarFileInfo"`
	Timestamp      bool
	Buffer         bytes.Buffer
	Structure      VSVersionInfo
	IconPath       string `json:"IconPath"`
	ManifestPath   string `json:"ManifestPath"`
}

// Translation with langid and charsetid.
type Translation struct {
	LangID    `json:"LangID"`
	CharsetID `json:"CharsetID"`
}

// FileVersion with 3 parts.
type FileVersion struct {
	Major int
	Minor int
	Patch int
	Build int
}

// FixedFileInfo contains file characteristics - leave most of them at the defaults.
type FixedFileInfo struct {
	FileVersion    `json:"FileVersion"`
	ProductVersion FileVersion
	FileFlagsMask  string
	FileFlags      string
	FileOS         string
	FileType       string
	FileSubType    string
}

// VarFileInfo is the translation container.
type VarFileInfo struct {
	Translation `json:"Translation"`
}

// StringFileInfo is what you want to change.
type StringFileInfo struct {
	Comments         string
	CompanyName      string
	FileDescription  string
	FileVersion      string
	InternalName     string
	LegalCopyright   string
	LegalTrademarks  string
	OriginalFilename string
	PrivateBuild     string
	ProductName      string
	ProductVersion   string
	SpecialBuild     string
}

// *****************************************************************************
// Helpers
// *****************************************************************************

type SizedReader struct {
	*bytes.Buffer
}

func (s SizedReader) Size() int64 {
	return int64(s.Buffer.Len())
}

func str2Uint32(s string) uint32 {
	if s == "" {
		return 0
	}
	u, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		log.Printf("Error parsing %q as uint32: %v", s, err)
		return 0
	}

	return uint32(u)
}

func padString(s string, zeros int) []byte {
	b := make([]byte, 0, len([]rune(s))*2)
	for _, x := range s {
		tt := int32(x)

		b = append(b, byte(tt))
		if tt > 255 {
			tt = tt >> 8
			b = append(b, byte(tt))
		} else {
			b = append(b, byte(0))
		}
	}

	for i := 0; i < zeros; i++ {
		b = append(b, 0x00)
	}

	return b
}

func padBytes(i int) []byte {
	return make([]byte, i)
}

func (f FileVersion) getVersionHighString() string {
	return fmt.Sprintf("%04x%04x", f.Major, f.Minor)
}

func (f FileVersion) getVersionLowString() string {
	return fmt.Sprintf("%04x%04x", f.Patch, f.Build)
}

// GetVersionString returns a string representation of the version
func (f FileVersion) GetVersionString() string {
	return fmt.Sprintf("%d.%d.%d.%d", f.Major, f.Minor, f.Patch, f.Build)
}

func (t Translation) getTranslationString() string {
	return fmt.Sprintf("%04X%04X", t.LangID, t.CharsetID)
}

func (t Translation) getTranslation() string {
	return fmt.Sprintf("%04x%04x", t.CharsetID, t.LangID)
}

// *****************************************************************************
// IO Methods
// *****************************************************************************

// Walk writes the data buffer with hexidecimal data from the structs
func (vi *VersionInfo) Walk() {
	// Create a buffer
	var b bytes.Buffer
	w := binutil.Writer{W: &b}

	// Write to the buffer
	binutil.Walk(vi.Structure, func(v reflect.Value, path string) error {
		if binutil.Plain(v.Kind()) {
			w.WriteLE(v.Interface())
		}
		return nil
	})

	vi.Buffer = b
}

// WriteSyso creates a resource file from the version info and optionally an icon.
// arch must be an architecture string accepted by coff.Arch, like "386" or "amd64"
func (vi *VersionInfo) WriteSyso(filename string, arch string) error {

	// Channel for generating IDs
	newID := make(chan uint16)
	go func() {
		for i := uint16(1); ; i++ {
			newID <- i
		}
	}()

	// Create a new RSRC section
	coff := coff.NewRSRC()

	// Set the architechture
	err := coff.Arch(arch)
	if err != nil {
		return err
	}

	// ID 16 is for Version Information
	coff.AddResource(16, 1, SizedReader{bytes.NewBuffer(vi.Buffer.Bytes())})

	// If manifest is enabled
	if vi.ManifestPath != "" {

		manifest, err := binutil.SizedOpen(vi.ManifestPath)
		if err != nil {
			return err
		}
		defer manifest.Close()

		id := <-newID
		coff.AddResource(rtManifest, id, manifest)
	}

	// If icon is enabled
	if vi.IconPath != "" {
		if err := addIcon(coff, vi.IconPath, newID); err != nil {
			return err
		}
	}

	coff.Freeze()

	// Write to file
	return writeCoff(coff, filename)
}

// WriteHex creates a hex file for debugging version info
func (vi *VersionInfo) WriteHex(filename string) error {
	return ioutil.WriteFile(filename, vi.Buffer.Bytes(), 0655)
}

func writeCoff(coff *coff.Coff, fnameout string) error {
	out, err := os.Create(fnameout)
	if err != nil {
		return err
	}
	if err = writeCoffTo(out, coff); err != nil {
		return fmt.Errorf("error writing %q: %v", fnameout, err)
	}
	return nil
}

func writeCoffTo(w io.WriteCloser, coff *coff.Coff) error {
	bw := binutil.Writer{W: w}

	// write the resulting file to disk
	binutil.Walk(coff, func(v reflect.Value, path string) error {
		if binutil.Plain(v.Kind()) {
			bw.WriteLE(v.Interface())
			return nil
		}
		vv, ok := v.Interface().(binutil.SizedReader)
		if ok {
			bw.WriteFromSized(vv)
			return binutil.WALK_SKIP
		}
		return nil
	})

	err := bw.Err
	if closeErr := w.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}
