package runtimeparams

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"devshard/runtimeconfig"
)

const (
	chainRESTParamsPath    = "/productscience/inference/inference/params"
	chainRESTEpochInfoPath = "/productscience/inference/inference/epoch_info"
)

// RESTChainFetcher reads Params + EpochInfo via the chain grpc-gateway REST API.
type RESTChainFetcher struct {
	baseURL string
	client  *http.Client
}

// NewRESTChainFetcher returns a chain fallback fetcher for processes that only
// have chain REST (e.g. devshardctl gateway).
func NewRESTChainFetcher(baseURL string, client *http.Client) *RESTChainFetcher {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &RESTChainFetcher{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  client,
	}
}

type restParamsResponse struct {
	Params *struct {
		ValidationParams *struct {
			LogprobsMode string `json:"logprobs_mode"`
		} `json:"validation_params"`
		DevshardEscrowParams *struct {
			DevshardRequestsEnabled bool   `json:"devshard_requests_enabled"`
			MaxNonce                uint32 `json:"max_nonce"`
			RefusalTimeout          int64  `json:"refusal_timeout,string"`
			ExecutionTimeout        int64  `json:"execution_timeout,string"`
			ValidationRate          uint32 `json:"validation_rate"`
			VoteThresholdFactor     uint32 `json:"vote_threshold_factor"`
		} `json:"devshard_escrow_params"`
	} `json:"params"`
}

type restEpochInfoResponse struct {
	LatestEpoch *struct {
		Index               uint64 `json:"index,string"`
		PocStartBlockHeight int64  `json:"poc_start_block_height,string"`
	} `json:"latest_epoch"`
}

func (f *RESTChainFetcher) FetchSnapshot(ctx context.Context) (runtimeconfig.Snapshot, error) {
	paramsResp, err := restDoGet[restParamsResponse](ctx, f.client, f.baseURL+chainRESTParamsPath)
	if err != nil {
		return runtimeconfig.Snapshot{}, fmt.Errorf("query params: %w", err)
	}
	epochResp, err := restDoGet[restEpochInfoResponse](ctx, f.client, f.baseURL+chainRESTEpochInfoPath)
	if err != nil {
		return runtimeconfig.Snapshot{}, fmt.Errorf("query epoch info: %w", err)
	}
	if paramsResp == nil || paramsResp.Params == nil {
		return runtimeconfig.Snapshot{}, fmt.Errorf("devshard escrow params missing from chain params response")
	}
	if epochResp == nil || epochResp.LatestEpoch == nil {
		return runtimeconfig.Snapshot{}, fmt.Errorf("epoch info missing from chain response")
	}

	out := runtimeconfig.Snapshot{
		ParamsBlockHeight: epochResp.LatestEpoch.PocStartBlockHeight,
		CurrentEpochID:    epochResp.LatestEpoch.Index,
	}
	if vp := paramsResp.Params.ValidationParams; vp != nil {
		out.LogprobsMode = vp.LogprobsMode
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

func restDoGet[T any](ctx context.Context, client *http.Client, rawURL string) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("HTTP GET %s: status %d", rawURL, resp.StatusCode)
	}
	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response from %s: %w", rawURL, err)
	}
	return &result, nil
}

var _ runtimeconfig.ChainParamsFetcher = (*RESTChainFetcher)(nil)
