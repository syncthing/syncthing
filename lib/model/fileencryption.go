package model

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// This function is used to encrypt or decrypt the file based on existance of the right file signature
func EncOrDecFiles(rootDir string, files []protocol.FileInfo) {
	for _, file := range files {
		if !file.IsDeleted() && !file.IsDirectory() { // non deleted files only
			l.Infoln("file:", file.Name, file.IsDeleted())

			if file.Type == 0 || file.Type == 4 { // files and symlinks only (ignore dirs and all the other bs for now)
				// some key for testing (32 bytes exactly, otherwise we need to truncate/pad to length)
				key := checkKeyLength([]byte("ThisiswaytoolongakeyforSyncthing"))
				aesblock, err := aes.NewCipher(key)
				if err != nil {
					l.Infoln(err)
				}
				pathToFile := rootDir + string(os.PathSeparator) + filepath.FromSlash(file.Name)

				plainOrCiphertext, err := ioutil.ReadFile(pathToFile)
				if err != nil {
					l.Infoln(err)
				}

				// if file sig exists we should decrypt instead
				if string(plainOrCiphertext[:5]) == "stenc" {
					if !strings.HasSuffix(rootDir, ".ENC") { // make sure we're not running decryption dir created to hold the enc files
						decryptFile(rootDir, pathToFile, key, aesblock, plainOrCiphertext)
					}
				} else {
					encryptFile(rootDir, pathToFile, key, aesblock, plainOrCiphertext)
				}
			}
		}
	}
}

func decryptFile(rootDir string, pathToFile string, key []byte, aesblock cipher.Block, ciphertext []byte) {
	ciphertext = ciphertext[5:] // strip file sig

	iv := ciphertext[:16]             // extract IV from next 16 bytes
	ciphertext = ciphertext[len(iv):] // and strip it

	plaintext := make([]byte, len(ciphertext))
	stream := cipher.NewCFBDecrypter(aesblock, iv)
	stream.XORKeyStream(plaintext, ciphertext)

	filepathNull := bytes.IndexByte(plaintext, 0) // find first null char (end of filename)
	if filepathNull == -1 {
		l.Infoln("Cannot find null byte, we may have the wrong key or a corrupt file")
	}
	newfilepath := string(plaintext[:filepathNull]) // extract filepath
	newfilepath = filepath.Base(newfilepath)
	plaintext = plaintext[filepathNull+1:] // remove filepath from file contents
	plaintext = plaintext[32:]             // strip off sha256 plaintext hash (since we're not using them right now)
	plaintext = plaintext[(8 * 3):]        // strip off file access times (since we're not using them right now)

	f, err := os.Create(rootDir + string(os.PathSeparator) + newfilepath)
	if err != nil {
		l.Infoln(err)
	}
	_, err = io.Copy(f, bytes.NewReader(plaintext))
	if err != nil {
		l.Infoln(err)
	}
}

func encryptFile(rootDir string, pathToFile string, key []byte, aesblock cipher.Block, plaintext []byte) {
	// Get file MAC times and make byte arrays out of them
	mtime, atime, ctime, err := fileStatTimes(pathToFile)
	bmtime, batime, bctime := make([]byte, 8), make([]byte, 8), make([]byte, 8)
	binary.LittleEndian.PutUint64(bmtime, uint64(mtime))
	binary.LittleEndian.PutUint64(batime, uint64(atime))
	binary.LittleEndian.PutUint64(bctime, uint64(ctime))
	// To revert to int again ==>     i = int64(binary.LittleEndian.Uint64(b))

	// The IV needs to be unique but not secure, so put it at beginning of ciphertext
	h := sha256.New()
	h.Write([]byte(pathToFile))
	var iv []byte
	iv = append(iv, h.Sum(nil)...)

	if _, err := io.Copy(h, bytes.NewReader(plaintext)); err != nil {
		log.Fatal(err)
	}
	sha256HashBytes := h.Sum(nil) // sha256 of just the filedata by itself

	var fileHeader []byte
	fileHeader = append([]byte(pathToFile), 0)          // null byte terminated
	fileHeader = append(fileHeader, sha256HashBytes...) // plaintext hash
	fileHeader = append(fileHeader, bmtime...)          // mod time
	fileHeader = append(fileHeader, batime...)          // access time
	fileHeader = append(fileHeader, bctime...)          // created time

	plaintext = append(fileHeader, plaintext...) // prepend fileheader to contents
	ciphertext := make([]byte, len(plaintext))

	iv = iv[:16] // make iv the first half of the sha256 hash
	stream := cipher.NewCFBEncrypter(aesblock, iv)
	stream.XORKeyStream(ciphertext, plaintext)

	filesig := []byte{115, 116, 101, 110, 99}   // "stenc" file sig
	ciphertext = append(iv, ciphertext...)      // prepend unencrypted file IV
	ciphertext = append(filesig, ciphertext...) // prepend file sig

	// make dir if not existing
	err = os.MkdirAll(rootDir+".ENC", os.ModeDir)
	if err != nil {
		l.Infoln(err)
	}
	// create blank file
	f, err := os.Create(rootDir + ".ENC" + string(os.PathSeparator) + hex.EncodeToString(iv[:8]))
	if err != nil {
		l.Infoln(err)
	}
	// fill file with data
	_, err = io.Copy(f, bytes.NewReader(ciphertext))
	if err != nil {
		l.Infoln(err)
	}
	f.Close()
}

// Enforce exactly 32 bytes long for the key
func checkKeyLength(key []byte) []byte {
	// Check if key too short, if so pad with 0's
	if len(key) < 32 {
		for i := 0; i < (aes.BlockSize); i++ {
			if i >= len(key) {
				key = append(key, '0')
			}
		}
	}
	// Check if key too long, if so crop it
	if len(key) > 32 {
		key = key[:32]
	}

	return key
}

// This function should get the MAC times of any file (in unixtimestamp format)
func fileStatTimes(name string) (mtime, atime, ctime int64, err error) {
	fi, err := os.Stat(name)
	if err != nil {
		return
	}
	mtime = fi.ModTime().Unix()

	if runtime.GOOS == "windows" {
		stat := fi.Sys().(*syscall.Win32FileAttributeData)
		atime = stat.LastAccessTime.Nanoseconds() / int64(time.Second)
		ctime = stat.CreationTime.Nanoseconds() / int64(time.Second)
	}

	// if runtime.GOOS == "Linux" {
	//     stat := fi.Sys().(*syscall.Stat_t)
	//     atime = time.Unix(int64(stat.Atim.Sec), int64(stat.Atim.Nsec))
	//     ctime = time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
	// }

	return
}
