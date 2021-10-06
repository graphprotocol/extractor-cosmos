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

	"github.com/golang/protobuf/proto"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/types"
)

const (
	subscriberName = "ExtractorService"
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
		return fmt.Errorf("extractor config is not provided")
	}
	if ex.config.OutputFile == "" {
		return fmt.Errorf("extractor output file must be set")
	}

	var outputFile string

	if strings.HasPrefix(ex.config.OutputFile, "/") {
		outputFile = ex.config.OutputFile
	} else {
		outputFile = filepath.Join(ex.config.RootDir, ex.config.OutputFile)
	}

	file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0666)
	if err != nil {
		return fmt.Errorf("extractor cant create output file: %w", err)
	}
	ex.handle = file

	blockSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventQueryNewBlock)
	if err != nil {
		return err
	}

	txsSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventQueryTx)
	if err != nil {
		return err
	}

	go ex.listen(file, blockSub, txsSub)

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
		if height < ex.config.StartHeight {
			ex.Logger.Info("skipped block", "height", height)
			continue
		}

		if err := indexBlock(w, sync, eventData); err != nil {
			ex.Logger.Error("failed to index block", "height", height, "err", err)
			continue
		}

		ex.Logger.Info("indexed block", "height", height)

		for i := int64(0); i < int64(len(eventData.Block.Txs)); i++ {
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

func indexTX(out io.Writer, sync *sync.Mutex, result *abci.TxResult) error {
	rawBytes, err := proto.Marshal(result)
	if err != nil {
		return err
	}

	sync.Lock()
	defer sync.Unlock()

	_, err = io.WriteString(out, fmt.Sprintf("DMLOG TX %d %d %s\n",
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

	io.WriteString(out, fmt.Sprintf("DMLOG BLOCK %d %d ", bh.Block.Header.Height, bh.Block.Header.Time.UnixMilli()))
	io.WriteString(out, base64.StdEncoding.EncodeToString(marshaledBlock))
	io.WriteString(out, "\n")

	for i, ev := range bh.ResultBeginBlock.Events {
		attrs := attributesString(ev.Attributes)
		io.WriteString(out, fmt.Sprintf("DMLOG BLOCK_BEGIN_EVENT %d %d %s %s \n", bh.Block.Header.Height, i, ev.Type, attrs))
	}

	for i, ev := range bh.ResultEndBlock.Events {
		attrs := attributesString(ev.Attributes)
		io.WriteString(out, fmt.Sprintf("DMLOG BLOCK_END_EVENT %d %d %s %s \n", bh.Block.Header.Height, i, ev.Type, attrs))
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
