package goversioninfo

import (
	"reflect"
)

// *****************************************************************************
// Structure Building
// *****************************************************************************

/*
Version Information Structures
http://msdn.microsoft.com/en-us/library/windows/desktop/ff468916.aspx

VersionInfo Names
http://msdn.microsoft.com/en-us/library/windows/desktop/aa381058.aspx#string-name

Translation: LangID
http://msdn.microsoft.com/en-us/library/windows/desktop/aa381058.aspx#langid

Translation: CharsetID
http://msdn.microsoft.com/en-us/library/windows/desktop/aa381058.aspx#charsetid

*/

// VSVersionInfo is the top level version container.
type VSVersionInfo struct {
	WLength      uint16
	WValueLength uint16
	WType        uint16
	SzKey        []byte
	Padding1     []byte
	Value        VSFixedFileInfo
	Padding2     []byte
	Children     VSStringFileInfo
	Children2    VSVarFileInfo
}

// VSFixedFileInfo - most of these should be left at the defaults.
type VSFixedFileInfo struct {
	DwSignature        uint32
	DwStrucVersion     uint32
	DwFileVersionMS    uint32
	DwFileVersionLS    uint32
	DwProductVersionMS uint32
	DwProductVersionLS uint32
	DwFileFlagsMask    uint32
	DwFileFlags        uint32
	DwFileOS           uint32
	DwFileType         uint32
	DwFileSubtype      uint32
	DwFileDateMS       uint32
	DwFileDateLS       uint32
}

// VSStringFileInfo holds multiple collections of keys and values,
// only allows for 1 collection in this package.
type VSStringFileInfo struct {
	WLength      uint16
	WValueLength uint16
	WType        uint16
	SzKey        []byte
	Padding      []byte
	Children     VSStringTable
}

// VSStringTable holds a collection of string keys and values.
type VSStringTable struct {
	WLength      uint16
	WValueLength uint16
	WType        uint16
	SzKey        []byte
	Padding      []byte
	Children     []VSString
}

// VSString holds the keys and values.
type VSString struct {
	WLength      uint16
	WValueLength uint16
	WType        uint16
	SzKey        []byte
	Padding      []byte
	Value        []byte
}

// VSVarFileInfo holds the translation collection of 1.
type VSVarFileInfo struct {
	WLength      uint16
	WValueLength uint16
	WType        uint16
	SzKey        []byte
	Padding      []byte
	Value        VSVar
}

// VSVar holds the translation key.
type VSVar struct {
	WLength      uint16
	WValueLength uint16
	WType        uint16
	SzKey        []byte
	Padding      []byte
	Value        uint32
}

func buildString(i int, v reflect.Value) (VSString, bool) {
	sValue := string(v.Field(i).Interface().(string))
	sName := v.Type().Field(i).Name

	ss := VSString{}

	// If the value is set
	if sValue != "" {
		// 0 for binary, 1 for text
		ss.WType = 0x01

		// Create key
		ss.SzKey = padString(sName, 0)

		// Align to 32-bit boundary
		soFar := 2
		for (len(ss.SzKey)+6+soFar)%4 != 0 {
			soFar += 2
		}
		ss.Padding = padBytes(soFar)
		soFar += len(ss.SzKey)

		// Align zeros to 32-bit boundary
		zeros := 2
		for (6+soFar+(len(padString(sValue, 0)))+zeros)%4 != 0 {
			zeros += 2
		}

		// Create value
		ss.Value = padString(sValue, zeros)

		// Length of text in words (2 bytes) plus zero terminate word
		ss.WValueLength = uint16(len(padString(sValue, 0))/2) + 1

		// Length of structure
		//ss.WLength = 6 + uint16(soFar) + (ss.WValueLength * 2)
		ss.WLength = uint16(6 + soFar + len(ss.Value))

		return ss, true
	}

	return ss, false
}

func buildStringTable(vi *VersionInfo) VSStringTable {
	st := VSStringTable{}

	// Always set to 0
	st.WValueLength = 0x00

	// 0 for binary, 1 for text
	st.WType = 0x01

	// Language identifier and Code page
	st.SzKey = padString(vi.VarFileInfo.Translation.getTranslationString(), 0)

	// Align to 32-bit boundary
	soFar := 2
	for (len(st.SzKey)+6+soFar)%4 != 0 {
		soFar += 2
	}
	st.Padding = padBytes(soFar)
	soFar += len(st.SzKey)

	// Loop through the struct fields
	v := reflect.ValueOf(vi.StringFileInfo)
	for i := 0; i < v.NumField(); i++ {
		// If the struct is valid
		if r, ok := buildString(i, v); ok {
			st.Children = append(st.Children, r)
			st.WLength += r.WLength
		}
	}

	st.WLength += 6 + uint16(soFar)

	return st
}

func buildStringFileInfo(vi *VersionInfo) VSStringFileInfo {
	sf := VSStringFileInfo{}

	// Always set to 0
	sf.WValueLength = 0x00

	// 0 for binary, 1 for text
	sf.WType = 0x01

	sf.SzKey = padString("StringFileInfo", 0)

	// Align to 32-bit boundary
	soFar := 2
	for (len(sf.SzKey)+6+soFar)%4 != 0 {
		soFar += 2
	}
	sf.Padding = padBytes(soFar)
	soFar += len(sf.SzKey)

	// Allows for more than one string table
	st := buildStringTable(vi)
	sf.Children = st

	sf.WLength = 6 + uint16(soFar) + st.WLength

	return sf
}

func buildVar(vfi VarFileInfo) VSVar {
	vs := VSVar{}

	// 0 for binary, 1 for text
	vs.WType = 0x00

	// Create key
	vs.SzKey = padString("Translation", 0)

	// Align to 32-bit boundary
	soFar := 2
	for (len(vs.SzKey)+6+soFar)%4 != 0 {
		soFar += 2
	}
	vs.Padding = padBytes(soFar)
	soFar += len(vs.SzKey)

	// Create value
	vs.Value = str2Uint32(vfi.Translation.getTranslation())

	// Length of text in bytes
	vs.WValueLength = 4

	// Length of structure
	vs.WLength = 6 + vs.WValueLength + uint16(soFar)

	return vs
}

func buildVarFileInfo(vfi VarFileInfo) VSVarFileInfo {
	vf := VSVarFileInfo{}

	// Always set to 0
	vf.WValueLength = 0x00

	// 0 for binary, 1 for text
	vf.WType = 0x01

	vf.SzKey = padString("VarFileInfo", 0)

	// Align to 32-bit boundary
	soFar := 2
	for (len(vf.SzKey)+6+soFar)%4 != 0 {
		soFar += 2
	}
	vf.Padding = padBytes(soFar)
	soFar += len(vf.SzKey)

	// TODO Allow for more than one var table
	st := buildVar(vfi)
	vf.Value = st
	vf.WLength = 6 + st.WLength + uint16(soFar)

	return vf
}

func buildFixedFileInfo(vi *VersionInfo) VSFixedFileInfo {
	ff := VSFixedFileInfo{}
	ff.DwSignature = 0xFEEF04BD
	ff.DwStrucVersion = 0x00010000
	ff.DwFileVersionMS = str2Uint32(vi.FixedFileInfo.FileVersion.getVersionHighString())
	ff.DwFileVersionLS = str2Uint32(vi.FixedFileInfo.FileVersion.getVersionLowString())
	ff.DwProductVersionMS = str2Uint32(vi.FixedFileInfo.ProductVersion.getVersionHighString())
	ff.DwProductVersionLS = str2Uint32(vi.FixedFileInfo.ProductVersion.getVersionLowString())
	ff.DwFileFlagsMask = str2Uint32(vi.FixedFileInfo.FileFlagsMask)
	ff.DwFileFlags = str2Uint32(vi.FixedFileInfo.FileFlags)
	ff.DwFileOS = str2Uint32(vi.FixedFileInfo.FileOS)
	ff.DwFileType = str2Uint32(vi.FixedFileInfo.FileType)
	ff.DwFileSubtype = str2Uint32(vi.FixedFileInfo.FileSubType)

	// According to the spec, these should be zero...ugh
	/*if vi.Timestamp {
		now := syscall.NsecToFiletime(time.Now().UnixNano())
		ff.DwFileDateMS = now.HighDateTime
		ff.DwFileDateLS = now.LowDateTime
	}*/

	return ff
}

// Build fills the structs with data from the config file
func (v *VersionInfo) Build() {
	vi := VSVersionInfo{}

	// 0 for binary, 1 for text
	vi.WType = 0x00

	vi.SzKey = padString("VS_VERSION_INFO", 0)

	// Align to 32-bit boundary
	// 6 is for the size of WLength, WValueLength, and WType (each is 1 word or 2 bytes: FF FF)
	soFar := 2
	for (len(vi.SzKey)+6+soFar)%4 != 0 {
		soFar += 2
	}
	vi.Padding1 = padBytes(soFar)
	soFar += len(vi.SzKey)

	vi.Value = buildFixedFileInfo(v)

	// Length of VSFixedFileInfo (always the same)
	vi.WValueLength = 0x34

	// Never needs padding, not included in WLength
	vi.Padding2 = []byte{}

	// Build strings
	vi.Children = buildStringFileInfo(v)

	// Build translation
	vi.Children2 = buildVarFileInfo(v.VarFileInfo)

	// Calculate the total size
	vi.WLength += 6 + uint16(soFar) + vi.WValueLength + vi.Children.WLength + vi.Children2.WLength

	v.Structure = vi
}
