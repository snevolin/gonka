package restface_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"common/chain"
	"devshard/cmd/devshardd/events"
	"devshard/signing"
	"devshard/testenv/mockchain/grpcface"
	"devshard/testenv/mockchain/restface"
	"devshard/testenv/mockchain/rpcface"
	"devshard/testenv/mockchain/seed"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	inferencetypes "github.com/productscience/inference/x/inference/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func startMockChainStack(t *testing.T) (restURL, rpcURL string, grpcClient *chain.Client, cleanup func()) {
	t.Helper()
	st := seed.Defaults()

	rpcSvc, err := rpcface.NewService(st, rpcface.Config{BlockInterval: time.Hour})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	rpcURL, rpcDetach, err := rpcSvc.AttachInProcess(ctx)
	require.NoError(t, err)

	_, restURL, restCleanup, err := restface.NewInProcessServer(st, rpcSvc)
	require.NoError(t, err)

	grpcSrv, lis, err := grpcface.NewInProcessServer(st)
	require.NoError(t, err)
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	client := chain.NewFromConn(conn)

	cleanup = func() {
		cancel()
		rpcDetach()
		restCleanup()
		rpcSvc.Stop() // need export Stop method
		grpcSrv.Stop()
		_ = lis.Close()
		_ = conn.Close()
	}
	return restURL, rpcURL, client, cleanup
}

func TestRESTAccountAndNodeInfo(t *testing.T) {
	restURL, _, _, cleanup := startMockChainStack(t)
	defer cleanup()

	resp, err := http.Get(restURL + "/cosmos/base/tendermint/v1beta1/node_info")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var nodeInfo map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&nodeInfo))
	require.Equal(t, "gonka-test", findStringField(nodeInfo, "network"))
}

func TestRESTBroadcastCreateEscrow(t *testing.T) {
	restURL, _, grpcClient, cleanup := startMockChainStack(t)
	defer cleanup()

	signer, err := signing.GenerateKey()
	require.NoError(t, err)

	accountResp, err := http.Get(restURL + "/cosmos/auth/v1beta1/accounts/" + signer.Address())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, accountResp.StatusCode)
	_ = accountResp.Body.Close()

	txBytes := buildMinimalCreateTx(t, signer.Address(), 1_000_000, "test-model")
	txHash := broadcastTx(t, restURL, txBytes)

	var gotID uint64
	require.Eventually(t, func() bool {
		id, ok := pollEscrowID(t, restURL, txHash)
		if ok {
			gotID = id
			return true
		}
		return false
	}, 2*time.Second, 20*time.Millisecond)

	resp, err := grpcClient.InferenceQueryClient().DevshardEscrow(context.Background(),
		&inferencetypes.QueryGetDevshardEscrowRequest{Id: gotID})
	require.NoError(t, err)
	require.True(t, resp.Found)
	require.Equal(t, signer.Address(), resp.Escrow.Creator)
	require.Equal(t, "test-model", resp.Escrow.ModelId)
}

func TestRESTCreateEmitsCometEvent(t *testing.T) {
	restURL, rpcURL, _, cleanup := startMockChainStack(t)
	defer cleanup()

	signer, err := signing.GenerateKey()
	require.NoError(t, err)

	l := events.NewListener(rpcURL)
	var created []events.DevshardEscrowCreatedEvent
	l.OnDevshardEscrowCreated(func(_ context.Context, e events.DevshardEscrowCreatedEvent) {
		created = append(created, e)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() { _ = l.Start(ctx) }()
	time.Sleep(200 * time.Millisecond)

	txBytes := buildMinimalCreateTx(t, signer.Address(), 500_000, "test-model")
	broadcastTx(t, restURL, txBytes)

	require.Eventually(t, func() bool {
		return len(created) >= 1
	}, 2*time.Second, 20*time.Millisecond)
}

func buildMinimalCreateTx(t *testing.T, creator string, amount uint64, modelID string) []byte {
	t.Helper()
	msg := &inferencetypes.MsgCreateDevshardEscrow{
		Creator: creator,
		Amount:  amount,
		ModelId: modelID,
	}
	msgBytes, err := msg.Marshal()
	require.NoError(t, err)
	anyMsg := &codectypes.Any{
		TypeUrl: "/inference.inference.MsgCreateDevshardEscrow",
		Value:   msgBytes,
	}
	body := &txtypes.TxBody{Messages: []*codectypes.Any{anyMsg}}
	bodyBytes, err := body.Marshal()
	require.NoError(t, err)
	raw := &txtypes.TxRaw{BodyBytes: bodyBytes}
	txBytes, err := raw.Marshal()
	require.NoError(t, err)
	return txBytes
}

func broadcastTx(t *testing.T, restURL string, txBytes []byte) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"tx_bytes": base64.StdEncoding.EncodeToString(txBytes),
		"mode":     "BROADCAST_MODE_SYNC",
	})
	require.NoError(t, err)
	resp, err := http.Post(restURL+"/cosmos/tx/v1beta1/txs", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var payload struct {
		TxResponse struct {
			Code   uint32 `json:"code"`
			TxHash string `json:"txhash"`
		} `json:"tx_response"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, uint32(0), payload.TxResponse.Code)
	require.NotEmpty(t, payload.TxResponse.TxHash)
	return payload.TxResponse.TxHash
}

func pollEscrowID(t *testing.T, restURL, txHash string) (uint64, bool) {
	t.Helper()
	resp, err := http.Get(restURL + "/cosmos/tx/v1beta1/txs/" + txHash)
	if err != nil || resp.StatusCode != http.StatusOK {
		return 0, false
	}
	defer resp.Body.Close()
	var payload struct {
		TxResponse struct {
			Events []struct {
				Type       string `json:"type"`
				Attributes []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"attributes"`
			} `json:"events"`
		} `json:"tx_response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, false
	}
	for _, ev := range payload.TxResponse.Events {
		if ev.Type != "devshard_escrow_created" {
			continue
		}
		for _, attr := range ev.Attributes {
			if attr.Key == "escrow_id" && attr.Value != "" {
				var id uint64
				_, err := fmt.Sscanf(attr.Value, "%d", &id)
				if err == nil && id > 0 {
					return id, true
				}
			}
		}
	}
	return 0, false
}

func findStringField(v any, key string) string {
	switch x := v.(type) {
	case map[string]any:
		if raw, ok := x[key]; ok {
			if s, ok := raw.(string); ok {
				return s
			}
		}
		for _, child := range x {
			if found := findStringField(child, key); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range x {
			if found := findStringField(child, key); found != "" {
				return found
			}
		}
	}
	return ""
}
