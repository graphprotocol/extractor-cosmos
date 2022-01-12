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
}
