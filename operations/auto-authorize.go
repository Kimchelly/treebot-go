package operations

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	githubapi "github.com/google/go-github/v40/github"
	"github.com/kimchelly/treebot-go/github"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

const (
	pastFlag          = "past"
	includeReadFlag   = "include-read"
	includeTitlesFlag = "include-titles"
	interactiveFlag   = "interactive"
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
			&cli.BoolFlag{
				Name:  interactiveFlag,
				Usage: "authorize PRs in interactive session",
			},
		},
		Action: func(c *cli.Context) error {
			defer zap.S().Sync()
			return autoAuthorizeDependabotPRsFromNotifications(c)
		},
	}
}

func autoAuthorizeDependabotPRsFromNotifications(c *cli.Context) error {
	token := os.Getenv("GITHUB_OAUTH_TOKEN")
	if token == "" {
		return errors.New("GITHUB_OAUTH_TOKEN environment variable is required")
	}

	zap.S().Info("checking for Dependabot PRs to auto-authorize")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ghc := github.NewClient(ctx, token)

	notifications, err := getDependabotPRNotifications(ctx, ghc, c)
	if err != nil {
		return errors.Wrap(err, "getting Dependabot PR notifications")
	}

	for _, n := range notifications {
		zap.S().Debugf("%s: considering whether Dependabot needs to be authorized for this notification", github.GetLogFormat(n))
		if err := checkAndAuthorizeDependabotPR(ctx, ghc, c, n); err != nil {
			return errors.Wrapf(err, "checking and authorizing Dependabot PR patch from notification")
		}
	}

	return nil
}

func getDependabotPRNotifications(ctx context.Context, ghc *github.Client, c *cli.Context) ([]githubapi.Notification, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	return ghc.GetNotifications(ctx, github.NotificationOptions{
		After:          time.Now().Add(-c.Duration(pastFlag)),
		IncludeRead:    c.Bool(includeReadFlag),
		IncludeTitles:  c.StringSlice(includeTitlesFlag),
		IncludeReasons: []string{github.ReasonReviewRequested},
		IncludeTypes:   []github.NotificationType{github.NotificationTypePullRequest},
		IncludeUsers: []github.NotificationFromUserOptions{
			{Name: "dependabot[bot]", Type: github.UserTypeBot},
		},
	})
}

func checkAndAuthorizeDependabotPR(ctx context.Context, ghc *github.Client, c *cli.Context, n githubapi.Notification) error {
	getCommitStatusCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	statuses, err := ghc.GetCommitStatusesFromNotification(getCommitStatusCtx, n)
	if err != nil {
		return errors.Wrap(err, "getting Dependabot PR status")
	}

	// Only consider PRs for which there's only 1 status ("failure",
	// "patch must be manually authorized") and there's a single
	// Dependabot-authored commit.

	if len(statuses) != 1 {
		zap.S().Debugf("%s: skipping notification because it has multiple commit statuses available, but there should be exactly 1 failed commit status for a Dependabot PR in need of manual authorization", github.GetLogFormat(n))
		return nil
	}
	latest := statuses[0]
	if state := latest.GetState(); state != github.CommitStatusFailure {
		zap.S().Debugf("%s: skipping notification because latest commit status should be a failure for a Dependabot PR in need of manual authorization, but the actual commit status is '%s'", github.GetLogFormat(n), state)
		return nil
	}
	if latest.GetDescription() != "patch must be manually authorized" {
		zap.S().Debugf("%s: skipping notification because it contains a commit status message other than the manual patch authorization message", github.GetLogFormat(n))
		return nil
	}

	getPRCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	pr, err := ghc.GetPRFromNotification(getPRCtx, n)
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

	if c.Bool(interactiveFlag) {
		yes, err := yesOrNo(fmt.Sprintf("%s\nAuthorize PR?", github.GetLogFormat(n)))
		if err != nil {
			return errors.Wrap(err, "asking user to authorize Dependabot PR")
		}
		if !yes {
			return nil
		}
	}

	zap.S().Infow("authorizing Dependabot PR",
		"title", pr.GetTitle(),
		"url", pr.GetURL(),
	)

	updatePRCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := ghc.UpdatePRFromNotification(updatePRCtx, n); err != nil {
		return errors.Wrap(err, "updating Dependabot PR")
	}

	return nil
}

func yesOrNo(message string) (bool, error) {
	for {
		fmt.Printf("%s [y/n] ", message)
		r := bufio.NewReader(os.Stdin)

		input, err := r.ReadString('\n')
		if err != nil {
			return false, errors.Wrap(err, "reading user input")
		}
		input = strings.TrimSpace(input)

		switch input {
		case "y":
			return true, nil
		case "n":
			return false, nil
		}
	}
}
