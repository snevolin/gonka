package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strings"

	"common/chain"
	mlnodeclient "common/nodemanager"
	"devshard/runtimeconfig"
	"devshard/runtimeparams"
)

func initGatewayRuntimeParams(ctx context.Context, chainREST, chainGRPC string) (*runtimeparams.Managed, func(), error) {
	env := runtimeparams.SettingsFromEnv()

	var nmClient *mlnodeclient.Client
	var nmClose func()
	if env.NodeManagerAddr != "" {
		client, err := mlnodeclient.NewClient(env.NodeManagerAddr)
		if err != nil {
			slog.Warn("runtime params: nodemanager dial failed; chain fallback only",
				"addr", env.NodeManagerAddr, "err", err)
		} else {
			nmClient = client
			nmClose = func() { _ = client.Close() }
		}
	}

	chainFetcher, chainClose, err := newGatewayChainFetcher(chainREST, chainGRPC)
	if err != nil {
		if nmClose != nil {
			nmClose()
		}
		return nil, nil, err
	}

	setup := runtimeparams.SetupConfig{
		Chain:  chainFetcher,
		Logger: slog.Default(),
		Env:    env,
	}
	if nmClient != nil {
		setup.GRPCClient = nmClient.NodeManagerClient()
	}

	managed, err := runtimeparams.NewManaged(ctx, setup)
	if err != nil {
		if chainClose != nil {
			chainClose()
		}
		if nmClose != nil {
			nmClose()
		}
		return nil, nil, err
	}

	closeAll := func() {
		managed.Close()
		if chainClose != nil {
			chainClose()
		}
		if nmClose != nil {
			nmClose()
		}
	}
	return managed, closeAll, nil
}

func newGatewayChainFetcher(chainREST, chainGRPC string) (runtimeconfig.ChainParamsFetcher, func(), error) {
	chainGRPC = strings.TrimSpace(chainGRPC)
	if chainGRPC != "" {
		client, err := chain.New(chainGRPC)
		if err != nil {
			return nil, nil, fmt.Errorf("chain gRPC dial %s: %w", chainGRPC, err)
		}
		closeFn := func() {
			if c, ok := client.Conn().(interface{ Close() error }); ok {
				_ = c.Close()
			}
		}
		slog.Info("runtime params chain fetcher", "transport", "grpc", "url", chainGRPC)
		return runtimeparams.NewGRPCChainFetcher(client), closeFn, nil
	}
	if strings.TrimSpace(chainREST) == "" {
		return nil, nil, fmt.Errorf("chain REST URL is required when chain gRPC URL is unset")
	}
	slog.Warn("runtime params chain fetcher: gRPC URL unset; using REST fallback", "rest", chainREST)
	return runtimeparams.NewRESTChainFetcher(chainREST, nil), nil, nil
}

func mustInitGatewayRuntimeParams(ctx context.Context, chainREST, chainGRPC string) (*runtimeparams.Managed, func()) {
	managed, closeFn, err := initGatewayRuntimeParams(ctx, chainREST, chainGRPC)
	if err != nil {
		log.Fatalf("runtime params provider: %v", err)
	}
	if managed == nil {
		log.Fatal("runtime params provider: nil setup")
	}
	return managed, closeFn
}

type runtimeBuildDeps struct {
	chainREST    string
	chainGRPC    string
	defaultModel string
	perf         *PerfTracker
}

func (d runtimeBuildDeps) validate() error {
	if d.chainREST == "" {
		return fmt.Errorf("chain REST URL is required")
	}
	return nil
}
