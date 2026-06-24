# Stack citest scenarios (S1–S6)

Implemented Go integration tests for the devshard testenv v2 stack. Each scenario
boots a real Docker Compose stack (mock-chain, mock-dapi, mock-openai, versiond × 2,
versiond-router, devshardctl, Postgres) and asserts production-like behaviour end to end.

**Design history:** [`testenv-v2-plan.md`](./testenv-v2-plan.md) (planning doc; scenarios below are what shipped).

## Stack under test

| Service | Role in tests |
|---------|----------------|
| **mock-chain** | Cosmos gRPC `:9090`, CometBFT RPC `:26657`, LCD REST `:1317`, admin `/testenv/*` |
| **mock-dapi** | NodeManager gRPC (`GetRuntimeConfig` long-poll), chainoracle HTTP, fault proxy |
| **mock-openai** | OpenAI-compatible ML upstream for devshardd after `AcquireMLNode` |
| **versiond-0 / versiond-1** | Supervise linux **devshardd** child (protocol `v2`) |
| **versiond-router** | Sticky nginx (`consistent_hash` on session id) |
| **devshardctl** | Gateway (`/v1/chat/completions`, `/v1/status`) |
| **devshard-postgres** | Shared payload store (required for 2× versiond) |

Citest uses an **isolated config** (subnet `172.31.0.0/24`, router `:18080`, gateway
`:18081`, mock ports `19xxx`) so tests can run while a dev `make up` stack is active on
default ports.

Harness: `citest/harness/` — temp workdir, `gencompose`, `docker compose up --wait`,
HTTP/gRPC helpers, log dump on failure.

## How to run

**Prerequisites:** Docker, Go 1.24+, linux **devshardd** binary for containers.

```bash
cd devshard/testenv
make build-devshardd
make citest-stack          # S1–S6
```

Or run a single scenario:

```bash
TESTENV_CITEST=1 go test -tags=testenvci ./citest/ -run TestS3_ParamsLongPoll -v -timeout 30m
```

| Variable / tag | Purpose |
|----------------|---------|
| `TESTENV_CITEST=1` | Opt-in gate (`harness.SkipUnlessEnv`) |
| `-tags=testenvci` | Build tag on `s*_*.go` tests |
| `make citest-stack` | Builds mock images + runs all S1–S6 |

Wrapper script: [`scripts/run-stack-citest.sh`](../scripts/run-stack-citest.sh).

CI: `workflow_dispatch` with `integration: true` → `make -C devshard ci-testenv-integration`.

## Scenario index

| ID | Name | What we validate | Test |
|----|------|------------------|------|
| **S1** | Stack smoke | Full stack boots; all boundaries healthy | `TestS1_StackSmoke` |
| **S2** | Router stickiness | Same session → same versiond upstream | `TestS2_RouterStickiness` |
| **S3** | Params long-poll | Governance patch wakes `GetRuntimeConfig` | `TestS3_ParamsLongPoll` |
| **S4** | Epoch switch | Epoch advance fast-forwards chain + bumps epoch in long-poll | `TestS4_EpochSwitch` |
| **S5** | Gateway chat | devshardctl → router → devshardd → mock-openai (stream + non-stream) | `TestS5_GatewayChat` |
| **S6** | versiond stop | Sticky routing when one versiond is stopped | `TestS6_VersiondStop` |

Source: `devshard/testenv/citest/s{1..6}_*.go`.

---

## S1 — Stack smoke

**What we test:** The generated multi-versiond compose comes up cleanly and every
service boundary responds.

**How:**

1. `harness.BootS1Stack` — writes 2-host citest config, runs `gencompose`, `docker compose up --wait`.
2. `harness.WaitS1Healthy` — polls:
   - mock-chain RPC `/health`
   - mock-dapi `/healthz` and `/v1/epochs/latest`
   - versiond-router `/healthz`
   - devshardctl `/v1/status`
3. `stack.RequireServicesRunning` — `docker compose ps` shows `mock-chain`, `mock-dapi`,
   `mock-openai`, `versiond-router`, `devshardctl`, `devshard-postgres`, `versiond-0`,
   `versiond-1` running.

**Pass criteria:** All health endpoints return 2xx; all listed containers running. Implies
devshardd children started under versiond (protocol `v2`, chain dial to mock-chain).

---

## S2 — Router stickiness

**What we test:** versiond-router **consistent-hash** pins a session id to one upstream
versiond across repeated requests, and at least two distinct upstreams are reachable.

**How:**

1. Boot S1 stack; wait for router `/healthz`.
2. Hit `/{version}/sessions/{sessionA}/healthz` **8 times**; read `X-Upstream-Addr` header
   (exposed by router nginx template).
3. Assert every retry returns the **same** upstream address.
4. Probe up to 64 other session ids until one lands on a **different** upstream.

**Pass criteria:** Stable upstream for session A; at least one session B routes elsewhere.
Validates deploy/join-style sticky routing before chat or long-poll scenarios depend on it.

---

## S3 — Params long-poll

**What we test:** Lane-C governance fields flow **mock-chain → mock-dapi →
GetRuntimeConfig long-poll** the way production devshardd consumes `NODE_MANAGER_ADDR`.

**How:**

1. Boot S1 stack; dial mock-dapi NodeManager gRPC.
2. Read baseline `GetRuntimeConfig` (`max_nonce`, `refusal_timeout`, `params_block_height`).
3. Start a **blocked long-poll** at the baseline height (`max_wait` ≈ 25s).
4. `POST /testenv/params` on mock-dapi (proxied to mock-chain) with patched
   `max_nonce`, `refusal_timeout`, `execution_timeout`.
5. Assert long-poll **wakes** with higher `params_block_height` and patched values.
6. Assert caught-up client gets `unchanged=true`; stale-height client still receives full snapshot.

**Pass criteria:** Long-poll unblocks within 20s after param patch; snapshot fields match
patch. Exercises `common/runtimeconfig` server + client path without production dapi.

---

## S4 — Epoch switch

**What we test:** Epoch transition on mock-chain (block fast-forward to `next_poc_start`,
roll `next_poc_start` forward) propagates into **GetRuntimeConfig** (`current_epoch_id` bump).

**How:**

1. Boot S1 stack; read mock-chain epoch snapshot (`epoch_index`, `next_poc_start`, block height).
2. Baseline `GetRuntimeConfig` — record `current_epoch_id` and `params_block_height`.
3. Blocked long-poll at baseline height.
4. `POST /testenv/epoch` `{advance: true}` on mock-dapi.
5. Assert long-poll wakes with **higher** `current_epoch_id` and `params_block_height`.
6. Re-read mock-chain snapshot — block height ≥ previous `next_poc_start`, `poc_start` moved,
   `next_poc_start` += `epoch_length`.

**Pass criteria:** Epoch index increments; chain block cursor catches up; long-poll clients
see new epoch. Covers CometBFT RPC face (3b) + params notification path used at epoch change.

---

## S5 — Gateway chat

**What we test:** Full **MVP+ chat path**: devshardctl creates/uses escrow, routes through
sticky versiond-router to devshardd, which calls mock-openai — **non-stream and SSE stream**.

**How:**

1. Boot S1 stack; wait gateway `/v1/status` and devshardd health via router
   `/{version}/healthz`.
2. `POST /v1/chat/completions` on devshardctl (pooled chat, `stream=false`) with test API key.
3. Assert HTTP 200 and mock-openai deterministic assistant content/role.
4. Repeat with `stream=true`; assemble SSE chunks; assert same mock-openai content shape.

On failure, dumps logs for `devshardctl`, `versiond-0`, `versiond-1`, `mock-openai`.

**Pass criteria:** Both stream modes return 200 with expected mock-openai payload. Requires
mock-chain LCD REST (3c) for gateway escrow tx and mock-dapi ML handoff.

---

## S6 — versiond stop (fault)

**What we test:** Behaviour when a **sticky upstream versiond is stopped** — nginx does not
silently proxy to a dead peer; sessions on a live upstream keep working.

**How:**

1. Boot S1 stack; `harness.FindDistinctStickySessions` — two session ids on **different**
   upstreams (`X-Upstream-Addr`).
2. `docker compose stop` the host mapped to session A's upstream.
3. Retry session A's router URL — expect **502/503** (peer down) **or** consistent-hash
   reroute to surviving upstream (ring shrink); not success via dead peer.
4. Session B (live upstream) must still return 200 with correct `X-Upstream-Addr`.

**Pass criteria:** Fault outcome classified (`FaultRouteFailed` or `FaultRouteRerouted`);
surviving session keeps working. Documents **no transparent failover** for pinned sessions
(matching production router semantics).

---

## Related tests (not S1–S6)

| Suite | Command | Scenarios |
|-------|---------|-----------|
| Adversarial | `make citest-adversarial` | A1–A4 (fault injection on mock-openai / mock-chain) |
| Observability | `make citest-observability` | O1 Jaeger + Loki + Prometheus smoke |
| Gateway smoke | `TESTENV_GATEWAY_SMOKE=1` | Phase 7 wiring without full citest tag |

See [`README.md`](../README.md) for adversarial and observability detail.

## Not yet implemented

| ID | Scenario | Notes |
|----|----------|-------|
| **S7** | Same-name sha swap | Phase 13 — rolling update |
| **S8** | Router host drain | Phase 13 — upstream evacuation |

Tracked in [`testenv-v2-plan.md`](./testenv-v2-plan.md) § Phase 13.
