# Evidence Log — MCP GitHub Insights on Unikraft Cloud

Real, captured output backing the article. Nothing here is paraphrased.

- **Date:** 2026-07-04
- **Metro:** `fra` (Frankfurt)
- **Account/profile:** `anonx`
- **Image:** `anonx/mcp-github-insights:latest`
- **Deployed FQDN:** `https://old-meadow-lmmoqtgy.fra.unikraft.app/mcp`
- **GitHub repo:** https://github.com/DesmondSanctity/mcp-github-insights

## Toolchain versions

- Go 1.24.1 → toolchain auto-upgraded to **1.25.5** (required by mcp-go)
- Docker 27.5.1, buildx 0.20.1
- Unikraft CLI **0.2.3** (darwin/arm64)

## Resolved dependency versions (`go get`)

```
github.com/mark3labs/mcp-go v0.55.1   (requires go >= 1.25.5)
github.com/google/go-github/v68 v68.0.0
golang.org/x/oauth2 v0.36.0
```

## Cold boot time (deployed instance)

Two Dockerfile variants were built and measured head-to-head on `fra`. The
static-PIE `scratch` build (now the canonical Dockerfile) boots fastest:

| Variant | rootfs | `boot-time` | `net-time` |
| --- | --- | --- | --- |
| **static-PIE (adopted)** | `FROM scratch` | **23.9ms** | 32.3ms |
| dynamic-PIE (initial fix) | `alpine:3.20` | 32.4ms | 41.1ms |

Neither reaches Unikraft's headline **<10ms** — that figure refers to stateful
snapshot/restore of an already-booted VM, not a cold boot of a fresh app. For a
Go binary, the Go runtime's own init sets a floor around ~24ms on cold boot.
Still, ~24ms is 10–40× faster than a typical container cold start (hundreds of ms
to seconds).

## Scale-to-zero → cold resume

After sitting idle past the 300s cooldown, the instance scaled to zero on its own:

```
state:   standby
reason:  platform stop
```

Hitting the FQDN then woke it and served the request. The platform-reported boot
on resume is essentially identical to a fresh boot:

```
state:      running
boot-time:  32.644ms
net-time:   41.315ms
```

Client-side round trip for the first (cold) request, from a machine outside the
Frankfurt metro:

```
http=200 total=0.512s  connect(TLS)=0.143s  ttfb=0.511s
```

The ~0.5s wall-clock is dominated by TLS + network latency to `fra`, not by the
VM boot — the actual resume is ~33ms. This is the "MCP tools don't need to stay
warm" argument: idle to zero cost, and a cold tool call still returns in one
round trip.

## Stateful snapshot/restore vs non-stateful cold boot (chasing <10ms)

Unikraft's `--scale-to-zero` supports a `stateful` flag. We measured both to see
which actually gets closer to the <10ms headline. Two metrics matter and they
tell **different stories**:

- `boot-time` = the platform-reported VM resume/boot time (CPU-side)
- wall-clock = the real end-to-end wake latency a client experiences

| Mode | reported `boot-time` | real wall-clock wake |
| --- | --- | --- |
| non-stateful (cold boot) | ~24ms | **~0.5s** |
| stateful (snapshot restore), run 1 | 14.4ms | ~4.6s |
| stateful (snapshot restore), run 2 | 15.3ms | ~7.6s |

**Takeaway:** stateful restore lowers the *reported* boot-time toward ~15ms (still
not <10ms for this workload), but the *real* wake is several seconds because the
128MiB memory snapshot has to be loaded from storage. For a tiny, fast-booting Go
server, **non-stateful cold boot wins end-to-end** (~0.5s vs several seconds).

Stateful snapshot/restore pays off for the opposite kind of workload — large
memory footprints or slow initialization (databases with warm caches, loaded ML
models, heavy runtimes) where re-initializing from scratch would cost far more
than loading a snapshot. Our MCP server is the wrong shape to benefit, so the
canonical deployment stays **non-stateful**.

## Real MCP client verification

Connected the deployed server to **VS Code Copilot Chat** via `.vscode/mcp.json`
(`type: http`, the deployed FQDN + `/mcp`). All three tools were invoked live
from the chat client (not curl) and returned real data:

- `get_repo_stats` → `unikraft/unikraft` (3762★)
- `list_open_prs` → `unikraft/kraftkit`, `min_age_days=90` → 2 aged PRs (#1028, #528)
- `search_code` → `mark3labs/mcp-go` for `NewStreamableHTTPServer` → 19 matches incl. the definition in `server/streamable_http.go`

Full path exercised: MCP client → TLS to the microVM on `fra` → live GitHub API → back.

## Live tool calls against the deployed FQDN

`get_repo_stats` for `unikraft/unikraft`:

```json
{
 "stars": 3762,
 "forks": 1467,
 "open_issues": 348,
 "default_branch": "staging",
 "last_commit_sha": "be744898b6947824e367e01765703401e08ce3c5",
 "last_commit_date": "2026-06-05T11:42:40Z"
}
```

`list_open_prs` for `unikraft/kraftkit` (local run) — `min_age_days` filtering
demonstrably changes the result count: **6 PRs** with `min_age_days=0`, **2 PRs**
with `min_age_days=180`.

`search_code` for `NewStreamableHTTPServer` in `mark3labs/mcp-go` returned real
matching file paths (e.g. `server/streamable_http.go`, `examples/*/main.go`).

Error path — `get_repo_stats` for a nonexistent repo:

```
could not fetch repository: repository or resource not found   (isError=true)
```

## Troubleshooting: getting a Go binary to boot on Unikraft (the useful part)

Three real failures, in order, before the server booted:

**1. `FROM scratch` + static non-PIE binary → loader rejects it**

```
ERR: [appelfloader] mcp-server: ELF executable is not position-independent!
ERR: [appelfloader] mcp-server: Parsing of ELF image failed: Exec format error (-8)
Application exit with 0x0
```

Unikraft's `base` runtime ELF loader requires a **position-independent
executable (PIE)**. A plain `CGO_ENABLED=0 go build` produces a non-PIE binary.

**2. `-buildmode=pie` on scratch → missing dynamic loader**

```
ERR: [appelfloader] <interp>: Failed to load /lib/ld-musl-x86_64.so.1: No such file
ERR: [appelfloader] mcp-server: Failed to load program interpreter /lib/ld-musl-x86_64.so.1
Application exit with 0xffffffff
```

Go emits a `PT_INTERP` header for `-buildmode=pie` **even with internal linking**
(confirmed: the ELF is "dynamically linked, interpreter …" despite being pure
Go). On `FROM scratch` there is no loader for it to point at.

**3. First working fix — dynamic PIE + an alpine rootfs that provides the musl loader**

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildmode=pie -ldflags "-s -w" -o /mcp-server .

FROM --platform=linux/amd64 alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /mcp-server /mcp-server
ENTRYPOINT ["/mcp-server"]
```

This booted cleanly (boot-time ~32ms), but ships a full alpine userspace.

**4. Adopted build — truly static PIE on `scratch` (smaller + faster)**

External linking with `-static-pie` produces a static PIE with **no** `PT_INTERP`,
so `FROM scratch` works with no loader. This needs a C toolchain (CGO) building
for amd64, plus `-tags netgo` to avoid libc for DNS:

```dockerfile
FROM --platform=linux/amd64 golang:1.25-bookworm AS build
...
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
      -buildmode=pie \
      -ldflags "-linkmode external -extldflags -static-pie -s -w" \
      -tags netgo \
      -o /mcp-server .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /mcp-server /mcp-server
ENTRYPOINT ["/mcp-server"]
```

Result — clean boot, ~24ms (vs ~32ms for alpine):

```
Powered by Unikraft Ijiraq (0.21.0~a653e50)
2026/07/05 00:18:59 MCP GitHub Insights server listening on 0.0.0.0:8080 (endpoint: /mcp)
```

Trade-off: the CGO/static-pie build must target amd64, so on an arm64 dev machine
it runs under emulation (slower build) — but the metro needs `GOARCH=amd64`
regardless.

## Final Kraftfile

```yaml
spec: v0.6
runtime: base:latest
rootfs: ./Dockerfile
cmd: ['/mcp-server']
env:
 PORT: '8080'
```

`GITHUB_TOKEN` is passed at deploy time via `unikraft run -e GITHUB_TOKEN=...`,
never committed. The CLI masks it as `*` in the instance summary output.
