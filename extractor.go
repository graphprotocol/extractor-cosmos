package extractor

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

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

type ExtractorService struct {
	service.BaseService

	config   *Config
	eventBus *types.EventBus
	writer   Writer
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

	if err := ex.initStreamOutput(); err != nil {
		ex.Logger.Error("stream output init failed", "err", err)
		return err
	}

	go ex.listen(blockSub, txsSub)

	return nil
}

func (ex *ExtractorService) OnStop() {
	if ex.eventBus.IsRunning() {
		if err := ex.eventBus.UnsubscribeAll(context.Background(), subscriberName); err != nil {
			ex.Logger.Error("failed to unsubscribe on stop", "err", err)
		}
	}

	ex.Logger.Info("closing stream output", "dest", ex.config.OutputFile)
	if err := ex.writer.Close(); err != nil {
		ex.Logger.Error("failed to close stream writer", "err", err)
	}
}

func (ex *ExtractorService) listen(blockSub, txsSub types.Subscription) {
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

		if err := indexBlock(ex.writer, sync, eventData); err != nil {
			ex.drainSubscription(txsSub, len(eventData.Block.Txs))
			ex.Logger.Error("failed to index block", "height", height, "err", err)
			continue
		}

		ex.Logger.Info("indexed block", "height", height)

		for i := 0; i < len(eventData.Block.Txs); i++ {
			txMsg := <-txsSub.Out()
			txResult := txMsg.Data().(types.EventDataTx).TxResult

			if err := indexTX(ex.writer, sync, &txResult); err != nil {
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

func (ex *ExtractorService) drainSubscription(sub types.Subscription, n int) error {
	for i := 0; i < n; i++ {
		<-sub.Out()
	}
	return nil
}

func (ex *ExtractorService) initStreamOutput() error {
	filename := ex.config.GetOutputFile()

	if ex.config.Bundle {
		ex.writer = NewBundleWriter(filename, ex.config.BundleSize)
	} else {
		ex.writer = NewFileWriter(filename)
	}

	ex.Logger.Info("configured stream output", "dest", filename)
	return nil
}

func indexTX(out Writer, sync *sync.Mutex, result *abci.TxResult) error {
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

	return out.WriteLine(fmt.Sprintf("%s %d %d %s",
		dmTx,
		result.Height,
		result.Index,
		base64.StdEncoding.EncodeToString(marshaledTx),
	))
}

func indexBlock(out Writer, sync *sync.Mutex, bh types.EventDataNewBlock) error {
	if err := out.SetHeight(int(bh.Block.Height)); err != nil {
		return err
	}

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

	return out.WriteLine(fmt.Sprintf("%s %d %d %s",
		dmBlock,
		bh.Block.Header.Height,
		bh.Block.Header.Time.UnixMilli(),
		base64.StdEncoding.EncodeToString(marshaledBlock),
	))
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
