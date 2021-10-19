package extractor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, false, config.Enabled)
	assert.Equal(t, "STDOUT", config.OutputFile)
	assert.Equal(t, int64(0), config.StartHeight)
	assert.Equal(t, int64(0), config.EndHeight)
	assert.Equal(t, "", config.RootDir)
}
