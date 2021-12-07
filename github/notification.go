package github

import (
	"context"
	"regexp"
	"time"

	"github.com/google/go-github/v40/github"
	"github.com/pkg/errors"
)

type NotificationType string

const NotificationTypePullRequest NotificationType = "PullRequest"

// Reasons why a notification was sent.
const (
	ReasonReviewRequested = "review_requested"
	ReasonAuthor          = "author"
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

	var filtered []github.Notification

	for _, n := range notifications {
		if !titleMatcher.shouldAdd(n) {
			continue
		}

		if !reasonMatcher.shouldAdd(n) {
			continue
		}

		if !typeMatcher.shouldAdd(n) {
			continue
		}

		match, err := userMatcher.shouldAdd(ctx, c, n)
		if err != nil {
			return nil, errors.Wrap(err, "checking user matches")
		}
		if !match {
			continue
		}

		filtered = append(filtered, *n)
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
