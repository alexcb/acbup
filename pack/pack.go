package pack

import (
	"bufio"
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
)

// Pack defines the pack interface
type Pack interface {
	AddDir(string) error
	AddFile(string) error
	Close() error
	List() ([]string, error)
}

type packImp struct {
	root     string
	refs     []*refEntry
	refIndex map[string]*refEntry
}

// New returns a new Pack
func New(packRoot string) (Pack, error) {
	refPathParts := []string{packRoot, "refs", "meta"}
	refPath := filepath.Join(refPathParts...)

	// TODO check packRoot exists first

	err := os.MkdirAll(filepath.Join(packRoot, "refs"), 0700)
	if err != nil {
		return nil, err
	}

	var refs []*refEntry
	refIndex := map[string]*refEntry{}
	if fileutil.FileExists(refPath) {
		refs, err = readRefs(refPath)
		if err != nil {
			return nil, err
		}
		refIndex = buildRefIndex(refs)
	}

	p := &packImp{
		root:     packRoot,
		refIndex: refIndex,
		refs:     refs,
	}

	return p, nil
}

// Close closes the pack
func (p *packImp) Close() error {
	refPathParts := []string{p.root, "refs", "meta"}
	refPath := filepath.Join(refPathParts...)
	return writeRefs(refPath, p.refs)
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

func copyFile(src, dst, expectedHash string) error {
	dirPath := filepath.Dir(dst)
	err := os.MkdirAll(dirPath, 0700)
	if err != nil {
		return err
	}

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
	return nil
}

var errInvalidSha1 = fmt.Errorf("invalid sha1")

func readExpectedSha1(path string) (string, error) {
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
	restoredSha1, err := getSha1(pathBkup)
	if err != nil {
		return err
	}
	if restoredSha1 != expectedSha1 {
		return fmt.Errorf("restore failed to produce matching sha1 expected %s vs actual %s", expectedSha1, restoredSha1)
	}
	fmt.Fprintf(os.Stderr, "restored %s from %s\n", path, pathBkup)
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

func readRefs(path string) ([]*refEntry, error) {
	refsSha1, err := getSha1(path)
	if err != nil {
		return nil, err
	}

	expectedSha1, err := readExpectedSha1(path + ".sha1")
	if err != nil {
		return nil, err
	}
	if refsSha1 != expectedSha1 {
		err = restoreFromBkup(path, expectedSha1)
		if err != nil {
			return nil, fmt.Errorf("%s corrupt: expected sha1 %s vs actual %s; attempted recovery failed: %s", path, expectedSha1, refsSha1, err)
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

func writeRefs(path string, refs []*refEntry) error {
	pathTmp := path + ".tmp"

	pathSha := path + ".sha1"
	pathShaTmp := pathSha + ".tmp"

	f, err := os.OpenFile(pathTmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)

	h := sha1.New()
	for _, ref := range refs {
		encPath := encodePath(ref.path)
		data := fmt.Sprintf("%s %s\n", encPath, ref.sha1)
		_, err := io.WriteString(w, data)
		if err != nil {
			return err
		}
		_, err = io.WriteString(h, data)
		if err != nil {
			return err
		}
	}

	err = w.Flush()
	if err != nil {
		return err
	}

	err = f.Close()
	if err != nil {
		return err
	}

	// write sha1
	hash := fmt.Sprintf("%x", h.Sum(nil))

	sha1File, err := os.OpenFile(pathShaTmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	_, err = io.WriteString(sha1File, hash)
	if err != nil {
		return err
	}

	err = sha1File.Close()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "renaming %s -> %s\n", pathTmp, path)
	err = os.Rename(pathTmp, path)
	if err != nil {
		return err
	}

	// TODO create parity bits instead
	pathCopy := path + ".bkup"
	fmt.Fprintf(os.Stderr, "creating backup %s -> %s\n", path, pathCopy)
	err = fileutil.CopyFileContents(path, pathCopy)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "renaming %s -> %s\n", pathShaTmp, pathSha)
	err = os.Rename(pathShaTmp, pathSha)
	if err != nil {
		return err
	}
	return nil
}

// AddFile adds a file to the pack
func (p *packImp) AddFile(path string) error {
	inputHash, err := getSha1(path)
	if err != nil {
		return err
	}

	if ref, ok := p.refIndex[path]; ok {
		if ref.sha1 != inputHash {
			fmt.Fprintf(os.Stderr, "ERROR: local copy of %s has been changed since backup; curent hash %s vs backed up %s\n", path, inputHash, ref.sha1)
			panic("TODO: prompt user on what to do -- accept file modification, or assume local copy was corrupted")
		}
	}

	dataPathParts := []string{p.root, "data"}
	dataPathParts = append(dataPathParts, splitShaToPath(inputHash)...)
	dataPath := filepath.Join(dataPathParts...)

	currentBackupSha1, err := getSha1(dataPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "%q -> %q; %s backing up\n", path, inputHash, dataPath)
			err = copyFile(path, dataPath, inputHash)
			if err != nil {
				return err
			}
			return p.addMeta(path, inputHash)
		}
		return err
	}
	if inputHash != currentBackupSha1 {
		// the backed up copy must be corrupt, if not then it would have been stored under a different path
		fmt.Fprintf(os.Stderr, "ERROR WARNING CORRUPT DATA FOUND!!!! re-backing up data %q -> %q; %s\n", path, inputHash, dataPath)
		return copyFile(path, dataPath, inputHash)
	}

	fmt.Fprintf(os.Stderr, "%q -> %q; %s already backedup (and verified)\n", path, inputHash, dataPath)
	return p.addMeta(path, inputHash)
}

// AddDir adds a dir to the pack
func (p *packImp) AddDir(path string) error {
	err := filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			return p.AddFile(path)
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
	if ref, ok := p.refIndex[path]; ok && ref.sha1 == sha1 {
		if ref.path != path {
			panic("ref path is corrupt")
		}
		return nil
	}

	ref := &refEntry{
		path: path,
		sha1: sha1,
	}
	p.refs = append(p.refs, ref)
	p.refIndex[path] = ref

	return nil
}
