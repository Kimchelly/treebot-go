package github

import (
	"context"
	"net/http"

	"github.com/google/go-github/v40/github"
	"github.com/pkg/errors"
)

func (c *Client) GetPRFromNotification(ctx context.Context, n github.Notification) (*github.PullRequest, error) {
	if n.Subject == nil || n.Subject.URL == nil {
		return nil, errors.New("missing required subject URL")
	}

	req, err := c.NewRequest(http.MethodGet, *n.Subject.URL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	var pr github.PullRequest
	resp, err := c.Do(ctx, req, &pr)
	if err != nil {
		return nil, errors.Wrap(err, "requesting PR information")
	}

	if err := github.CheckResponse(resp.Response); err != nil {
		return nil, errors.Wrap(err, "GitHub response")
	}

	return &pr, nil
}
