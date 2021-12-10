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
		Name:    "auto-merge",
		Aliases: []string{"am"},
		Usage:   "automatically merge Dependabot PRs that pass all CI tests",
		Flags:   autoGitHubFlags(),
		Action: func(c *cli.Context) error {
			return autoMergeDependabotPRsFromNotifications(c)
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

	var unresolved []githubapi.Notification
	for i, n := range notifications {
		zap.S().Infof("Notification #%d: %s", i+1, github.GetLogFormat(n))

		res, err := checkAndMergeDependabotPR(ctx, ghc, c, n)
		fmt.Println()
		if err != nil {
			zap.S().Error(errors.Wrapf(err, "checking and merging Dependabot PR from notification"))
			continue
		}

		if res == skipped {
			unresolved = append(unresolved, notifications[i])
		}
	}

	zap.S().Info("Unresolved notifications:")
	for _, n := range notifications {
		zap.S().Info(github.GetLogFormat(n))
	}

	return nil
}

func checkAndMergeDependabotPR(ctx context.Context, ghc *github.Client, c *cli.Context, n githubapi.Notification) (operationResult, error) {
	var mergeable bool
	var pr *githubapi.PullRequest
	var err error
	var loggedURLForPR bool
	// A PR might not be immediately mergeable if a previous PR was just merged
	// for this repo. This retry loop just polls hoping that it'll be mergeable
	// soon.
	for i := 0; !mergeable && i < 10; i++ {
		getPRCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		pr, err = ghc.GetPRFromNotification(getPRCtx, n)
		if err != nil {
			return errored, errors.Wrap(err, "getting PR from notification")
		}
		if !loggedURLForPR {
			zap.S().Info("URL: ", github.GetHumanReadableURLForPR(n, *pr))
			loggedURLForPR = true
		}

		if state := pr.GetState(); state != github.PRStateOpen {
			zap.S().Debugf("skipping because PR state is '%s'", state)
			if state == github.PRStateClosed {
				return alreadyDone, nil
			}
			return skipped, nil
		}

		switch pr.GetMergeableState() {
		case github.MergeableStateClean:
		case github.MergeableStateUnstable:
		case github.MergeableStateUnknown:
			zap.S().Debugf("PR check attempt #%d: uncertain if PR is mergeable", i+1)
			time.Sleep(time.Second)
			continue
		default:
			zap.S().Debugf("skipping PR because it is not cleanly mergeable - mergeable status is '%s'", pr.GetMergeableState())
			return skipped, nil
		}

		if !pr.GetMergeable() {
			zap.S().Debugf("PR check attempt #%d: PR is not mergeable", i+1)
			time.Sleep(time.Second)
			continue
		}

		mergeable = true
	}
	if !mergeable {
		zap.S().Debugf("skipping because PR is not mergeable")
		return skipped, nil
	}

	commits, err := ghc.GetCommitsFromNotification(ctx, n)
	if err != nil {
		return errored, errors.Wrap(err, "getting commits from notification")
	}
	if len(commits) == 0 {
		zap.S().Debugf("skipping because PR has no commits")
		return skipped, nil
	}

	latest := commits[len(commits)-1]
	statuses, err := ghc.GetStatusesFromNotificationAndCommit(ctx, n, latest)
	if err != nil {
		return errored, errors.Wrap(err, "getting statuses from latest commit")
	}
	if len(statuses) == 0 {
		zap.S().Debugf("skipping notification because the latest commit has no statuses available")
		return skipped, nil
	}

	var patchFinished bool
	for _, s := range statuses {
		if state := s.GetState(); state == github.CommitStatusFailure {
			zap.S().Debugf("skipping notification because its latest commit cannot have a failure for a Dependabot PR, but the actual commit status is '%s'", state)
			return skipped, nil
		}
		if strings.Contains(s.GetDescription(), "patch finished") {
			patchFinished = true
		}
	}
	if !patchFinished {
		zap.S().Debugf("skipping notification because the commit status messages indicate that the patch has not finished")
		return skipped, nil
	}

	zap.S().Info("Statuses for latest commit in PR:")
	for _, s := range statuses {
		zap.S().Infow("",
			"context", s.GetContext(),
			"state", s.GetState(),
			"description", s.GetDescription(),
			"target_url", s.GetTargetURL(),
		)
	}

	if c.Bool(interactiveFlag) {
		fmt.Println()
		yes, err := yesOrNo("Merge this PR?")
		if err != nil {
			return errored, errors.Wrap(err, "asking user to merge Dependabot PR")
		}
		if !yes {
			return skipped, nil
		}
		fmt.Println()
	}

	zap.S().Infow("merging Dependabot PR",
		"title", pr.GetTitle(),
		"url", pr.GetURL(),
	)

	mergePRCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := ghc.MergePRFromNotification(mergePRCtx, n); err != nil {
		return errored, errors.Wrap(err, "merging Dependabot PR")
	}

	return done, nil
}
