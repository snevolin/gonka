# Port Plan: Tier A → `edge-api`

Extract the **22 Tier A read-only `/v1/` routes** from `decentralized-api` into a new
standalone service at repo root: **`edge-api/`**.

Move `common/queryapi` into `edge-api` (not `common/`, not `decentralized-api`). Apply
handler fixes so **edge-api matches current dapi client contracts** (not raw pixelplex
queryapi shapes). Wire **edge-api** behind the public proxy for Tier A paths. **Remove**
the duplicate handlers from dapi when the port is complete.

Related: [merge-plan.md](./merge-plan.md) Phase C (proxy — steer Tier A to edge-api, not
devshardd).

## Progress

| Phase | Status |
|-------|--------|
| 0 — Scaffold `edge-api/` | **Done** |
| 1 — Move `common/queryapi` | **Done** |
| 2 — Handler fixes (dapi compatibility) | **Done** |
| 3 — Deploy & proxy routing | **Done** |
| 4 — Validation | **Done** |
| 5 — Remove Tier A from dapi | **Done** |
| 6 — Docs & merge-plan | **Done** |

---

## Goals

| Goal | Detail |
|------|--------|
| Single owner for Tier A | One process, one OpenAPI spec, one handler tree |
| devshardd = devshard only | No `/v1/` query surface on devshardd |
| dapi = inference + ops | Chat, payloads, PoC proofs, stats, bridge queue, admin paths stay on dapi |
| No pixelplex proxy split to devshardd | Do **not** use `PUBLIC_DEVSHARD_ROUTE_PATHS` → versiond |
| Chain transport | gRPC only (`common/chain` + `cmtservice`); no Tendermint HTTP RPC in edge-api |
| Backward compatible JSON | Match dapi responses where clients exist (especially `/v1/models`) |

---

## Target architecture

```text
Client
  │
  ├─ /v1/status, /v1/models, … (22 Tier A paths)
  │     → proxy → edge-api:PORT
  │
  ├─ /v1/chat/completions, /v1/poc/proofs, … (dapi-only)
  │     → proxy → decentralized-api:9000
  │
  └─ /devshard/<version>/sessions/…
        → proxy → versiond → devshardd
```

**Dependencies of edge-api:** `common/chain`, `common/logging`, `common/utils` only. No
broker, stats DB, payload storage, keyring, or ML nodes.

---

## `edge-api/` layout (new root)

```
edge-api/
  go.mod                    # module edge-api; replace common, inference-chain
  Makefile
  Dockerfile
  cmd/edge-api/
    main.go                 # config, chain client, Echo listen
  internal/
    server/
      server.go             # Echo bootstrap, middleware, RegisterHandlers
  queryapi/                 # moved from common/queryapi
    openapi.yaml
    generate.go
    gen/                    # oapi-codegen output (models + server)
    handlers.go
    status.go models.go …   # handler impls
    tests/
  tools.go                  # oapi-codegen pin (optional)
```

**Module path:** `edge-api` with `replace common => ../common`.

**OpenAPI codegen:** keep existing `oapi-codegen` flow (`gen/models-codegen.yaml`,
`gen/server-codegen.yaml`, `go generate` in `queryapi/`). Echo registration stays
`gen.RegisterHandlers(e, handlers)`.

---

## Completed: Phase 0 — Scaffold `edge-api/`

**Status:** **Done**

| Deliverable | Path |
|-------------|------|
| Module + replaces | `edge-api/go.mod`, `edge-api/go.sum` |
| Entrypoint | `edge-api/cmd/edge-api/main.go` (`EDGE_API_PORT`, `CHAIN_GRPC_URL`, graceful shutdown) |
| Echo server | `edge-api/internal/server/server.go` (`/healthz` + `edge-api/queryapi` handlers) |
| Build | `edge-api/Makefile` |
| Container image | `edge-api/Dockerfile` (repo-root build context) |

**Validated:** `cd edge-api && go build ./...`

**Intentionally not done yet (later phases):** proxy wiring, dapi route removal, handler
compatibility fixes (Phase 2).

---

## Completed: Phase 1 — Move `queryapi` → `edge-api/queryapi`

**Status:** **Done**

| Step | Result |
|------|--------|
| `git mv common/queryapi` → `edge-api/queryapi` | Done |
| Import paths `edge-api/queryapi`, `edge-api/queryapi/gen` | Done |
| `edge-api/internal/server/server.go` wired to local queryapi | Done |
| `devshard/cmd/devshardd/server.go` — queryapi removed; `/healthz` + session routes only | Done |
| `common/Makefile` — queryapi generate removed | Done |
| `edge-api/Makefile` — `generate-api`, `check-gen` added | Done |
| `edge-api/tools.go` — oapi-codegen pin | Done |
| `common/tools.go` — oapi-codegen removed | Done |

**Validated:**

```bash
cd edge-api && go build ./... && go test ./queryapi/... -count=1
cd devshard && go build ./cmd/devshardd/...
cd common && go mod tidy && go build ./... && go test ./... -count=1
```

All passed (2025-06-19, Docker available for `common/storage/payloads` testcontainers).

**Not in Phase 1 scope:** `decentralized-api` (no `queryapi` import; unchanged).

---

## Completed: Phase 2 — Handler fixes (dapi compatibility)

**Status:** **Done**

Apply merge recommendations before cutting traffic over. **edge-api is the canonical
implementation**; fixes go in `edge-api/queryapi`, not dapi.

### 2.1 Response shapes (client-visible)

| Route | Result |
|-------|--------|
| **`GET /v1/models`** | OpenAPI + handler return `{object:"list", data:[ModelDescriptor...]}` (ported from dapi). |
| `GET /v1/governance/models` | Returns proto `models` slice like dapi (`governanceModelsResponse`). |
| `GET /v1/governance/models-legacy` | Returns `{model: [...]}` map like dapi. |
| `GET /v1/pricing` | OpenAPI/DTOs use `Uint64` for `unit_of_compute_price`, `price_per_token`. |
| `GET /v1/versions` | Error body `{"error":"..."}`; timestamp RFC3339 string. |
| `GET /v1/participants` | Removed hardcoded `refunds_owed`/`reputation` (zero defaults; not on chain proto). |

### 2.2 HTTP semantics

| Route | Result |
|-------|--------|
| `GET /v1/participants/{address}` | Nil-response → 404 (`Account not found`). |
| `GET /v1/poc-batches/{epoch}` | Nil / empty → **404** (was 403). |
| Restrictions (×3) | Unchanged — `grpcErrorToHTTP`. |
| `POST /v1/verify-proof`, `verify-block`, `GET /v1/debug/verify` | Unchanged — **400** on verification failure (queryapi behavior). |

### 2.3 Transport (Class D — already correct in queryapi)

| Item | Result |
|------|--------|
| gRPC `cmtservice` for epoch-participants / debug-verify / verify-block | Unchanged |
| **OTel** | `common/observability` chain tracer (`chain.store.query`, `chain.grpc.query`) — mirrors r2 `decentralized-api/cosmosclient/query.go` + `query_client_conn.go` |
| ABCI store queries | Explicit `StartStoreQuery` spans on `getEpochParticipants` ABCIQuery calls (`edge-api/queryapi/epoch.go`) |
| Other gRPC (GetBlockByHeight, GetValidatorSetByHeight, inference queries) | Auto-instrumented via `common/observability.NewObservedConn` in `common/chain/client.go` |
| HTTP ingress | `edge-api/observability` Echo middleware + `EDGE_API_OTEL_ENABLED` / `OTEL_ENDPOINT` init in `cmd/edge-api/main.go` |

### 2.4 BLS decompression

Kept local copy in `edge-api/queryapi/bls.go`.

### 2.5 Proof-bearing response

`ActiveParticipantWithProof` contract preserved.

| Test | Purpose |
|------|---------|
| `edge-api/queryapi/tests/testdata/dapi_epoch_participants_golden.json` | dapi golden top-level JSON shape |
| `TestEpochParticipantsJSONMatchesDapiGolden` | Handler integration test — same keys as golden + `active_participants_bytes` + `proof_ops` present |

**Validated:**

```bash
cd edge-api/queryapi && go generate ./...
cd edge-api && go build ./... && go test ./queryapi/... -count=1
cd common && go test ./observability/... ./chain/... -count=1
```

---

## Phase 2 (reference) — Handler fixes checklist

---

## Completed: Phase 3 — Deploy & proxy routing

**Status:** **Done**

### 3.1 Runtime placement

`edge-api` runs as a sibling container next to dapi (`api` / `chain-node` gRPC at `:9090`).

| Variable | Purpose |
|----------|---------|
| `EDGE_API_PORT` | Listen port inside container (default `18080`) |
| `CHAIN_GRPC_URL` | Chain gRPC (e.g. `genesis-node:9090` or `node:9090`) |
| `EDGE_API_SERVICE_NAME` | Proxy upstream hostname (`edge-api` or `edge-api-router`) |
| `EDGE_API_ROUTE_PATHS` | Optional override of the 22 Tier A paths (defaults in `proxy/entrypoint.sh`) |

### 3.2 `local-test-net/`

| File | Purpose |
|------|---------|
| `docker-compose-base.yml` | Single `edge-api` + proxy `EDGE_API_SERVICE_NAME=edge-api` |
| `docker-compose.edge-api.yml` | Adds `edge-api-2`, `edge-api-3`, `edge-api-router` (round-robin) |
| `docker-compose.edge-api-router-proxy.yml` | Genesis overlay: `EDGE_API_SERVICE_NAME=edge-api-router` |

Proxy (`proxy/entrypoint.sh`):

- **22 Tier A paths** → `edge_api_backend` (edge-api or edge-api-router).
- Remaining `/v1/` → dapi (`api`).
- `/v1/devshard/*` → rewrite to `/devshard/v1/*` → versiond (legacy clients).
- `/devshard/` → versiond (unchanged).
- **No** `PUBLIC_DEVSHARD_ROUTE_PATHS` → devshardd.

### 3.3 `deploy/join/`

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Single `edge-api` + proxy env (`EDGE_API_SERVICE_NAME`, `EDGE_API_PORT`) |
| `docker-compose.edge-api-multi.yml` | Optional: `edge-api2`, `edge-api3`, `edge-api-router`; proxy → router |

### 3.4 `edge-api-router/`

Nginx round-robin across `EDGE_API_HOSTS` (mirrors `versiond-router/` pattern; no sticky hash).

### 3.5 `devshardd`

`devshard/cmd/devshardd/server.go` serves only `GET /healthz` and `/sessions/:id/*` (Phase 1).

### 3.6 Build targets

Root `make build-docker` builds `edge-api` and `edge-api-router` images.

---

## Phase 3 (reference) — Deploy & proxy routing

## Completed: Phase 4 — Validation (before dapi removal)

**Status:** **Done**

### 4.1 Contract tests (dapi JSON shape)

| Test | Path |
|------|------|
| Top-level JSON keys vs dapi contract | `edge-api/queryapi/tests/dapi_contract_test.go` |
| OpenAPI ↔ proxy ↔ canonical 22-route list | `edge-api/queryapi/tests/routes_contract_test.go` |
| Epoch participants golden keys + `proof_ops` | `edge-api/queryapi/tests/epoch_participants_golden_test.go` |
| Live two-server diff (optional) | `edge-api/queryapi/tests/compatibility/` (`-tags compat`, `-endpoint1` / `-endpoint2`) |

Compatibility harness: when full JSON bodies differ only in dynamic values (block height, timestamps), **top-level keys** must still match.

### 4.2 Proof path

| Test | Purpose |
|------|---------|
| `TestEpochParticipantsProofBundleAcceptedByVerifyProof` | `GET /v1/epochs/{epoch}/participants` returns `proof_ops`; `POST /v1/verify-proof` parses the bundle and attempts verification |
| `TestVerifyProofRejectsMalformedBundle` | Malformed verify-proof body returns 400 |

### 4.3 Proxy / compose render

`scripts/validate-edge-api.sh` runs:

- `go test ./edge-api/queryapi/...`
- `docker compose config` for local-test-net (base, multi edge-api + router) and deploy/join (+ multi overlay)

### 4.4 Smoke (optional, live stack)

Set `PROXY_URL` when running the validation script:

```bash
PROXY_URL=http://localhost bash scripts/validate-edge-api.sh
```

Checks `/v1/status`, `/v1/models`, `/v1/epochs/latest/participants` via proxy; confirms `/v1/chat/completions` still reaches dapi.

Live dapi vs edge-api diff:

```bash
EDGE_API_URL=http://localhost:18080 DAPI_URL=http://localhost:9000 \
  bash scripts/validate-edge-api.sh
```

### 4.5 CI

`.github/workflows/verify.yml` — job `build-and-test-edge-api` runs unit tests + validation script (compose render).

**Validated:**

```bash
make -C edge-api validate
```

---

## Phase 4 (reference) — Validation (before dapi removal)

1. **Contract tests:** port `edge-api/queryapi/tests/` + compatibility client; add dapi vs
   edge-api JSON diff tests for the 22 routes (Testermint or Go integration).
2. **Proof path:** `GET /v1/epochs/{epoch}/participants` — verify `proof_ops` present;
   `POST /v1/verify-proof` accepts returned bundle.
3. **Proxy render:** `docker compose … config` for genesis/join stacks with edge-api overlay.
4. **Smoke:** `curl $proxy/v1/status`, `/v1/models`, `/v1/epochs/latest/participants` hit
   edge-api; `curl $proxy/v1/chat/completions` still hits dapi.
5. **Load (optional):** epoch-participants latency vs dapi baseline (expect gRPC win).

---

## Phase 5 — Remove Tier A from `decentralized-api` (final)

**Status:** **Done**

**Only after Phase 4 passes in CI and staging.**

### 5.1 Delete routes from `internal/server/public/server.go`

Remove registrations for all **22 Tier A paths** (see Appendix A).

Keep on dapi:

- `POST /v1/chat/completions`, `/v1/completions`, `GET /v1/chat/completions`
- `GET /v1/inference/payloads`, `POST /v1/participants`
- `GET /v1/governance/pricing`, `/v1/stats/*`
- `POST /v1/poc/proofs`, `GET /v1/poc/artifacts/state`
- `GET /v1/bridge/status`, `GET /v1/identity`
- `/v2/*`

### 5.2 Delete or trim handler files

| File | Action |
|------|--------|
| `get_models_handler.go` | Remove Tier A handlers; keep if governance pricing shares file — else delete |
| `get_pricing_handler.go` | Remove `getPricing` only; keep `getGovernancePricing` |
| `get_participants_handler.go` | Remove list/by-address/epoch-participants; keep v2 participant if still registered |
| `get_epoch.go` | Delete file |
| `get_poc_batches_handler.go` | Delete file |
| `bls_handlers.go` | Delete file |
| `restrictions_handlers.go` | Delete file |
| `bridge_handlers.go` | Remove `getBridgeAddresses` only; keep `BridgeQueue` + status/postBlock |
| `active_participants_verification_handlers.go` | Delete file |
| `debug_handlers.go` | Delete file |
| `app_info_handlers.go` | Delete file (versions moved) |
| `server.go` | Remove inline `getStatus` |

### 5.3 Clean up entities / tests

- Remove unused DTOs from `entities.go`, `public_api_types.go` if only Tier A used them.
- Delete or relocate tests in `*_test.go` that only cover removed handlers.
- Grep dapi for dead imports (`cosmos_client.QueryByKey`, epoch proof helpers only used by
  Tier A).

### 5.4 Verify dapi build

```bash
cd decentralized-api && go build ./... && go test ./internal/server/public/...
```

---

## Completed: Phase 6 — Docs & merge-plan updates

**Status:** **Done**

1. **[merge-plan.md](./merge-plan.md)** updated:
   - [Runtime topology](./merge-plan.md#runtime-topology-edge-api-versiond-and-devshardd) — proxy split, instance counts, answers on multi devshard / multi edge-api.
   - Phase C proxy: Tier A → **edge-api** (`EDGE_API_ROUTE_PATHS`); not `PUBLIC_DEVSHARD_ROUTE_PATHS` → devshardd.
   - `edge-api/` and `edge-api-router/` added to scope summary and root checklist.
   - `common/queryapi` marked relocated to `edge-api/queryapi/`.
2. **`local-test-net/README.md`** — edge-api and versiond overlay sections added.
3. **`proxy/README.md`** — already documents edge-api vs dapi routing (Phase 3).

---

## Phase 6 (reference) — Docs & merge-plan updates

1. Update [merge-plan.md](./merge-plan.md):
   - Phase C proxy: Tier A → **edge-api**, not devshardd / `PUBLIC_DEVSHARD_ROUTE_PATHS`.
   - Add `edge-api/` to root checklist.
2. Mark `common/queryapi` as relocated (remove from `common/` scope).
3. Document edge-api in deploy README / `local-test-net` launch notes.

---

## Execution order (summary)

```text
Phase 0  Scaffold edge-api/ (module, main, healthz)                    [Done]
Phase 1  git mv common/queryapi → edge-api/queryapi; fix imports     [Done]
Phase 2  Handler + OpenAPI fixes (models shape, pricing types, participants, transport OTel) [Done]
Phase 3  Docker + proxy routing to edge-api; strip queryapi from devshardd [Done]
Phase 4  Tests + staging validation [Done]
Phase 5  Remove 22 routes + dead handlers from decentralized-api              [Done]
Phase 6  Docs & merge-plan updates                                        [Done]
```

---

## Risks & mitigations

| Risk | Mitigation |
|------|------------|
| `/v1/models` JSON break | Phase 2.1 — match dapi `ModelDescriptor` before cutover |
| Proxy path ordering | Emit Tier A locations **before** generic `/v1/` → dapi |
| devshardctl / gateway calls `/v1/epochs/...` | Still works via proxy → edge-api (same paths) |
| Duplicate routes during migration | Phases 3–4: both dapi and edge-api may exist; proxy sends Tier A to edge-api only |
| Proof verification regressions | Golden test epoch-participants + verify-proof roundtrip |

---

## Appendix A — Tier A route list (22)

| # | Method | Path |
|---|--------|------|
| 1 | GET | `/v1/status` |
| 2 | GET | `/v1/versions` |
| 3 | GET | `/v1/models` |
| 4 | GET | `/v1/governance/models` |
| 5 | GET | `/v1/governance/models-legacy` |
| 6 | GET | `/v1/pricing` |
| 7 | GET | `/v1/participants` |
| 8 | GET | `/v1/participants/{address}` |
| 9 | GET | `/v1/epochs/{epoch}` |
| 10 | GET | `/v1/epochs/{epoch}/participants` |
| 11 | GET | `/v1/poc-batches/{epoch}` |
| 12 | GET | `/v1/restrictions/status` |
| 13 | GET | `/v1/restrictions/exemptions` |
| 14 | GET | `/v1/restrictions/exemptions/{id}/usage/{account}` |
| 15 | GET | `/v1/bls/epoch/{id}` |
| 16 | GET | `/v1/bls/epochs/{id}` |
| 17 | GET | `/v1/bls/signatures/{request_id}` |
| 18 | GET | `/v1/bridge/addresses` |
| 19 | POST | `/v1/verify-proof` |
| 20 | POST | `/v1/verify-block` |
| 21 | GET | `/v1/debug/pubkey-to-addr/{pubkey}` |
| 22 | GET | `/v1/debug/verify/{height}` |

**Remove all of the above from dapi in Phase 5.**

---

## Appendix B — Transport classes (edge-api target)

| Class | Routes | Transport |
|-------|--------|-----------|
| None | status | — |
| Module gRPC | models, pricing, participants, epochs, poc-batches, restrictions, BLS, bridge addresses | `common/chain` inference/BLS/restrictions clients |
| Comet gRPC | versions | `cmtservice.GetNodeInfo` |
| Comet store/block/validators | epochs/…/participants, debug/verify, verify-block | `ABCIQuery`, `GetBlockByHeight`, `GetValidatorSetByHeight` |
| Local crypto | verify-proof, pubkey-to-addr | `common/utils` |

No Tendermint HTTP RPC (`:26657`) in edge-api.

---

## Appendix C — Proof-bearing responses

| Route | Returns proof? |
|-------|----------------|
| `GET /v1/epochs/{epoch}/participants` | **Yes** — `proof_ops`, `active_participants_bytes`, `block`, `validators` |
| `POST /v1/verify-proof` | No (accepts proof in request body) |
| `POST /v1/verify-block` | No (verifies commit signatures) |
| Other 19 Tier A routes | No |

---

## Appendix D — Source mapping (move reference)

| edge-api/queryapi | Former dapi |
|-------------------|-------------|
| `status.go` | `server.go`, `app_info_handlers.go` |
| `models.go` | `get_models_handler.go`, `get_pricing_handler.go` |
| `participants.go`, `epoch.go` | `get_participants_handler.go`, `get_epoch.go` |
| `poc.go` | `get_poc_batches_handler.go` |
| `bls.go` | `bls_handlers.go` |
| `bridge.go` | `bridge_handlers.go` (addresses only) |
| `restrictions.go` | `restrictions_handlers.go` |
| `verification.go`, `debug.go` | `active_participants_verification_handlers.go`, `debug_handlers.go` |
