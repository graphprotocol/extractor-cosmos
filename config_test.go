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

func TestValidate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		config := Config{}

		assert.NoError(t, config.Validate())
	})

	t.Run("enabled", func(t *testing.T) {
		config := Config{}

		assert.NoError(t, config.Validate())
		assert.Equal(t, false, config.Enabled)
		assert.Equal(t, "STDOUT", config.OutputFile)
	})

	t.Run("stdout grouping", func(t *testing.T) {
		config := Config{
			Enabled: true,
			Bundle:  true,
		}
		assert.Equal(t, "cant use output grouping with STDOUT", config.Validate().Error())
	})
}
