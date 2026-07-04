# Evidence Log — MCP GitHub Insights on Unikraft Cloud

Real, captured output backing the article. Nothing here is paraphrased.

- **Date:** 2026-07-04
- **Metro:** `fra` (Frankfurt)
- **Account/profile:** `anonx`
- **Image:** `anonx/mcp-github-insights:latest`
- **Deployed FQDN:** `https://green-night-htkc4jbs.fra.unikraft.app/mcp`
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

```
timing.boot-time: 32.504ms
timing.net-time:  41.073ms
state:            running
```

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

**3. Fix — dynamic PIE + an alpine rootfs that provides the musl loader**

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildmode=pie -ldflags "-s -w" -o /mcp-server .

FROM --platform=linux/amd64 alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /mcp-server /mcp-server
ENTRYPOINT ["/mcp-server"]
```

Result — clean boot:

```
Powered by Unikraft Ijiraq (0.21.0~a653e50)
2026/07/04 22:17:21 MCP GitHub Insights server listening on 0.0.0.0:8080 (endpoint: /mcp)
```

Also note: the metro is `x86_64`, so `GOARCH=amd64` is required — the arm64 dev
machine would otherwise cross-compile the wrong architecture.

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
