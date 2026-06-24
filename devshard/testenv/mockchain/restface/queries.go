package restface

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	inferencetypes "github.com/productscience/inference/x/inference/types"
)

const (
	devshardEscrowPathPrefix = "/productscience/inference/inference/devshard_escrow/"
	participantPathPrefix    = "/productscience/inference/inference/participant/"
)

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, devshardEscrowPathPrefix):
		s.handleDevshardEscrowQuery(w, strings.TrimPrefix(path, devshardEscrowPathPrefix))
		return true
	case strings.HasPrefix(path, participantPathPrefix):
		s.handleParticipantQuery(w, strings.TrimPrefix(path, participantPathPrefix))
		return true
	default:
		return false
	}
}

func (s *Server) handleDevshardEscrowQuery(w http.ResponseWriter, idStr string) {
	idStr = strings.TrimSpace(strings.TrimSuffix(idStr, "/"))
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		writeRESTError(w, http.StatusBadRequest, "invalid escrow id")
		return
	}
	e := s.store.GetEscrow(id)
	if e == nil {
		writeJSON(w, http.StatusOK, map[string]any{"found": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"found":  true,
		"escrow": devshardEscrowToREST(e),
	})
}

func (s *Server) handleParticipantQuery(w http.ResponseWriter, address string) {
	address = strings.TrimSpace(strings.TrimSuffix(address, "/"))
	if address == "" {
		writeRESTError(w, http.StatusBadRequest, "address required")
		return
	}
	p := s.store.GetParticipant(address)
	if p == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"participant": map[string]any{
			"index":         p.Index,
			"address":       p.Address,
			"inference_url": p.InferenceUrl,
			"validator_key": p.ValidatorKey,
		},
	})
}

func devshardEscrowToREST(e *inferencetypes.DevshardEscrow) map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"id":                            fmt.Sprintf("%d", e.Id),
		"creator":                       e.Creator,
		"amount":                        fmt.Sprintf("%d", e.Amount),
		"slots":                         append([]string(nil), e.Slots...),
		"epoch_index":                   fmt.Sprintf("%d", e.EpochIndex),
		"app_hash":                      e.AppHash,
		"settled":                       e.Settled,
		"model_id":                      e.ModelId,
		"token_price":                   fmt.Sprintf("%d", e.TokenPrice),
		"create_devshard_fee":           fmt.Sprintf("%d", e.CreateDevshardFee),
		"fee_per_nonce":                 fmt.Sprintf("%d", e.FeePerNonce),
		"inference_seal_grace_nonces":   e.InferenceSealGraceNonces,
		"inference_seal_grace_seconds":  e.InferenceSealGraceSeconds,
		"auto_seal_every_n_nonces":      e.AutoSealEveryNNonces,
		"validation_rate":               e.ValidationRate,
		"vote_threshold_factor":         e.VoteThresholdFactor,
	}
}
