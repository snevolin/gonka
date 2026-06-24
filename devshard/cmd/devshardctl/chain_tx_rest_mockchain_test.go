package main

import (
	"context"
	"testing"
	"time"

	"common/chain"
	"devshard/signing"
	"devshard/testenv/mockchain/grpcface"
	"devshard/testenv/mockchain/restface"
	"devshard/testenv/mockchain/rpcface"
	"devshard/testenv/mockchain/seed"
	"devshard/testenv/mockchain/store"

	inferencetypes "github.com/productscience/inference/x/inference/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRESTChainTxClient_CreateDevshardEscrow_MockChain(t *testing.T) {
	st := seed.Defaults()
	rpcSvc, err := rpcface.NewService(st, rpcface.Config{BlockInterval: time.Hour})
	require.NoError(t, err)

	restURL, restCleanup, err := startRESTOnly(st, rpcSvc)
	require.NoError(t, err)
	t.Cleanup(restCleanup)

	grpcSrv, lis, err := grpcface.NewInProcessServer(st)
	require.NoError(t, err)
	t.Cleanup(func() {
		grpcSrv.Stop()
		_ = lis.Close()
	})
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	chainClient := chain.NewFromConn(conn)

	signer, err := signing.GenerateKey()
	require.NoError(t, err)

	client, err := NewRESTChainTxClient(RESTChainTxConfig{
		BaseURL:      restURL,
		ChainID:      "gonka-test",
		FeeAmount:    123,
		GasLimit:     456,
		PollInterval: time.Millisecond,
		PollTimeout:  2 * time.Second,
	})
	require.NoError(t, err)

	result, err := client.CreateDevshardEscrow(t.Context(), signer, 1_000_000, "test-model")
	require.NoError(t, err)
	require.Greater(t, result.EscrowID, uint64(1))
	require.Equal(t, signer.Address(), result.Creator)

	resp, err := chainClient.InferenceQueryClient().DevshardEscrow(context.Background(),
		&inferencetypes.QueryGetDevshardEscrowRequest{Id: result.EscrowID})
	require.NoError(t, err)
	require.True(t, resp.Found)
	require.Equal(t, result.EscrowID, resp.Escrow.Id)
}

func startRESTOnly(st *store.Store, rpcSvc *rpcface.Service) (string, func(), error) {
	_, url, cleanup, err := restface.NewInProcessServer(st, rpcSvc)
	return url, cleanup, err
}
