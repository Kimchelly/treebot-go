package github

import (
	"context"
	"fmt"
	"net/http"
	"os"

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

func (c *Client) UpdatePRFromNotification(ctx context.Context, n github.Notification) error {
	pr, err := c.GetPRFromNotification(ctx, n)
	if err != nil {
		return errors.Wrap(err, "getting PR metadata from notification")
	}

	if n.Repository == nil || n.Repository.Owner == nil || n.Repository.Owner.Name == nil || n.Repository.Name == nil {
		return errors.New("missing required repository information")
	}

	owner := *n.Repository.Owner.Name
	repo := *n.Repository.Name
	prNum := *pr.Number

	res, resp, err := c.PullRequests.UpdateBranch(ctx, owner, repo, prNum, nil)
	if err != nil {
		return errors.Wrap(err, "updating branch")
	}

	if err := github.CheckResponse(resp.Response); err != nil {
		return errors.Wrap(err, "GitHub response")
	}

	fmt.Fprintf(os.Stdout, "%s (%s)\n", *res.Message, *res.URL)

	return nil
}
