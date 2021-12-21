package pack

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Pack interface {
	AddDir(string) error
	AddFile(string) error
}

type packImp struct {
	root string
}

func New(packRoot string) Pack {
	return &packImp{
		root: packRoot,
	}
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
			return copyFile(path, dataPath, inputHash)
		}
		return err
	}
	if inputHash != currentBackupSha1 {
		// the backed up copy must be corrupt, if not then it would have been stored under a different path
		panic("backup is corrupt, gotta replace it!")
	}

	fmt.Printf("%q -> %q; %s already backedup (and verified)\n", path, inputHash, dataPath)
	return nil
}

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
