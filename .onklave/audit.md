# Template audit

- **Last audited:** 2026-08-04
- **Audited by:** Onklave platform maintenance (automated, Claude Code)
- **Next review due:** 2026-11-04 (quarterly, or sooner on a dependency alert)

## Why this file exists
So we know when this template was last deliberately checked, and what was true at
the time. Apps are generated from this repo — stale or vulnerable dependencies
here propagate to every app created from it.

## Scope of this audit
- Build/test/vet correctness of the Go module, run against the same toolchain the
  Dockerfile builds with.
- Module dependency currency (`go list -m -u all`) and known-vulnerability scan
  (`govulncheck`).
- The `go` directive vs Dockerfile builder drift (`go 1.22` vs `golang:1.26-alpine`),
  including what that drift actually changes in the produced binary.
- Dockerfile security posture: base image currency and pinning, non-root user,
  read-only root filesystem compatibility, image size/contents.
- Repo hygiene: secrets in the working tree *and* in full git history,
  `.dockerignore` / `.gitignore` correctness.
- HTTP server hardening: request/response/idle timeouts (slowloris exposure),
  verified empirically against a running container, before and after the fix.

Not in scope: application logic beyond the two sample handlers, TLS termination
(handled by Onklave ingress, not this service), and load/performance testing.

## Verification run
No Go toolchain is installed on the audit host, so every Go command was run inside
`golang:1.26-alpine` — the exact image the Dockerfile builds with — via
`docker run --rm -v "$PWD":/src -w /src`. Toolchain resolved to **go1.26.5**.

| Check | Command | Result |
|---|---|---|
| Formatting | `gofmt -l .` | Pass — no files listed |
| Vet | `go vet ./...` | Pass — exit 0, no diagnostics |
| Tests | `go test ./...` | Pass — `internal/server` ok (3 tests); `cmd/server` no test files |
| Tests (race) | `CGO_ENABLED=1 go test -race ./...` | Pass — exit 0 (required `apk add gcc musl-dev`; alpine image has no C toolchain by default) |
| Build | `go build ./...` | Pass — exit 0 |
| Dependency updates | `go list -m -u all` | Only the main module — **zero third-party dependencies**, nothing to update |
| Vulnerabilities | `govulncheck ./...` | **No vulnerabilities found** — govulncheck v1.6.0, vuln DB updated 2026-07-27 |
| Builder image currency | `docker manifest inspect golang:1.2x-alpine` | `1.26-alpine` is the newest published tag (`1.27-alpine` does not exist yet) |
| Base image currency | `docker pull gcr.io/distroless/static:nonroot` | Pulled successfully, tag current, runs as uid **65532** |
| Image build | `docker build -t tgws:audited .` | Pass — built in ~5s, final image **14.4 MB** |
| Runtime (hardened) | `docker run --read-only --cap-drop=ALL --security-opt=no-new-privileges` | Pass — container healthy under a read-only rootfs with all capabilities dropped |
| Endpoints | `curl /healthz`, `curl /`, `curl /nope` | `200 {"status":"ok"}`, `200` JSON greeting, `404` — all as documented |
| Graceful shutdown | `docker stop -t 15` (SIGTERM) | Pass — logged `server stopped cleanly`, exit code **0** |
| Slowloris probe (before) | idle keep-alive hold + trickled request body | **FAIL** — idle conn still open after 25s; trickled body accepted for 24s |
| Slowloris probe (after) | same probe against the fixed build | **Pass** — idle conn closed at ~60s; trickled body connection dropped at the 15s read deadline |
| GODEBUG defaults (after) | `go version -m <binary>` | No `DefaultGODEBUG` line — clean Go 1.26 defaults (see Finding 1) |

## Dependency status
| Item | Before | After | Note |
|---|---|---|---|
| Third-party Go modules | none | none | Stdlib only; no `go.sum` exists. Nothing to upgrade — this is the template's main strength from a supply-chain view. |
| `go` directive (`go.mod`) | `go 1.22` | `go 1.26` | Raised to reconcile the drift with the Dockerfile builder. See Finding 1. |
| Builder image | `golang:1.26-alpine` | `golang:1.26-alpine` | **Already current** — resolves to go1.26.5; Go 1.27 is not released. Left unchanged. |
| Runtime base | `gcr.io/distroless/static:nonroot` | `gcr.io/distroless/static:nonroot` | **Already current.** Correct choice: no shell, no package manager, non-root by default. Left unchanged. |

Deliberately **not** changed:
- Base images were not pinned to a digest — see Finding 3, this needs a human
  decision about maintenance cadence now that CI has been removed.
- No dependencies were added (no logging, router or config library). The stdlib
  covers everything this template does, and every dependency added here is
  inherited by every generated app.

## Findings

1. **HIGH — `go 1.22` directive silently shipped Go-1.22-era security defaults.**
   `go.mod` declared `go 1.22` while the Dockerfile built with Go 1.26. This is not
   cosmetic: the `go` directive sets the GODEBUG compatibility baseline, so the
   1.26 toolchain baked a long list of *opt-out-of-hardening* defaults into the
   binary. Confirmed by diffing `go version -m` between a `go 1.22` and a `go 1.26`
   build — the 1.22 build carried `DefaultGODEBUG=...` including:
   - `tls3des=1`, `tlssha1=1` — 3DES cipher suites and SHA-1 signatures re-enabled in TLS.
   - `tlsmlkem=0`, `tlssecpmlkem=0` — post-quantum hybrid key exchange (X25519MLKEM768) disabled.
   - `rsa1024min=0` — the 1024-bit minimum RSA key size not enforced.
   - `x509negativeserial=1`, `x509usepolicies=0`, `x509rsacrt=0`, `x509sha256skid=0` — assorted X.509 verification hardening disabled.
   - `urlmaxqueryparams=0`, `httpcookiemaxnum=0` — the DoS caps on query-parameter and cookie counts removed. **These apply directly to this HTTP server.**
   - `containermaxprocs=0`, `updatemaxprocs=0` — cgroup-aware `GOMAXPROCS` disabled, so a pod with a fractional CPU limit would size its scheduler to the whole node's core count. A reliability/latency issue in Kubernetes, not just a security one.

   The TLS/X.509 items do not affect this template as shipped (Onklave terminates
   TLS at the ingress), but they are inherited by any generated app that makes
   outbound HTTPS calls or adds its own TLS.
   **Action taken:** raised the directive to `go 1.26`. The rebuilt binary carries
   **no** `DefaultGODEBUG` line at all — clean 1.26 defaults. Vet/test/build/govulncheck
   all still pass.

2. **MEDIUM — `http.Server` had no read, write or idle timeout (slowloris exposure).**
   Only `ReadHeaderTimeout: 5s` was set. A zero-value timeout in `net/http` means
   *no deadline*, so the remaining phases were unbounded. Demonstrated against the
   running container: an idle keep-alive connection was still open after 25s, and a
   request body trickled at 1 byte / 2s was accepted for 24s without being dropped.
   Both let a small number of clients pin connections indefinitely.
   **Action taken:** added `ReadTimeout: 15s`, `WriteTimeout: 30s`,
   `IdleTimeout: 60s` alongside the existing `ReadHeaderTimeout`, each with a
   comment explaining what it bounds and when to change it. Re-ran the same probes:
   the idle connection is now closed at ~60s and the trickled body is dropped at the
   read deadline. `MaxHeaderBytes` was left at its default (1 MB), which is adequate.

3. **LOW/MEDIUM — base images use floating tags, not digests.**
   `golang:1.26-alpine` and `gcr.io/distroless/static:nonroot` are both mutable
   tags. Floating tags pick up patch fixes automatically (good, and the reason they
   were probably chosen), but they make builds non-reproducible and mean a
   compromised or regressed upstream tag is consumed silently. The usual mitigation
   — pin the digest, let a bot bump it — has no owner here, because this repo's
   GitHub Actions CI was deliberately removed in favour of Onklave's in-cluster
   build.
   **Action:** not changed; recorded as an open item for a human decision. Pinning
   digests in a *template* has a real cost: every generated app would start life
   with a digest that goes stale and nothing to bump it.

4. **LOW — local env files were ignored by neither git nor Docker.**
   Neither `.gitignore` nor `.dockerignore` excluded `.env`. The build stage does
   `COPY . .`, so a developer's `.env` would be readable in that layer and in the
   build cache (it would not reach the final image, since only `/out/server` is
   copied to the distroless stage — so this was exposure, not a shipped leak).
   **Action taken:** added `.env` / `.env.*` to both files.

5. **NONE — no secrets committed.** Pattern-scanned the working tree and every blob
   across all reachable commits (`git grep` over `git rev-list --all`) for private
   keys, AWS/GitHub/Slack token formats and `key/secret/password/token` assignments.
   Clean. Only 10 files have ever existed in this repo's history.

6. **INFO — read-only root filesystem is compatible.** Verified, not assumed: the
   service writes nothing to disk and logs JSON to stdout. It runs correctly with
   `--read-only`, `--cap-drop=ALL` and `--security-opt=no-new-privileges`, so
   generated apps can safely set `readOnlyRootFilesystem: true`,
   `allowPrivilegeEscalation: false` and drop all capabilities in their pod spec.
   Non-root is confirmed at uid 65532 via the distroless `nonroot` base plus an
   explicit `USER nonroot:nonroot`.

7. **INFO — `.dockerignore` still lists `.github`,** which no longer exists (CI was
   removed in favour of `onklave.yaml`). Harmless and arguably useful if a consumer
   re-adds workflows. Left as-is.

8. **INFO — no panic-recovery middleware.** A panic in a handler is recovered
   per-connection by `net/http` and does not crash the process, so this is not a
   availability bug. Left out deliberately: the template favours minimal, obvious
   stdlib code, and recovery semantics are an application-level choice.

## Changes made in this audit
- `go.mod`: `go 1.22` → `go 1.26`, reconciling the directive with the Dockerfile
  builder and clearing the inherited GODEBUG hardening opt-outs (Finding 1).
- `cmd/server/main.go`: added `ReadTimeout`, `WriteTimeout` and `IdleTimeout` to
  the `http.Server`, with comments explaining each bound (Finding 2).
- `.gitignore`: ignore `.env` / `.env.*` (Finding 4).
- `.dockerignore`: exclude `.env` / `.env.*` from the build context, and `.onklave`
  (audit metadata, not needed to build) (Finding 4).
- `README.md`: documented the Go 1.26+ requirement and the `GOTOOLCHAIN=auto`
  fallback, since raising the directive is consumer-visible.
- Added this file.

`onklave.yaml` was deliberately **not** touched — it was written and verified in a
previous pass.

## Open items
1. **Decide the base-image pinning policy for templates** (Finding 3). Options:
   keep floating tags for automatic patch uptake, or pin digests and give Onklave a
   mechanism to bump them across all templates. This is a fleet-wide policy call,
   not a per-template one, and it interacts with the removal of per-repo CI.
2. **Decide how these templates get re-verified without per-repo CI.** This audit
   was run by hand. Nothing currently re-runs `govulncheck` or notices that
   `golang:1.27-alpine` has shipped. A scheduled Onklave agent run across the
   template fleet would be the natural owner, and would keep this file honest.
3. **Confirm `go 1.26` is acceptable for consumers with `GOTOOLCHAIN=local`.**
   With the default `GOTOOLCHAIN=auto` any Go ≥1.21 self-heals by downloading the
   right toolchain, so this is a non-issue for almost everyone. In a locked-down
   environment that pins `GOTOOLCHAIN=local` with an older Go, the build would now
   fail until the toolchain is updated.
4. **Revisit `WriteTimeout: 30s` if a generated app streams responses** (SSE, long
   polling, large downloads). `WriteTimeout` bounds the entire response write, so
   streaming handlers must raise it or clear it for those routes. Called out in a
   code comment, but worth repeating to anyone building on this template.
