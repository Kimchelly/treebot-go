package operations

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	githubapi "github.com/google/go-github/v40/github"
	"github.com/kimchelly/treebot-go/github"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func AutoMerge() *cli.Command {
	return &cli.Command{
		Name:  "auto-merge",
		Usage: "automatically merge Dependabot PRs that pass all CI tests",
		Action: func(c *cli.Context) error {
			return nil
		},
	}
}

func autoMergeDependabotPRsFromNotifications(c *cli.Context) error {
	token := os.Getenv("GITHUB_OAUTH_TOKEN")
	if token == "" {
		return errors.New("GITHUB_OAUTH_TOKEN environment variable is required")
	}

	zap.S().Info("checking for Dependabot PRs to auto-merge")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ghc := github.NewClient(ctx, token)

	notifications, err := getDependabotPRNotifications(ctx, ghc, c)
	if err != nil {
		return errors.Wrap(err, "getting Dependabot PR notifications")
	}

	for _, n := range notifications {
		zap.S().Debugf("%s: considering whether Dependabot PR needs to be merged for this notification", github.GetLogFormat(n))
	}

	return nil
}

func checkAndMergeDependabotPR(ctx context.Context, ghc *github.Client, c *cli.Context, n githubapi.Notification) error {
	getCommitStatusCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	statuses, err := ghc.GetCommitStatusesFromNotification(getCommitStatusCtx, n)
	if err != nil {
		return errors.Wrap(err, "getting Dependabot PR status")
	}

	if len(statuses) == 0 {
		zap.S().Debugf("%s: skipping notification because it has no commit statuses")
		return nil
	}

	latest := statuses[0]
	if state := latest.GetState(); state != github.CommitStatusSuccess {
		zap.S().Debugf("%s: skipping notification because its latest commit status should be success for a Dependabot PR, but the actual commit status is '%s'", github.GetLogFormat(n), state)
		return nil
	}
	if !strings.Contains(latest.GetDescription(), "patch finished") {
		zap.S().Debugf("%s: skipping notification because it contains a commit status message other than the patch finished message", github.GetLogFormat(n))
	}

	getPRCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	pr, err := ghc.GetPRFromNotification(getPRCtx, n)
	if err != nil {
		return errors.Wrap(err, "getting PR from notification")
	}
	if !pr.GetMergeable() {
		return errors.Errorf("%s: cannot determine if PR is mergeable", github.GetLogFormat(n))
	}
	if pr.GetMergeableState() != "clean" {
		return errors.Errorf("%s: PR is not cleanly mergeable", github.GetLogFormat(n))
	}
	if state := pr.GetState(); state != github.PRStateOpen {
		zap.S().Debugf("%s: skipping because PR state is '%s'", github.GetLogFormat(n), state)
		return nil
	}

	if c.Bool(interactiveFlag) {
		yes, err := yesOrNo(fmt.Sprintf("%s\nMerge PR?", github.GetLogFormat(n)))
		if err != nil {
			return errors.Wrap(err, "asking user to authorize Dependabot PR")
		}
		if !yes {
			return nil
		}
	}

	zap.S().Infow("merging Dependabot PR",
		"title", pr.GetTitle(),
		"url", pr.GetURL(),
	)

	mergePRCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := ghc.MergePRFromNotification(mergePRCtx, n); err != nil {
		return errors.Wrap(err, "merging Dependabot PR")
	}

	return nil
}
