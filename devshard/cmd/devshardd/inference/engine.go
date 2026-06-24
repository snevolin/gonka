package inference

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"common/chain"
	mlnodeclient "common/nodemanager"
	mlnodegen "common/nodemanager/gen"
	"devshard"
	"devshard/observability"
)

// Engine implements devshard.InferenceEngine for the standalone devshardd binary.
// It acquires a locked ML node via NodeManager gRPC, POSTs directly, and releases
// with an outcome reflecting the result.
type Engine struct {
	mlClient     *mlnodeclient.Client
	payloadStore PayloadStore
	httpClient   *http.Client
	chainParams  ChainParamsProvider
	phase        *chain.Phase
}

// NewEngine creates an Engine backed by a NodeManager gRPC client.
func NewEngine(
	mlClient *mlnodeclient.Client,
	payloadStore PayloadStore,
	chainParams ChainParamsProvider,
	phase *chain.Phase,
) *Engine {
	return &Engine{
		mlClient:     mlClient,
		payloadStore: payloadStore,
		httpClient:   NewNoRedirectClient(mlNodeHTTPTimeout),
		chainParams:  chainParams,
		phase:        phase,
	}
}

func (e *Engine) Execute(ctx context.Context, req devshard.ExecuteRequest) (*devshard.ExecuteResult, error) {
	return executeInference(ctx, req, e.payloadStore, e.phase.EpochID(), e.executeMLRequest, e.chainParams)
}

func (e *Engine) executeMLRequest(ctx context.Context, model string, body []byte) (*http.Response, error) {
	resp, err := e.doWithLockedNode(ctx, observability.PathExecute, model, func(endpoint string) (*http.Response, error) {
		url := endpoint + "/v1/chat/completions"
		httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if reqErr != nil {
			return nil, observability.Classify(observability.ReasonApplicationErr, observability.WhereEngineMLNodeCall, reqErr)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		observability.InjectRequestContext(ctx, httpReq.Header)
		observability.AttachRequestID(httpReq)
		return e.httpClient.Do(httpReq)
	})
	if err != nil {
		return nil, fmt.Errorf("execute inference: %w", err)
	}
	return resp, nil
}

// doWithLockedNode acquires a locked ML node via NodeManager gRPC, runs fn, and
// releases the lock. Retries up to maxAcquireAttempts, excluding nodes that failed
// with a transport-class error on earlier attempts.
func (e *Engine) doWithLockedNode(
	ctx context.Context,
	path observability.Path,
	model string,
	fn func(endpoint string) (*http.Response, error),
) (*http.Response, error) {
	// More attempts than the in-process broker path because dapi's broker may
	// need a few seconds to update node IntendedStatus after an epoch phase
	// transition. The 2s sleep between attempts covers that lag.
	const maxAcquireAttempts = 10
	var excluded []string
	var lastErr error
	lastReason := observability.ReasonAcquireErr

	for attempt := 0; attempt < maxAcquireAttempts; attempt++ {
		acq, err := e.mlClient.Acquire(ctx, model, excluded)
		if err != nil {
			lastReason = observability.ReasonAcquireErr
			observability.IncMLNodeAttempt(path, lastReason, "")
			lastErr = fmt.Errorf("acquire: %w", err)
			select {
			case <-ctx.Done():
				lastReason = observability.ReasonTimeout
				return nil, observability.Classify(lastReason, observability.WhereEngineMLNodeCall, ctx.Err())
			case <-time.After(2 * time.Second):
			}
			continue
		}

		started := time.Now()
		resp, httpErr := fn(acq.Endpoint)
		outcome := mlnodegen.ReleaseOutcome_SUCCESS

		lastReason = observability.ClassifyMLNodeHTTP(resp, httpErr, ctx.Err())
		observability.IncMLNodeAttempt(path, lastReason, acq.NodeId)
		observability.ObserveMLNodeCall(path, acq.NodeId, observability.MetricPhaseTotal, started)

		switch lastReason {
		case observability.ReasonTransportErr, observability.ReasonTimeout:
			outcome = mlnodegen.ReleaseOutcome_TRANSPORT_ERROR
			lastErr = httpErr
		case observability.ReasonHTTP5xx:
			if resp != nil {
				resp.Body.Close()
			}
			outcome = mlnodegen.ReleaseOutcome_TRANSPORT_ERROR
			if resp != nil {
				lastErr = fmt.Errorf("upstream status %d", resp.StatusCode)
			}
			resp = nil
		case observability.ReasonHTTP4xx:
			// 4xx surfaced to caller without rotation.
		}

		if releaseErr := e.mlClient.Release(ctx, acq.LockId, outcome); releaseErr != nil {
			observability.IncMLNodeAttempt(path, observability.ReasonReleaseErr, acq.NodeId)
			if lastErr == nil {
				lastReason = observability.ReasonReleaseErr
				lastErr = fmt.Errorf("release: %w", releaseErr)
			}
		}

		if outcome == mlnodegen.ReleaseOutcome_SUCCESS {
			return resp, nil
		}

		if acq.NodeId != "" {
			excluded = append(excluded, acq.NodeId)
		}
	}

	if lastErr == nil {
		lastErr = errors.New("no attempts made")
	}
	if lastReason == observability.ReasonOK {
		lastReason = observability.ReasonTransportErr
	}
	return nil, observability.Classify(lastReason, observability.WhereEngineMLNodeCall, lastErr)
}

var _ devshard.InferenceEngine = (*Engine)(nil)
