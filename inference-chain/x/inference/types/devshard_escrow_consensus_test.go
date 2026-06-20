package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevshardValidationRateForCreate(t *testing.T) {
	require.Equal(t, DefaultDevshardValidationRate, DevshardValidationRateForCreate(nil))
	require.Equal(t, DefaultDevshardValidationRate, DevshardValidationRateForCreate(&DevshardEscrowParams{}))
	require.Equal(t, uint32(3000), DevshardValidationRateForCreate(&DevshardEscrowParams{ValidationRate: 3000}))
}

func TestDevshardVoteThresholdFactorForCreate(t *testing.T) {
	require.Equal(t, uint32(0), DevshardVoteThresholdFactorForCreate(nil))
	require.Equal(t, uint32(0), DevshardVoteThresholdFactorForCreate(&DevshardEscrowParams{}))
	require.Equal(t, uint32(67), DevshardVoteThresholdFactorForCreate(&DevshardEscrowParams{VoteThresholdFactor: 67}))
}
