package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	ghclient "mcp-github-insights/github"
)

// newClient builds the authenticated GitHub client. It is a package variable so
// tests can inject a client pointed at a mock API.
var newClient = ghclient.New

// Register wires all GitHub Insights tools onto the MCP server.
func Register(s *server.MCPServer) {
	s.AddTool(repoStatsTool(), repoStatsHandler)
	s.AddTool(openPRsTool(), openPRsHandler)
	s.AddTool(searchCodeTool(), searchCodeHandler)
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("failed to encode result: " + err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// ghError converts a go-github error into a clear, structured message covering
// rate limits, missing resources, and auth failures.
func ghError(action string, err error) string {
	var rle *github.RateLimitError
	if errors.As(err, &rle) {
		return fmt.Sprintf("could not %s: GitHub API rate limit exceeded (resets at %s)", action, rle.Rate.Reset.Format(time.RFC3339))
	}
	var arle *github.AbuseRateLimitError
	if errors.As(err, &arle) {
		return fmt.Sprintf("could not %s: GitHub secondary (abuse) rate limit hit, retry later", action)
	}
	var er *github.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		switch er.Response.StatusCode {
		case 404:
			return fmt.Sprintf("could not %s: repository or resource not found", action)
		case 401, 403:
			return fmt.Sprintf("could not %s: authentication failed or access forbidden (check GITHUB_TOKEN scopes)", action)
		}
		return fmt.Sprintf("could not %s: %s", action, er.Message)
	}
	return fmt.Sprintf("could not %s: %s", action, err.Error())
}
