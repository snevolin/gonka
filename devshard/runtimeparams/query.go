package runtimeparams

import inferencetypes "github.com/productscience/inference/x/inference/types"

// QueryClientProvider issues chain gRPC queries for runtime params fallback.
type QueryClientProvider interface {
	NewInferenceQueryClient() inferencetypes.QueryClient
}
