# MCP GitHub Insights

A [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server written in Go
that exposes three tools backed by the live GitHub REST API, deployed as a microVM on
[Unikraft Cloud](https://unikraft.com).

Every tool call returns real data from real GitHub repositories — there are no mocks.

## Tools

| Tool             | Inputs                                     | Returns                                                           |
| ---------------- | ------------------------------------------ | ----------------------------------------------------------------- |
| `get_repo_stats` | `owner`, `repo`                            | stars, forks, open issues, default branch, last commit SHA + date |
| `list_open_prs`  | `owner`, `repo`, `min_age_days` (optional) | open PRs with number, title, author, age, review decision         |
| `search_code`    | `owner`, `repo`, `query`                   | matching file paths with line snippets                            |

## Requirements

- Go 1.25+
- Docker with BuildKit (for `unikraft build`)
- The [Unikraft CLI](https://unikraft.com/docs) (`curl --proto '=https' --tlsv1.2 -fsSL https://unikraft.com/cli/install.sh | sh`)
- A Unikraft Cloud account + `unikraft login`
- A GitHub fine-grained PAT (read-only: `Contents: read`, `Issues: read`, `Pull requests: read`)

## Run locally

```bash
cp .env.example .env      # then set GITHUB_TOKEN
export $(grep -v '^#' .env | xargs)
go run .
```

The server listens on `0.0.0.0:$PORT` (default `8080`) using the MCP **Streamable HTTP**
transport at the `/mcp` endpoint.

Quick smoke test (MCP requires an `initialize` handshake before other calls):

```bash
# 1. initialize — capture the Mcp-Session-Id response header
curl -si -X POST http://127.0.0.1:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'

# 2. list tools — pass the session id from step 1
curl -s -X POST http://127.0.0.1:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Mcp-Session-Id: <id-from-step-1>' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
```

## Deploy to Unikraft Cloud

```bash
unikraft login
unikraft build . --output <your-org>/mcp-github-insights:latest
unikraft run \
  --metro fra \
  --image <your-org>/mcp-github-insights:latest \
  -p 443:8080/http+tls \
  -e GITHUB_TOKEN=<your-github-token> \
  --scale-to-zero policy=on,cooldown-time=300 \
  --follow
```

The GitHub token is passed as a **runtime env var** (`-e`), never baked into the image
or committed to the Kraftfile. Note the assigned FQDN from the output.

## Connect an MCP client

Point VS Code Copilot Chat (or Claude Code) at the deployed server. Example
`.vscode/mcp.json`:

```json
{
 "servers": {
  "github-insights": {
   "type": "http",
   "url": "https://<your-fqdn>.fra.unikraft.app/mcp"
  }
 }
}
```

## Project layout

```
main.go              entrypoint: wires the MCP server + Streamable HTTP transport
tools/               the three tool implementations + shared helpers
github/client.go     authenticated go-github client (reads GITHUB_TOKEN)
Dockerfile           multi-stage build → FROM scratch (with CA certs)
Kraftfile            Unikraft Cloud image spec
```
