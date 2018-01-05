// Contribution by Tamás Gulácsi

package goversioninfo

import (
	"encoding/json"
	"strconv"
)

// CharsetID must use be a character-set identifier from:
// https://msdn.microsoft.com/en-us/library/windows/desktop/aa381058(v=vs.85).aspx#charsetID
type CharsetID uint16

// CharsetID constants
const (
	Cs7ASCII       = CharsetID(0)    // Cs7ASCII:	0	0000	7-bit ASCII
	CsJIS          = CharsetID(932)  // CsJIS:	932	03A4	Japan (Shift ? JIS X-0208)
	CsKSC          = CharsetID(949)  // CsKSC:	949	03B5	Korea (Shift ? KSC 5601)
	CsBig5         = CharsetID(950)  // CsBig5:	950	03B6	Taiwan (Big5)
	CsUnicode      = CharsetID(1200) // CsUnicode:	1200	04B0	Unicode
	CsLatin2       = CharsetID(1250) // CsLatin2:	1250	04E2	Latin-2 (Eastern European)
	CsCyrillic     = CharsetID(1251) // CsCyrillic:	1251	04E3	Cyrillic
	CsMultilingual = CharsetID(1252) // CsMultilingual:	1252	04E4	Multilingual
	CsGreek        = CharsetID(1253) // CsGreek:	1253	04E5	Greek
	CsTurkish      = CharsetID(1254) // CsTurkish:	1254	04E6	Turkish
	CsHebrew       = CharsetID(1255) // CsHebrew:	1255	04E7	Hebrew
	CsArabic       = CharsetID(1256) // CsArabic:	1256	04E8	Arabic
)

// UnmarshalJSON converts the string to a CharsetID
func (cs *CharsetID) UnmarshalJSON(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	if p[0] != '"' {
		var u uint16
		if err := json.Unmarshal(p, &u); err != nil {
			return err
		}
		*cs = CharsetID(u)
		return nil
	}
	var s string
	if err := json.Unmarshal(p, &s); err != nil {
		return err
	}
	u, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return err
	}
	*cs = CharsetID(u)
	return nil
}

// LangID must use be a character-set identifier from:
// https://msdn.microsoft.com/en-us/library/windows/desktop/aa381058(v=vs.85).aspx#langID
type LangID uint16

// UnmarshalJSON converts the string to a LangID
func (lng *LangID) UnmarshalJSON(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	if p[0] != '"' {
		var u uint16
		if err := json.Unmarshal(p, &u); err != nil {
			return err
		}
		*lng = LangID(u)
		return nil
	}
	var s string
	if err := json.Unmarshal(p, &s); err != nil {
		return err
	}
	u, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return err
	}
	*lng = LangID(u)
	return nil
}

// LangID constants
const (
	LngArabic                = LangID(0x0401) // LngArabic: 0x0401 Arabic
	LngBulgarian             = LangID(0x0402) // LngBulgarian: 0x0402 Bulgarian
	LngCatalan               = LangID(0x0403) // LngCatalan: 0x0403 Catalan
	LngTraditionalChinese    = LangID(0x0404) // LngTraditionalChinese: 0x0404 Traditional Chinese
	LngCzech                 = LangID(0x0405) // LngCzech: 0x0405 Czech
	LngDanish                = LangID(0x0406) // LngDanish: 0x0406 Danish
	LngGerman                = LangID(0x0407) // LngGerman: 0x0407 German
	LngGreek                 = LangID(0x0408) // LngGreek: 0x0408 Greek
	LngUSEnglish             = LangID(0x0409) // LngUSEnglish: 0x0409 U.S. English
	LngCastilianSpanish      = LangID(0x040A) // LngCastilianSpanish: 0x040A Castilian Spanish
	LngFinnish               = LangID(0x040B) // LngFinnish: 0x040B Finnish
	LngFrench                = LangID(0x040C) // LngFrench: 0x040C French
	LngHebrew                = LangID(0x040D) // LngHebrew: 0x040D Hebrew
	LngHungarian             = LangID(0x040E) // LngHungarian: 0x040E Hungarian
	LngIcelandic             = LangID(0x040F) // LngIcelandic: 0x040F Icelandic
	LngItalian               = LangID(0x0410) // LngItalian: 0x0410 Italian
	LngJapanese              = LangID(0x0411) // LngJapanese: 0x0411 Japanese
	LngKorean                = LangID(0x0412) // LngKorean: 0x0412 Korean
	LngDutch                 = LangID(0x0413) // LngDutch: 0x0413 Dutch
	LngNorwegianBokmal       = LangID(0x0414) // LngNorwegianBokmal: 0x0414 Norwegian ? Bokmal
	LngPolish                = LangID(0x0415) // LngPolish: 0x0415 Polish
	LngPortugueseBrazil      = LangID(0x0416) // LngPortugueseBrazil: 0x0416 Portuguese (Brazil)
	LngRhaetoRomanic         = LangID(0x0417) // LngRhaetoRomanic: 0x0417 Rhaeto-Romanic
	LngRomanian              = LangID(0x0418) // LngRomanian: 0x0418 Romanian
	LngRussian               = LangID(0x0419) // LngRussian: 0x0419 Russian
	LngCroatoSerbianLatin    = LangID(0x041A) // LngCroatoSerbianLatin: 0x041A Croato-Serbian (Latin)
	LngSlovak                = LangID(0x041B) // LngSlovak: 0x041B Slovak
	LngAlbanian              = LangID(0x041C) // LngAlbanian: 0x041C Albanian
	LngSwedish               = LangID(0x041D) // LngSwedish: 0x041D Swedish
	LngThai                  = LangID(0x041E) // LngThai: 0x041E Thai
	LngTurkish               = LangID(0x041F) // LngTurkish: 0x041F Turkish
	LngUrdu                  = LangID(0x0420) // LngUrdu: 0x0420 Urdu
	LngBahasa                = LangID(0x0421) // LngBahasa: 0x0421 Bahasa
	LngSimplifiedChinese     = LangID(0x0804) // LngSimplifiedChinese: 0x0804 Simplified Chinese
	LngSwissGerman           = LangID(0x0807) // LngSwiss German: 0x0807 Swiss German
	LngUKEnglish             = LangID(0x0809) // LngUKEnglish: 0x0809 U.K. English
	LngSpanishMexico         = LangID(0x080A) // LngSpanishMexico: 0x080A Spanish (Mexico)
	LngBelgianFrench         = LangID(0x080C) // LngBelgian French: 0x080C Belgian French
	LngSwissItalian          = LangID(0x0810) // LngSwiss Italian: 0x0810 Swiss Italian
	LngBelgianDutch          = LangID(0x0813) // LngBelgian Dutch: 0x0813 Belgian Dutch
	LngNorwegianNynorsk      = LangID(0x0814) // LngNorwegianNynorsk: 0x0814 Norwegian ? Nynorsk
	LngPortuguesePortugal    = LangID(0x0816) // LngPortuguese (Portugal): 0x0816 Portuguese (Portugal)
	LngSerboCroatianCyrillic = LangID(0x081A) // LngSerboCroatianCyrillic: 0x081A Serbo-Croatian (Cyrillic)
	LngCanadianFrench        = LangID(0x0C0C) // LngCanadian French: 0x0C0C Canadian French
	LngSwissFrench           = LangID(0x100C) // LngSwiss French: 0x100C Swiss French
)
