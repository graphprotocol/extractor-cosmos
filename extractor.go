package extractor

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/figment-networks/tendermint-protobuf-def/codec"
	"github.com/golang/protobuf/proto"
	abci "github.com/tendermint/tendermint/abci/types"
	tmcrypto "github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/proto/tendermint/crypto"
	"github.com/tendermint/tendermint/types"
)

const (
	subscriberName = "ExtractorService"

	dmPrefix    = "DMLOG "
	dmBlock     = dmPrefix + "BLOCK"
	dmTx        = dmPrefix + "TX"
	dmValidator = dmPrefix + "VALIDATOR_SET_UPDATES"
)

type ExtractorService struct {
	service.BaseService

	config        *Config
	eventBus      *types.EventBus
	writer        Writer
	currentHeight int64
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

	valSetUpdatesSub, err := ex.eventBus.SubscribeUnbuffered(context.Background(), subscriberName, types.EventQueryValidatorSetUpdates)
	if err != nil {
		return err
	}

	if err := ex.initStreamOutput(); err != nil {
		ex.Logger.Error("stream output init failed", "err", err)
		return err
	}

	go ex.listen(blockSub, txsSub, valSetUpdatesSub)

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

func (ex *ExtractorService) listen(blockSub, txsSub, valSetUpdatesSub types.Subscription) {
	sync := &sync.Mutex{}

	for {
		select {
		case blockMsg := <-blockSub.Out():
			eventData := blockMsg.Data().(types.EventDataNewBlock)
			height := eventData.Block.Header.Height
			ex.currentHeight = height

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

		case valSetMsg := <-valSetUpdatesSub.Out():
			setUpdates := valSetMsg.Data().(types.EventDataValidatorSetUpdates)

			if err := indexValSetUpdates(ex.writer, sync, &setUpdates, ex.currentHeight); err != nil {
				ex.Logger.Error("failed to index Validator Set Data", "err", err)
			}

		default:
			continue
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

		ex.Logger.Info("configured stream output",
			"dest", filename,
			"bundle", ex.config.Bundle,
			"bundle_size", ex.config.BundleSize,
		)
		return nil
	}

	switch filename {
	case "", "stdout", "STDOUT":
		ex.writer = NewConsoleWriter(os.Stdout)
	case "stderr", "STDERR":
		ex.writer = NewConsoleWriter(os.Stderr)
	default:
		ex.writer = NewFileWriter(filename)
	}

	ex.Logger.Info("configured stream output", "dest", filename)
	return nil
}

func indexTX(out Writer, sync *sync.Mutex, result *abci.TxResult) error {
	tx := &codec.EventTx{
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

	mappedCommitSignatures, err := mapSignatures(bh.Block.LastCommit.Signatures)
	if err != nil {
		return err
	}

	nb := &codec.EventBlock{
		Block: &codec.Block{
			Header: &codec.Header{
				Version: &codec.Consensus{
					Block: bh.Block.Header.Version.Block,
					App:   bh.Block.Header.Version.App,
				},
				ChainId:            bh.Block.Header.ChainID,
				Height:             uint64(bh.Block.Header.Height),
				Time:               mapTimestamp(bh.Block.Header.Time),
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
				Signatures: mappedCommitSignatures,
			},
			Evidence: &codec.EvidenceList{},
			Data:     &codec.Data{},
		},
	}

	nb.BlockId = &codec.BlockID{
		Hash: bh.Block.Header.Hash(),
		PartSetHeader: &codec.PartSetHeader{
			Total: bh.Block.LastBlockID.PartSetHeader.Total,
			Hash:  bh.Block.LastBlockID.PartSetHeader.Hash,
		},
	}

	if len(bh.Block.Data.Txs) > 0 {
		for _, tx := range bh.Block.Data.Txs {
			nb.Block.Data.Txs = append(nb.Block.Data.Txs, tx)
		}
	}

	if len(bh.Block.Evidence.Evidence) > 0 {
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
						Timestamp:        mapTimestamp(evN.Timestamp),
					},
				}
			case *types.LightClientAttackEvidence:
				mappedSetValidators, err := mapValidators(evN.ConflictingBlock.ValidatorSet.Validators)
				if err != nil {
					return err
				}

				mappedByzantineValidators, err := mapValidators(evN.ByzantineValidators)
				if err != nil {
					return err
				}

				mappedCommitSignatures, err := mapSignatures(evN.ConflictingBlock.Commit.Signatures)
				if err != nil {
					return err
				}

				newEv.Sum = &codec.Evidence_LightClientAttackEvidence{
					LightClientAttackEvidence: &codec.LightClientAttackEvidence{
						ConflictingBlock: &codec.LightBlock{
							SignedHeader: &codec.SignedHeader{
								Header: &codec.Header{
									Version: &codec.Consensus{
										Block: evN.ConflictingBlock.Version.Block,
										App:   evN.ConflictingBlock.Version.App,
									},
									ChainId:            evN.ConflictingBlock.Header.ChainID,
									Height:             uint64(evN.ConflictingBlock.Header.Height),
									Time:               mapTimestamp(evN.ConflictingBlock.Header.Time),
									LastBlockId:        mapBlockID(evN.ConflictingBlock.Header.LastBlockID),
									LastCommitHash:     evN.ConflictingBlock.Header.LastCommitHash,
									DataHash:           evN.ConflictingBlock.Header.DataHash,
									ValidatorsHash:     evN.ConflictingBlock.Header.ValidatorsHash,
									NextValidatorsHash: evN.ConflictingBlock.Header.NextValidatorsHash,
									ConsensusHash:      evN.ConflictingBlock.Header.ConsensusHash,
									AppHash:            evN.ConflictingBlock.Header.AppHash,
									LastResultsHash:    evN.ConflictingBlock.Header.LastResultsHash,
									EvidenceHash:       evN.ConflictingBlock.Header.EvidenceHash,
									ProposerAddress: &codec.Address{
										Address: evN.ConflictingBlock.Header.ProposerAddress,
									},
								},
								Commit: &codec.Commit{
									Height:     uint64(evN.ConflictingBlock.Commit.Height),
									Round:      evN.ConflictingBlock.Commit.Round,
									BlockId:    mapBlockID(evN.ConflictingBlock.Commit.BlockID),
									Signatures: mappedCommitSignatures,
								},
							},
							ValidatorSet: &codec.ValidatorSet{
								Validators:       mappedSetValidators,
								Proposer:         mapProposer(evN.ConflictingBlock.ValidatorSet.Proposer),
								TotalVotingPower: evN.ConflictingBlock.ValidatorSet.TotalVotingPower(),
							},
						},
						CommonHeight:        evN.CommonHeight,
						ByzantineValidators: mappedByzantineValidators,
						TotalVotingPower:    evN.TotalVotingPower,
						Timestamp:           mapTimestamp(evN.Timestamp),
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
			val, err := mapValidatorUpdate(v)
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

func indexValSetUpdates(out Writer, sync *sync.Mutex, updates *types.EventDataValidatorSetUpdates, height int64) error {
	if len(updates.ValidatorUpdates) == 0 {
		return nil
	}

	result := &codec.EventValidatorSetUpdates{}

	for _, update := range updates.ValidatorUpdates {
		nPK := &codec.PublicKey{}

		switch update.PubKey.Type() {
		case "ed25519":
			nPK.Sum = &codec.PublicKey_Ed25519{Ed25519: update.PubKey.Bytes()}
		case "secp256k1":
			nPK.Sum = &codec.PublicKey_Secp256K1{Secp256K1: update.PubKey.Bytes()}
		default:
			return fmt.Errorf("unsupported pubkey type: %T", update.PubKey)
		}

		result.ValidatorUpdates = append(result.ValidatorUpdates, &codec.Validator{
			Address:          update.Address.Bytes(),
			VotingPower:      update.VotingPower,
			ProposerPriority: update.ProposerPriority,
			PubKey:           nPK,
		})
	}

	data, err := proto.Marshal(result)
	if err != nil {
		return fmt.Errorf("cant marshal validator: %v", err)
	}

	sync.Lock()
	defer sync.Unlock()

	return out.WriteLine(fmt.Sprintf("%s %d %d %s",
		dmValidator,
		height,
		0, // validator update dont have index
		base64.StdEncoding.EncodeToString(data),
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

func mapProposer(val *types.Validator) *codec.Validator {
	nPK := &codec.PublicKey{}

	return &codec.Validator{
		Address:          val.Address,
		PubKey:           nPK,
		ProposerPriority: 0,
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

func mapVote(edv *types.Vote) *codec.EventVote {
	return &codec.EventVote{
		Eventvotetype: codec.SignedMsgType(edv.Type),
		Height:        uint64(edv.Height),
		Round:         edv.Round,
		BlockId:       mapBlockID(edv.BlockID),
		Timestamp:     mapTimestamp(edv.Timestamp),
		ValidatorAddress: &codec.Address{
			Address: edv.ValidatorAddress,
		},
		ValidatorIndex: edv.ValidatorIndex,
		Signature:      edv.Signature,
	}
}

func mapSignatures(cs []types.CommitSig) ([]*codec.CommitSig, error) {
	signatures := []*codec.CommitSig{}
	if len(cs) > 0 {
		for _, cs := range cs {
			sig := cs
			signature, err := mapSignature(sig)
			if err != nil {
				return nil, err
			}
			signatures = append(signatures, signature)
		}
	}
	return signatures, nil
}

func mapSignature(s types.CommitSig) (*codec.CommitSig, error) {

	return &codec.CommitSig{
		BlockIdFlag:      codec.BlockIDFlag(s.BlockIDFlag),
		ValidatorAddress: &codec.Address{Address: s.ValidatorAddress.Bytes()},
		Timestamp:        mapTimestamp(s.Timestamp),
		Signature:        s.Signature,
	}, nil
}

func mapValidatorUpdate(v abci.ValidatorUpdate) (*codec.Validator, error) {
	nPK := &codec.PublicKey{}
	var address []byte

	switch key := v.PubKey.Sum.(type) {
	case *crypto.PublicKey_Ed25519:
		nPK.Sum = &codec.PublicKey_Ed25519{Ed25519: key.Ed25519}
		address = tmcrypto.AddressHash(nPK.GetEd25519())
	case *crypto.PublicKey_Secp256K1:
		nPK.Sum = &codec.PublicKey_Secp256K1{Secp256K1: key.Secp256K1}
		address = tmcrypto.AddressHash(nPK.GetSecp256K1())
	default:
		return nil, fmt.Errorf("given type %T of PubKey mapping doesn't exist ", key)
	}

	// NOTE on ProposerPriority field: Priority value seems to be calcuulated in the
	// context of the validator set. We're already processing this as a separate event.
	// More info in here: https://docs.tendermint.com/v0.34/spec/consensus/proposer-selection.html

	return &codec.Validator{
		Address:          address,
		PubKey:           nPK,
		VotingPower:      v.Power,
		ProposerPriority: 0,
	}, nil
}

func mapValidators(v []*types.Validator) ([]*codec.Validator, error) {
	validators := []*codec.Validator{}
	if len(v) > 0 {
		for _, v := range v {
			valv := v
			val, err := mapValidator(valv)
			if err != nil {
				return nil, err
			}
			validators = append(validators, val)
		}
	}
	return validators, nil
}

func mapValidator(v *types.Validator) (*codec.Validator, error) {
	nPK := &codec.PublicKey{}

	// key := v.PubKey.Type()
	// switch key {
	// case "Ed25519":
	// 	nPK.Sum = &codec.PublicKey_Ed25519{Ed25519: key.Ed25519}
	// case "Secp256K1":
	// 	nPK.Sum = &codec.PublicKey_Secp256K1{Secp256K1: key.Secp256K1}
	// default:
	// 	return nil, fmt.Errorf("given type %T of PubKey mapping doesn't exist ", key)
	// }

	return &codec.Validator{
		Address:          v.Address,
		PubKey:           nPK, // TODO (mpreston): this needs to be finalized as it doesn't have .Sum available
		VotingPower:      v.VotingPower,
		ProposerPriority: v.ProposerPriority,
	}, nil
}

func mapTimestamp(time time.Time) *codec.Timestamp {
	return &codec.Timestamp{
		Seconds: time.Unix(),
		Nanos:   int32(time.UnixNano() - time.Unix()*1000000000),
	}
}
