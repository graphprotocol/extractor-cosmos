package extractor

// Config declares extractor service configuration options
type Config struct {
	Enabled     bool
	RootDir     string
	OutputFile  string
	StartHeight int64
}
