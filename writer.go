package extractor

type Writer interface {
	SetHeight(height int) error
	WriteLine(data string) error
	Close() error
}

var (
	_ Writer = (*bundleWriter)(nil)
	_ Writer = (*fileWriter)(nil)
)
