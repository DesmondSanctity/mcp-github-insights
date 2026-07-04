package ghclient

import (
	"context"
	"errors"
	"os"

	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

// ErrNoToken is returned when GITHUB_TOKEN is not set in the environment.
var ErrNoToken = errors.New("GITHUB_TOKEN is not set; provide a GitHub token via the GITHUB_TOKEN environment variable")

// New builds an authenticated go-github client from the GITHUB_TOKEN
// environment variable. The token is never logged.
func New(ctx context.Context) (*github.Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, ErrNoToken
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return github.NewClient(oauth2.NewClient(ctx, ts)), nil
}
