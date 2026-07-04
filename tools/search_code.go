package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v68/github"
	"github.com/mark3labs/mcp-go/mcp"
)

type codeMatch struct {
	Path        string `json:"path"`
	LineSnippet string `json:"line_snippet"`
}

func searchCodeTool() mcp.Tool {
	return mcp.NewTool("search_code",
		mcp.WithDescription("Search code within a single GitHub repository and return matching file paths with snippets."),
		mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner (user or organization).")),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name.")),
		mcp.WithString("query", mcp.Required(), mcp.Description("Code search query.")),
	)
}

func searchCodeHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner, err := req.RequireString("owner")
	if err != nil {
		return mcp.NewToolResultError("owner is required"), nil
	}
	repo, err := req.RequireString("repo")
	if err != nil {
		return mcp.NewToolResultError("repo is required"), nil
	}
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	client, err := newClient(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	q := fmt.Sprintf("%s repo:%s/%s", query, owner, repo)
	res, _, err := client.Search.Code(ctx, q, &github.SearchOptions{
		TextMatch:   true,
		ListOptions: github.ListOptions{PerPage: 30},
	})
	if err != nil {
		// Code search has a stricter rate-limit tier; ghError reports it cleanly.
		return mcp.NewToolResultError(ghError("search code", err)), nil
	}

	out := make([]codeMatch, 0, len(res.CodeResults))
	budget := 10 // cap fallback content fetches to bound latency/rate-limit cost
	for _, cr := range res.CodeResults {
		snippet := ""
		if len(cr.TextMatches) > 0 {
			snippet = cr.TextMatches[0].GetFragment()
		}
		if snippet == "" && budget > 0 {
			budget--
			snippet = matchedLine(ctx, client, owner, repo, cr.GetPath(), query)
		}
		out = append(out, codeMatch{Path: cr.GetPath(), LineSnippet: snippet})
	}

	return jsonResult(out)
}

// matchedLine fetches a file and returns the first line containing the query,
// used as a fallback when GitHub's code search omits the text-match fragment.
func matchedLine(ctx context.Context, client *github.Client, owner, repo, path, query string) string {
	file, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil || file == nil {
		return ""
	}
	content, err := file.GetContent()
	if err != nil || content == "" {
		return ""
	}
	needle := strings.ToLower(query)
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(strings.ToLower(line), needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
