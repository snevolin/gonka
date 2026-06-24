package restface

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/cometbft/cometbft/crypto/tmhash"

	"devshard/testenv/mockchain/rpcface"
	"devshard/testenv/mockchain/store"
)

// Server is the Cosmos LCD REST face for mock-chain (Phase 3c).
type Server struct {
	store *store.Store
	rpc   *rpcface.Service

	mu   sync.RWMutex
	txs  map[string]*storedTx
}

type storedTx struct {
	Code   uint32
	Hash   string
	Events []storedEvent
}

// NewServer returns an LCD server backed by store and rpc event publisher.
func NewServer(st *store.Store, rpc *rpcface.Service) (*Server, error) {
	if st == nil || rpc == nil {
		return nil, fmt.Errorf("mockchain rest: store and rpc service are required")
	}
	return &Server{
		store: st,
		rpc:   rpc,
		txs:   make(map[string]*storedTx),
	}, nil
}

// Serve listens on addr until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("mockchain rest listen %s: %w", addr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- http.Serve(lis, mux)
	}()

	select {
	case <-ctx.Done():
		_ = lis.Close()
		return ctx.Err()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

// NewInProcessServer starts REST on a random localhost port for tests.
func NewInProcessServer(st *store.Store, rpc *rpcface.Service) (*Server, string, func(), error) {
	srv, err := NewServer(st, rpc)
	if err != nil {
		return nil, "", nil, err
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handle)
	go func() { _ = http.Serve(lis, mux) }()
	cleanup := func() { _ = lis.Close() }
	return srv, "http://" + lis.Addr().String(), cleanup, nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if s.handleQuery(w, r) {
		return
	}
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/cosmos/auth/v1beta1/accounts/"):
		s.handleAccount(w, r, strings.TrimPrefix(path, "/cosmos/auth/v1beta1/accounts/"))
	case r.Method == http.MethodGet && path == "/cosmos/base/tendermint/v1beta1/node_info":
		s.handleNodeInfo(w, r)
	case r.Method == http.MethodPost && path == "/cosmos/tx/v1beta1/txs":
		s.handleBroadcast(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/cosmos/tx/v1beta1/txs/"):
		s.handleGetTx(w, r, strings.TrimPrefix(path, "/cosmos/tx/v1beta1/txs/"))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAccount(w http.ResponseWriter, _ *http.Request, address string) {
	address = strings.TrimSpace(address)
	if address == "" {
		writeRESTError(w, http.StatusBadRequest, "address required")
		return
	}
	acc := s.store.GetOrCreateAccount(address)
	writeJSON(w, http.StatusOK, map[string]any{
		"account": map[string]any{
			"@type":          "/cosmos.auth.v1beta1.BaseAccount",
			"address":        address,
			"account_number": strconvFormatUint(acc.AccountNumber),
			"sequence":       strconvFormatUint(acc.Sequence),
		},
	})
}

func (s *Server) handleNodeInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"default_node_info": map[string]any{
			"network": s.store.GetChainID(),
		},
	})
}

func (s *Server) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TxBytes string `json:"tx_bytes"`
		Mode    string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRESTError(w, http.StatusBadRequest, err.Error())
		return
	}
	txBytes, err := base64.StdEncoding.DecodeString(req.TxBytes)
	if err != nil {
		writeRESTError(w, http.StatusBadRequest, "invalid tx_bytes")
		return
	}
	msgs, err := decodeTxMessages(txBytes)
	if err != nil {
		writeRESTError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := execMessages(s.store, s.rpc, msgs)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"tx_response": map[string]any{
				"code":      1,
				"codespace": "mockchain",
				"raw_log":   err.Error(),
			},
		})
		return
	}
	hash := txHash(txBytes)
	s.mu.Lock()
	s.txs[hash] = &storedTx{Code: 0, Hash: hash, Events: result.events}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"tx_response": map[string]any{
			"code":   0,
			"txhash": hash,
			"events": eventsToJSON(result.events),
		},
	})
}

func (s *Server) handleGetTx(w http.ResponseWriter, _ *http.Request, hash string) {
	hash = strings.TrimSpace(hash)
	s.mu.RLock()
	tx, ok := s.txs[hash]
	s.mu.RUnlock()
	if !ok {
		writeRESTError(w, http.StatusNotFound, "tx not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tx_response": map[string]any{
			"code":   tx.Code,
			"txhash": tx.Hash,
			"events": eventsToJSON(tx.Events),
		},
	})
}

func txHash(txBytes []byte) string {
	sum := tmhash.Sum(txBytes)
	return strings.ToUpper(hex.EncodeToString(sum))
}

func eventsToJSON(events []storedEvent) []map[string]any {
	out := make([]map[string]any, len(events))
	for i, ev := range events {
		attrs := make([]map[string]string, len(ev.Attributes))
		for j, a := range ev.Attributes {
			attrs[j] = map[string]string{"key": a.Key, "value": a.Value}
		}
		out[i] = map[string]any{"type": ev.Type, "attributes": attrs}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeRESTError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"message": msg})
}

func strconvFormatUint(v uint64) string {
	return fmt.Sprintf("%d", v)
}
