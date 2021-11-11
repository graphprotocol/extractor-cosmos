package extractor

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBundleWriter(t *testing.T) {
	basePath := fmt.Sprintf("/tmp/%v", time.Now().Unix())

	t.Run("file creation", func(t *testing.T) {
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
	})

	t.Run("writing", func(t *testing.T) {
		writer := NewBundleWriter(basePath, 1000)
		assert.Equal(t, writer.WriteLine("test").Error(), "output file is not initialized")

		assert.NoError(t, writer.SetHeight(0))
		assert.NoError(t, writer.WriteLine("test"))
		assert.NoError(t, writer.WriteLine("test2"))
		assert.Equal(t, "test\ntest2\n", fileContent(basePath+".0000000000"))
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileContent(path string) string {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}
