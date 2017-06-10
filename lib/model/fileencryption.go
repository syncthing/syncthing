package model

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
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

// This function is used as wrapper to encrypt or decrypt the file based on
// existance of the file signature and do some other setup first as well
func (m *Model) EncOrDecFiles(folder string, files []protocol.FileInfo) {
	rootDir := m.folderCfgs[folder].Path()

	for _, file := range files {
		if !file.IsDeleted() && !file.IsDirectory() { // non deleted files only
			if file.Type == 0 || file.Type == 4 { // files and symlinks only (ignore dirs and all the other bs for now)
				key := m.getKeyFromConfig(folder)
				key = stripOrPadKeyLength(key, 32)
				aesblock, err := aes.NewCipher(key)
				if err != nil {
					l.Infoln(err)
				}

				pathToFile := rootDir + string(os.PathSeparator) + filepath.FromSlash(file.Name)
				plainOrCiphertext, err := ioutil.ReadFile(pathToFile)
				if err != nil {
					l.Infoln(err)
				}

				if strings.HasSuffix(rootDir, ".ENC") { // make sure we're not running decryption in regular dir (only enc dir)
					rootDir = rootDir[:len(rootDir)-4]
					// now strip ".ENC" from folder path and find the folder ID for that path so we use the same key of the
					// folder that encrypted the file.  This avoids an error where using 2 different keys causes either
					// decyrption to fail or produce further gibberish which can be somewhat hard/confusing to debug
					for id, fldrCfg := range m.folderCfgs {
						if rootDir == fldrCfg.Path() { // rootDir now without the ".ENC"
							folder = id
							key = []byte(m.folderCfgs[folder].EncryptionKey)
						}
					}

					if string(plainOrCiphertext[:5]) == "stenc" { // if file sig exists we should decrypt
						decryptFile(rootDir, pathToFile, aesblock, plainOrCiphertext)
					}
				} else { // otherwise we're in normal dir
					encryptFile(rootDir, pathToFile, aesblock, plainOrCiphertext)
				}
			}
		}
	}
}

// Perform file decryption on ciphertext
func decryptFile(rootDir string, pathToFile string, aesblock cipher.Block, ciphertext []byte) {
	ciphertext = ciphertext[5:] // strip file sig

	iv := ciphertext[:16]             // extract IV from next 16 bytes
	ciphertext = ciphertext[len(iv):] // and strip IV

	plaintext := make([]byte, len(ciphertext))
	stream := cipher.NewCFBDecrypter(aesblock, iv)
	stream.XORKeyStream(plaintext, ciphertext)

	filepathNull := bytes.IndexByte(plaintext, 0) // find first null char (end of filename)
	if filepathNull == -1 {
		l.Infoln("Cannot find null byte, we may have the wrong key or a corrupt file")
	}
	newFilepath := string(plaintext[:filepathNull]) // extract filepath
	newFilepath = filepath.Base(newFilepath)
	plaintext = plaintext[filepathNull+1:] // remove filepath from file contents
	plaintext = plaintext[32:]             // strip off sha256 plaintext hash (since we're not using them right now)
	plaintext = plaintext[(8 * 3):]        // strip off file access times (since we're not using them right now)

	if _, err := os.Stat(rootDir + string(os.PathSeparator) + newFilepath); os.IsNotExist(err) {
		f, err := os.Create(rootDir + string(os.PathSeparator) + newFilepath)
		if err != nil {
			l.Infoln(err)
		}
		_, err = io.Copy(f, bytes.NewReader(plaintext))
		if err != nil {
			l.Infoln(err)
		}
	} else {
		l.Debugln("File already exists... ignoring decryption of file")
	}
}

// Perform file encryption on plaintext
func encryptFile(rootDir string, pathToFile string, aesblock cipher.Block, plaintext []byte) {
	// Get file MAC times and make byte arrays out of them
	mtime, atime, ctime, err := fileStatTimes(pathToFile)
	bmtime, batime, bctime := make([]byte, 8), make([]byte, 8), make([]byte, 8)
	binary.LittleEndian.PutUint64(bmtime, uint64(mtime))
	binary.LittleEndian.PutUint64(batime, uint64(atime))
	binary.LittleEndian.PutUint64(bctime, uint64(ctime))
	// To revert to int again ==>     i = int64(binary.LittleEndian.Uint64(b))

	// The IV needs to be unique but not secure, so put it at beginning of ciphertext
	h := sha256.New()

	// Never use more than 2^32 random nonces with given key because of risk of repeat
	iv := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err.Error())
	}
	// h.Write([]byte(pathToFile))
	// var iv []byte
	// iv = append(iv, h.Sum(nil)...)

	if _, err := io.Copy(h, bytes.NewReader(plaintext)); err != nil {
		log.Fatal(err)
	}

	sha256HashBytes := h.Sum(nil) // sha256 of just the filedata by itself

	fileHeader := append([]byte(pathToFile), 0)         // null byte terminated
	fileHeader = append(fileHeader, sha256HashBytes...) // plaintext hash
	fileHeader = append(fileHeader, bmtime...)          // mod time
	fileHeader = append(fileHeader, batime...)          // access time
	fileHeader = append(fileHeader, bctime...)          // created time

	plaintext = append(fileHeader, plaintext...) // prepend fileheader to contents
	ciphertext := make([]byte, len(plaintext))

	iv = iv[:16] // make sure we're using only the first 16 bytes of iv (if there happen to be more)
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

// This function should ensure we get the key from the same folderCfg despite working with 2 different folders
func (m *Model) getKeyFromConfig(folder string) []byte {
	rootDir := m.folderCfgs[folder].Path()
	key := []byte(m.folderCfgs[folder].EncryptionKey)

	if strings.HasSuffix(rootDir, ".ENC") { // make sure we're not running decryption in regular dir (only enc dir)
		// now strip ".ENC" from folder path and find the folder ID for that path so we use the same key of the
		// folder that encrypted the file.  This avoids an error where using 2 different keys causes either
		// decyrption to fail or produce further gibberish which can be somewhat hard/confusing to debug
		for id, fldrCfg := range m.folderCfgs {
			if rootDir[:len(rootDir)-4] == fldrCfg.Path() { // rootDir without the ".ENC"
				folder = id
				key = []byte(m.folderCfgs[folder].EncryptionKey)
			}
		}
	}

	return key
}

// Enforce exactly 32 bytes long for the key
func stripOrPadKeyLength(key []byte, size int) []byte {
	// Check if key too short, if so pad with 0's
	if len(key) < size {
		for i := len(key); i < (size); i++ {
			if i >= len(key) {
				key = append(key, '0')
			}
		}
	}
	// Check if key too long, if so crop it
	if len(key) > size {
		key = key[:size]
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
