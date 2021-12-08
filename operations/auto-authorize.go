package operations

import (
	"context"
	"os"
	"time"

	"github.com/robfig/cron"
	"go.uber.org/zap"

	githubapi "github.com/google/go-github/v40/github"
	"github.com/kimchelly/treebot-go/github"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

const (
	pastFlag          = "past"
	cronExprFlag      = "cron-expr"
	includeReadFlag   = "include-read"
	includeTitlesFlag = "include-titles"
)

func AutoAuthorize() *cli.Command {
	return &cli.Command{
		Name:  "auto-authorize",
		Usage: "auto-authorize Dependabot PRs",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  includeReadFlag,
				Usage: "include already-read notifications in Dependabot authorization checks",
			},
			&cli.StringSliceFlag{
				Name:  includeTitlesFlag,
				Usage: "include notifications matching the given title pattern(s)",
			},
			&cli.DurationFlag{
				Name:  pastFlag,
				Usage: "how long to search backwards in time for notifications",
				Value: -24 * time.Hour,
			},
			&cli.StringFlag{
				Name:  cronExprFlag,
				Usage: "cron expression for when to run the cron job (docs: https://pkg.go.dev/github.com/robfig/cron@v1.2.0#hdr-CRON_Expression_Format)",
			},
		},
		Action: func(c *cli.Context) error {
			defer zap.S().Sync()
			cronExpr := c.String(cronExprFlag)
			if cronExpr == "" {
				return autoAuthorizeFromNotifications(c)
			}

			schedule := cron.New()
			schedule.AddFunc(cronExpr, func() {
				autoAuthorizeFromNotifications(c)
			})

			schedule.Start()

			// The infinite loop just keeps the program alive to run the cron
			// job.
			for {
				time.Sleep(time.Hour)
			}
		},
	}
}

func autoAuthorizeFromNotifications(c *cli.Context) error {
	zap.S().Info("checking for Dependabot PRs to auto-authorize")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ghc := github.NewClient(ctx, os.Getenv("GITHUB_OAUTH_TOKEN"))

	notifications, err := ghc.GetNotifications(ctx, github.NotificationOptions{
		After:          time.Now().Add(-c.Duration(pastFlag)),
		IncludeRead:    c.Bool(includeReadFlag),
		IncludeTitles:  c.StringSlice(includeTitlesFlag),
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
		zap.S().Debugf("%s: considering whether Dependabot needs to be authorized for this notification", github.GetLogFormat(n))
		if err := checkAndAuthorizeDependabotPRPatch(ctx, ghc, n); err != nil {
			return errors.Wrapf(err, "checking and authorizing Dependabot PR patch from notification")
		}
	}

	return nil
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
		zap.S().Debugf("%s: skipping notification because it has multiple statuses available", github.GetLogFormat(n))
		return nil
	}

	status := statuses[0]
	if status.GetState() != "failure" {
		zap.S().Debugf("%s: skipping notification because of non-failure status", github.GetLogFormat(n))
		return nil
	}
	if status.GetDescription() != "patch must be manually authorized" {
		zap.S().Debugf("%s: skipping notification because it contains a status message other than the manual patch authorization message", github.GetLogFormat(n))
		return nil
	}

	pr, err := ghc.GetPRFromNotification(ctx, n)
	if err != nil {
		return errors.Wrap(err, "getting PR from notification")
	}

	if state := pr.GetState(); state != github.PRStateOpen {
		zap.S().Debugf("%s: skipping because PR state is '%s'", github.GetLogFormat(n), state)
		return nil
	}

	if numCommits := pr.GetCommits(); numCommits != 1 {
		zap.S().Debugf("%s: skipping PR which has %d commits - auto-authorization requires that there should be exactly 1 Dependabot commit", github.GetLogFormat(n), numCommits)
		return nil
	}

	zap.S().Infow("updating Dependabot PR",
		"title", pr.GetTitle(),
		"url", pr.GetURL(),
	)

	if err := ghc.UpdatePRFromNotification(ctx, n); err != nil {
		return errors.Wrap(err, "updating Dependabot PR")
	}

	return nil
}
