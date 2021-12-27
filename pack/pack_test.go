package pack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecode(t *testing.T) {
	path := "/some/path/to/file.txt"
	s := encodePath(path)
	s2, err := decodePath(s)
	assert.Nil(t, err)
	assert.Equal(t, path, s2)
}
