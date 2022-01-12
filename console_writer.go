package extractor

import (
	"fmt"
	"io"
)

type consoleWriter struct {
	dst io.Writer
}

func NewConsoleWriter(dst io.Writer) Writer {
	return &consoleWriter{
		dst: dst,
	}
}

func (c consoleWriter) SetHeight(height int64) error {
	return nil
}

func (w consoleWriter) WriteLine(data string) error {
	_, err := fmt.Fprintf(w.dst, "%s\n", data)
	return err
}

func (w consoleWriter) Close() error {
	return nil
}
