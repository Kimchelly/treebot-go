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
		After:  time.Now().Add(-14 * 24 * time.Hour),
		Titles: []string{"^CHORE:"},
		Types:  []github.NotificationType{github.NotificationTypePullRequest},
		Users: []github.NotificationFromUserOptions{
			{Name: "dependabot[bot]", Type: github.UserTypeBot},
		},
	})

	for _, n := range notifications {
		pp.Println("notification:", n.Subject)
		statuses, err := c.GetCommitStatusesFromNotification(ctx, n)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}
		for _, s := range statuses {
			if *s.State == "failure" && *s.Description == "patch must be manually authorized" {
				pp.Println("status info:", s.State, s.Description, s.TargetURL)
			}
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
