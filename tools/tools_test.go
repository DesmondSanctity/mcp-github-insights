package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockGitHub spins up an httptest server and points newClient at it for the
// duration of a test. It restores the original newClient on cleanup.
func mockGitHub(t *testing.T, handler http.Handler) {
	t.Helper()
	srv := httptest.NewServer(handler)
	base, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	orig := newClient
	newClient = func(_ context.Context) (*github.Client, error) {
		c := github.NewClient(srv.Client())
		c.BaseURL = base
		return c, nil
	}
	t.Cleanup(func() {
		newClient = orig
		srv.Close()
	})
}

func callRequest(name string, args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// resultText returns the concatenated text content and whether the result is an error.
func resultText(t *testing.T, res *mcp.CallToolResult) (string, bool) {
	t.Helper()
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

func TestGetRepoStats(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/octocat/hello", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"stargazers_count": 42, "forks_count": 7, "open_issues_count": 3,
			"default_branch": "main",
		})
	})
	mux.HandleFunc("/repos/octocat/hello/commits", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"sha": "abc123", "commit": map[string]any{"committer": map[string]any{"date": "2026-01-02T03:04:05Z"}}},
		})
	})
	mockGitHub(t, mux)

	res, err := repoStatsHandler(context.Background(), callRequest("get_repo_stats", map[string]any{"owner": "octocat", "repo": "hello"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text, isErr := resultText(t, res)
	if isErr {
		t.Fatalf("unexpected error result: %s", text)
	}
	var got repoStats
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, text)
	}
	if got.Stars != 42 || got.Forks != 7 || got.OpenIssues != 3 || got.DefaultBranch != "main" {
		t.Errorf("unexpected stats: %+v", got)
	}
	if got.LastCommitSHA != "abc123" {
		t.Errorf("last commit sha = %q, want abc123", got.LastCommitSHA)
	}
}

func TestGetRepoStatsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/octocat/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"message": "Not Found"})
	})
	mockGitHub(t, mux)

	res, err := repoStatsHandler(context.Background(), callRequest("get_repo_stats", map[string]any{"owner": "octocat", "repo": "missing"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text, isErr := resultText(t, res)
	if !isErr {
		t.Fatalf("expected error result, got: %s", text)
	}
	if !strings.Contains(text, "not found") {
		t.Errorf("error text = %q, want it to mention 'not found'", text)
	}
}

func TestGetRepoStatsMissingArg(t *testing.T) {
	res, err := repoStatsHandler(context.Background(), callRequest("get_repo_stats", map[string]any{"owner": "octocat"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text, isErr := resultText(t, res)
	if !isErr || !strings.Contains(text, "repo is required") {
		t.Errorf("want 'repo is required' error, got isErr=%v text=%q", isErr, text)
	}
}

func TestListOpenPRsFilters(t *testing.T) {
	// One PR ~1000 days old, one ~1 day old.
	old := "2023-01-01T00:00:00Z"
	recent := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/octocat/hello/pulls", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"number": 1, "title": "old", "user": map[string]any{"login": "a"}, "created_at": old},
			{"number": 2, "title": "new", "user": map[string]any{"login": "b"}, "created_at": recent},
		})
	})
	// No reviews -> review_decision "unknown".
	mux.HandleFunc("/repos/octocat/hello/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("[]")) })
	mux.HandleFunc("/repos/octocat/hello/pulls/2/reviews", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("[]")) })
	mockGitHub(t, mux)

	// min_age_days=100 should drop the recent PR, keeping only #1.
	res, err := openPRsHandler(context.Background(), callRequest("list_open_prs", map[string]any{"owner": "octocat", "repo": "hello", "min_age_days": float64(100)}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text, isErr := resultText(t, res)
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	var prs []openPR
	if err := json.Unmarshal([]byte(text), &prs); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, text)
	}
	if len(prs) != 1 || prs[0].Number != 1 {
		t.Fatalf("expected only PR #1 after filtering, got %+v", prs)
	}
	if prs[0].AgeDays < 100 {
		t.Errorf("age_days = %d, want >= 100", prs[0].AgeDays)
	}
}

func TestSearchCodeSnippetFallback(t *testing.T) {
	mux := http.NewServeMux()
	// Search returns a hit with NO text-match fragment.
	mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"items":       []map[string]any{{"path": "main.go"}},
		})
	})
	// Content fetch used by the fallback.
	fileBody := "package main\n\nfunc TargetSymbol() {}\n"
	mux.HandleFunc("/repos/octocat/hello/contents/main.go", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"type": "file", "encoding": "base64", "path": "main.go",
			"content": base64.StdEncoding.EncodeToString([]byte(fileBody)),
		})
	})
	mockGitHub(t, mux)

	res, err := searchCodeHandler(context.Background(), callRequest("search_code", map[string]any{"owner": "octocat", "repo": "hello", "query": "TargetSymbol"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text, isErr := resultText(t, res)
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	var matches []codeMatch
	if err := json.Unmarshal([]byte(text), &matches); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, text)
	}
	if len(matches) != 1 || matches[0].Path != "main.go" {
		t.Fatalf("unexpected matches: %+v", matches)
	}
	if matches[0].LineSnippet != "func TargetSymbol() {}" {
		t.Errorf("snippet = %q, want the matched line via fallback", matches[0].LineSnippet)
	}
}

func TestGhError(t *testing.T) {
	notFound := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusNotFound},
		Message:  "Not Found",
	}
	if got := ghError("fetch", notFound); !strings.Contains(got, "not found") {
		t.Errorf("404 -> %q", got)
	}

	forbidden := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusForbidden},
		Message:  "Forbidden",
	}
	if got := ghError("fetch", forbidden); !strings.Contains(got, "forbidden") {
		t.Errorf("403 -> %q", got)
	}

	generic := errors.New("boom")
	if got := ghError("do thing", generic); !strings.Contains(got, "boom") {
		t.Errorf("generic -> %q", got)
	}
}
