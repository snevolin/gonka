package client

import (
	commrc "common/runtimeconfig"
)

// Snapshot and ApprovedVersion alias the common transport-agnostic types.
type Snapshot = commrc.Snapshot
type ApprovedVersion = commrc.ApprovedVersion

// EpochChangeListener fires once per CurrentEpochID transition observed by the
// provider after the first successful apply.
type EpochChangeListener func(old, new uint64)

// Provider is the surface engine/validation/storage code consumes instead of
// going to chain.
type Provider interface {
	Snapshot() Snapshot
	LogprobsMode() string
	CurrentEpochID() uint64
	Availability() AvailabilityView
	OnEpochChange(EpochChangeListener) (cancel func())
}
