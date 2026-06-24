//go:build testenvci

package citest

import (
	"strconv"
	"testing"

	"devshard/testenv/citest/harness"
)

// TestA3_StaleEscrow verifies POST /testenv/escrow marks the active escrow settled on mock-chain gRPC.
func TestA3_StaleEscrow(t *testing.T) {
	harness.SkipUnlessEnv(t, "TESTENV_CITEST")
	harness.RequireDocker(t)

	stack, cfg, eps := harness.BootAdversarialStack(t, "citest-a3-*")
	client := harness.HTTPClient()
	mockDapi := harness.MockDAPIFromConfig(cfg)
	t.Cleanup(func() {
		if t.Failed() {
			harness.DumpComposeLogs(t, stack, "mock-chain", "mock-dapi")
		}
	})

	escrowID := harness.GetGatewayEscrowID(t, client, eps.GatewayHTTP)
	id, err := strconv.ParseUint(escrowID, 10, 64)
	if err != nil {
		t.Fatalf("parse escrow_id %q: %v", escrowID, err)
	}

	harness.Step(t, "settle escrow %d on mock-chain while 2× versiond stack is up", id)
	harness.PatchTestenvEscrowSettle(t, client, mockDapi.HTTP, id)
	harness.RequireEscrowSettledOnChain(t, cfg, id)
}
