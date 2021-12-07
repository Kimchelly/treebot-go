package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/k0kubun/pp/v3"
	"github.com/kimchelly/treebot-go/github"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c := github.NewClient(ctx, os.Getenv("GITHUB_OAUTH_TOKEN"))

	notifications, err := c.GetNotifications(ctx, github.NotificationOptions{
		After:          time.Now().Add(-14 * 24 * time.Hour),
		IncludeTitles:  []string{"^CHORE:"},
		IncludeReasons: []string{github.ReasonReviewRequested},
		IncludeTypes:   []github.NotificationType{github.NotificationTypePullRequest},
		IncludeUsers: []github.NotificationFromUserOptions{
			{Name: "dependabot[bot]", Type: github.UserTypeBot},
		},
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	for _, n := range notifications {
		statuses, err := c.GetCommitStatusesFromNotification(ctx, n)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			continue
		}
		// NOTE: Only consider PRs for which there's only 1 status ("failure",
		// "patch must be manually authorized") and there's a single
		// Dependabot-authored commit.
		if len(statuses) != 1 {
			continue
		}

		pr, err := c.GetPRFromNotification(ctx, n)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			continue
		}

		if pr.Commits == nil {
			fmt.Fprintf(os.Stderr, "No commits found for PR '%#v'", *pr)
			continue
		}

		if *pr.Commits == 1 {
			pp.Println("found PR currently in need of manual push authorization:", n.Subject)
		}
	}
}
