//go:build testenvci

package citest

import (
	"testing"

	"devshard/testenv/citest/harness"
	"devshard/testenv/config"
	"devshard/testenv/mockchain/adminface"
)

// TestA4_BadWarmKey verifies POST /testenv/grantees revokes the configured warm grantee on mock-chain.
func TestA4_BadWarmKey(t *testing.T) {
	harness.SkipUnlessEnv(t, "TESTENV_CITEST")
	harness.RequireDocker(t)

	_, cfg, _ := harness.BootAdversarialStack(t, "citest-a4-*")
	client := harness.HTTPClient()
	mockDapi := harness.MockDAPIFromConfig(cfg)
	requireHosts(t, cfg, 2)

	granter := cfg.Hosts[0].Address
	warm := cfg.WarmGrantee.Address
	requireNonEmpty(t, granter, "host[0] address")
	requireNonEmpty(t, warm, "warm_grantee address")

	harness.Step(t, "revoke warm key %s for granter %s via /testenv/grantees", warm, granter)
	harness.PatchTestenvGrantees(t, client, mockDapi.HTTP, adminface.GranteesRequest{
		GranterAddress: granter,
		Grantees:       []string{"gonka1badwarm000000000000000000000000000"},
	})
	harness.RequireWarmKeyRevoked(t, cfg, granter, warm)
}

func requireHosts(t *testing.T, cfg *config.File, n int) {
	t.Helper()
	if len(cfg.Hosts) < n {
		t.Fatalf("expected >= %d hosts, got %d", n, len(cfg.Hosts))
	}
}

func requireNonEmpty(t *testing.T, value, label string) {
	t.Helper()
	if value == "" {
		t.Fatalf("missing %s", label)
	}
}
