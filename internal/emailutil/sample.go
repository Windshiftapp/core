package emailutil

import "time"

// SampleData returns canned template data matching the variable schema for a
// given template name. Used by the admin preview endpoint so editors can see
// how the rendered email will look before saving.
func SampleData(name string) any {
	switch name {
	case TemplateMagicLink:
		return struct {
			FirstName    string
			MagicLinkURL string
			ExpiresIn    string
		}{
			FirstName:    "Alex",
			MagicLinkURL: "https://app.example.com/portal/acme/verify#token=preview-token",
			ExpiresIn:    "30 minutes",
		}
	case TemplateEmailVerification:
		return struct {
			FirstName       string
			VerificationURL string
			ExpiresIn       string
		}{
			FirstName:       "Alex",
			VerificationURL: "https://app.example.com/verify-email?token=preview-token",
			ExpiresIn:       "24 hours",
		}
	case TemplateInvitation:
		return struct {
			FirstName     string
			InvitationURL string
			ExpiresIn     string
		}{
			FirstName:     "Alex",
			InvitationURL: "https://app.example.com/set-password/preview-token",
			ExpiresIn:     "7 days",
		}
	case TemplateMention:
		return struct {
			RecipientName string
			ActorName     string
			ItemKey       string
			ItemTitle     string
			SourceLabel   string
			Content       string
			ActionURL     string
		}{
			RecipientName: "Alex",
			ActorName:     "Sam Patel",
			ItemKey:       "ACME-42",
			ItemTitle:     "Cannot reset password",
			SourceLabel:   "a comment",
			Content:       "@Alex could you take a look at the token refresh path? I think the expiry check runs before the clock is injected.",
			ActionURL:     "https://app.example.com/workspaces/1/items/42",
		}
	case TemplatePortalReply:
		return struct {
			AuthorName      string
			ItemKey         string
			ItemTitle       string
			Content         string
			OriginalSubject string
			PortalURL       string
		}{
			AuthorName:      "Sam Patel",
			ItemKey:         "ACME-42",
			ItemTitle:       "Cannot reset password",
			Content:         "Thanks for the report — I've reproduced the issue locally and pushed a fix. Could you confirm it works on your end after refreshing?",
			OriginalSubject: "Cannot reset password",
			PortalURL:       "https://app.example.com/portal/acme?view=requests&id=42",
		}
	case TemplateIntakeConfirmation:
		return struct {
			FirstName string
			ItemKey   string
			ItemTitle string
			PortalURL string
		}{
			FirstName: "Alex",
			ItemKey:   "ACME-42",
			ItemTitle: "Cannot reset password",
			PortalURL: "https://app.example.com/portal/acme?view=requests&id=42",
		}
	case TemplateApprovalRequested:
		return struct {
			FirstName   string
			ItemKey     string
			ItemTitle   string
			ApprovalURL string
			ExpiresIn   string
		}{
			FirstName:   "Alex",
			ItemKey:     "ACME-42",
			ItemTitle:   "Quarterly budget proposal",
			ApprovalURL: "https://app.example.com/portal/acme/verify#token=preview-token&next=/portal/acme/approvals/17",
			ExpiresIn:   "24 hours",
		}
	case TemplateNotificationBatch:
		type entry struct {
			Title         string
			Message       string
			Type          string
			AccentColor   string
			FormattedTime string
		}
		now := time.Date(2026, 4, 28, 9, 30, 0, 0, time.UTC)
		return struct {
			UserName          string
			NotificationCount int
			Notifications     []entry
		}{
			UserName:          "Alex",
			NotificationCount: 3,
			Notifications: []entry{
				{
					Title:         "ITEM-42 was assigned to you",
					Message:       "Sam Patel assigned this item to you in the Platform workspace.",
					Type:          "assignment",
					AccentColor:   "#8b5cf6",
					FormattedTime: now.Format("January 2, 2006 at 3:04 PM"),
				},
				{
					Title:         "New comment on ITEM-17",
					Message:       "Mira left a comment: \"Looks good — ready for review.\"",
					Type:          "comment",
					AccentColor:   "#06b6d4",
					FormattedTime: now.Add(-15 * time.Minute).Format("January 2, 2006 at 3:04 PM"),
				},
				{
					Title:         "Status changed on ITEM-08",
					Message:       "ITEM-08 moved from In Progress to Done.",
					Type:          "status_change",
					AccentColor:   "#f97316",
					FormattedTime: now.Add(-2 * time.Hour).Format("January 2, 2006 at 3:04 PM"),
				},
			},
		}
	default:
		return struct{}{}
	}
}

// HasSampleData reports whether the named template has preview data, i.e. is a
// built-in transactional template whose variables can be validated against a
// concrete schema.
func HasSampleData(name string) bool {
	_, empty := SampleData(name).(struct{})
	return !empty
}
