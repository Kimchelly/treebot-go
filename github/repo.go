package github

import (
	"context"
	"net/http"

	"github.com/google/go-github/v40/github"
	"github.com/pkg/errors"
)

type CommitStatusOptions struct {
	Owner string
	Repo  string
	Ref   string
}

func (c *Client) GetCommitStatusesFromNotification(ctx context.Context, n github.Notification) ([]github.RepoStatus, error) {
	pr, err := c.GetPRFromNotification(ctx, n)
	if err != nil {
		return nil, errors.Wrap(err, "getting PR metadata from notification")
	}

	if pr.StatusesURL == nil {
		return nil, errors.Wrap(err, "missing URL to get PR statuses")
	}

	req, err := c.NewRequest(http.MethodGet, *pr.StatusesURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	statuses := []github.RepoStatus{}
	resp, err := c.Do(ctx, req, &statuses)
	if err != nil {
		return nil, errors.Wrap(err, "requesting repository status information")
	}

	if err := github.CheckResponse(resp.Response); err != nil {
		return nil, errors.Wrap(err, "GitHub response")
	}

	return statuses, nil
}
