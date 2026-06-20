package runtimeparams

import (
	"context"
	"fmt"

	"devshard/runtimeconfig"

	inferencetypes "github.com/productscience/inference/x/inference/types"
)

// GRPCChainFetcher adapts chain gRPC queries into runtimeconfig.ChainParamsFetcher.
type GRPCChainFetcher struct {
	qcp QueryClientProvider
}

// NewGRPCChainFetcher returns a fetcher that issues Params + EpochInfo over gRPC.
func NewGRPCChainFetcher(qcp QueryClientProvider) *GRPCChainFetcher {
	return &GRPCChainFetcher{qcp: qcp}
}

func (f *GRPCChainFetcher) FetchSnapshot(ctx context.Context) (runtimeconfig.Snapshot, error) {
	qc := f.qcp.NewInferenceQueryClient()

	paramsResp, err := qc.Params(ctx, &inferencetypes.QueryParamsRequest{})
	if err != nil {
		return runtimeconfig.Snapshot{}, fmt.Errorf("query params: %w", err)
	}
	epochResp, err := qc.EpochInfo(ctx, &inferencetypes.QueryEpochInfoRequest{})
	if err != nil {
		return runtimeconfig.Snapshot{}, fmt.Errorf("query epoch info: %w", err)
	}

	out := runtimeconfig.Snapshot{
		ParamsBlockHeight: epochResp.LatestEpoch.PocStartBlockHeight,
		CurrentEpochID:    epochResp.LatestEpoch.Index,
		LogprobsMode:      paramsResp.Params.ValidationParams.GetLogprobsMode(),
	}
	if dep := paramsResp.Params.DevshardEscrowParams; dep != nil {
		out.DevshardRequestsEnabled = dep.DevshardRequestsEnabled
		out.MaxNonce = dep.MaxNonce
		out.RefusalTimeout = dep.RefusalTimeout
		out.ExecutionTimeout = dep.ExecutionTimeout
		out.ValidationRate = dep.ValidationRate
		out.VoteThresholdFactor = dep.VoteThresholdFactor
	}
	return out, nil
}

var _ runtimeconfig.ChainParamsFetcher = (*GRPCChainFetcher)(nil)
