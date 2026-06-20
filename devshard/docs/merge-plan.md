# Merge Plan: `pixelplex-refactoring` → `devshard-0.2.13-v2-r2`

Integration branch: `merge/pixelplex-refactoring-into-r2`  
Merge-base: `d8b8e9073`

## Scope Summary

Original branch-to-branch delta (roots):

| Root | Files | Status |
|------|------:|--------|
| `decentralized-api` | 112 | **Done** |
| `common` | 84 | **Done** |
| `devshard` | 49+ | **Done** (Phase B) |
| `proxy` | 3 | Pending |
| `versiond-router` | 4 | **Done** |
| `local-test-net` | 3 | **Done** (hybrid) |
| `proxy-ssl` | 1 | **Skipped** (keep r2) |
| `deploy` | 1 | Pending (selective) |
| `inference-chain` | — | **Skipped** (out of scope) |
| `test-net-cloud` | 10 | **Done** (`devshard-testing/` only) |
| `testermint` | 5 | Pending (selective) |

---

## Completed: Phase A — `common/` + `decentralized-api/`

Merged pixelplex `common/` and refactored `decentralized-api/` with these **retained r2 decisions**:

| Area | Decision |
|------|----------|
| Runtime config | Keep r2 `ConfigManager` publisher, `runtime_config_*`, `epoch_change_listeners`, `devshard_versions_from_params` |
| Block dispatcher | Keep r2 `new_block_dispatcher.go` (`DevshardVersionsCacheFromParams`, `ApplyRuntimeConfigBlockIfChanged`) |
| Cosmos client | Keep r2 `cosmosclient.go` + `tx_manager.go` (constant gas, OTel); skip pixelplex per-msg gas estimation |
| Payload storage | Keep r2 `DeleteInference`, `ensurePartition` comments; `common/logging` import |
| NodeManager gRPC | Keep r2 `GetRuntimeConfig` long-poll; proto extended in **`common/nodemanager/nodemanager.proto`** |
| Legacy embedded host | **Removed** `decentralized-api/internal/devshard/`, `decentralized-api/cmd/devshardd/` |
| Observability | `common/observability/otelutil` shared; dapi `observability/` kept with OTel wrappers |
| `go.mod` | `common => ../common`, `inference-chain` replace; **no** `devshard` dependency in dapi |

**Validated:** `go build ./...` for `common/` and `decentralized-api/`.

**Known gap after Phase A:** per-inference payload pruning (`prune_sink` → `DeleteInference`) is **not wired** in dapi. Only `ManagedStorage` epoch sweeps (retain 3 epochs) run. Re-wire in Phase B against standalone `devshardd` (see payload section below).

---

## Next: Phase C — routing and deployment runtime

See [Phase C](#phase-c-routing-and-deployment-runtime) below.

---

## Completed: Phase B — `devshard/` root

**Validated:** `go build ./...`; unit tests pass for `cmd/devshardd/session`, `bridge`, `host`, `server` (postgres/testcontainers tests need Docker).

### Architectural fork (context)

| Concern | r2 (`devshard-0.2.13-v2-r2`) | pixelplex (`pixelplex-refactoring`) |
|---------|------------------------------|-------------------------------------|
| Process model | Embedded host lived in **dapi** (`decentralized-api/internal/devshard/`) | Standalone **`devshard/cmd/devshardd/`** spawned by versiond |
| ML node client | `devshard/mlnode/` + `devshard/nodemanager/` protos | **`common/nodemanager`** (already merged in dapi) |
| Runtime params | `devshard/runtimeconfig/` adaptive gRPC long-poll → dapi NodeManager, chain fallback | devshardd chain-only at bind (`GetSessionBindParams`) — **regression fixed**: wire `newParamsProvider` + `RuntimeParamsProvider` like r2 |
| Session bind (lane B) | `ApplyLiveSessionParams` via long-poll snapshot (`validation_rate=0` → keep default 5000) | `ApplyChainSessionBindParams` direct chain query — **removed**; keep `ApplyLiveSessionParams` only |
| Payload storage | dapi `payloadstorage` + `prune_sink` adapter | **`common/storage/payloads`** inside devshardd + epoch `DropEpoch` on new block |
| HTTP surface | `transport.Server.Register` → `/v1/devshard/*` in dapi | Routes via `devshard/server` + versiond path `/devshard/<version>/*` |
| Bridge API | `escrowID string`, `GetValidationThreshold`, richer `EscrowInfo` (no `QueryParams` at bind) | `escrowID uint64`, slimmer bridge |
| Host pruning | `PruneEventSink` / `emitSealPrunesLocked` in `host/host.go` | **Removed** from pixelplex host |
| Observability | Echo middleware on `server/routes.go`, devshard metrics | Simplified routes |

Phase A removed embedded host from dapi. Phase B lands **pixelplex devshardd** while **keeping r2 semantics** where pixelplex dropped them (bridge, prune events, observability, tests).

---

### Phase B resolution order (do in this sequence)

#### Step 0 — `go.mod` / `go.sum` first (**before** other devshard file merges)

Merge dependency sets, then tidy, so later compiles have a stable module graph.

| Source | Adds |
|--------|------|
| pixelplex | `common`, `inference-chain`, `cosmos-sdk` replace block; chain query deps for devshardd |
| r2 (branch) | `common` replace, OTel, testcontainers, prometheus |

**Decision:** **Union** — keep all replaces (`common`, `inference-chain`, `cosmos-sdk`); retain r2 OTel/testcontainers/prometheus deps.

```bash
# Start from r2 devshard/go.mod, add pixelplex requires/replaces, then:
cd devshard && go mod tidy
go build ./...   # expect failures until Steps 1–5 complete; mod graph should resolve
```

Do **not** merge conflicting `.go` files until Step 0 completes and `go mod tidy` is clean.

#### Step 1 — Clean adds (pixelplex, no conflict)

```bash
git checkout pixelplex-refactoring -- \
  devshard/cmd/devshardd/ \
  devshard/Dockerfile \
  devshard/signing/
```

Wire: `common/chain`, `common/nodemanager`, `common/storage/payloads`, `common/storage/validationlease`, `devshard/storage`, `devshard/signing`.

#### Step 2 — Clean deletes

```bash
git rm -r devshard/mlnode/
git rm -r devshard/nodemanager/   # after runtimeconfig import fix → common/nodemanager/gen
```

Repoint `devshard/runtimeconfig/*`: `devshard/nodemanager/gen` → `common/nodemanager/gen`.

#### Step 3 — r2-only packages (keep from HEAD)

```
devshard/runtimeconfig/          # gRPC long-poll to dapi GetRuntimeConfig
devshard/host/prune.go           # PruneEventSink types
devshard/host/prune_test.go
devshard/observability/          # init.go already uses common/observability/otelutil
```

#### Step 4 — Resolve the 17 “changed in both” files (decisions below)

#### Step 5 — Post-merge wiring

- Implement `PruneEventSink` adapter in `cmd/devshardd/session/` → `common/storage/payloads` delete API (replaces deleted dapi `prune_sink`).
- Wire `cmd/devshardd/app.go` with `newParamsProvider` → `runtimeconfig.NewAdaptive` (prefer dAPI `GetRuntimeConfig` long-poll, chain fallback); `HostManager.SetRuntimeParamsProvider(runtimeparams.FromSnapshot(...))`.
- Adapt `cmd/devshardd/bridge/chain.go` to r2 `MainnetBridge` (string escrow IDs, richer `EscrowInfo`, `GetValidationThreshold`). Lane-B bind params come from long-poll, **not** bridge `QueryParams`.
- `cd devshard && go mod tidy && go test ./...`

---

### Conflict resolution decisions (Step 4 detail)

#### 1. `devshard/bridge/*` — **keep r2**

| File | Decision |
|------|----------|
| `bridge/interface.go` | **r2** — `escrowID string`; `GetValidationThreshold`; full `EscrowInfo` (fees, seal grace, `EpochID`) |
| `bridge/rest.go` | **r2** — REST impl for escrow, validation threshold (no `/params` bind path) |
| `bridge/group.go`, `group_test.go`, `rest_test.go` | **r2** — update `cmd/devshardd/bridge/chain.go` to match r2 interface (not pixelplex uint64/slim API) |

**Note:** pixelplex `cmd/devshardd/bridge/ChainBridge` was written for uint64 escrow IDs. After taking r2 bridge types, **refactor ChainBridge** to implement r2 `MainnetBridge` (string IDs, threshold query, session bind params). This may diverge from pixelplex until inference-chain escrow ID types are unified — prefer r2 contract on this branch.

`user/bind_config.go` + `SessionConfigFromEscrow` depend on richer `EscrowInfo`; do not take pixelplex slim `EscrowInfo`.

#### 2. `devshard/host/host.go` (+ `host_test.go`) — **manual merge, keep r2 prune**

| Keep from r2 | Take from pixelplex |
|--------------|---------------------|
| `pruneSink`, `prunedFired`, `emitSealPrunesLocked`, `WithPruneSink` (6 refs) | Host struct refactors, error handling, other non-prune edits |

**Procedure:**

1. Check out pixelplex `host/host.go` as base.
2. Re-apply r2 prune fields, `WithPruneSink`, `emitSealPrunesLocked` call in `applyAndPersist`, and `prunedFired` dedupe.
3. Keep `host/prune.go` + `host/prune_test.go` from r2 (Step 3).
4. Run `go test ./host/...`.

#### 3. `devshard/transport/server.go` (+ `transport/server_test.go`) — **pixelplex**

- **Decision:** take **pixelplex** — removes `Register()` (legacy `/v1/devshard` mount in dapi; correct after Phase A).
- Update tests for removed `Register()` and pixelplex handler signatures.

#### 4. `devshard/server/routes.go` (+ related) — **manual merge**

| Keep from r2 | Take from pixelplex |
|--------------|---------------------|
| `observability.EchoMiddleware()`, `RequestIDMiddleware` | Route registration shape (no `recordChatTerminal` bool on `withSessionAuth` if pixelplex signature is cleaner — re-add observability inside wrappers) |
| `recordSessionResolution`, `sessionHTTPError`, `sessionResolutionStatus` | `withSession` / `withSessionAuth` structure |
| `ErrInitializing` → 503, version/epoch conflict → 409 | |
| `routeLabel`, chat terminal / no-receipt metrics | |

**Procedure:** start from pixelplex `routes.go` (fits versiond/devshardd), graft r2 observability middleware and error classification from current `server/routes.go`.

Also check: `transport/client_test.go`, `protocol/http_test.go` — **keep r2 tests** where they cover observability/error paths; adapt imports/signatures for pixelplex transport.

#### 5. Smaller conflicts

| Path | Decision |
|------|----------|
| `engine.go` | manual — prefer pixelplex structure; fix for **string** escrow IDs if needed |
| `storage/memory.go`, `storage/sqlite.go` | pixelplex implementation unless r2 has a clear bugfix; **keep r2 tests** (`*_test.go`, `shared_test.go`) |
| `user/httpsession.go` | manual — align with r2 string escrow IDs + r2 bridge |
| `host/host_test.go` | **r2** tests + fixes for merged host |

**Tests rule:** when r2 and pixelplex both changed a `*_test.go`, **keep r2** and update for pixelplex API surface.

---

### Expected conflict files (`merge-tree`: changed in both)

```
devshard/bridge/group.go              → r2
devshard/bridge/group_test.go         → r2
devshard/bridge/interface.go          → r2
devshard/bridge/rest.go               → r2
devshard/bridge/rest_test.go          → r2
devshard/engine.go                    → manual (string escrow)
devshard/go.mod                       → Step 0 first (union)
devshard/go.sum                       → Step 0 first (union)
devshard/host/host.go                 → manual (pixelplex + r2 prune)
devshard/host/host_test.go            → r2
devshard/protocol/http_test.go        → r2 tests
devshard/storage/memory.go            → pixelplex code / r2 tests
devshard/storage/sqlite.go            → pixelplex code / r2 tests
devshard/transport/client_test.go     → r2 tests
devshard/transport/server.go          → pixelplex
devshard/transport/server_test.go     → r2 tests, adapt for no Register()
devshard/user/httpsession.go          → manual (r2 string escrow)
```

### Payload pruning (post Step 5)

| Layer | Target after merge |
|-------|-------------------|
| Policy | r2 `host` emits `InferencePruneEvent` via `PruneEventSink` |
| Adapter | **new** in `cmd/devshardd/session/` (not dapi `prune_sink`) |
| Store | `common/storage/payloads` per-inference delete + epoch `DropEpoch` backstop |

---

### Phase B workflow (summary)

```bash
# Step 0 — mod union FIRST
# Edit devshard/go.mod: union pixelplex + r2 requires/replaces
cd devshard && go mod tidy

# Step 1 — pixelplex adds
git checkout pixelplex-refactoring -- devshard/cmd/devshardd devshard/Dockerfile devshard/signing/

# Step 2 — deletes + import repoint
git rm -r devshard/mlnode devshard/nodemanager
# fix devshard/runtimeconfig/* → common/nodemanager/gen

# Step 3 — keep r2-only
git checkout HEAD -- devshard/runtimeconfig devshard/host/prune.go devshard/host/prune_test.go devshard/observability

# Step 4 — per-file decisions (table above)
git checkout HEAD -- devshard/bridge/
git checkout pixelplex-refactoring -- devshard/transport/server.go
# manual: host/host.go, server/routes.go, engine.go, user/httpsession.go

# Step 5 — wire ChainBridge + prune adapter; tidy + test
cd devshard && go mod tidy && go test ./...
```

---

## Phase C: Routing and deployment runtime

**Sources:** `pixelplex-refactoring` vs `devshard-0.2.13-v2-r2` (`git diff devshard-0.2.13-v2-r2..pixelplex-refactoring -- proxy/ versiond-router/ proxy-ssl/ local-test-net/ deploy/join/docker-compose.versiond.yml` → 23 files, +484/−967 lines).

**Goal:** wire **versiond → devshardd** behind the public proxy with optional **multi-versiond sticky routing**, without regressing r2 production-proxy features.

---

### What is `local-test-net/`?

`local-test-net/` is the **modular Docker Compose lab** for running a small Gonka network on a developer machine. It is **not** the production join stack (`deploy/join/`); it is the fixture Testermint and manual integration tests use.

| Piece | Role |
|-------|------|
| `docker-compose-base.yml` | Core stack per node: `chain-node`, `api` (dapi), `mock-server` |
| `docker-compose.genesis.yml` / `join.yml` | Genesis vs join-node overlays |
| `docker-compose.versiond.yml` | Adds **versiond** (+ test fixtures) so devshardd runs as a versiond child binary |
| `docker-compose.dns*.yml` | Wildcard DNS for `ml-*.KEY_NAME.test` → mock-server |
| `launch.sh`, `launch_full.sh`, `stop.sh` | One-command bring-up/teardown of genesis + join nodes |
| `test_build.sh`, `stop-rebuild.sh` | Rebuild images and recycle stacks |

Typical Testermint usage layers files per key, e.g. `base + genesis + versiond` for `DevshardStandaloneTests` / `VersiondTests` / `DevsharddRuntimeConfigTests`. Paths in compose files are relative to `local-test-net/`; build contexts often point at repo root (`../versioned`, `./build/devshardd`).

**Pixelplex reshapes the versiond overlay** (see below) and splits Postgres: dapi payload Postgres is removed from `docker-compose-base.yml`; devshardd gets a shared `devshard-postgres` service instead.

---

### `proxy/` — nginx edge + Go sidecar

**Role today (r2):** single public entry (80/443) for dapi, chain RPC/API/gRPC, explorer, SSL via `proxy-ssl`, rate limits, fail2ban sidecar, **Jaeger/Grafana UI proxying**, **RPC method logging** (nginx `mirror` → sidecar Unix socket), and `/devshard/<version>/*` → versiond.

| File | Pixelplex change | r2-only to **keep** on merge |
|------|------------------|------------------------------|
| `entrypoint.sh` | Adds `PUBLIC_DEVSHARD_VERSION`, `PUBLIC_DEVSHARD_ROUTE_PATHS` (default list from `edge-api/queryapi/openapi.yaml`); `append_public_devshard_route_locations()` rewrites selected `/v1/...` paths to `/${PUBLIC_DEVSHARD_VERSION}$uri` → `versiond_backend` **before** generic `/v1/` locations | Jaeger/Grafana env defaults, `htpasswd` gate, startup validation; existing `/devshard/` location block |
| `nginx.unified.conf.template` | Drops Jaeger/Grafana upstream blocks; removes `request_id` from access logs; removes RPC `mirror` + `/__rpc_method_log` internal location; fixes `$$limit_zone_name` → `$limit_zone_name` typos | Jaeger/Grafana locations; RPC method mirror; `request_id` in JSON log (if sidecar still uses it) |
| `sidecar/main.go` | **Large deletion** (~450 lines): removes RPC method log ingestion (`rpc_method_log.sock`, JSON-RPC parse, param hashing) | Entire RPC method logging pipeline + `ProcessEntry` refactor pieces tied to it |
| `sidecar/main_test.go` | **Deleted** (173 lines of RPC logging tests) | Restore tests if keeping RPC logging |
| `Dockerfile` | Drops `apache2-utils` (was for Jaeger basic auth) | Keep `apache2-utils` if keeping Jaeger htpasswd |
| `Makefile` | Drops `blst-portable.mk`; hardcodes `linux/amd64`; sanitizes git describe (`tr '/' '-'`) | Keep `blst-portable.mk` / `DOCKER_PLATFORM` for Apple Silicon local builds |
| `README.md` | Removes Jaeger/Grafana docs and observability compose examples | Keep observability UI security section |

**Merge decision:** **manual** — take pixelplex **public devshard route splitting** (`PUBLIC_DEVSHARD_*`) and nginx `$$` fixes; **retain r2** observability UI proxy, RPC method logging, and portable build Makefile hooks unless explicitly deprecating those features.

**New env vars (pixelplex):**

| Variable | Default | Effect |
|----------|---------|--------|
| `PUBLIC_DEVSHARD_VERSION` | `v0.2.12` | Version prefix prepended when steering public query routes to versiond |
| `PUBLIC_DEVSHARD_ROUTE_PATHS` | long `/v1/...` list | Space-separated paths (supports `{param}` segments) routed to devshardd instead of dapi |
| `VERSIOND_SERVICE_NAME` | `versiond` | Upstream for `/devshard/` and public devshard routes; set to `versiond-router` when using sticky LB |
| `DISABLE_DEVSHARD_PROXY` | `false` | Disables all devshard/versiond proxy locations |

---

### `versiond-router/` — **new in pixelplex** (absent on r2)

Small **nginx:alpine** image that sits in front of **N versiond** instances and provides **consistent hashing on escrow/session ID** so retries stick to the same devshardd child.

| File | Purpose |
|------|---------|
| `nginx.conf.template` | `upstream versiond_pool` with `hash $sticky_key consistent`; `$sticky_key` = escrow ID captured from `/<version>/sessions/<id>/...` (or full URI fallback) |
| `entrypoint.sh` | Renders `${UPSTREAM_SERVERS}` from `VERSIOND_HOSTS` (space-separated) + `VERSIOND_PORT` |
| `Dockerfile` / `Makefile` | Publishes `ghcr.io/product-science/versiond-router:<tag>` |

**Traffic path:**

```
Client → proxy (/devshard/ stripped) → versiond-router:8080 → versiond-N:8080 → devshardd child
```

**Merge decision:** **port wholesale** from pixelplex (new directory). Required for multi-versiond local tests and `deploy/join/docker-compose.versiond.yml`.

---

### `proxy-ssl/` — **skipped** (keep r2)

**Role:** small Go service; `proxy` calls it on startup to obtain Let's Encrypt certs (`CERT_ISSUER_DOMAIN`, JWT auth).

**Pixelplex delta:** `Makefile` only — drops `blst-portable.mk`, hardcodes `linux/amd64`, adds `VERSION` sanitization (`tr '/' '-'`). No Go, Dockerfile, or README changes.

**Decision:** **skip merge** — keep r2 entirely. Pixelplex Makefile edits regress Apple Silicon (`DOCKER_PLATFORM`) for no functional gain. Optional `VERSION` sanitization can be cherry-picked repo-wide later if needed; not blocking this branch.

---

### `local-test-net/` — compose + scripts

| File | Pixelplex change | Notes |
|------|------------------|-------|
| `docker-compose.versiond.yml` | YAML anchor `x-versiond`; **3 versiond** (`versiond`, `versiond-2`, `versiond-3`) + **`versiond-router`**; build context `../versioned`; override key `VERSIOND_OVERRIDE_v0_2_11`; **hardcoded** `PGHOST=devshard-postgres` | Replaces single-versiond r2 overlay |
| `docker-compose.devshard-postgres.yml` | **New** — shared `devshard-postgres` (postgres:16, db/user `devshardd`) | Genesis brings this up once for all versiond children |
| `docker-compose.devshard-router-proxy.yml` | **New** — sets `proxy.environment.VERSIOND_SERVICE_NAME=versiond-router` on genesis only | Wires public proxy to sticky router |
| `docker-compose-base.yml` | **Removes** per-node `postgres` service + dapi `PGHOST`/`depends_on` | dapi payload Postgres no longer in base stack |
| `docker-compose.postgres.yml` | **Deleted** (was genesis-only host port expose for Testermint JDBC) | May need r2 equivalent if JDBC asserts still required |
| `launch.sh` | Stops including `docker-compose.postgres.yml` on genesis up | |
| `stop.sh` | `down` without `-v` (volumes preserved across restarts) | r2 used `down -v` |
| `stop-rebuild.sh` | Drops `blst-portable.sh`; pins `DEVSHARD_VERSION=v0.2.11`; drops `versiond-build-docker` | r2 rebuilds versiond image too |
| `test_build.sh` | Simpler `make build-docker` (no blst-portable source) | |

**Merge decision:** port **versiond multi-instance + router + devshard-postgres** overlays; **reconcile** dapi Postgres story (r2 had per-node `payloads` DB in base — confirm whether dapi still needs it after Phase A); **keep** r2 `blst-portable.sh` / `versiond-build-docker` in scripts unless Testermint docs updated.

---

### Completed: `local-test-net/` (hybrid)

**Status:** merged on `merge/pixelplex-refactoring-into-r2`.

| File | Decision |
|------|----------|
| `docker-compose.versiond.yml` | **Hybrid** — pixelplex `x-versiond` anchor + 3× versiond + `versiond-router`; r2 `DOCKER_PLATFORM`, `api` env patch, `VERSIOND_OVERRIDE_dev` / `v1` / `v0_2_11` / `v0_2_13`; build contexts `./versioned` / `./versiond-router` (repo-root `--project-directory`); `${PGHOST:-}` empty default → SQLite without postgres overlay |
| `docker-compose.devshard-postgres.yml` | **pixelplex** + patches `versiond` / `versiond-2` / `versiond-3` PG env + `depends_on` |
| `docker-compose.devshard-router-proxy.yml` | **pixelplex** — `VERSIOND_SERVICE_NAME=versiond-router` on genesis `proxy` |
| `docker-compose-base.yml` | **r2** — per-node dapi `postgres` (`payloads` DB) retained |
| `docker-compose.postgres.yml` | **r2** — host port for Testermint JDBC (`DevshardPostgresStorageTests`) |
| `launch.sh`, `launch_full.sh` | **r2** — still include `docker-compose.postgres.yml` on genesis |
| `stop.sh` | **r2** — `down -v` |
| `stop-rebuild.sh`, `test_build.sh` | **r2** — `blst-portable.sh`, dynamic `DEVSHARD_VERSION`, `versiond-build-docker` |

**`versiond-router/`:** ported wholesale from pixelplex (required by multi-versiond overlay).

**Testermint (not done here):** `DevshardStandaloneTests` still lists only `docker-compose.versiond.yml` per pair; add `docker-compose.devshard-postgres.yml` on genesis (and optionally `docker-compose.devshard-router-proxy.yml` for sticky-router tests) when updating Phase E.

**Validated:** full genesis stack `docker compose ... config` succeeds (base + genesis + versiond [+ devshard-postgres + devshard-router-proxy]).

---

### `deploy/join/` (selective)

| File | Pixelplex change |
|------|------------------|
| `docker-compose.versiond.yml` | **New overlay**: `devshard-postgres`, `versiond` + `versiond2`, `versiond-router`, patches `proxy` env (`VERSIOND_SERVICE_NAME=versiond-router`, `PUBLIC_DEVSHARD_VERSION`) |

Production join stack: layer on existing `docker-compose.yml` when running multi-versiond devshardd.

---

### Phase C resolution order

1. **`versiond-router/`** — **Done** — clean add from pixelplex (`Dockerfile`, `Makefile`, `entrypoint.sh`, `nginx.conf.template`)
2. **`proxy/`** — manual merge (public devshard routes + retain r2 observability/RPC logging)
3. **`local-test-net/`** — **Done** (hybrid; see [Completed: local-test-net](#completed-local-test-net-hybrid))
4. **`deploy/join/docker-compose.versiond.yml`** — port overlay
5. **`proxy-ssl/`** — **skipped** (keep r2; see [proxy-ssl](#proxy-ssl--skipped-keep-r2))

### Validation

```bash
# Render checks (from repo root; overlays must be layered with base + versiond)
KEY_NAME=genesis VERSIOND_SIGNER_KEY_NAME=genesis docker compose \
  -f local-test-net/docker-compose-base.yml \
  -f local-test-net/docker-compose.genesis.yml \
  -f local-test-net/docker-compose.versiond.yml \
  --project-directory . config

KEY_NAME=genesis VERSIOND_SIGNER_KEY_NAME=genesis docker compose \
  -f local-test-net/docker-compose-base.yml \
  -f local-test-net/docker-compose.genesis.yml \
  -f local-test-net/docker-compose.versiond.yml \
  -f local-test-net/docker-compose.devshard-postgres.yml \
  -f local-test-net/docker-compose.devshard-router-proxy.yml \
  --project-directory . config

# After merge, from deploy/join with config.env sourced:
docker compose -f docker-compose.yml -f docker-compose.versiond.yml config
```

---

## Phase D: `inference-chain` — **skipped**

**Decision:** do **not** merge pixelplex `inference-chain` changes on this branch. Chain stays at the r2 baseline.

Rationale:

- Phase B intentionally keeps **r2 devshard bridge** (`escrowID string`, richer `EscrowInfo`, `GetValidationThreshold`) regardless of pixelplex uint64/slim bridge. Session bind lane B uses **`ApplyLiveSessionParams`** from the runtime-config long-poll snapshot, not direct chain `QueryParams` on the bridge.
- Pixelplex chain commits (`d7ed8c128` fee/gas, `4c1e38797` validation token verification, `cf363ec10` v0.2.13 upgrade handler) would force bridge/proto reconciliation and are out of scope for the dapi/common/devshard merge.
- Any future chain alignment (e.g. escrow ID type unification) is a **separate** branch/PR against `inference-chain`, not Phase D of this plan.

**Implication:** `cmd/devshardd/bridge/chain.go` must continue to implement r2 `MainnetBridge` against the **unchanged** r2 chain types.

---

## Phase E: Test harness

- `testermint` — legacy `DevshardTests.kt` vs versiond paths (pending)
- `test-net-cloud` — **Done** (selective; see below)

### Completed: `test-net-cloud/devshard-testing/` (selective)

**Decision:** clean add from pixelplex; **keep r2** for all other `test-net-cloud/` paths (`k8s/`, `nebius/bridge/`, scripts, etc.).

| Path | Decision |
|------|----------|
| `test-net-cloud/devshard-testing/` | **pixelplex** — cloud E2E harness: chain escrow create → local `devshardctl` → remote `--route-prefix` on live testnet |
| `test-net-cloud/nebius/bridge/` | **r2** — pixelplex lacks r2 wrap/unwrap tooling (`a5dcfeae1`); do not take pixelplex nebius tree |
| `test-net-cloud/k8s/`, `gonka-client-testing/`, rest | **r2** — unchanged |

**Validated:** `cd test-net-cloud/devshard-testing && go mod tidy && go build . && go test ./...`

**Usage:** see `test-net-cloud/devshard-testing/README.md` (SSH tunnel to testnet, `--route-prefix /devshard/<version>`, optional `--finalize`).

---

## Root block checklist

| Root | Status |
|------|--------|
| `common` | **Done** |
| `decentralized-api` | **Done** |
| `devshard` | **Done** (Phase B) |
| `proxy` | Pending (see Phase C — manual merge) |
| `versiond-router` | **Done** |
| `local-test-net` | **Done** (hybrid overlays; r2 base/scripts retained) |
| `proxy-ssl` | **Skipped** (keep r2; Makefile-only pixelplex delta) |
| `deploy` | Selective (`docker-compose.versiond.yml`) |
| `inference-chain` | **Skipped** |
| `testermint` | Selective |
| `test-net-cloud` | **Done** (`devshard-testing/` from pixelplex; r2 elsewhere) |

---

## Per-file decision log (running)

| Path | Decision | Reason |
|------|----------|--------|
| `common/` | theirs | new shared module |
| `decentralized-api/main.go` | theirs + manual | no HostManager; nodemanager 3-arg server |
| `decentralized-api/apiconfig/*` | ours (r2) | runtime config publisher |
| `decentralized-api/cosmosclient/*` | ours (r2) | gas + OTel |
| `common/nodemanager/nodemanager.proto` | manual | merged GetRuntimeConfig from r2 devshard proto |
| `decentralized-api/internal/devshard/` | **delete** | legacy embedded host |
| `devshard/go.mod` | **union first** | pixelplex chain deps + r2 OTel/testcontainers; tidy before .go merges |
| `devshard/bridge/*` | **r2** | string escrow ID, `GetValidationThreshold`, rich `EscrowInfo`; no `SessionBindParamsBridge` |
| `devshard/cmd/devshardd/` | theirs + adapt | standalone runtime; adaptive long-poll params; refactor `bridge/chain.go` to r2 `MainnetBridge` |
| `devshard/mlnode/` | **delete** | → `common/nodemanager` |
| `devshard/host/host.go` | **manual** | pixelplex base + r2 `PruneEventSink` / `emitSealPrunesLocked` |
| `devshard/host/prune.go` | **r2** | prune event types |
| `devshard/transport/server.go` | **pixelplex** | no legacy `Register()` |
| `devshard/server/routes.go` | **manual** | pixelplex routes + r2 observability/error classification |
| `devshard/*_test.go` (conflicts) | **r2** | keep r2 tests; adapt signatures for pixelplex surface |
| `devshard/storage/memory.go`, `sqlite.go` | pixelplex code | r2 tests retained |
| `proxy/` | **manual** | pixelplex `PUBLIC_DEVSHARD_ROUTE_PATHS` + r2 Jaeger/Grafana/RPC logging |
| `versiond-router/` | **pixelplex** | **Done** — sticky LB; clean add |
| `local-test-net/docker-compose.versiond.yml` | **hybrid** | **Done** — multi-versiond + router; r2 overrides/platform/paths |
| `local-test-net/docker-compose.devshard-postgres.yml` | **pixelplex** + patches | **Done** — shared devshardd PG on genesis |
| `local-test-net/docker-compose.devshard-router-proxy.yml` | **pixelplex** | **Done** — proxy → versiond-router |
| `local-test-net/docker-compose-base.yml` | **r2** | **Done** — kept per-node dapi postgres |
| `local-test-net/*.sh` | **r2** | **Done** — blst-portable, postgres overlay, `down -v` |
| `proxy-ssl/` | **skip** | keep r2; pixelplex Makefile-only (drops arm64) |
| `deploy/join/docker-compose.versiond.yml` | **pixelplex** | production multi-versiond overlay |
| `inference-chain/` | **skip** | out of scope; r2 chain baseline retained |
| `test-net-cloud/devshard-testing/` | **pixelplex** | **Done** — cloud smoke harness; r2 nebius/k8s kept |

---

## Follow-up todos

Post-merge refactors to evaluate (not blocking Phases C–E):

- [ ] **Observability → `common/`** — Assess whether `devshard/observability/` (and overlapping dapi wiring such as `decentralized-api/internal/validation/observability_wire.go`) can consolidate into `common/observability/` beyond the existing `otelutil` helpers. Goal: one shared OTel/metrics/request-ID surface for dapi, devshardd, and tests without cross-module imports (`devshard` ↔ `decentralized-api`).

- [ ] **Cosmos chain clients → `common/`** — Assess whether `devshard/cmd/devshardd` chain usage (`bridge/chain.go`, `tx/manager.go`, session signing) and dapi’s `cosmosclient/` overlap enough to move into `common/chain/` (or extend it — `common/chain/client.go` already exists). Goal: shrink `devshard/go.mod` indirect closure by sharing a thin chain query/tx wrapper instead of each binary importing `inference-chain` + full `cosmos-sdk` graphs independently.
