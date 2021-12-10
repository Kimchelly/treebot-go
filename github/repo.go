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

	CombinedStatusPending = "pending"
	CombinedStatusSuccess = "success"
	CombinedStatusFailure = "failure"
)

func (c *Client) GetCommitStatusesFromNotification(ctx context.Context, n PullRequestNotification) ([]github.RepoStatus, error) {
	req, err := c.NewRequest(http.MethodGet, n.PullRequest.GetStatusesURL(), nil)
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

func (c *Client) GetCommitsFromNotification(ctx context.Context, n PullRequestNotification) ([]github.Commit, error) {
	req, err := c.NewRequest(http.MethodGet, n.PullRequest.GetCommitsURL(), nil)
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

func (c *Client) GetCombinedStatusFromNotificationAndCommit(ctx context.Context, n github.Notification, commit github.Commit) (*github.CombinedStatus, error) {
	owner := n.Repository.Owner.GetLogin()
	repo := n.Repository.GetName()

	res, resp, err := c.Repositories.GetCombinedStatus(ctx, owner, repo, commit.GetSHA(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "requesting commit status information")
	}
	defer resp.Body.Close()

	return res, nil
}
