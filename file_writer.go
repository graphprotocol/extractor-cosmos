package extractor

import (
	"fmt"
	"os"
)

type fileWriter struct {
	filename string
	file     *os.File
}

func NewFileWriter(filename string) Writer {
	return &fileWriter{
		filename: filename,
	}
}

func (w *fileWriter) SetHeight(height int64) error {
	return nil
}

func (w *fileWriter) WriteLine(data string) error {
	if w.file == nil {
		if err := w.prepareOutput(); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(w.file, "%s\n", data)
	return err
}

func (w *fileWriter) Close() error {
	if w.file == nil {
		return nil
	}
	if err := w.file.Sync(); err != nil {
		return nil
	}
	return w.file.Close()
}

func (w *fileWriter) prepareOutput() error {
	file, err := os.OpenFile(w.filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0666)
	if err != nil {
		return err
	}

	w.file = file
	return nil
}
