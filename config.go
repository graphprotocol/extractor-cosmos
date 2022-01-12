package extractor

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

var (
	timeFormats = map[string]string{
		"$date": "20060102",
		"$time": "150405",
		"$ts":   "20060102-150405",
	}
)

// Config declares extractor service configuration options
type Config struct {
	RootDir     string `mapstructure:"home"`
	Enabled     bool   `mapstructure:"enabled"`
	OutputFile  string `mapstructure:"output_file"`
	StartHeight int64  `mapstructure:"start_height"`
	EndHeight   int64  `mapstructure:"end_height"`
	Bundle      bool   `mapstructure:"bundle"`
	BundleSize  int64  `mapstructure:"bundle_size"`
}

// DefaultConfig returns a default extractor config
func DefaultConfig() *Config {
	return &Config{
		Enabled:    false,
		OutputFile: "STDOUT",
		Bundle:     false,
		BundleSize: 1000,
	}
}

func (c *Config) Validate() error {
	if c.OutputFile == "" {
		c.OutputFile = "STDOUT"
	}

	if !c.Enabled {
		return nil
	}

	if c.OutputFile == "STDOUT" && c.Bundle {
		return errors.New("cant use bundling with STDOUT")
	}

	return nil
}

func (c *Config) GetOutputFile() string {
	if strings.ToLower(c.OutputFile) == "stdout" || strings.ToLower(c.OutputFile) == "stderr" {
		return c.OutputFile
	}

	name := c.OutputFile
	if !strings.HasPrefix(name, "/") {
		name = filepath.Join(c.RootDir, name)
	}

	now := time.Now().UTC()
	for format, val := range timeFormats {
		name = strings.Replace(name, format, now.Format(val), -1)
	}

	return name
}
