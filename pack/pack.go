package pack

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexcb/acbup/util/fileutil"
	"github.com/alexcb/acbup/util/promptutil"
)

// Pack defines the pack interface
type Pack interface {
	AddDir(string, string) error
	AddFile(string, string) error
	Close() error
	List() ([]string, error)
	Verify() bool
	Recover() (int, int, int, error)
	Restore(string, string) error
}

type packImp struct {
	root        string
	refs        []*refEntry
	refIndex    map[string]*refEntry
	readOnly    bool
	interactive bool
	parityBits  int
}

var errInvalidParityBitsConfig = fmt.Errorf("invalid parity bits config")

// New returns a new Pack
func New(packRoot string, readOnly, interactive bool, parityBits int) (Pack, error) {
	var refs []*refEntry
	refIndex := map[string]*refEntry{}

	if parityBits < 0 || parityBits > 1 {
		fmt.Printf("got %d\n", parityBits)
		return nil, errInvalidParityBitsConfig
	}

	refsPath := filepath.Join(packRoot, "refs")
	if fileutil.FileExists(refsPath) {
		refsSha1, err := readFileContainingSha1Reference(refsPath)
		if err != nil {
			return nil, err
		}

		path, err := getShaPath(packRoot, refsSha1, false)
		if err != nil {
			return nil, err
		}

		refs, err = readRefs(path, refsSha1, readOnly)
		if err != nil {
			return nil, err
		}
		refIndex = buildRefIndex(refs)

	}

	p := &packImp{
		root:        packRoot,
		refIndex:    refIndex,
		refs:        refs,
		readOnly:    readOnly,
		interactive: interactive,
		parityBits:  parityBits,
	}

	return p, nil
}

// Close closes the pack
func (p *packImp) Close() error {
	return p.writeRefs(p.refs)
}

func splitShaToPath(s string) []string {
	if len(s) != 40 {
		panic("s is not a sha1")
	}
	return []string{
		s[0:2],
		s[2:4],
		s[4:6],
		s,
	}
}

func getShaPath(packRoot, sha1 string, mkDir bool) (string, error) {
	if len(sha1) != 40 {
		panic("invalid sha")
	}

	dataPathParts := []string{packRoot, "data"}
	dataPathParts = append(dataPathParts, splitShaToPath(sha1)...)
	path := filepath.Join(dataPathParts...)

	if mkDir {
		dirPath := filepath.Dir(path)
		err := os.MkdirAll(dirPath, 0700)
		if err != nil {
			return "", err
		}
	}

	return path, nil
}

func getSha1(path string) (string, error) {
	h := sha1.New()

	const bufferSize = 1024 * 1024 * 16
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, bufferSize)

	for {
		bytesread, err := file.Read(buffer)
		if err != nil {
			if err != io.EOF {
				return "", err
			}
			break
		}
		io.WriteString(h, string(buffer[:bytesread]))
	}

	hash := fmt.Sprintf("%x", h.Sum(nil))
	return hash, nil
}

func copyFile(src, dst, expectedHash string, parityBits int) error {
	const bufferSize = 1024 * 1024 * 16
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	h := sha1.New()
	buffer := make([]byte, bufferSize)

	for {
		bytesread, err := srcFile.Read(buffer)
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		io.WriteString(dstFile, string(buffer[:bytesread]))
		io.WriteString(h, string(buffer[:bytesread]))
	}
	hash := fmt.Sprintf("%x", h.Sum(nil))
	if hash != expectedHash {
		panic("hash missmatch, perhaps someone else wrote to the file while the copy was happening?")
	}

	// TODO create parity bits instead
	if parityBits == 1 {
		pathCopy := dst + ".bkup"
		fmt.Fprintf(os.Stderr, "creating backup %s -> %s\n", dst, pathCopy)
		err = fileutil.CopyFileContents(dst, pathCopy)
		if err != nil {
			return err
		}
	}

	return nil
}

var errInvalidSha1 = fmt.Errorf("invalid sha1")

func readFileContainingSha1Reference(path string) (string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := string(data)
	if len(s) != 40 {
		return "", errInvalidSha1
	}
	return s, nil
}

func restoreFromBkup(path, expectedSha1 string) error {
	pathBkup := path + ".bkup"
	actualSha1, err := getSha1(pathBkup)
	if err != nil {
		return err
	}
	if actualSha1 != expectedSha1 {
		return fmt.Errorf("bkup sha1 expected %s vs actual %s", expectedSha1, actualSha1)
	}
	err = fileutil.CopyFileContents(pathBkup, path)
	if err != nil {
		return err
	}
	restoredSha1, err := getSha1(path)
	if err != nil {
		return err
	}
	if restoredSha1 != expectedSha1 {
		return fmt.Errorf("restore failed to produce matching sha1 expected %s vs actual %s", expectedSha1, restoredSha1)
	}
	fmt.Fprintf(os.Stderr, "restored %s from %s\n", path, pathBkup)
	return nil
}

func rebuildBkup(path, expectedSha1 string) error {
	pathBkup := path + ".bkup"
	actualSha1, err := getSha1(path)
	if err != nil {
		return err
	}
	if actualSha1 != expectedSha1 {
		return fmt.Errorf("original sha1 expected %s vs actual %s", expectedSha1, actualSha1)
	}
	err = fileutil.CopyFileContents(path, pathBkup)
	if err != nil {
		return err
	}
	restoredSha1, err := getSha1(pathBkup)
	if err != nil {
		return err
	}
	if restoredSha1 != expectedSha1 {
		return fmt.Errorf("restore failed to produce matching sha1 expected %s vs actual %s", expectedSha1, restoredSha1)
	}
	fmt.Fprintf(os.Stderr, "rebuilt %s from %s\n", pathBkup, path)
	return nil
}

type refEntry struct {
	path string
	sha1 string
}

func buildRefIndex(refs []*refEntry) map[string]*refEntry {
	m := map[string]*refEntry{}
	for _, ref := range refs {
		m[ref.path] = ref
	}
	return m
}

func readRefs(path, expectedSha1 string, readOnly bool) ([]*refEntry, error) {
	refsSha1, err := getSha1(path)
	if err != nil {
		return nil, err
	}

	if refsSha1 != expectedSha1 {
		if readOnly {
			return nil, fmt.Errorf("detected corruption in %s while reading refs: expected sha1 %s but got %s", path, expectedSha1, refsSha1)
		}
		err = restoreFromBkup(path, expectedSha1)
		if err != nil {
			return nil, fmt.Errorf("detected corruption in %s while reading refs: expected sha1 %s but got %s; attempted recovery failed: %s", path, expectedSha1, refsSha1, err)
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	refs := []*refEntry{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			panic("corrupt meta file")
		}
		encodedPath := fields[0]
		dataRef := fields[1]

		path, err := decodePath(encodedPath)
		if err != nil {
			return nil, err
		}
		refs = append(refs, &refEntry{
			path: path,
			sha1: dataRef,
		})
	}
	return refs, nil
}

func (p *packImp) writeRefs(refs []*refEntry) error {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	for _, ref := range refs {
		encPath := encodePath(ref.path)
		data := fmt.Sprintf("%s %s\n", encPath, ref.sha1)
		_, err := io.WriteString(w, data)
		if err != nil {
			return err
		}
	}

	err := w.Flush()
	if err != nil {
		return err
	}

	data := buf.String()

	// write sha1
	h := sha1.New()
	h.Write([]byte(data))
	hash := fmt.Sprintf("%x", h.Sum(nil))

	dataPath, err := getShaPath(p.root, hash, true)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "writing to %s\n", dataPath)
	sha1File, err := os.OpenFile(dataPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	_, err = io.WriteString(sha1File, data)
	if err != nil {
		return err
	}

	err = sha1File.Close()
	if err != nil {
		return err
	}

	// TODO create parity bits instead
	if p.parityBits == 1 {
		pathCopy := dataPath + ".bkup"
		fmt.Fprintf(os.Stderr, "creating backup %s -> %s\n", dataPath, pathCopy)
		err = fileutil.CopyFileContents(dataPath, pathCopy)
		if err != nil {
			return err
		}
	}

	refsPath := filepath.Join(p.root, "refs")
	fmt.Fprintf(os.Stderr, "writing to %s\n", refsPath)
	file, err := os.OpenFile(refsPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = io.WriteString(file, hash)
	if err != nil {
		return err
	}

	return file.Close()
}

// AddFile adds a file to the pack
func (p *packImp) AddFile(path, alias string) error {
	inputHash, err := getSha1(path)
	if err != nil {
		return err
	}

	var pathAndAlias string
	if path == alias {
		pathAndAlias = path
	} else {
		pathAndAlias = fmt.Sprintf("%s (%s)", path, alias)
	}

	if ref, ok := p.refIndex[alias]; ok {
		if ref.sha1 != inputHash {
			fmt.Fprintf(os.Stderr, "ERROR: local copy of %s has been changed since backup; curent hash %s vs backed up %s\n", pathAndAlias, inputHash, ref.sha1)
			if !p.interactive {
				return fmt.Errorf("unable to save/skip changed file in non-interactive mode")
			}
			choice, err := promptutil.Prompt("Save the new version of %s? [y/N] ", []string{"y", "n"}, 1, true)
			if err != nil {
				return err
			}
			if choice == "n" {
				return nil
			}
		}
	}

	dataPath, err := getShaPath(p.root, inputHash, true)
	if err != nil {
		return err
	}

	currentBackupSha1, err := getSha1(dataPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "%q -> %q; %s backing up\n", pathAndAlias, inputHash, dataPath)
			err = copyFile(path, dataPath, inputHash, p.parityBits)
			if err != nil {
				return err
			}
			return p.addMeta(alias, inputHash)
		}
		return err
	}
	if inputHash != currentBackupSha1 {
		// the backed up copy must be corrupt, if not then it would have been stored under a different path
		fmt.Fprintf(os.Stderr, "ERROR WARNING CORRUPT DATA FOUND!!!! re-backing up data %q -> %q; %s\n", pathAndAlias, inputHash, dataPath)
		return copyFile(path, dataPath, inputHash, p.parityBits)
	}

	// TODO why is there another call to addMeta? perhaps for a last-seen timestamp?
	fmt.Fprintf(os.Stderr, "%q -> %q; %s already backedup (and verified)\n", pathAndAlias, inputHash, dataPath)
	return p.addMeta(alias, inputHash)
}

// AddDir adds a dir to the pack
func (p *packImp) AddDir(path, alias string) error {
	if alias != path {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("path must start with /")
		}
		if !strings.HasPrefix(alias, "/") {
			return fmt.Errorf("alias must start with /")
		}
	}
	n := len(path)
	err := filepath.Walk(path,
		func(walkPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			walkAlias := alias + walkPath[n:]
			return p.AddFile(walkPath, walkAlias)
		})
	return err
}

// List lists files in the pack
func (p *packImp) List() ([]string, error) {
	files := []string{}
	for _, ref := range p.refIndex {
		files = append(files, ref.path)
	}
	sort.Strings(files)
	return files, nil
}

func (p *packImp) verifyData(sha1 string) error {
	dataPath, err := getShaPath(p.root, sha1, false)
	if err != nil {
		return err
	}

	actualSha1, err := getSha1(dataPath)
	if err != nil {
		return err
	}
	if actualSha1 != sha1 {
		return fmt.Errorf("%s is corrupt; should be %s but instead is %s", dataPath, sha1, actualSha1)
	}
	return nil
}

func (p *packImp) verifyDataBkup(sha1 string) error {
	dataPath, err := getShaPath(p.root, sha1, false)
	if err != nil {
		return err
	}
	dataPath += ".bkup"

	actualSha1, err := getSha1(dataPath)
	if err != nil {
		return err
	}
	if actualSha1 != sha1 {
		return fmt.Errorf("%s is corrupt; should be %s but instead is %s", dataPath, sha1, actualSha1)
	}
	return nil
}

// Verify verifies integrety of backup
func (p *packImp) Verify() bool {
	failed := false
	for _, ref := range p.refs {
		fmt.Fprintf(os.Stderr, "verifying %s -> %s... ", ref.path, ref.sha1)
		err := p.verifyData(ref.sha1)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: %s\n", err)
			failed = true
			continue
		}

		if p.parityBits > 0 {
			err = p.verifyDataBkup(ref.sha1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAILED: %s\n", err)
				failed = true
				continue
			}
		}
		fmt.Fprintf(os.Stderr, "OK\n")
	}
	return !failed
}

// Recover attempts to recover
func (p *packImp) Recover() (int, int, int, error) {
	numOK := 0
	numRecovered := 0
	numFailed := 0
	for _, ref := range p.refs {
		fmt.Fprintf(os.Stderr, "verifying %s -> %s... ", ref.path, ref.sha1)
		err := p.verifyData(ref.sha1)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: %s\n", err)

			path, err := getShaPath(p.root, ref.sha1, false)
			if err != nil {
				return 0, 0, 0, err
			}
			err = restoreFromBkup(path, ref.sha1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "RECOVERY-FAILED: %s\n", err)
				numFailed++
			} else {
				numRecovered++
				fmt.Fprintf(os.Stderr, "recovered\n")
			}
			continue
		}

		if p.parityBits > 0 {
			err = p.verifyDataBkup(ref.sha1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAILED: %s\n", err)

				path, err := getShaPath(p.root, ref.sha1, false)
				if err != nil {
					return 0, 0, 0, err
				}
				err = rebuildBkup(path, ref.sha1)
				if err != nil {
					fmt.Fprintf(os.Stderr, "RECOVERY-FAILED: %s\n", err)
					numFailed++
				} else {
					numRecovered++
					fmt.Fprintf(os.Stderr, "recovered\n")
				}
				continue
			}
		}

		numOK++
		fmt.Fprintf(os.Stderr, "OK\n")
	}
	return numOK, numRecovered, numFailed, nil
}

// Restore overwrites the local file with the backed up file
func (p *packImp) Restore(aliasPath, localPath string) error {
	ref, ok := p.refIndex[aliasPath]
	if !ok {
		return fmt.Errorf("%s not in backup", aliasPath)
	}

	bkupPath, err := getShaPath(p.root, ref.sha1, false)
	if err != nil {
		return err
	}

	actualSha1, err := getSha1(bkupPath)
	if err != nil {
		return err
	}
	if ref.sha1 != actualSha1 {
		return fmt.Errorf("bkup %s is corrupt; expected %s but actual hash is %s", bkupPath, ref.sha1, actualSha1)
	}

	err = os.MkdirAll(filepath.Dir(localPath), 0700)
	if err != nil {
		return err
	}

	return fileutil.CopyFileContents(bkupPath, localPath)
}

func encodePath(path string) string {
	return base64.StdEncoding.EncodeToString([]byte(path))
}

func decodePath(encPath string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (p *packImp) addMeta(path, sha1 string) error {

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absPath, "/") {
		panic("bad path")
	}

	// keep existing if up to date
	if ref, ok := p.refIndex[absPath]; ok && ref.sha1 == sha1 {
		if ref.path != absPath {
			panic("ref path is corrupt")
		}
		return nil
	}

	ref := &refEntry{
		path: absPath,
		sha1: sha1,
	}
	p.refs = append(p.refs, ref)
	p.refIndex[absPath] = ref

	return nil
}
