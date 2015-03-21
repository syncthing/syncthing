#!/usr/bin/env python
# ***** BEGIN LICENSE BLOCK *****
# Version: MPL 1.1/GPL 2.0/LGPL 2.1
#
# The contents of this file are subject to the Mozilla Public License Version
# 1.1 (the "License"); you may not use this file except in compliance with
# the License. You may obtain a copy of the License at
# http://www.mozilla.org/MPL/
#
# Software distributed under the License is distributed on an "AS IS" basis,
# WITHOUT WARRANTY OF ANY KIND, either express or implied. See the License
# for the specific language governing rights and limitations under the
# License.
#
# The Original Code is font utility code.
#
# The Initial Developer of the Original Code is Mozilla Corporation.
# Portions created by the Initial Developer are Copyright (C) 2009
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   John Daggett <jdaggett@mozilla.com>
#
# Alternatively, the contents of this file may be used under the terms of
# either the GNU General Public License Version 2 or later (the "GPL"), or
# the GNU Lesser General Public License Version 2.1 or later (the "LGPL"),
# in which case the provisions of the GPL or the LGPL are applicable instead
# of those above. If you wish to allow use of your version of this file only
# under the terms of either the GPL or the LGPL, and not to allow others to
# use your version of this file under the terms of the MPL, indicate your
# decision by deleting the provisions above and replace them with the notice
# and other provisions required by the GPL or the LGPL. If you do not delete
# the provisions above, a recipient may use your version of this file under
# the terms of any one of the MPL, the GPL or the LGPL.
#
# ***** END LICENSE BLOCK ***** */

# eotlitetool.py - create EOT version of OpenType font for use with IE
#
# Usage: eotlitetool.py [-o output-filename] font1 [font2 ...]
#

# OpenType file structure
# http://www.microsoft.com/typography/otspec/otff.htm
# 
# Types:
# 
# BYTE            8-bit unsigned integer.
# CHAR            8-bit signed integer.
# USHORT          16-bit unsigned integer.
# SHORT           16-bit signed integer.
# ULONG           32-bit unsigned integer.
# Fixed           32-bit signed fixed-point number (16.16)
# LONGDATETIME    Date represented in number of seconds since 12:00 midnight, January 1, 1904. The value is represented as a signed 64-bit integer.
# 
# SFNT Header
# 
# Fixed   sfnt version         // 0x00010000 for version 1.0.
# USHORT  numTables            // Number of tables.
# USHORT  searchRange          // (Maximum power of 2 <= numTables) x 16.
# USHORT  entrySelector        // Log2(maximum power of 2 <= numTables).
# USHORT  rangeShift           // NumTables x 16-searchRange.
# 
# Table Directory
# 
# ULONG   tag                  // 4-byte identifier.
# ULONG   checkSum             // CheckSum for this table.
# ULONG   offset               // Offset from beginning of TrueType font file.
# ULONG   length               // Length of this table.
# 
# OS/2 Table (Version 4)
# 
# USHORT  version              // 0x0004
# SHORT   xAvgCharWidth 
# USHORT  usWeightClass    
# USHORT  usWidthClass     
# USHORT  fsType   
# SHORT   ySubscriptXSize      
# SHORT   ySubscriptYSize      
# SHORT   ySubscriptXOffset    
# SHORT   ySubscriptYOffset    
# SHORT   ySuperscriptXSize    
# SHORT   ySuperscriptYSize    
# SHORT   ySuperscriptXOffset      
# SHORT   ySuperscriptYOffset      
# SHORT   yStrikeoutSize   
# SHORT   yStrikeoutPosition   
# SHORT   sFamilyClass     
# BYTE    panose[10]   
# ULONG   ulUnicodeRange1      // Bits 0-31
# ULONG   ulUnicodeRange2      // Bits 32-63
# ULONG   ulUnicodeRange3      // Bits 64-95
# ULONG   ulUnicodeRange4      // Bits 96-127
# CHAR    achVendID[4]     
# USHORT  fsSelection      
# USHORT  usFirstCharIndex     
# USHORT  usLastCharIndex      
# SHORT   sTypoAscender    
# SHORT   sTypoDescender   
# SHORT   sTypoLineGap     
# USHORT  usWinAscent      
# USHORT  usWinDescent     
# ULONG   ulCodePageRange1      // Bits 0-31
# ULONG   ulCodePageRange2      // Bits 32-63
# SHORT   sxHeight     
# SHORT   sCapHeight   
# USHORT  usDefaultChar    
# USHORT  usBreakChar      
# USHORT  usMaxContext     
# 
# 
# The Naming Table is organized as follows:
# 
# [name table header]
# [name records]
# [string data]
# 
# Name Table Header
# 
# USHORT  format               // Format selector (=0).
# USHORT  count                // Number of name records.
# USHORT  stringOffset         // Offset to start of string storage (from start of table).
# 
# Name Record
# 
# USHORT  platformID           // Platform ID.
# USHORT  encodingID           // Platform-specific encoding ID.
# USHORT  languageID           // Language ID.
# USHORT  nameID               // Name ID.
# USHORT  length               // String length (in bytes).
# USHORT  offset               // String offset from start of storage area (in bytes).
# 
# head Table
# 
# Fixed   tableVersion         // Table version number     0x00010000 for version 1.0.
# Fixed   fontRevision         // Set by font manufacturer.
# ULONG   checkSumAdjustment   // To compute: set it to 0, sum the entire font as ULONG, then store 0xB1B0AFBA - sum.
# ULONG   magicNumber          // Set to 0x5F0F3CF5.
# USHORT  flags   
# USHORT  unitsPerEm           // Valid range is from 16 to 16384. This value should be a power of 2 for fonts that have TrueType outlines.
# LONGDATETIME    created      // Number of seconds since 12:00 midnight, January 1, 1904. 64-bit integer
# LONGDATETIME    modified     // Number of seconds since 12:00 midnight, January 1, 1904. 64-bit integer
# SHORT   xMin                 // For all glyph bounding boxes.
# SHORT   yMin    
# SHORT   xMax    
# SHORT   yMax    
# USHORT  macStyle
# USHORT  lowestRecPPEM        // Smallest readable size in pixels.
# SHORT   fontDirectionHint
# SHORT   indexToLocFormat     // 0 for short offsets, 1 for long.
# SHORT   glyphDataFormat      // 0 for current format.
# 
# 
# 
# Embedded OpenType (EOT) file format
# http://www.w3.org/Submission/EOT/
# 
# EOT version 0x00020001
# 
# An EOT font consists of a header with the original OpenType font
# appended at the end.  Most of the data in the EOT header is simply a
# copy of data from specific tables within the font data.  The exceptions
# are the 'Flags' field and the root string name field.  The root string
# is a set of names indicating domains for which the font data can be
# used.  A null root string implies the font data can be used anywhere.
# The EOT header is in little-endian byte order but the font data remains
# in big-endian order as specified by the OpenType spec.
# 
# Overall structure:
# 
#   [EOT header]
#   [EOT name records]
#   [font data]
# 
# EOT header
# 
# ULONG   eotSize              // Total structure length in bytes (including string and font data)
# ULONG   fontDataSize         // Length of the OpenType font (FontData) in bytes
# ULONG   version              // Version number of this format - 0x00020001
# ULONG   flags                // Processing Flags (0 == no special processing)
# BYTE    fontPANOSE[10]       // OS/2 Table panose
# BYTE    charset              // DEFAULT_CHARSET (0x01)
# BYTE    italic               // 0x01 if ITALIC in OS/2 Table fsSelection is set, 0 otherwise
# ULONG   weight               // OS/2 Table usWeightClass
# USHORT  fsType               // OS/2 Table fsType (specifies embedding permission flags)
# USHORT  magicNumber          // Magic number for EOT file - 0x504C.
# ULONG   unicodeRange1        // OS/2 Table ulUnicodeRange1
# ULONG   unicodeRange2        // OS/2 Table ulUnicodeRange2
# ULONG   unicodeRange3        // OS/2 Table ulUnicodeRange3
# ULONG   unicodeRange4        // OS/2 Table ulUnicodeRange4
# ULONG   codePageRange1       // OS/2 Table ulCodePageRange1
# ULONG   codePageRange2       // OS/2 Table ulCodePageRange2
# ULONG   checkSumAdjustment   // head Table CheckSumAdjustment
# ULONG   reserved[4]          // Reserved - must be 0
# USHORT  padding1             // Padding - must be 0
# 
# EOT name records
# 
# USHORT  FamilyNameSize       // Font family name size in bytes
# BYTE    FamilyName[FamilyNameSize] // Font family name (name ID = 1), little-endian UTF-16
# USHORT  Padding2             // Padding - must be 0
# 
# USHORT  StyleNameSize        // Style name size in bytes
# BYTE    StyleName[StyleNameSize]  // Style name (name ID = 2), little-endian UTF-16
# USHORT  Padding3             // Padding - must be 0
# 
# USHORT  VersionNameSize      // Version name size in bytes
# bytes   VersionName[VersionNameSize]  // Version name (name ID = 5), little-endian UTF-16
# USHORT  Padding4             // Padding - must be 0
# 
# USHORT  FullNameSize         // Full name size in bytes
# BYTE    FullName[FullNameSize]  // Full name (name ID = 4), little-endian UTF-16
# USHORT  Padding5             // Padding - must be 0
# 
# USHORT  RootStringSize       // Root string size in bytes
# BYTE    RootString[RootStringSize]  // Root string, little-endian UTF-16



import optparse
import struct

class FontError(Exception):
    """Error related to font handling"""
    pass

def multichar(str):
    vals = struct.unpack('4B', str[:4])
    return (vals[0] << 24) + (vals[1] << 16) + (vals[2] << 8) + vals[3]

def multicharval(v):
    return struct.pack('4B', (v >> 24) & 0xFF, (v >> 16) & 0xFF, (v >> 8) & 0xFF, v & 0xFF)

class EOT:
    EOT_VERSION = 0x00020001
    EOT_MAGIC_NUMBER = 0x504c
    EOT_DEFAULT_CHARSET = 0x01
    EOT_FAMILY_NAME_INDEX = 0    # order of names in variable portion of EOT header
    EOT_STYLE_NAME_INDEX = 1
    EOT_VERSION_NAME_INDEX = 2
    EOT_FULL_NAME_INDEX = 3
    EOT_NUM_NAMES = 4
    
    EOT_HEADER_PACK = '<4L10B2BL2H7L18x'

class OpenType:
    SFNT_CFF = multichar('OTTO')            # Postscript CFF SFNT version
    SFNT_TRUE = 0x10000                     # Standard TrueType version
    SFNT_APPLE = multichar('true')          # Apple TrueType version
    
    SFNT_UNPACK = '>I4H'
    TABLE_DIR_UNPACK = '>4I'
    
    TABLE_HEAD = multichar('head')          # TrueType table tags
    TABLE_NAME = multichar('name')
    TABLE_OS2 = multichar('OS/2')
    TABLE_GLYF = multichar('glyf')
    TABLE_CFF = multichar('CFF ')
    
    OS2_FSSELECTION_ITALIC = 0x1
    OS2_UNPACK = '>4xH2xH22x10B4L4xH14x2L'
    
    HEAD_UNPACK = '>8xL'
    
    NAME_RECORD_UNPACK = '>6H'
    NAME_ID_FAMILY = 1
    NAME_ID_STYLE = 2
    NAME_ID_UNIQUE = 3
    NAME_ID_FULL = 4
    NAME_ID_VERSION = 5
    NAME_ID_POSTSCRIPT = 6
    PLATFORM_ID_UNICODE = 0                 # Mac OS uses this typically
    PLATFORM_ID_MICROSOFT = 3
    ENCODING_ID_MICROSOFT_UNICODEBMP = 1    # with Microsoft platformID BMP-only Unicode encoding
    LANG_ID_MICROSOFT_EN_US = 0x0409         # with Microsoft platformID EN US lang code

def eotname(ttf):
    i = ttf.rfind('.')
    if i != -1:
        ttf = ttf[:i]
    return ttf + '.eotlite'

def readfont(f):
    data = open(f, 'rb').read()
    return data

def get_table_directory(data):
    """read the SFNT header and table directory"""
    datalen = len(data)
    sfntsize = struct.calcsize(OpenType.SFNT_UNPACK)
    if sfntsize > datalen:
        raise FontError, 'truncated font data'
    sfntvers, numTables = struct.unpack(OpenType.SFNT_UNPACK, data[:sfntsize])[:2]
    if sfntvers != OpenType.SFNT_CFF and sfntvers != OpenType.SFNT_TRUE:
        raise FontError, 'invalid font type';
    
    font = {}
    font['version'] = sfntvers
    font['numTables'] = numTables
    
    # create set of offsets, lengths for tables
    table_dir_size = struct.calcsize(OpenType.TABLE_DIR_UNPACK)
    if sfntsize + table_dir_size * numTables > datalen:
        raise FontError, 'truncated font data, table directory extends past end of data'
    table_dir = {}
    for i in range(0, numTables):
        start = sfntsize + i * table_dir_size
        end = start + table_dir_size
        tag, check, bongo, dirlen = struct.unpack(OpenType.TABLE_DIR_UNPACK, data[start:end])
        table_dir[tag] = {'offset': bongo, 'length': dirlen, 'checksum': check}
    
    font['tableDir'] = table_dir
    
    return font

def get_name_records(nametable):
    """reads through the name records within name table"""
    name = {}
    # read the header
    headersize = 6
    count, strOffset = struct.unpack('>2H', nametable[2:6])
    namerecsize = struct.calcsize(OpenType.NAME_RECORD_UNPACK)
    if count * namerecsize + headersize > len(nametable):
        raise FontError, 'names exceed size of name table'
    name['count'] = count
    name['strOffset'] = strOffset
    
    # read through the name records
    namerecs = {}
    for i in range(0, count):
        start = headersize + i * namerecsize
        end = start + namerecsize
        platformID, encodingID, languageID, nameID, namelen, offset = struct.unpack(OpenType.NAME_RECORD_UNPACK, nametable[start:end])
        if platformID != OpenType.PLATFORM_ID_MICROSOFT or \
           encodingID != OpenType.ENCODING_ID_MICROSOFT_UNICODEBMP or \
           languageID != OpenType.LANG_ID_MICROSOFT_EN_US:
            continue
        namerecs[nameID] = {'offset': offset, 'length': namelen}
        
    name['namerecords'] = namerecs
    return name

def make_eot_name_headers(fontdata, nameTableDir):
    """extracts names from the name table and generates the names header portion of the EOT header"""
    nameoffset = nameTableDir['offset']
    namelen = nameTableDir['length']
    name = get_name_records(fontdata[nameoffset : nameoffset + namelen])
    namestroffset = name['strOffset']
    namerecs = name['namerecords']
    
    eotnames = (OpenType.NAME_ID_FAMILY, OpenType.NAME_ID_STYLE, OpenType.NAME_ID_VERSION, OpenType.NAME_ID_FULL)
    nameheaders = []
    for nameid in eotnames:
        if nameid in namerecs:
            namerecord = namerecs[nameid]
            noffset = namerecord['offset']
            nlen = namerecord['length']
            nformat = '%dH' % (nlen / 2)		# length is in number of bytes
            start = nameoffset + namestroffset + noffset
            end = start + nlen
            nstr = struct.unpack('>' + nformat, fontdata[start:end])
            nameheaders.append(struct.pack('<H' + nformat + '2x', nlen, *nstr))
        else:
            nameheaders.append(struct.pack('4x'))  # len = 0, padding = 0
    
    return ''.join(nameheaders)

# just return a null-string (len = 0)
def make_root_string():
    return struct.pack('2x')

def make_eot_header(fontdata):
    """given ttf font data produce an EOT header"""
    fontDataSize = len(fontdata)
    font = get_table_directory(fontdata)
    
    # toss out .otf fonts, t2embed library doesn't support these
    tableDir = font['tableDir']
    
    # check for required tables
    required = (OpenType.TABLE_HEAD, OpenType.TABLE_NAME, OpenType.TABLE_OS2)
    for table in required:
        if not (table in tableDir):
            raise FontError, 'missing required table ' + multicharval(table)
            
    # read name strings
    
    # pull out data from individual tables to construct fixed header portion
    # need to calculate eotSize before packing
    version = EOT.EOT_VERSION
    flags = 0
    charset = EOT.EOT_DEFAULT_CHARSET
    magicNumber = EOT.EOT_MAGIC_NUMBER
    
    # read values from OS/2 table
    os2Dir = tableDir[OpenType.TABLE_OS2]
    os2offset = os2Dir['offset']
    os2size = struct.calcsize(OpenType.OS2_UNPACK)
    
    if os2size > os2Dir['length']:
        raise FontError, 'OS/2 table invalid length'
    
    os2fields = struct.unpack(OpenType.OS2_UNPACK, fontdata[os2offset : os2offset + os2size])
    
    panose = []
    urange = []
    codepage = []
    
    weight, fsType = os2fields[:2]
    panose[:10] = os2fields[2:12]
    urange[:4] = os2fields[12:16]
    fsSelection = os2fields[16]
    codepage[:2] = os2fields[17:19]
    
    italic = fsSelection & OpenType.OS2_FSSELECTION_ITALIC
    
    # read in values from head table
    headDir = tableDir[OpenType.TABLE_HEAD]
    headoffset = headDir['offset']
    headsize = struct.calcsize(OpenType.HEAD_UNPACK)
    
    if headsize > headDir['length']:
        raise FontError, 'head table invalid length'
        
    headfields = struct.unpack(OpenType.HEAD_UNPACK, fontdata[headoffset : headoffset + headsize])
    checkSumAdjustment = headfields[0]
    
    # make name headers
    nameheaders = make_eot_name_headers(fontdata, tableDir[OpenType.TABLE_NAME])
    rootstring = make_root_string()
    
    # calculate the total eot size
    eotSize = struct.calcsize(EOT.EOT_HEADER_PACK) + len(nameheaders) + len(rootstring) + fontDataSize
    fixed = struct.pack(EOT.EOT_HEADER_PACK,
                        *([eotSize, fontDataSize, version, flags] + panose + [charset, italic] +
                          [weight, fsType, magicNumber] + urange + codepage + [checkSumAdjustment]))
    
    return ''.join((fixed, nameheaders, rootstring))
 
    
def write_eot_font(eot, header, data):
    open(eot,'wb').write(''.join((header, data)))
    return

def main():

    # deal with options
    p = optparse.OptionParser()
    p.add_option('--output', '-o', default="world")
    options, args = p.parse_args()
    
    # iterate over font files
    for f in args:
        data = readfont(f)
        if len(data) == 0:
            print 'Error reading %s' % f
        else:
            eot = eotname(f)
            header = make_eot_header(data)
            write_eot_font(eot, header, data)
        

if __name__ == '__main__':
    main()
    
    