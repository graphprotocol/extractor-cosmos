package extractor

import (
	"errors"
)

// Config declares extractor service configuration options
type Config struct {
	RootDir     string `mapstructure:"home"`
	Enabled     bool   `mapstructure:"enabled"`
	OutputFile  string `mapstructure:"output_file"`
	StartHeight int64  `mapstructure:"start_height"`
	EndHeight   int64  `mapstructure:"end_height"`
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
