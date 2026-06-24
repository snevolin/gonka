package restface

import (
	"fmt"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/gogoproto/proto"
	inferencetypes "github.com/productscience/inference/x/inference/types"
)

const (
	createEscrowMsgTypeURL = "/inference.inference.MsgCreateDevshardEscrow"
	settleEscrowMsgTypeURL = "/inference.inference.MsgSettleDevshardEscrow"
)

type decodedMsg struct {
	create *inferencetypes.MsgCreateDevshardEscrow
	settle *inferencetypes.MsgSettleDevshardEscrow
}

func decodeTxMessages(txBytes []byte) ([]decodedMsg, error) {
	var raw txtypes.TxRaw
	if err := proto.Unmarshal(txBytes, &raw); err != nil {
		return nil, fmt.Errorf("decode tx raw: %w", err)
	}
	var body txtypes.TxBody
	if err := proto.Unmarshal(raw.BodyBytes, &body); err != nil {
		return nil, fmt.Errorf("decode tx body: %w", err)
	}
	if len(body.Messages) == 0 {
		return nil, fmt.Errorf("tx has no messages")
	}
	out := make([]decodedMsg, 0, len(body.Messages))
	for _, anyMsg := range body.Messages {
		if anyMsg == nil {
			continue
		}
		switch anyMsg.TypeUrl {
		case createEscrowMsgTypeURL:
			var msg inferencetypes.MsgCreateDevshardEscrow
			if err := proto.Unmarshal(anyMsg.Value, &msg); err != nil {
				return nil, fmt.Errorf("decode MsgCreateDevshardEscrow: %w", err)
			}
			out = append(out, decodedMsg{create: &msg})
		case settleEscrowMsgTypeURL:
			var msg inferencetypes.MsgSettleDevshardEscrow
			if err := proto.Unmarshal(anyMsg.Value, &msg); err != nil {
				return nil, fmt.Errorf("decode MsgSettleDevshardEscrow: %w", err)
			}
			out = append(out, decodedMsg{settle: &msg})
		default:
			return nil, fmt.Errorf("unsupported message type %q", anyMsg.TypeUrl)
		}
	}
	return out, nil
}
