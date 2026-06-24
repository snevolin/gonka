// Package runtimeconfig holds the transport-agnostic server core for the
// NodeManager GetRuntimeConfig long-poll. It is intentionally free of any
// decentralized-api or devshard imports so it can back dapi (production),
// the testenv mock-dapi, and (later) edge-api from one implementation.
package runtimeconfig

import (
	"time"

	"common/nodemanager/gen"
)

// ApprovedVersion mirrors gen.ApprovedVersion without exposing the proto type
// to snapshot producers.
type ApprovedVersion struct {
	Name   string
	Binary string
	SHA256 string
}

// Snapshot is the transport-agnostic view of chain-driven runtime params served
// to devshardd via GetRuntimeConfig. Its fields mirror gen.RuntimeConfig 1:1 so
// ToProto is a straight copy.
type Snapshot struct {
	// ParamsBlockHeight is the chain block height at which the last published
	// runtime revision was recorded. It advances with the published content
	// snapshot; cache writes alone do not move it.
	ParamsBlockHeight       int64
	CurrentEpochID          uint64
	LogprobsMode            string
	DevshardRequestsEnabled bool
	MaxNonce                uint32
	ApprovedVersions        []ApprovedVersion
	ServedAt                time.Time
	RefusalTimeout          int64
	ExecutionTimeout        int64
	ValidationRate          uint32
	VoteThresholdFactor     uint32
}

// ToProto converts the snapshot to the wire type. Equivalent to the dapi
// runtimeConfigFromSnapshot it replaces.
func (s Snapshot) ToProto() *gen.RuntimeConfig {
	versions := make([]*gen.ApprovedVersion, len(s.ApprovedVersions))
	for i, v := range s.ApprovedVersions {
		versions[i] = &gen.ApprovedVersion{
			Name:   v.Name,
			Binary: v.Binary,
			Sha256: v.SHA256,
		}
	}
	return &gen.RuntimeConfig{
		ParamsBlockHeight:       s.ParamsBlockHeight,
		CurrentEpochId:          s.CurrentEpochID,
		LogprobsMode:            s.LogprobsMode,
		DevshardRequestsEnabled: s.DevshardRequestsEnabled,
		MaxNonce:                s.MaxNonce,
		ApprovedVersions:        versions,
		ServedAtUnix:            s.ServedAt.Unix(),
		RefusalTimeout:          s.RefusalTimeout,
		ExecutionTimeout:        s.ExecutionTimeout,
		ValidationRate:          s.ValidationRate,
		VoteThresholdFactor:     s.VoteThresholdFactor,
	}
}
