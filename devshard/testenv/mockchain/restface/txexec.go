package restface

import (
	"fmt"
	"strconv"
	"strings"

	inferencetypes "github.com/productscience/inference/x/inference/types"

	"devshard/testenv/mockchain/rpcface"
	"devshard/testenv/mockchain/store"
)

type txExecResult struct {
	events   []storedEvent
	signer   string
	escrowID uint64
}

type storedEvent struct {
	Type       string
	Attributes []storedAttribute
}

type storedAttribute struct {
	Key   string
	Value string
}

func execMessages(st *store.Store, rpc *rpcface.Service, msgs []decodedMsg) (txExecResult, error) {
	if len(msgs) != 1 {
		return txExecResult{}, fmt.Errorf("mock-chain accepts exactly one message per tx, got %d", len(msgs))
	}
	msg := msgs[0]
	switch {
	case msg.create != nil:
		return execCreate(st, rpc, msg.create)
	case msg.settle != nil:
		return execSettle(st, rpc, msg.settle)
	default:
		return txExecResult{}, fmt.Errorf("empty decoded message")
	}
}

func execCreate(st *store.Store, rpc *rpcface.Service, msg *inferencetypes.MsgCreateDevshardEscrow) (txExecResult, error) {
	creator := strings.TrimSpace(msg.GetCreator())
	if creator == "" {
		return txExecResult{}, fmt.Errorf("creator is required")
	}
	if msg.GetAmount() == 0 {
		return txExecResult{}, fmt.Errorf("amount is required")
	}
	modelID := strings.TrimSpace(msg.GetModelId())
	if modelID == "" {
		return txExecResult{}, fmt.Errorf("model_id is required")
	}

	id := st.AllocateEscrowID()
	epoch := st.GetEpoch()
	escrow := buildEscrowFromTemplate(st, id, creator, msg.GetAmount(), modelID, epoch.Index)
	st.PutEscrow(escrow)
	if err := rpc.PublishEscrowCreated(id); err != nil {
		return txExecResult{}, err
	}
	st.IncrementSequence(creator)

	events := []storedEvent{{
		Type: "devshard_escrow_created",
		Attributes: []storedAttribute{
			{Key: "escrow_id", Value: strconv.FormatUint(id, 10)},
			{Key: "creator", Value: creator},
			{Key: "amount", Value: strconv.FormatUint(msg.GetAmount(), 10)},
			{Key: "epoch_index", Value: strconv.FormatUint(epoch.Index, 10)},
			{Key: "model_id", Value: modelID},
		},
	}}
	return txExecResult{events: events, signer: creator, escrowID: id}, nil
}

func execSettle(st *store.Store, rpc *rpcface.Service, msg *inferencetypes.MsgSettleDevshardEscrow) (txExecResult, error) {
	settler := strings.TrimSpace(msg.GetSettler())
	if settler == "" {
		return txExecResult{}, fmt.Errorf("settler is required")
	}
	id := msg.GetEscrowId()
	if id == 0 {
		return txExecResult{}, fmt.Errorf("escrow_id is required")
	}
	if !st.MarkEscrowSettled(id) {
		return txExecResult{}, fmt.Errorf("escrow %d not found", id)
	}
	fees := msg.GetFees()
	totalPayout := fees
	remainder := uint64(0)
	if err := rpc.PublishEscrowSettled(id, settler, totalPayout, fees, remainder); err != nil {
		return txExecResult{}, err
	}
	st.IncrementSequence(settler)

	events := []storedEvent{{
		Type: "devshard_escrow_settled",
		Attributes: []storedAttribute{
			{Key: "escrow_id", Value: strconv.FormatUint(id, 10)},
			{Key: "settler", Value: settler},
			{Key: "total_payout", Value: strconv.FormatUint(totalPayout, 10)},
			{Key: "fees", Value: strconv.FormatUint(fees, 10)},
			{Key: "remainder", Value: strconv.FormatUint(remainder, 10)},
		},
	}}
	return txExecResult{events: events, signer: settler, escrowID: id}, nil
}

func buildEscrowFromTemplate(st *store.Store, id uint64, creator string, amount uint64, modelID string, epochIndex uint64) *inferencetypes.DevshardEscrow {
	tmpl := st.TemplateEscrow()
	escrow := &inferencetypes.DevshardEscrow{
		Id:         id,
		Creator:    creator,
		Amount:     amount,
		ModelId:    modelID,
		EpochIndex: epochIndex,
		Slots:      []string{"http://versiond-router:8080/devshard/v1"},
		AppHash:    "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
	}
	if tmpl != nil {
		if len(tmpl.Slots) > 0 {
			escrow.Slots = append([]string(nil), tmpl.Slots...)
		}
		if tmpl.AppHash != "" {
			escrow.AppHash = tmpl.AppHash
		}
		escrow.TokenPrice = tmpl.TokenPrice
		escrow.CreateDevshardFee = tmpl.CreateDevshardFee
		escrow.FeePerNonce = tmpl.FeePerNonce
		escrow.InferenceSealGraceNonces = tmpl.InferenceSealGraceNonces
		escrow.InferenceSealGraceSeconds = tmpl.InferenceSealGraceSeconds
		escrow.AutoSealEveryNNonces = tmpl.AutoSealEveryNNonces
		escrow.ValidationRate = tmpl.ValidationRate
		escrow.VoteThresholdFactor = tmpl.VoteThresholdFactor
	}
	return escrow
}
