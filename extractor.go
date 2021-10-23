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

	//	cc "github.com/figment-networks/extractor-tendermint/codec"
	"github.com/figment-networks/tendermint-protobuf-def/codec"
	"github.com/golang/protobuf/proto"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/proto/tendermint/crypto"
	"github.com/tendermint/tendermint/types"
)

const (
	subscriberName = "ExtractorService"

	dmPrefix = "DMLOG "
	dmBlock  = dmPrefix + "BLOCK"
	dmTx     = dmPrefix + "TX"
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

	// voteSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventDataVote)
	// if err != nil {
	// 	return err
	// }

	// roundStateSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventDataRoundState)
	// if err != nil {
	// 	return err
	// }

	// newRoundSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventDataNewRound)
	// if err != nil {
	// 	return err
	// }

	// completePropSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventDataCompleteProposal)
	// if err != nil {
	// 	return err
	// }

	/*	valSetUpdatesSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventQueryValidatorSetUpdates)
		if err != nil {
			return err
		}
	*/
	// stringSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventDataString)
	// if err != nil {
	// 	return err
	// }

	// blockSyncStatusSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventDataBlockSyncStatus)
	// if err != nil {
	// 	return err
	// }

	// stateSyncStatusSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventDataStateSyncStatus)
	// if err != nil {
	// 	return err
	// }

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
		ex.Logger.Info("closing stream output", "dest", ex.config.OutputFile)
		ex.handle.Close()
	}
}

func (ex *ExtractorService) listen(w io.Writer, blockSub, txsSub types.Subscription) {
	sync := &sync.Mutex{}

	for {
		blockMsg := <-blockSub.Out()
		eventData := blockMsg.Data().(types.EventDataNewBlock)
		height := eventData.Block.Header.Height

		// Skip extraction on unwanted heights
		if ex.shouldSkipHeight(height) {
			ex.drainSubscription(txsSub, len(eventData.Block.Txs))
			ex.Logger.Info("skipped block", "height", height)
			continue
		}
		// we need to drain all

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

		// not sure how we approach this because its gonna be a repeat
		// valSetMsg := <-valSetUpdatesSub.Out()
		// validatorData := valSetMsg.Data()
		// validator := validatorData.(codec.EventDataValidatorSetUpdates).ValidatorUpdates

		// ex.Logger.Info("Validator Set Update", "validator", validator)

	}
}

func (ex *ExtractorService) shouldSkipHeight(height int64) bool {
	return ex.config.StartHeight > 0 && height < ex.config.StartHeight ||
		ex.config.EndHeight > 0 && height > ex.config.EndHeight
}

func (ex *ExtractorService) drainSubscription(sub types.Subscription, n int) error {
	for i := 0; i < n; i++ {
		<-sub.Out()
	}
	return nil
}

func (ex *ExtractorService) initStreamOutput() (io.Writer, error) {
	var (
		writer     io.Writer
		outputFile string
	)

	switch ex.config.OutputFile {
	case "", "stdout", "STDOUT":
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

		ex.config.OutputFile = outputFile
		ex.handle = file
		writer = file
	}

	ex.Logger.Info("configured stream output", "dest", outputFile)
	return writer, nil
}

func indexTX(out io.Writer, sync *sync.Mutex, result *abci.TxResult) error {
	tx := &codec.EventDataTx{
		TxResult: &codec.TxResult{
			Height: uint64(result.Height),
			Index:  result.Index,
			Tx:     result.Tx,
			Result: &codec.ResponseDeliverTx{
				Code:      result.Result.Code,
				Data:      result.Result.Data,
				Log:       result.Result.Log,
				Info:      result.Result.Info,
				GasWanted: result.Result.GasWanted,
				GasUsed:   result.Result.GasUsed,
				Codespace: result.Result.Codespace,
			},
		},
	}

	for _, ev := range result.Result.Events {
		tx.TxResult.Result.Events = append(tx.TxResult.Result.Events, mapEvent(ev))
	}

	marshaledTx, err := proto.Marshal(tx)
	if err != nil {
		return err
	}

	sync.Lock()
	defer sync.Unlock()
	_, err = io.WriteString(out, fmt.Sprintf("%s %d %d %s\n",
		dmTx,
		result.Height,
		result.Index,
		base64.StdEncoding.EncodeToString(marshaledTx),
	))

	return err
}

func indexBlock(out io.Writer, sync *sync.Mutex, bh types.EventDataNewBlock) error {
	nb := &codec.EventDataNewBlock{
		Block: &codec.Block{
			Header: &codec.Header{
				Version: &codec.Consensus{
					Block: bh.Block.Header.Version.Block,
					App:   bh.Block.Header.Version.App,
				},
				ChainId: bh.Block.Header.ChainID,
				Height:  uint64(bh.Block.Header.Height),
				Time: &codec.Timestamp{
					Seconds: bh.Block.Header.Time.Unix(),
					Nanos:   0, // TODO(lukanus): do it properly
				},
				LastBlockId:        mapBlockID(bh.Block.LastBlockID),
				LastCommitHash:     bh.Block.Header.LastCommitHash,
				DataHash:           bh.Block.Header.DataHash,
				ValidatorsHash:     bh.Block.Header.ValidatorsHash,
				NextValidatorsHash: bh.Block.Header.NextValidatorsHash,
				ConsensusHash:      bh.Block.Header.ConsensusHash,
				AppHash:            bh.Block.Header.AppHash,
				LastResultsHash:    bh.Block.Header.LastResultsHash,
				EvidenceHash:       bh.Block.Header.EvidenceHash,
				ProposerAddress: &codec.Address{
					Address: bh.Block.Header.ProposerAddress,
				},
			},
			LastCommit: &codec.Commit{
				Height:     uint64(bh.Block.LastCommit.Height),
				Round:      bh.Block.LastCommit.Round,
				BlockId:    mapBlockID(bh.Block.LastCommit.BlockID),
				Signatures: []*codec.CommitSig{}, // TODO(lukanus): do it properly
			},
		},
	}

	// (lukanus): need to construct block id ... somehow
	nb.BlockId = &codec.BlockID{
		Hash: bh.Block.Header.DataHash, // (lukanus): is this the same as hash?
		/*		PartSetHeader: &codec.PartSetHeader{
				Total: bid.PartSetHeader.Total,        not sure where to get it ?
				Hash:  bid.PartSetHeader.Hash,
			},*/
	}

	if len(bh.Block.Data.Txs) > 0 {
		nb.Block.Data = &codec.Data{}
		for _, tx := range bh.Block.Data.Txs {
			nb.Block.Data.Txs = append(nb.Block.Data.Txs, tx)
		}
	}

	if len(bh.Block.Evidence.Evidence) > 0 {
		nb.Block.Evidence = &codec.EvidenceList{}
		for _, ev := range bh.Block.Evidence.Evidence {

			newEv := &codec.Evidence{}
			switch evN := ev.(type) {
			case *types.DuplicateVoteEvidence:
				newEv.Sum = &codec.Evidence_DuplicateVoteEvidence{
					DuplicateVoteEvidence: &codec.DuplicateVoteEvidence{
						VoteA:            mapVote(evN.VoteA),
						VoteB:            mapVote(evN.VoteB),
						TotalVotingPower: evN.TotalVotingPower,
						ValidatorPower:   evN.ValidatorPower,
						Timestamp: &codec.Timestamp{
							Seconds: evN.Timestamp.Unix(),
							Nanos:   0, // TODO(lukanus): do it properly
						},
					},
				}
			case *types.LightClientAttackEvidence:
				newEv.Sum = &codec.Evidence_LightClientAttackEvidence{
					LightClientAttackEvidence: &codec.LightClientAttackEvidence{
						ConflictingBlock: &codec.LightBlock{
							SignedHeader: &codec.SignedHeader{}, // TODO(lukanus): do it properly
							ValidatorSet: &codec.ValidatorSet{}, // TODO(lukanus): do it properly
						},
						CommonHeight:        evN.CommonHeight,
						ByzantineValidators: []*codec.Validator{}, // TODO(lukanus): do it properly
						TotalVotingPower:    evN.TotalVotingPower,
						Timestamp: &codec.Timestamp{
							Seconds: evN.Timestamp.Unix(),
							Nanos:   0, // TODO(lukanus): do it properly
						},
					},
				}
			default:
				return fmt.Errorf("given type %T of EvidenceList mapping doesn't exist ", ev)
			}

			nb.Block.Evidence.Evidence = append(nb.Block.Evidence.Evidence, newEv)
		}
	}

	if len(bh.ResultBeginBlock.Events) > 0 {
		nb.ResultBeginBlock = &codec.ResponseBeginBlock{}
		for _, ev := range bh.ResultBeginBlock.Events {
			nb.ResultBeginBlock.Events = append(nb.ResultBeginBlock.Events, mapEvent(ev))
		}
	}

	if len(bh.ResultEndBlock.Events) > 0 || len(bh.ResultEndBlock.ValidatorUpdates) > 0 || bh.ResultEndBlock.ConsensusParamUpdates != nil {
		nb.ResultEndBlock = &codec.ResponseEndBlock{
			ConsensusParamUpdates: &codec.ConsensusParams{},
		}

		for _, ev := range bh.ResultEndBlock.Events {
			nb.ResultEndBlock.Events = append(nb.ResultEndBlock.Events, mapEvent(ev))
		}

		for _, v := range bh.ResultEndBlock.ValidatorUpdates {
			val, err := mapValidator(v)
			if err != nil {
				return err
			}
			nb.ResultEndBlock.ValidatorUpdates = append(nb.ResultEndBlock.ValidatorUpdates, val)
		}
	}

	marshaledBlock, err := proto.Marshal(nb)
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

	return err
}

func formatFilename(name string) string {
	now := time.Now().UTC()
	for format, val := range timeFormats {
		name = strings.Replace(name, format, now.Format(val), -1)
	}
	return name
}

func mapBlockID(bid types.BlockID) *codec.BlockID {
	return &codec.BlockID{
		Hash: bid.Hash,
		PartSetHeader: &codec.PartSetHeader{
			Total: bid.PartSetHeader.Total,
			Hash:  bid.PartSetHeader.Hash,
		},
	}
}

func mapEvent(ev abci.Event) *codec.Event {
	cev := &codec.Event{Eventtype: ev.Type}

	for _, at := range ev.Attributes {
		cev.Attributes = append(cev.Attributes, &codec.EventAttribute{
			Key:   string(at.Key),
			Value: string(at.Value),
			Index: at.Index,
		})
	}

	return cev
}

func mapVote(edv *types.Vote) *codec.EventDataVote {
	return &codec.EventDataVote{
		Eventvotetype: codec.SignedMsgType(edv.Type),
		Height:        uint64(edv.Height),
		Round:         edv.Round,
		BlockId:       mapBlockID(edv.BlockID),
		Timestamp: &codec.Timestamp{
			Seconds: edv.Timestamp.Unix(),
			Nanos:   0, // TODO(lukanus): do it properly
		},
		ValidatorAddress: &codec.Address{
			Address: edv.ValidatorAddress,
		},
		ValidatorIndex: edv.ValidatorIndex,
		Signature:      edv.Signature,
	}
}

func mapValidator(v abci.ValidatorUpdate) (*codec.Validator, error) {
	nPK := &codec.PublicKey{}

	switch key := v.PubKey.Sum.(type) {
	case *crypto.PublicKey_Ed25519:
		nPK.Sum = &codec.PublicKey_Ed25519{Ed25519: key.Ed25519}
	case *crypto.PublicKey_Secp256K1:
		nPK.Sum = &codec.PublicKey_Secp256K1{Secp256K1: key.Secp256K1}
	default:
		return nil, fmt.Errorf("given type %T of PubKey mapping doesn't exist ", key)
	}

	return &codec.Validator{
		Address:          nil, // TODO(lukanus):  figure out what's up with that
		PubKey:           nPK,
		VotingPower:      v.Power,
		ProposerPriority: 0, // TODO(lukanus):  figure out what's up with that
	}, nil
}
