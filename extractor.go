package extractor

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/types"
)

const (
	subscriberName = "ExtractorService"

	dmPrefix     = "DMLOG "
	dmBlock      = dmPrefix + "BLOCK"
	dmBeginEvent = dmPrefix + "BLOCK_BEGIN_EVENT"
	dmEndEvent   = dmPrefix + "BLOCK_END_EVENT"
	dmTx         = dmPrefix + "TX"
)

var (
	timeFormats = map[string]string{
		"$date": "20060102",
		"$time": "150405",
		"$ts":   "20060102-150405",
	}
)

type ExtractorService struct {
	service.BaseService

	config   *Config
	eventBus *types.EventBus
	handle   io.Closer
}

func NewExtractorService(eventBus *types.EventBus, config *Config) *ExtractorService {
	is := &ExtractorService{
		eventBus: eventBus,
		config:   config,
	}
	is.BaseService = *service.NewBaseService(nil, subscriberName, is)

	return is
}

func (ex *ExtractorService) OnStart() error {
	if ex.config == nil {
		ex.Logger.Info("extractor config is not provided, using default one")
		ex.config = DefaultConfig()
	}
	if err := ex.config.Validate(); err != nil {
		ex.Logger.Error("extractor config validation error", "err", err)
		return err
	}

	blockSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventQueryNewBlock)
	if err != nil {
		return err
	}

	txsSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventQueryTx)
	if err != nil {
		return err
	}

	writer, err := ex.initStreamOutput()
	if err != nil {
		ex.Logger.Error("stream output init failed", "err", err)
		return err
	}

	go ex.listen(writer, blockSub, txsSub)

	return nil
}

func (ex *ExtractorService) OnStop() {
	if ex.eventBus.IsRunning() {
		if err := ex.eventBus.UnsubscribeAll(context.Background(), subscriberName); err != nil {
			ex.Logger.Error("failed to unsubscribe on stop", "err", err)
		}
	}

	if ex.handle != nil {
		ex.handle.Close()
	}
}

func (ex *ExtractorService) listen(w io.Writer, blockSub, txsSub types.Subscription) {
	sync := &sync.Mutex{}

	for {
		msg := <-blockSub.Out()
		eventData := msg.Data().(types.EventDataNewBlock)
		height := eventData.Block.Header.Height

		// Skip extraction on unwanted heights
		if ex.shouldSkipHeight(height) {
			ex.drainSubscription(txsSub, len(eventData.Block.Txs))
			ex.Logger.Info("skipped block", "height", height)
			continue
		}

		if err := indexBlock(w, sync, eventData); err != nil {
			ex.drainSubscription(txsSub, len(eventData.Block.Txs))
			ex.Logger.Error("failed to index block", "height", height, "err", err)
			continue
		}

		ex.Logger.Info("indexed block", "height", height)

		for i := 0; i < len(eventData.Block.Txs); i++ {
			txMsg := <-txsSub.Out()
			txResult := txMsg.Data().(types.EventDataTx).TxResult

			if err := indexTX(w, sync, &txResult); err != nil {
				ex.Logger.Error("failed to index block txs", "height", height, "err", err)
			} else {
				ex.Logger.Debug("indexed block txs", "height", height)
			}
		}
	}
}

func (ex *ExtractorService) shouldSkipHeight(height int64) bool {
	return ex.config.StartHeight > 0 && height < ex.config.StartHeight ||
		ex.config.EndHeight > 0 && height > ex.config.EndHeight
}

func (ex *ExtractorService) drainSubscription(sub types.Subscription, n int) {
	for i := 0; i < n; i++ {
		<-sub.Out()
	}
}

func (ex *ExtractorService) initStreamOutput() (io.Writer, error) {
	var (
		writer     io.Writer
		outputFile string
	)

	switch ex.config.OutputFile {
	case "stdout", "STDOUT":
		writer = os.Stdout
		outputFile = "STDOUT"
	case "stderr", "STDERR":
		writer = os.Stderr
		outputFile = "STDERR"
	default:
		if strings.HasPrefix(ex.config.OutputFile, "/") {
			outputFile = ex.config.OutputFile
		} else {
			outputFile = filepath.Join(ex.config.RootDir, ex.config.OutputFile)
		}

		outputFile = formatFilename(outputFile)
		file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0666)
		if err != nil {
			return nil, err
		}

		ex.handle = file
		writer = file
	}

	ex.Logger.Info("configured stream output", "dest", outputFile)
	return writer, nil
}

func indexTX(out io.Writer, sync *sync.Mutex, result *abci.TxResult) error {
	rawBytes, err := proto.Marshal(result)
	if err != nil {
		return err
	}

	sync.Lock()
	defer sync.Unlock()

	_, err = io.WriteString(out, fmt.Sprintf("%s %d %d %s\n",
		dmTx,
		result.Height,
		result.Index,
		base64.StdEncoding.EncodeToString(rawBytes),
	))

	return err
}

func indexBlock(out io.Writer, sync *sync.Mutex, bh types.EventDataNewBlock) error {
	blockP, err := bh.Block.ToProto()
	if err != nil {
		return err
	}

	marshaledBlock, err := blockP.Marshal()
	if err != nil {
		return err
	}

	sync.Lock()
	defer sync.Unlock()

	_, err = fmt.Fprintf(out, "%s %d %d %s\n",
		dmBlock,
		bh.Block.Header.Height,
		bh.Block.Header.Time.UnixMilli(),
		base64.StdEncoding.EncodeToString(marshaledBlock),
	)
	if err != nil {
		return err
	}

	for i, ev := range bh.ResultBeginBlock.Events {
		attrs := attributesString(ev.Attributes)
		_, err = io.WriteString(out, fmt.Sprintf("%s %d %d %s %s \n", dmBeginEvent, bh.Block.Header.Height, i, ev.Type, attrs))
		if err != nil {
			return err
		}
	}

	for i, ev := range bh.ResultEndBlock.Events {
		attrs := attributesString(ev.Attributes)
		_, err = io.WriteString(out, fmt.Sprintf("%s %d %d %s %s \n", dmEndEvent, bh.Block.Header.Height, i, ev.Type, attrs))
		if err != nil {
			return err
		}
	}

	return nil
}

func attributesString(attrs []abci.EventAttribute) string {
	out := strings.Builder{}

	for _, at := range attrs {
		out.WriteString("@@")
		out.Write(at.Key)
		out.WriteString(":")
		out.Write(at.Value)
	}

	return out.String()
}

func formatFilename(name string) string {
	now := time.Now().UTC()
	for format, val := range timeFormats {
		name = strings.Replace(name, format, now.Format(val), -1)
	}
	return name
}
