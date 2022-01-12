package extractor

import (
	"fmt"
	"sync"
)

const (
	MsgBegin              = "BEGIN"
	MsgEnd                = "END"
	MsgBlock              = "BLOCK"
	MsgBlockHeader        = "BLOCK_HEADER"
	MsgEvidence           = "EVIDENCE"
	MsgTx                 = "TX"
	MsgValidatorSetUpdate = "VSET_UPDATE"
)

var (
	dmPrefix   = "DMLOG "
	writer     Writer
	writerLock = sync.Mutex{}
)

func SetPrefix(val string) {
	if val == "" {
		dmPrefix = ""
		return
	}
	dmPrefix = val + " "
}

func GetPrefix() string {
	return dmPrefix
}

func SetWriter(w Writer) {
	writer = w
}

func SetWriterFromConfig(config *Config) {
	writer = InitWriter(config)
}

func GetWriter() Writer {
	return writer
}

func SetHeight(height int64) error {
	if writer == nil {
		panic("writer is not set")
	}

	writerLock.Lock()
	defer writerLock.Unlock()

	return writer.SetHeight(height)
}

func WriteLine(kind string, format string, args ...interface{}) error {
	if writer == nil {
		panic("writer is not set")
	}

	writerLock.Lock()
	defer writerLock.Unlock()

	return writer.WriteLine(dmPrefix + kind + " " + fmt.Sprintf(format, args...))
}
