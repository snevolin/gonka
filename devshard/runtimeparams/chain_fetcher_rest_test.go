package runtimeparams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRESTChainFetcher_FetchSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case chainRESTParamsPath:
			json.NewEncoder(w).Encode(map[string]any{
				"params": map[string]any{
					"validation_params": map[string]any{"logprobs_mode": "raw"},
					"devshard_escrow_params": map[string]any{
						"devshard_requests_enabled": true,
						"max_nonce":                 500,
						"refusal_timeout":           "60",
						"execution_timeout":         "1200",
						"validation_rate":           6000,
						"vote_threshold_factor":     50,
					},
				},
			})
		case chainRESTEpochInfoPath:
			json.NewEncoder(w).Encode(map[string]any{
				"latest_epoch": map[string]any{
					"index":                  "12",
					"poc_start_block_height": "1200",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	snap, err := NewRESTChainFetcher(srv.URL, srv.Client()).FetchSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(12), snap.CurrentEpochID)
	assert.Equal(t, int64(1200), snap.ParamsBlockHeight)
	assert.Equal(t, uint32(6000), snap.ValidationRate)
	assert.Equal(t, uint32(50), snap.VoteThresholdFactor)
	assert.True(t, snap.DevshardRequestsEnabled)
}

func TestSettingsFromEnv_AcceptsDevshardAliases(t *testing.T) {
	t.Setenv("DEVSHARD_PARAMS_SOURCE", "chain")
	t.Setenv("DEVSHARD_NODE_MANAGER_ADDR", "nm:9400")
	t.Setenv("DEVSHARD_RUNTIME_CONFIG_MAX_WAIT_SECONDS", "45")

	s := SettingsFromEnv()
	assert.Equal(t, SourceChain, s.Source)
	assert.Equal(t, "nm:9400", s.NodeManagerAddr)
	assert.Equal(t, 45*time.Second, s.ServerMaxWait)
}
