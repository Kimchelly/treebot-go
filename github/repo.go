package github

import (
	"context"
	"net/http"

	"github.com/google/go-github/v40/github"
	"github.com/pkg/errors"
)

const (
	CommitStatusPending = "pending"
	CommitStatusError   = "error"
	CommitStatusSuccess = "success"
	CommitStatusFailure = "failure"
)

func (c *Client) GetCommitStatusesFromNotification(ctx context.Context, n github.Notification) ([]github.RepoStatus, error) {
	pr, err := c.GetPRFromNotification(ctx, n)
	if err != nil {
		return nil, errors.Wrap(err, "getting PR metadata from notification")
	}

	req, err := c.NewRequest(http.MethodGet, pr.GetStatusesURL(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	statuses := []github.RepoStatus{}
	resp, err := c.Do(ctx, req, &statuses)
	if err != nil {
		return nil, errors.Wrap(err, "requesting repository status information")
	}
	defer resp.Body.Close()

	return statuses, nil
}

func (c *Client) GetCommitsFromNotification(ctx context.Context, n github.Notification) ([]github.Commit, error) {
	pr, err := c.GetPRFromNotification(ctx, n)
	if err != nil {
		return nil, errors.Wrap(err, "getting PR metadata from notification")
	}

	req, err := c.NewRequest(http.MethodGet, pr.GetCommitsURL(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	commits := []github.Commit{}
	resp, err := c.Do(ctx, req, &commits)
	if err != nil {
		return nil, errors.Wrap(err, "requesting commit information")
	}
	defer resp.Body.Close()

	return commits, nil
}

func (c *Client) GetStatusesFromNotificationAndCommit(ctx context.Context, n github.Notification, commit github.Commit) ([]github.RepoStatus, error) {
	owner := n.Repository.Owner.GetLogin()
	repo := n.Repository.GetName()

	res, resp, err := c.Repositories.ListStatuses(ctx, owner, repo, commit.GetSHA(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "requesting commit status information")
	}
	defer resp.Body.Close()
	var statuses []github.RepoStatus
	for _, s := range res {
		statuses = append(statuses, *s)
	}

	return statuses, nil
}
