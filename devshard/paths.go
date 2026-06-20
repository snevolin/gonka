package devshard

import (
	"fmt"
	"strings"

	"devshard/types"
)

func VersionedRoutePrefix(version string) string {
	return "/devshard/" + version
}

// DefaultRoutePrefix is the HTTP mount used when no explicit route prefix is set.
func DefaultRoutePrefix() string {
	return VersionedRoutePrefix(types.EffectiveStateRootAndProtocolVersion)
}

func NormalizeRoutePrefix(routePrefix string) string {
	if strings.TrimSpace(routePrefix) == "" {
		return DefaultRoutePrefix()
	}
	return routePrefix
}

func ResolveVersionedRoutePrefix(version, routePrefix string) string {
	if routePrefix != "" {
		return routePrefix
	}
	return VersionedRoutePrefix(version)
}

// VersionForRoutePrefix maps an HTTP route prefix to the session bind tag used when
// creating a user-side session (state machine + optional SQLite row).
//
//   - VersionedRoutePrefix (/devshard/<name>): <name> from approved_versions
//
// HTTP clients use routePrefix on transport.HTTPClient; this function resolves
// the session / state-root / settlement tag. See devshard/docs/upgrade.md.
func VersionForRoutePrefix(routePrefix string) (string, error) {
	normalized := NormalizeRoutePrefix(routePrefix)

	trimmed := strings.Trim(normalized, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 2 && parts[0] == "devshard" && parts[1] != "" {
		return parts[1], nil
	}

	return "", fmt.Errorf("unsupported devshard route prefix %q", routePrefix)
}

func SessionPayloadPath(routePrefix, escrowID string) string {
	normalized := strings.TrimPrefix(NormalizeRoutePrefix(routePrefix), "/")
	return fmt.Sprintf("%s/sessions/%s/payloads", normalized, escrowID)
}

func VersionedSessionPayloadPath(version, escrowID string) string {
	return SessionPayloadPath(VersionedRoutePrefix(version), escrowID)
}
