package extractor

import (
	"errors"
	"fmt"
	"os"
)

type bundleWriter struct {
	filename    string
	file        *os.File
	height      int
	chunkSize   int
	chunkHeight int
}

func NewBundleWriter(filename string, chunkSize int) Writer {
	return &bundleWriter{
		filename:  filename,
		chunkSize: chunkSize,
	}
}

func (w *bundleWriter) SetHeight(val int) error {
	if w.chunkSize == 0 {
		return errors.New("chunk size is zero")
	}

	chunkHeight := val - (val % w.chunkSize)
	if chunkHeight == w.chunkHeight {
		return nil
	}

	w.chunkHeight = chunkHeight
	return w.prepareOutput()
}

func (w *bundleWriter) prepareOutput() error {
	if err := w.Close(); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s.%010d", w.filename, w.chunkHeight)
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0666)
	if err != nil {
		return err
	}
	w.file = file

	return nil
}

func (w *bundleWriter) WriteLine(data string) error {
	if w.file == nil {
		return errors.New("output file is not initialized")
	}

	_, err := fmt.Fprintf(w.file, "%s\n", data)
	return err
}

func (w *bundleWriter) Close() error {
	if w.file == nil {
		return nil
	}
	if err := w.file.Sync(); err != nil {
		return err
	}
	return w.file.Close()
}
