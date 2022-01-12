package extractor

import "os"

type Writer interface {
	SetHeight(height int64) error
	WriteLine(data string) error
	Close() error
}

var (
	_ Writer = (*bundleWriter)(nil)
	_ Writer = (*fileWriter)(nil)
	_ Writer = (*consoleWriter)(nil)
)

func InitWriter(config *Config) Writer {
	filename := config.GetOutputFile()

	if config.Bundle {
		return NewBundleWriter(filename, config.BundleSize)
	}

	switch filename {
	case "", "stdout", "STDOUT":
		return NewConsoleWriter(os.Stdout)
	case "stderr", "STDERR":
		return NewConsoleWriter(os.Stderr)
	default:
		return NewFileWriter(filename)
	}
}
