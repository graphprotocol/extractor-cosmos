package extractor

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBundleWriter(t *testing.T) {
	basePath := fmt.Sprintf("/tmp/%v", time.Now().Unix())

	writer := NewBundleWriter(basePath, 0)
	assert.Equal(t, writer.SetHeight(0).Error(), "chunk size is zero")

	writer = NewBundleWriter(basePath, 1000)
	assert.NoError(t, writer.SetHeight(0))
	assert.True(t, fileExists(basePath+".0000000000"))

	writer.SetHeight(100)
	assert.True(t, fileExists(basePath+".0000000000"))
	assert.False(t, fileExists(basePath+".0000001000"))

	writer.SetHeight(1234)
	assert.True(t, fileExists(basePath+".0000001000"))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
