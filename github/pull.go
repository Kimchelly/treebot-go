package github

import (
	"context"
	"fmt"
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
	if err != nil && (resp == nil || !errors.Is(github.CheckResponse(resp.Response), &github.AcceptedError{})) {
		return errors.Wrap(err, "updating branch")
	}

	zap.S().Debugw("updated PR successfully",
		"message", res.GetMessage(),
		"url", res.GetURL(),
	)

	return nil
}

func (c *Client) MergePRFromNotification(ctx context.Context, n github.Notification) error {
	pr, err := c.GetPRFromNotification(ctx, n)
	if err != nil {
		return errors.Wrap(err, "getting PR metadata from notification")
	}

	owner := n.Repository.Owner.GetLogin()
	repo := n.Repository.GetName()
	prNum := pr.GetNumber()
	commitMsg := fmt.Sprintf("%s (#%d)", pr.GetTitle(), prNum)
	opts := github.PullRequestOptions{
		MergeMethod: "squash",
	}

	res, _, err := c.PullRequests.Merge(ctx, owner, repo, prNum, commitMsg, &opts)
	if err != nil {
		return errors.Wrap(err, "merging PR")
	}
	if !res.GetMerged() {
		return errors.New("PR was not merged")
	}

	zap.S().Debugw("merged PR successfully",
		"message", res.GetMessage(),
	)

	return nil
}
