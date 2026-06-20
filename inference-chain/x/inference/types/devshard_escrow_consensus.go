package types

// DevshardValidationRateForCreate returns the validation_rate snapshotted onto a
// DevshardEscrow at create. Governance zero falls back to the compiled default.
func DevshardValidationRateForCreate(ep *DevshardEscrowParams) uint32 {
	if ep == nil || ep.ValidationRate == 0 {
		return DefaultDevshardValidationRate
	}
	return ep.ValidationRate
}

// DevshardVoteThresholdFactorForCreate returns the vote_threshold_factor
// snapshotted onto a DevshardEscrow at create. Zero is preserved (legacy
// groupSize/2 semantics at session bind).
func DevshardVoteThresholdFactorForCreate(ep *DevshardEscrowParams) uint32 {
	if ep == nil {
		return 0
	}
	return ep.VoteThresholdFactor
}
