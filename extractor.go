package extractor

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/types"
)

const (
	subscriber = "ExtractorService"
)

type ExtractorService struct {
	service.BaseService

	eventBus *types.EventBus
	handle   io.Closer
}

func NewExtractorService(eventBus *types.EventBus) *ExtractorService {
	is := &ExtractorService{eventBus: eventBus}
	is.BaseService = *service.NewBaseService(nil, "ExtractorService", is)
	return is
}

func (ex *ExtractorService) OnStart() error {

	file, _ := os.Create("log-" + time.Now().Format("2006-01-02_15:04:05") + ".log")
	ex.handle = file

	blockSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriber, types.EventQueryNewBlock)
	if err != nil {
		return err
	}

	txsSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriber, types.EventQueryTx)
	if err != nil {
		return err
	}
	go ex.listen(file, blockSub, txsSub)
	return nil
}

func (ex *ExtractorService) listen(w io.Writer, blockSub, txsSub types.Subscription) {

	sync := &sync.Mutex{}

	for {
		msg := <-blockSub.Out()
		eventData := msg.Data().(types.EventDataNewBlock)
		height := eventData.Block.Header.Height

		if err := IndexBlock(w, sync, eventData); err != nil {
			ex.Logger.Error("failed to index block", "height", height, "err", err)
		} else {
			ex.Logger.Info("indexed block", "height", height)
		}

		for i := int64(0); i < int64(len(eventData.Block.Txs)); i++ {
			msg2 := <-txsSub.Out()
			txr := msg2.Data().(types.EventDataTx).TxResult
			if err := IndexTx(w, sync, &txr); err != nil {
				ex.Logger.Error("failed to index block txs", "height", height, "err", err)
			} else {
				ex.Logger.Debug("indexed block txs", "height", height) // , "num_txs", eventData.NumTxs)
			}
		}

	}
}

func (ex *ExtractorService) OnStop() {
	if ex.eventBus.IsRunning() {
		_ = ex.eventBus.UnsubscribeAll(context.Background(), subscriber)
	}

	if ex.handle != nil {
		ex.handle.Close()
	}
}

func IndexTx(out io.Writer, sync *sync.Mutex, result *abci.TxResult) error {
	rawBytes, err := proto.Marshal(result)
	if err != nil {
		return err
	}
	sync.Lock()
	io.WriteString(out, fmt.Sprintf("DMLOG TX %d %d ", result.Height, result.Index))
	io.WriteString(out, base64.StdEncoding.EncodeToString(rawBytes))
	io.WriteString(out, "\n")
	sync.Unlock()
	return err
}

func IndexBlock(out io.Writer, sync *sync.Mutex, bh types.EventDataNewBlock) error {
	//headerB, err := bh.Block.Header.ToProto().Marshal()

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

	//io.WriteString(out, fmt.Sprintf("DMLOG BLOCK_HEADER %d %d ", bh.Block.Header.Height, bh.Block.Header.Time.UnixMilli()))
	//io.WriteString(out, base64.StdEncoding.EncodeToString(headerB))
	//io.WriteString(out, "\n")

	io.WriteString(out, fmt.Sprintf("DMLOG BLOCK %d %d ", bh.Block.Header.Height, bh.Block.Header.Time.UnixMilli()))
	io.WriteString(out, base64.StdEncoding.EncodeToString(marshaledBlock))
	io.WriteString(out, "\n")

	for i, ev := range bh.ResultBeginBlock.Events {
		b := strings.Builder{}
		for _, at := range ev.Attributes {
			b.WriteString("@@")
			b.Write(at.Key)
			b.WriteString(":")
			b.Write(at.Value)
		}
		io.WriteString(out, fmt.Sprintf("DMLOG BLOCK_BEGIN_EVENT %d %d %s %s \n", bh.Block.Header.Height, i, ev.Type, b.String()))
	}

	for i, ev := range bh.ResultEndBlock.Events {
		b := strings.Builder{}
		for _, at := range ev.Attributes {
			b.WriteString("@@")
			b.Write(at.Key)
			b.WriteString(":")
			b.Write(at.Value)
		}
		io.WriteString(out, fmt.Sprintf("DMLOG BLOCK_END_EVENT %d %d %s %s \n", bh.Block.Header.Height, i, ev.Type, b.String()))
	}

	return err
}
