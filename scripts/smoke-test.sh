#!/usr/bin/env bash
# Local real-data smoke test for the MCP GitHub Insights server.
# Loads GITHUB_TOKEN from .env, starts the server, and exercises all 3 tools
# plus an error path. Never prints the token.
set -euo pipefail

URL="http://127.0.0.1:8080/mcp"
HDRS=(-H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream')

# Load .env without echoing
set -a; source ./.env; set +a

go build -o /tmp/mcp-server .
/tmp/mcp-server >/tmp/mcp.log 2>&1 &
SRV=$!
trap 'kill $SRV 2>/dev/null || true' EXIT
sleep 1

# initialize -> capture session id
curl -s -D /tmp/hdr.txt "${HDRS[@]}" -X POST "$URL" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}' >/dev/null
SID=$(grep -i 'mcp-session-id' /tmp/hdr.txt | awk '{print $2}' | tr -d '\r')
SIDH=(-H "Mcp-Session-Id: $SID")
# notifications/initialized
curl -s "${HDRS[@]}" "${SIDH[@]}" -X POST "$URL" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' >/dev/null

call() { # name, arguments-json
  echo "===== $1 ====="
  curl -s "${HDRS[@]}" "${SIDH[@]}" -X POST "$URL" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" \
  | sed -e 's/^data: //' | python3 -c 'import sys,json
for line in sys.stdin:
    line=line.strip()
    if not line: continue
    try:
        d=json.loads(line)
    except Exception:
        continue
    r=d.get("result",{})
    for c in r.get("content",[]):
        print(c.get("text",""))
    if r.get("isError"): print("(isError=true)")'
  echo
}

call get_repo_stats '{"owner":"mark3labs","repo":"mcp-go"}'
call get_repo_stats '{"owner":"unikraft","repo":"unikraft"}'
call list_open_prs  '{"owner":"unikraft","repo":"kraftkit","min_age_days":0}'
call list_open_prs  '{"owner":"unikraft","repo":"kraftkit","min_age_days":180}'
call search_code    '{"owner":"mark3labs","repo":"mcp-go","query":"NewStreamableHTTPServer"}'
call get_repo_stats '{"owner":"unikraft","repo":"this-repo-does-not-exist-xyz"}'
