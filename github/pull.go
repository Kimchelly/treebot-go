package github

import (
	"context"
	"net/http"

	"github.com/google/go-github/v40/github"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	PRStateOpen   = "open"
	PRStateClosed = "closed"
)

func (c *Client) GetPRFromNotification(ctx context.Context, n github.Notification) (*github.PullRequest, error) {
	req, err := c.NewRequest(http.MethodGet, n.Subject.GetURL(), nil)
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

func (c *Client) UpdatePRFromNotification(ctx context.Context, n github.Notification) error {
	pr, err := c.GetPRFromNotification(ctx, n)
	if err != nil {
		return errors.Wrap(err, "getting PR metadata from notification")
	}

	owner := n.Repository.Owner.GetLogin()
	repo := n.Repository.GetName()
	prNum := pr.GetNumber()

	res, resp, err := c.PullRequests.UpdateBranch(ctx, owner, repo, prNum, nil)
	// GitHub returns 202 Accepted to indicate a background job will handle the
	// branch update, which manifests as an error even though the request was
	// successfully submitted.
	if err != nil && (resp == nil || github.CheckResponse(resp.Response) != nil) {
		return errors.Wrap(err, "updating branch")
	}

	zap.S().Debug("updated branch successfully",
		"message", res.GetMessage(),
		"url", res.GetURL(),
	)

	return nil
}
