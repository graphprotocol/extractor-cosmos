package extractor

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetWriterFromConfig(t *testing.T) {
	SetWriterFromConfig(&Config{})
	assert.IsType(t, &consoleWriter{}, writer)

	SetWriterFromConfig(&Config{OutputFile: "STDOUT"})
	assert.IsType(t, &consoleWriter{}, writer)

	SetWriterFromConfig(&Config{OutputFile: "STDERR"})
	assert.IsType(t, &consoleWriter{}, writer)

	SetWriterFromConfig(&Config{OutputFile: fmt.Sprintf("/tmp/%v", time.Now().Unix())})
	assert.IsType(t, &fileWriter{}, writer)

	SetWriterFromConfig(&Config{
		OutputFile: fmt.Sprintf("/tmp/%v", time.Now().Unix()),
		Bundle:     true,
	})
	assert.IsType(t, &bundleWriter{}, writer)
}

func TestExtractorWriteLine(t *testing.T) {
	output := bytes.NewBuffer(nil)
	writer := NewConsoleWriter(output)
	SetWriter(writer)

	assert.NoError(t, WriteLine("TYPE", "%s", "some message"))

	// Osmosis block
	assert.NoError(t, WriteLine("BLOCK", "%s", "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"))
}
