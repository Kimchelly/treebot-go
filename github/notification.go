package github

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/google/go-github/v40/github"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type NotificationType string

const NotificationTypePullRequest NotificationType = "PullRequest"

// Reasons why a notification was sent.
const (
	ReasonAssign          = "assign"
	ReasonAuthor          = "author"
	ReasonCIActivity      = "ci_activity"
	ReasonComment         = "comment"
	ReasonInvitation      = "invitation"
	ReasonManual          = "manual"
	ReasonMention         = "mention"
	ReasonSecurityAlert   = "security_alert"
	ReasonReviewRequested = "review_requested"
	ReasonStateChange     = "state_change"
	ReasonTeamMention     = "team_mention"
)

type NotificationOptions struct {
	IncludeRead    bool
	Before         time.Time
	After          time.Time
	IncludeTitles  []string
	IncludeReasons []string
	IncludeTypes   []NotificationType
	IncludeUsers   []NotificationFromUserOptions
}

type UserType string

const (
	UserTypeBot  UserType = "Bot"
	UserTypeUser UserType = "User"
)

const DependabotUsername = "dependabot[bot]"

type NotificationFromUserOptions struct {
	Name string
	Type UserType
}

func (c *Client) GetNotifications(ctx context.Context, opts NotificationOptions) ([]github.Notification, error) {
	var titleMatcher titleFilters
	for _, expr := range opts.IncludeTitles {
		titleMatcher.matches = append(titleMatcher.matches, regexp.MustCompile(expr))
	}
	reasonMatcher := reasonFilters{reasons: opts.IncludeReasons}
	typeMatcher := typeFilters{types: opts.IncludeTypes}
	userMatcher := notificationsFromUserFilter{users: opts.IncludeUsers}

	notifications, resp, err := c.Activity.ListNotifications(ctx, &github.NotificationListOptions{
		All:    opts.IncludeRead,
		Before: opts.Before,
		Since:  opts.After,
	})
	if err != nil {
		return nil, errors.Wrap(err, "listing notifications")
	}
	defer resp.Body.Close()

	zap.S().Debugw("found notifications matching notification filters",
		"count", len(notifications),
	)

	var filtered []github.Notification

	for _, n := range notifications {
		if !titleMatcher.shouldAdd(n) {
			zap.S().Debugf("%s: skipping notification due to unmatched notification title", GetLogFormat(*n))
			continue
		}

		if !reasonMatcher.shouldAdd(n) {
			zap.S().Debugf("%s: skipping notification due to unmatched notification reason", GetLogFormat(*n))
			continue
		}

		if !typeMatcher.shouldAdd(n) {
			zap.S().Debugf("%s: skipping notification due to unmatched notification type", GetLogFormat(*n))
			continue
		}

		match, err := userMatcher.shouldAdd(ctx, c, n)
		if err != nil {
			zap.S().Debugf("%s: skipping notification due to unmatched user", GetLogFormat(*n))
			return nil, errors.Wrap(err, "checking user matches")
		}
		if !match {
			continue
		}

		filtered = append(filtered, *n)
	}

	if len(filtered) != len(notifications) {
		zap.S().Debugw("filtered notifications to smaller candidate set",
			"count", len(filtered),
		)
	}

	return filtered, nil
}

type titleFilters struct {
	matches []*regexp.Regexp
}

func (f *titleFilters) shouldAdd(n *github.Notification) bool {
	if len(f.matches) == 0 {
		return true
	}

	for _, expr := range f.matches {
		if !expr.MatchString(n.Subject.GetTitle()) {
			continue
		}
		return true
	}

	return false
}

type reasonFilters struct {
	reasons []string
}

func (f *reasonFilters) shouldAdd(n *github.Notification) bool {
	if len(f.reasons) == 0 {
		return true
	}

	for _, r := range f.reasons {
		if r != n.GetReason() {
			continue
		}
		return true
	}

	return false
}

type typeFilters struct {
	types []NotificationType
}

func (f *typeFilters) shouldAdd(n *github.Notification) bool {
	if len(f.types) == 0 {
		return true
	}

	for _, t := range f.types {
		if string(t) != n.Subject.GetType() {
			continue
		}
		return true
	}

	return false
}

type notificationsFromUserFilter struct {
	users []NotificationFromUserOptions
}

func (f *notificationsFromUserFilter) shouldAdd(ctx context.Context, c *Client, n *github.Notification) (bool, error) {
	if len(f.users) == 0 {
		return true, nil
	}

	switch n.Subject.GetType() {
	case string(NotificationTypePullRequest):
		if n.Subject.GetURL() == "" {
			return false, nil
		}
		pr, err := c.GetPRFromNotification(ctx, *n)
		if err != nil {
			return false, errors.Wrap(err, "getting PR metadata from notification")
		}

		for _, u := range f.users {
			if u.Name != "" {
				if u.Name != pr.User.GetLogin() {
					continue
				}
			}

			if u.Type != "" {
				if string(u.Type) != pr.User.GetType() {
					continue
				}
			}

			return true, nil
		}
		return false, nil
	default:
		return false, nil
	}
}

func GetLogFormat(n github.Notification) string {
	return fmt.Sprintf("\"%s (%s)\"", n.Subject.GetTitle(), n.Repository.GetFullName())
}
