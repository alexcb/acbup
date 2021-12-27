package pack

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Pack defines the pack interface
type Pack interface {
	AddDir(string) error
	AddFile(string) error
	Close() error
	List() ([]string, error)
}

type packImp struct {
	root string
	refs map[string]string
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

	refs, err := readRefs(refPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		refs = map[string]string{}
	}

	p := &packImp{
		root: packRoot,
		refs: refs,
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

func readRefs(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	m := map[string]string{}

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
		m[path] = dataRef
	}
	return m, nil
}

func writeRefs(path string, refs map[string]string) error {

	pathTmp := path + ".tmp"

	f, err := os.OpenFile(pathTmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)

	for path, ref := range refs {
		encPath := encodePath(path)
		_, err := fmt.Fprintf(w, "%s %s\n", encPath, ref)
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

	// TODO write parity bits, then move old file to bkup location before moving tmp to real

	return os.Rename(pathTmp, path)
}

// AddFile adds a file to the pack
func (p *packImp) AddFile(path string) error {
	inputHash, err := getSha1(path)
	if err != nil {
		return err
	}

	dataPathParts := []string{p.root, "data"}
	dataPathParts = append(dataPathParts, splitShaToPath(inputHash)...)
	dataPath := filepath.Join(dataPathParts...)

	currentBackupSha1, err := getSha1(dataPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("%q -> %q; %s backing up\n", path, inputHash, dataPath)
			err = copyFile(path, dataPath, inputHash)
			if err != nil {
				return err
			}
			return p.addMeta(path, inputHash)
		}
		return err
	}
	if inputHash != currentBackupSha1 {
		fmt.Printf("ERROR WARNING CORRUPT DATA FOUND!!!! re-backing up data %q -> %q; %s\n", path, inputHash, dataPath)
		return copyFile(path, dataPath, inputHash)
		// the backed up copy must be corrupt, if not then it would have been stored under a different path
		//panic("backup is corrupt, gotta replace it!")
	}

	fmt.Printf("%q -> %q; %s already backedup (and verified)\n", path, inputHash, dataPath)
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
	for k := range p.refs {
		files = append(files, k)
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

	p.refs[path] = sha1

	return nil
}
