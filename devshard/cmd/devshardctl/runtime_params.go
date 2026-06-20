package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"

	mlnodeclient "common/nodemanager"
	"devshard/runtimeparams"
)

func initGatewayRuntimeParams(ctx context.Context, chainREST string) (*runtimeparams.Managed, func(), error) {
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

	setup := runtimeparams.SetupConfig{
		Chain:  runtimeparams.NewRESTChainFetcher(chainREST, nil),
		Logger: slog.Default(),
		Env:    env,
	}
	if nmClient != nil {
		setup.GRPCClient = nmClient.NodeManagerClient()
	}

	managed, err := runtimeparams.NewManaged(ctx, setup)
	if err != nil {
		if nmClose != nil {
			nmClose()
		}
		return nil, nil, err
	}

	closeAll := func() {
		managed.Close()
		if nmClose != nil {
			nmClose()
		}
	}
	return managed, closeAll, nil
}

func mustInitGatewayRuntimeParams(ctx context.Context, chainREST string) (*runtimeparams.Managed, func()) {
	managed, closeFn, err := initGatewayRuntimeParams(ctx, chainREST)
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
	defaultModel string
	perf         *PerfTracker
}

func (d runtimeBuildDeps) validate() error {
	if d.chainREST == "" {
		return fmt.Errorf("chain REST URL is required")
	}
	return nil
}
