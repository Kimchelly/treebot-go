package github

import (
	"context"
	"regexp"
	"time"

	"github.com/google/go-github/v40/github"
	"github.com/k0kubun/pp/v3"
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
	if err := github.CheckResponse(resp.Response); err != nil {
		return nil, errors.Wrap(err, "GitHub response")
	}

	var filtered []github.Notification

	for _, n := range notifications {
		// pp.Println("checking notification", n.ID, n.Subject, n.Reason)

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

	pp.Println("found notifications:")
	for _, n := range filtered {
		pp.Println(n.Subject)
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

	if n.Subject == nil || n.Subject.Title == nil {
		return false
	}

	for _, expr := range f.matches {
		if !expr.MatchString(*n.Subject.Title) {
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

	if n.Reason == nil {
		return false
	}

	for _, r := range f.reasons {
		if r != *n.Reason {
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

	if n.Subject == nil || n.Subject.Type == nil {
		return false
	}

	for _, t := range f.types {
		if string(t) != *n.Subject.Type {
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

	if n.Subject == nil || n.Subject.Type == nil {
		return false, nil
	}

	switch *n.Subject.Type {
	case string(NotificationTypePullRequest):
		if n.Subject.URL == nil {
			return false, nil
		}
		pr, err := c.GetPRFromNotification(ctx, *n)
		if err != nil {
			return false, errors.Wrap(err, "getting PR metadata from notification")
		}

		if pr.User == nil {
			return false, nil
		}

		for _, u := range f.users {
			if u.Name != "" {
				if pr.User.Login == nil || u.Name != *pr.User.Login {
					continue
				}
			}

			if u.Type != "" {
				if pr.User.Type == nil || string(u.Type) != *pr.User.Type {
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
