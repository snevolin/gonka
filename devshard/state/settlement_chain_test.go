package state

import (
	"cmp"
	"fmt"
	"slices"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"devshard/internal/testutil"
	"devshard/signing"
	"devshard/types"

	"github.com/productscience/inference/x/inference/keeper"
	chaintypes "github.com/productscience/inference/x/inference/types"
)

const (
	chainSettlementEscrowID     = "1"
	chainSettlementEscrowUintID = uint64(1)
)

func newChainSettlementSM(t *testing.T, hosts []*signing.Secp256k1Signer, balance uint64) (*StateMachine, *signing.Secp256k1Signer, []types.SlotAssignment) {
	t.Helper()
	user := testutil.MustGenerateKey(t)
	group := testutil.MakeGroup(hosts)
	config := testutil.DefaultConfig(len(hosts))
	verifier := signing.NewSecp256k1Verifier()
	store := testutil.MustMemoryStore(t, chainSettlementEscrowID, user.Address(), config, group, balance)
	sm, err := NewStateMachine(chainSettlementEscrowID, config, group, balance, user.Address(), verifier, store)
	require.NoError(t, err)
	return sm, user, group
}

func applyStartConfirmForEscrow(t *testing.T, sm *StateMachine, user *signing.Secp256k1Signer, hosts []*signing.Secp256k1Signer, escrowID string, inferenceID uint64) {
	t.Helper()
	executorSlotIdx := inferenceID % uint64(len(hosts))
	nonce := sm.LatestNonce() + 1

	diff := testutil.SignDiff(t, user, escrowID, nonce, []*types.DevshardTx{txStart(&types.MsgStartInference{
		InferenceId: inferenceID, PromptHash: []byte("prompt"), Model: "llama",
		InputLength: 100, MaxTokens: 50, StartedAt: 1000,
	})})
	_, err := sm.ApplyDiff(diff)
	require.NoError(t, err)

	execSig := testutil.SignExecutorReceipt(t, hosts[executorSlotIdx], escrowID, inferenceID, []byte("prompt"), "llama", 100, 50, 1000, 1000)
	nonce++
	diff = testutil.SignDiff(t, user, escrowID, nonce, []*types.DevshardTx{txConfirm(&types.MsgConfirmStart{
		InferenceId: inferenceID, ExecutorSig: execSig, ConfirmedAt: 1000,
	})})
	_, err = sm.ApplyDiff(diff)
	require.NoError(t, err)
}

func advanceToSettlementForEscrow(t *testing.T, sm *StateMachine, user *signing.Secp256k1Signer, escrowID string, groupSize int) {
	t.Helper()
	nonce := sm.LatestNonce() + 1
	diff := testutil.SignDiff(t, user, escrowID, nonce, []*types.DevshardTx{txFinalize()})
	_, err := sm.ApplyDiff(diff)
	require.NoError(t, err)
	require.Equal(t, types.PhaseFinalizing, sm.Phase())

	st := sm.SnapshotState()
	for n := st.LatestNonce + 1; n <= st.FinalizeNonce+uint64(groupSize); n++ {
		diff = testutil.SignDiff(t, user, escrowID, n, nil)
		_, err = sm.ApplyDiff(diff)
		require.NoError(t, err)
	}
	require.Equal(t, types.PhaseSettlement, sm.Phase())
}

func signSettlementQuorum(t *testing.T, payload *SettlementPayload, signers []*signing.Secp256k1Signer) map[uint32][]byte {
	t.Helper()
	hostStatsHash, err := ComputeHostStatsHash(payload.HostStats)
	require.NoError(t, err)
	stateRoot := ComputeStateRootFromRestHash(
		hostStatsHash, payload.RestHash, payload.Fees,
		types.PhaseSettlement, payload.StateRootAndProtocolVersion,
	)

	sigContent := &types.StateSignatureContent{
		StateRoot: stateRoot,
		EscrowId:  payload.EscrowID,
		Nonce:     payload.Nonce,
	}
	sigData, err := deterministicMarshal.Marshal(sigContent)
	require.NoError(t, err)

	sigs := make(map[uint32][]byte, len(signers))
	for i, signer := range signers {
		sig, err := signer.Sign(sigData)
		require.NoError(t, err)
		sigs[uint32(i)] = sig
	}
	return sigs
}

func hostStatsToChain(hostStats map[uint32]*types.HostStats) []*chaintypes.DevshardSettlementHostStats {
	slotIDs := make([]uint32, 0, len(hostStats))
	for id := range hostStats {
		slotIDs = append(slotIDs, id)
	}
	slices.SortFunc(slotIDs, func(a, b uint32) int { return cmp.Compare(a, b) })

	out := make([]*chaintypes.DevshardSettlementHostStats, 0, len(slotIDs))
	for _, id := range slotIDs {
		hs := hostStats[id]
		out = append(out, &chaintypes.DevshardSettlementHostStats{
			SlotId:               id,
			Missed:               hs.Missed,
			Invalid:              hs.Invalid,
			Cost:                 hs.Cost,
			RequiredValidations:  hs.RequiredValidations,
			CompletedValidations: hs.CompletedValidations,
		})
	}
	return out
}

func chainEscrowFromGroup(t *testing.T, creator string, amount uint64, group []types.SlotAssignment) chaintypes.DevshardEscrow {
	t.Helper()
	slots := make([]string, len(group))
	for _, sa := range group {
		if int(sa.SlotID) >= len(slots) {
			t.Fatalf("slot %d out of range for group size %d", sa.SlotID, len(group))
		}
		slots[sa.SlotID] = sa.ValidatorAddress
	}
	return chaintypes.DevshardEscrow{
		Id:      chainSettlementEscrowUintID,
		Creator: creator,
		Amount:  amount,
		Slots:   slots,
	}
}

func settlementPayloadToChainMsg(t *testing.T, payload SettlementPayload, creator string, group []types.SlotAssignment) *chaintypes.MsgSettleDevshardEscrow {
	t.Helper()
	chainHostStats := hostStatsToChain(payload.HostStats)

	devshardHash, err := ComputeHostStatsHash(payload.HostStats)
	require.NoError(t, err)
	chainHash, err := keeper.ComputeDevshardHostStatsHash(chainHostStats)
	require.NoError(t, err)
	require.Equal(t, devshardHash, chainHash, "host stats hash must match across devshard and chain proto encodings")

	stateRoot := ComputeStateRootFromRestHash(
		chainHash, payload.RestHash, payload.Fees,
		types.PhaseSettlement, payload.StateRootAndProtocolVersion,
	)

	var sigs []*chaintypes.DevshardSlotSignature
	for slotID, sig := range payload.Signatures {
		sigs = append(sigs, &chaintypes.DevshardSlotSignature{
			SlotId:    slotID,
			Signature: sig,
		})
	}
	slices.SortFunc(sigs, func(a, b *chaintypes.DevshardSlotSignature) int {
		return cmp.Compare(a.SlotId, b.SlotId)
	})

	return &chaintypes.MsgSettleDevshardEscrow{
		Settler:                     creator,
		EscrowId:                    chainSettlementEscrowUintID,
		StateRootAndProtocolVersion: payload.StateRootAndProtocolVersion,
		StateRoot:                   stateRoot,
		Nonce:                       payload.Nonce,
		Fees:                        payload.Fees,
		RestHash:                    payload.RestHash,
		HostStats:                   chainHostStats,
		Signatures:                  sigs,
	}
}

func verifyAutoFinishedSettlementOnChain(
	t *testing.T,
	sm *StateMachine,
	user *signing.Secp256k1Signer,
	hosts []*signing.Secp256k1Signer,
	group []types.SlotAssignment,
	initialAmount uint64,
) SettlementPayload {
	t.Helper()
	sdk.GetConfig().SetBech32PrefixForAccount("gonka", "gonka")

	st := sm.SnapshotState()
	require.Equal(t, types.PhaseSettlement, st.Phase)

	payload, err := BuildSettlement(chainSettlementEscrowID, st, nil, sm.LatestNonce())
	require.NoError(t, err)

	payload.Signatures = signSettlementQuorum(t, payload, hosts)

	verifier := signing.NewSecp256k1Verifier()
	root, err := VerifySettlement(*payload, group, verifier, nil)
	require.NoError(t, err)
	require.Len(t, root, 32)

	var totalCost uint64
	for _, hs := range st.HostStats {
		totalCost += hs.Cost
	}
	require.LessOrEqual(t, totalCost+st.Fees, initialAmount)

	escrow := chainEscrowFromGroup(t, user.Address(), initialAmount, group)
	msg := settlementPayloadToChainMsg(t, *payload, user.Address(), group)
	require.Equal(t, fmt.Sprint(chainSettlementEscrowUintID), chainSettlementEscrowID)

	err = keeper.VerifyDevshardSettlement(escrow, msg, &chaintypes.DevshardEscrowParams{
		MaxNonce: chaintypes.DefaultDevshardMaxNonce,
	}, nil)
	require.NoError(t, err)

	return *payload
}

func TestBuildSettlement_AutoFinished_CensoredFinish_VerifyDevshardSettlement(t *testing.T) {
	hosts := []*signing.Secp256k1Signer{
		testutil.MustGenerateKey(t), testutil.MustGenerateKey(t), testutil.MustGenerateKey(t),
	}
	const initialAmount = uint64(10000)
	sm, user, group := newChainSettlementSM(t, hosts, initialAmount)
	applyStartConfirmForEscrow(t, sm, user, hosts, chainSettlementEscrowID, 1)

	before := sm.SnapshotState()
	require.Equal(t, uint64(0), before.HostStats[1].Cost)

	advanceToSettlementForEscrow(t, sm, user, chainSettlementEscrowID, len(hosts))

	after := sm.SnapshotState()
	require.Equal(t, uint64(150), after.HostStats[1].Cost)

	verifyAutoFinishedSettlementOnChain(t, sm, user, hosts, group, initialAmount)
}

func driveMixedAutoFinishSession(t *testing.T, hosts []*signing.Secp256k1Signer) (*StateMachine, *signing.Secp256k1Signer, []types.SlotAssignment) {
	t.Helper()
	const initialAmount = uint64(20000)
	sm, user, group := newChainSettlementSM(t, hosts, initialAmount)
	escrowID := chainSettlementEscrowID

	diff := testutil.SignDiff(t, user, escrowID, 1, []*types.DevshardTx{txStart(&types.MsgStartInference{
		InferenceId: 1, PromptHash: []byte("prompt"), Model: "llama",
		InputLength: 100, MaxTokens: 50, StartedAt: 1000,
	})})
	_, err := sm.ApplyDiff(diff)
	require.NoError(t, err)

	for nonce := uint64(2); nonce <= 3; nonce++ {
		diff = testutil.SignDiff(t, user, escrowID, nonce, nil)
		_, err = sm.ApplyDiff(diff)
		require.NoError(t, err)
	}

	diff = testutil.SignDiff(t, user, escrowID, 4, []*types.DevshardTx{txStart(&types.MsgStartInference{
		InferenceId: 4, PromptHash: []byte("prompt"), Model: "llama",
		InputLength: 100, MaxTokens: 50, StartedAt: 1000,
	})})
	_, err = sm.ApplyDiff(diff)
	require.NoError(t, err)

	execSig := testutil.SignExecutorReceipt(t, hosts[4], escrowID, 4, []byte("prompt"), "llama", 100, 50, 1000, 1000)
	diff = testutil.SignDiff(t, user, escrowID, 5, []*types.DevshardTx{txConfirm(&types.MsgConfirmStart{
		InferenceId: 4, ExecutorSig: execSig, ConfirmedAt: 1000,
	})})
	_, err = sm.ApplyDiff(diff)
	require.NoError(t, err)

	diff = testutil.SignDiff(t, user, escrowID, 6, nil)
	_, err = sm.ApplyDiff(diff)
	require.NoError(t, err)

	diff = testutil.SignDiff(t, user, escrowID, 7, []*types.DevshardTx{txStart(&types.MsgStartInference{
		InferenceId: 7, PromptHash: []byte("prompt"), Model: "llama",
		InputLength: 100, MaxTokens: 50, StartedAt: 1000,
	})})
	_, err = sm.ApplyDiff(diff)
	require.NoError(t, err)

	execSig = testutil.SignExecutorReceipt(t, hosts[2], escrowID, 7, []byte("prompt"), "llama", 100, 50, 1000, 1000)
	diff = testutil.SignDiff(t, user, escrowID, 8, []*types.DevshardTx{txConfirm(&types.MsgConfirmStart{
		InferenceId: 7, ExecutorSig: execSig, ConfirmedAt: 1000,
	})})
	_, err = sm.ApplyDiff(diff)
	require.NoError(t, err)

	finishMsg := &types.MsgFinishInference{
		InferenceId: 7, ResponseHash: []byte("response"),
		InputTokens: 80, OutputTokens: 40, ExecutorSlot: 2,
		EscrowId: escrowID,
	}
	finishMsg.ProposerSig = testutil.SignProposerTx(t, hosts[2], finishMsg)
	diff = testutil.SignDiff(t, user, escrowID, 9, []*types.DevshardTx{txFinish(finishMsg)})
	_, err = sm.ApplyDiff(diff)
	require.NoError(t, err)

	advanceToSettlementForEscrow(t, sm, user, escrowID, len(hosts))
	return sm, user, group
}

func TestBuildSettlement_AutoFinished_Mixed_VerifyDevshardSettlement(t *testing.T) {
	hosts := []*signing.Secp256k1Signer{
		testutil.MustGenerateKey(t), testutil.MustGenerateKey(t), testutil.MustGenerateKey(t),
		testutil.MustGenerateKey(t), testutil.MustGenerateKey(t),
	}

	sm, user, group := driveMixedAutoFinishSession(t, hosts)
	st := sm.SnapshotState()
	require.Equal(t, uint64(0), st.HostStats[1].Cost)
	require.Equal(t, uint64(150), st.HostStats[4].Cost)
	require.Equal(t, uint64(120), st.HostStats[2].Cost)

	verifyAutoFinishedSettlementOnChain(t, sm, user, hosts, group, 20000)
}

func TestBuildSettlement_AutoFinished_Mixed_ChainVerifyDeterministic(t *testing.T) {
	hosts := []*signing.Secp256k1Signer{
		testutil.MustGenerateKey(t), testutil.MustGenerateKey(t), testutil.MustGenerateKey(t),
		testutil.MustGenerateKey(t), testutil.MustGenerateKey(t),
	}

	smA, userA, groupA := driveMixedAutoFinishSession(t, hosts)
	smB, userB, groupB := driveMixedAutoFinishSession(t, hosts)

	payloadA := verifyAutoFinishedSettlementOnChain(t, smA, userA, hosts, groupA, 20000)
	payloadB := verifyAutoFinishedSettlementOnChain(t, smB, userB, hosts, groupB, 20000)

	require.Equal(t, payloadA.RestHash, payloadB.RestHash)
	require.Equal(t, payloadA.Fees, payloadB.Fees)
	for slot, hsA := range payloadA.HostStats {
		hsB := payloadB.HostStats[slot]
		require.NotNil(t, hsB)
		require.Equal(t, hsA.Cost, hsB.Cost)
		require.Equal(t, hsA.Missed, hsB.Missed)
		require.Equal(t, hsA.Invalid, hsB.Invalid)
	}
}
