package extractor

import (
	"errors"
)

// Config declares extractor service configuration options
type Config struct {
	Enabled     bool
	RootDir     string
	OutputFile  string
	StartHeight int64
	EndHeight   int64
}

// DefaultConfig returns a default extractor config
func DefaultConfig() *Config {
	return &Config{
		Enabled:    false,
		OutputFile: "STDOUT",
	}
}

func (c *Config) Validate() error {
	if c.OutputFile == "" {
		c.OutputFile = "STDOUT"
	}
	if c.Enabled && c.OutputFile == "" {
		return errors.New("output file is not provided")
	}
	return nil
}
