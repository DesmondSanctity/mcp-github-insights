package tools

import (
	"context"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/mark3labs/mcp-go/mcp"
)

type openPR struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	CreatedAt      string `json:"created_at"`
	AgeDays        int    `json:"age_days"`
	ReviewDecision string `json:"review_decision"`
}

func openPRsTool() mcp.Tool {
	return mcp.NewTool("list_open_prs",
		mcp.WithDescription("List open pull requests for a GitHub repository, optionally filtered by minimum age in days."),
		mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner (user or organization).")),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name.")),
		mcp.WithNumber("min_age_days", mcp.Description("Only include PRs at least this many days old. Defaults to 0.")),
	)
}

func openPRsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner, err := req.RequireString("owner")
	if err != nil {
		return mcp.NewToolResultError("owner is required"), nil
	}
	repo, err := req.RequireString("repo")
	if err != nil {
		return mcp.NewToolResultError("repo is required"), nil
	}
	minAge := req.GetInt("min_age_days", 0)

	client, err := newClient(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	prs, _, err := client.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return mcp.NewToolResultError(ghError("list open pull requests", err)), nil
	}

	out := make([]openPR, 0, len(prs))
	for _, pr := range prs {
		age := int(time.Since(pr.GetCreatedAt().Time).Hours() / 24)
		if age < minAge {
			continue
		}
		out = append(out, openPR{
			Number:         pr.GetNumber(),
			Title:          pr.GetTitle(),
			Author:         pr.GetUser().GetLogin(),
			CreatedAt:      pr.GetCreatedAt().Format(time.RFC3339),
			AgeDays:        age,
			ReviewDecision: reviewDecision(ctx, client, owner, repo, pr.GetNumber()),
		})
	}

	return jsonResult(out)
}

// reviewDecision derives an aggregate review state from a PR's reviews.
// Best-effort: on any error it returns "unknown" rather than failing the call.
func reviewDecision(ctx context.Context, client *github.Client, owner, repo string, number int) string {
	reviews, _, err := client.PullRequests.ListReviews(ctx, owner, repo, number, &github.ListOptions{PerPage: 100})
	if err != nil || len(reviews) == 0 {
		return "unknown"
	}
	latest := map[string]string{}
	for _, r := range reviews {
		switch r.GetState() {
		case "APPROVED", "CHANGES_REQUESTED":
			latest[r.GetUser().GetLogin()] = r.GetState()
		}
	}
	if len(latest) == 0 {
		return "pending"
	}
	for _, state := range latest {
		if state == "CHANGES_REQUESTED" {
			return "changes_requested"
		}
	}
	return "approved"
}
