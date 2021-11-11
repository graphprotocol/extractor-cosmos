package extractor

import (
	"testing"
	"time"

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
		assert.Equal(t, "cant use bundling with STDOUT", config.Validate().Error())
	})
}

func TestGetOutputFile(t *testing.T) {
	examples := []struct {
		input    string
		expected string
	}{
		{"file", "file"},
		{"file-$date", "file-" + time.Now().UTC().Format("20060102")},
		{"file-$time", "file-" + time.Now().UTC().Format("150405")},
		{"file-$ts", "file-" + time.Now().UTC().Format("20060102-150405")},
	}

	for _, ex := range examples {
		config := Config{OutputFile: ex.input}
		assert.Equal(t, ex.expected, config.GetOutputFile())
	}
}
