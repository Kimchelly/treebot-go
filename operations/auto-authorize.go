package operations

import (
	"context"
	"os"
	"time"

	"github.com/kimchelly/treebot-go/log"

	githubapi "github.com/google/go-github/v40/github"
	"github.com/kimchelly/treebot-go/github"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

func AutoAuthorize() *cli.Command {
	return &cli.Command{
		Name:  "auto-authorize",
		Usage: "auto-authorize Dependabot PRs",
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			ghc := github.NewClient(ctx, os.Getenv("GITHUB_OAUTH_TOKEN"))

			notifications, err := ghc.GetNotifications(ctx, github.NotificationOptions{
				// kim: TODO: make configurable
				After:          time.Now().Add(-14 * 24 * time.Hour),
				IncludeRead:    true,
				IncludeTitles:  []string{"^CHORE:"},
				IncludeReasons: []string{github.ReasonReviewRequested},
				IncludeTypes:   []github.NotificationType{github.NotificationTypePullRequest},
				IncludeUsers: []github.NotificationFromUserOptions{
					{Name: "dependabot[bot]", Type: github.UserTypeBot},
				},
			})
			if err != nil {
				return errors.Wrap(err, "getting Dependabot notifications")
			}

			for _, n := range notifications {
				if err := checkAndAuthorizeDependabotPRPatch(ctx, ghc, n); err != nil {
					return errors.Wrapf(err, "checking and authorizing Dependabot PR patch from notification")
				}
			}

			return nil
		},
	}
}

func checkAndAuthorizeDependabotPRPatch(ctx context.Context, ghc *github.Client, n githubapi.Notification) error {
	statuses, err := ghc.GetCommitStatusesFromNotification(ctx, n)
	if err != nil {
		return errors.Wrap(err, "getting Dependabot PR status")
	}

	// Only consider PRs for which there's only 1 status ("failure",
	// "patch must be manually authorized") and there's a single
	// Dependabot-authored commit.

	if len(statuses) != 1 {
		log.Logger.Debugf("skipping notification '%s' because it has multiple statuses", n.Subject.GetTitle())
		return nil
	}

	status := statuses[0]
	if status.State == nil || status.Description == nil {
		log.Logger.Debugf("skipping notification '%s' because of status", n.Subject.GetTitle())
		return nil
	}

	if *status.State != "failure" {
		log.Logger.Debugf("skipping notification '%s' because of non-failure status", n.Subject.GetTitle())
		return nil
	}
	if *status.Description != "patch must be manually authorized" {
		log.Logger.Debugf("skipping notification '%s' because of non-authorize message", n.Subject.GetTitle())
		return nil
	}

	pr, err := ghc.GetPRFromNotification(ctx, n)
	if err != nil {
		log.Logger.Debugf("cannot get PR from notification '%s'", n.Subject.GetTitle())
		return errors.Wrap(err, "getting PR from notification")
	}

	if numCommits := pr.GetCommits(); numCommits != 1 {
		log.Logger.Debugf("PR from notification '%s' has %d commits, but auto-authorization requires that there should only be 1 Dependabot commit", n.Subject.GetTitle(), numCommits)
		return nil
	}

	log.Logger.Infow("updating PR",
		"title", pr.GetTitle(),
		"url", pr.GetURL(),
	)

	if err := ghc.UpdatePRFromNotification(ctx, n); err != nil {
		return errors.Wrap(err, "updating Dependabot PR")
	}

	return nil
}
