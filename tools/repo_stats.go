package tools

import (
	"context"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/mark3labs/mcp-go/mcp"
)

type repoStats struct {
	Stars          int    `json:"stars"`
	Forks          int    `json:"forks"`
	OpenIssues     int    `json:"open_issues"`
	DefaultBranch  string `json:"default_branch"`
	LastCommitSHA  string `json:"last_commit_sha"`
	LastCommitDate string `json:"last_commit_date"`
}

func repoStatsTool() mcp.Tool {
	return mcp.NewTool("get_repo_stats",
		mcp.WithDescription("Get star, fork and open-issue counts plus the last commit for a GitHub repository."),
		mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner (user or organization).")),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name.")),
	)
}

func repoStatsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner, err := req.RequireString("owner")
	if err != nil {
		return mcp.NewToolResultError("owner is required"), nil
	}
	repo, err := req.RequireString("repo")
	if err != nil {
		return mcp.NewToolResultError("repo is required"), nil
	}

	client, err := newClient(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	r, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return mcp.NewToolResultError(ghError("fetch repository", err)), nil
	}

	out := repoStats{
		Stars:         r.GetStargazersCount(),
		Forks:         r.GetForksCount(),
		OpenIssues:    r.GetOpenIssuesCount(),
		DefaultBranch: r.GetDefaultBranch(),
	}

	commits, _, err := client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return mcp.NewToolResultError(ghError("fetch last commit", err)), nil
	}
	if len(commits) > 0 {
		out.LastCommitSHA = commits[0].GetSHA()
		out.LastCommitDate = commits[0].GetCommit().GetCommitter().GetDate().Format(time.RFC3339)
	}

	return jsonResult(out)
}
