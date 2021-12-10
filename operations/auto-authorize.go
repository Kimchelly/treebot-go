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
	pastFlag                = "past"
	includeReadFlag         = "include-read"
	includeTitlesFlag       = "include-titles"
	interactiveFlag         = "interactive"
	checkDependabotUserFlag = "check-dependabot-user"
)

func autoGitHubFlags() []cli.Flag {
	return []cli.Flag{
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
		&cli.BoolFlag{
			Name:  checkDependabotUserFlag,
			Usage: "do an extra check to ensure that the notification is from Dependabot",
		},
	}
}

func AutoAuthorize() *cli.Command {
	return &cli.Command{
		Name:    "auto-authorize",
		Aliases: []string{"aa"},
		Usage:   "auto-authorize Dependabot PRs",
		Flags:   autoGitHubFlags(),
		Action: func(c *cli.Context) error {
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

	var unresolved []githubapi.Notification
	for i, n := range notifications {
		zap.S().Debugf("Notification #%d: %s", i+1, github.GetLogFormat(n))

		res, err := checkAndAuthorizeDependabotPR(ctx, ghc, c, n)
		fmt.Println()
		if err != nil {
			zap.S().Error(errors.Wrapf(err, "checking and authorizing Dependabot PR patch from notification"))
			continue
		}

		if res == skipped {
			unresolved = append(unresolved, notifications[i])
		}
	}

	zap.S().Info("Unresolved notifications:")
	for _, n := range unresolved {
		zap.S().Info(github.GetLogFormat(n))
	}

	return nil
}

func getDependabotPRNotifications(ctx context.Context, ghc *github.Client, c *cli.Context) ([]githubapi.Notification, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	opts := github.NotificationOptions{
		After:          time.Now().Add(-c.Duration(pastFlag)),
		IncludeRead:    c.Bool(includeReadFlag),
		IncludeTitles:  c.StringSlice(includeTitlesFlag),
		IncludeReasons: []string{github.ReasonReviewRequested},
		IncludeTypes:   []github.NotificationType{github.NotificationTypePullRequest},
	}
	if c.Bool(checkDependabotUserFlag) {
		// This check is quite expensive, so put it behind a flag.
		opts.IncludeUsers = []github.NotificationFromUserOptions{
			{Name: github.DependabotUsername, Type: github.UserTypeBot},
		}
	}
	return ghc.GetNotifications(ctx, opts)
}

type operationResult string

const (
	done        operationResult = "done"
	alreadyDone operationResult = "already-done"
	skipped     operationResult = "skipped"
	errored     operationResult = "errored"
)

func checkAndAuthorizeDependabotPR(ctx context.Context, ghc *github.Client, c *cli.Context, n githubapi.Notification) (operationResult, error) {
	getPRCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	pr, err := ghc.GetPRFromNotification(getPRCtx, n)
	if err != nil {
		return errored, errors.Wrap(err, "getting PR from notification")
	}

	zap.S().Info("URL: ", github.GetHumanReadableURLForPR(n, *pr))

	if state := pr.GetState(); state != github.PRStateOpen {
		zap.S().Debugf("skipping because PR state is '%s'", state)
		if state == github.PRStateClosed {
			return alreadyDone, nil
		}
		return skipped, nil
	}
	if numCommits := pr.GetCommits(); numCommits != 1 {
		zap.S().Debugf("skipping PR which has %d commits - auto-authorization requires that there should be exactly 1 Dependabot commit", numCommits)
		return skipped, nil
	}

	getCommitStatusCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	statuses, err := ghc.GetCommitStatusesFromNotification(getCommitStatusCtx, n)
	if err != nil {
		return errored, errors.Wrap(err, "getting Dependabot PR status")
	}

	// Only consider PRs for which there's only 1 status ("failure",
	// "patch must be manually authorized") and there's a single
	// Dependabot-authored commit.

	if len(statuses) != 1 {
		zap.S().Debugf("skipping notification because it has multiple commit statuses available, but there should be exactly 1 failed commit status for a Dependabot PR in need of manual authorization")
		return skipped, nil
	}
	latest := statuses[0]
	if state := latest.GetState(); state != github.CommitStatusFailure {
		zap.S().Debugf("skipping notification because latest commit status should be a failure for a Dependabot PR in need of manual authorization, but the actual commit status is '%s'", state)
		return skipped, nil
	}
	if latest.GetDescription() != "patch must be manually authorized" {
		zap.S().Debugf("skipping notification because it contains a commit status message other than the manual patch authorization message")
		return skipped, nil
	}

	if c.Bool(interactiveFlag) {
		fmt.Println()
		yes, err := yesOrNo("Authorize this PR?")
		if err != nil {
			return errored, errors.Wrap(err, "asking user to authorize Dependabot PR")
		}
		if !yes {
			return skipped, nil
		}
		fmt.Println()
	}

	zap.S().Infow("authorizing Dependabot PR",
		"title", pr.GetTitle(),
		"url", pr.GetURL(),
	)

	updatePRCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := ghc.UpdatePRFromNotification(updatePRCtx, n); err != nil {
		return errored, errors.Wrap(err, "updating Dependabot PR")
	}

	return done, nil
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
		fmt.Println("Invalid input, please try again.")
	}
}
