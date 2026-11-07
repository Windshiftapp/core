package emailutil

// DefaultTemplate is a built-in email template used as the fallback when the
// admin-editable DB row is missing, inactive, or never customized. The same
// content seeds the notification_templates table on a fresh install.
type DefaultTemplate struct {
	Name        string
	Description string
	Subject     string
	HTMLBody    string
	TextBody    string
}

// DefaultTemplates returns the built-in transactional email templates shipped
// with Windshift. Each template uses email-safe HTML (table layout, inline
// styles, no flexbox or CSS variables) and the brand color #2874bb.
//
// Copy is localized at render time through the `t` helper, bound to the
// recipient's locale (see internal/i18n). English is the fallback.
func DefaultTemplates() []DefaultTemplate {
	return []DefaultTemplate{
		{
			Name:        TemplateMagicLink,
			Description: "Sent when a portal customer requests a magic-link sign-in.",
			Subject:     `{{t "email.magicLink.subject"}}`,
			HTMLBody:    magicLinkHTML,
			TextBody:    magicLinkText,
		},
		{
			Name:        TemplateEmailVerification,
			Description: "Sent to users to verify their email address.",
			Subject:     `{{t "email.verification.subject"}}`,
			HTMLBody:    emailVerificationHTML,
			TextBody:    emailVerificationText,
		},
		{
			Name:        TemplateInvitation,
			Description: "Sent to invite a new user and prompt them to set a password.",
			Subject:     `{{t "email.invitation.subject"}}`,
			HTMLBody:    invitationHTML,
			TextBody:    invitationText,
		},
		{
			Name:        TemplateNotificationBatch,
			Description: "Sent when a user has unread notifications batched for delivery.",
			Subject:     `{{if eq .NotificationCount 1}}{{t "email.notificationBatch.subjectOne"}}{{else}}{{t "email.notificationBatch.subjectOther" "count" .NotificationCount}}{{end}}`,
			HTMLBody:    notificationBatchHTML,
			TextBody:    notificationBatchText,
		},
		{
			Name:        TemplateMention,
			Description: "Sent to a user who was @mentioned in a comment or item description. Includes the message body.",
			Subject:     `{{t "email.mention.subject" "actor" .ActorName "itemKey" .ItemKey}}`,
			HTMLBody:    mentionHTML,
			TextBody:    mentionText,
		},
		{
			Name:        TemplatePortalReply,
			Description: "Threaded reply sent to a portal customer when an internal user comments on their ticket (email or portal origin).",
			Subject:     `{{t "email.portalReply.subject" "subject" .OriginalSubject}}`,
			HTMLBody:    portalReplyHTML,
			TextBody:    portalReplyText,
		},
		{
			Name:        TemplateApprovalRequested,
			Description: "Sent to a portal customer when an approval step opens that requires their decision. Includes a magic-link to the portal approval page.",
			Subject:     `{{t "email.approval.subject" "itemKey" .ItemKey "itemTitle" .ItemTitle}}`,
			HTMLBody:    approvalRequestedHTML,
			TextBody:    approvalRequestedText,
		},
		{
			Name:        TemplateIntakeConfirmation,
			Description: "Receipt sent to the sender when an email intake creates a new request. Includes a portal link when the intake exposes the request to a portal.",
			Subject:     `{{t "email.intake.subject" "itemKey" .ItemKey}}`,
			HTMLBody:    intakeConfirmationHTML,
			TextBody:    intakeConfirmationText,
		},
	}
}

// Template name constants. Senders look up rows by these keys.
const (
	TemplateMagicLink          = "magic_link"
	TemplateEmailVerification  = "email_verification"
	TemplateInvitation         = "invitation"
	TemplateMention            = "mention"
	TemplateNotificationBatch  = "notification_batch"
	TemplatePortalReply        = "portal_reply"
	TemplateApprovalRequested  = "approval_requested"
	TemplateIntakeConfirmation = "intake_confirmation"
)

const emailShellOpen = `<!DOCTYPE html>
<html lang="{{.Locale}}" dir="{{.Dir}}">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Subject}}</title>
<style>
.ws-md p{margin:0 0 12px;}
.ws-md p:last-child{margin-bottom:0;}
.ws-md a{color:#2874bb;text-decoration:underline;}
.ws-md h1,.ws-md h2,.ws-md h3{margin:16px 0 8px;color:#0f172a;}
.ws-md ul,.ws-md ol{margin:0 0 12px;padding-left:22px;}
.ws-md li{margin:0 0 4px;}
.ws-md table{border-collapse:collapse;width:100%;margin:0 0 12px;}
.ws-md th,.ws-md td{border:1px solid #d1d5db;padding:6px 10px;text-align:left;vertical-align:top;}
.ws-md th{background:#f3f4f6;font-weight:600;}
.ws-md code{font-family:ui-monospace,monospace;font-size:13px;background:#f3f4f6;padding:1px 4px;border-radius:3px;}
.ws-md pre{background:#f3f4f6;padding:12px;border-radius:6px;overflow-x:auto;}
.ws-md pre code{background:none;padding:0;}
.ws-md blockquote{margin:0 0 12px;padding:0 12px;border-left:3px solid #d1d5db;color:#4b5563;}
</style>
</head>
<body style="margin:0;padding:0;background:#f4f5f7;font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#1f2937;-webkit-font-smoothing:antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f5f7;padding:40px 16px;">
<tr><td align="center">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border:1px solid #e5e7eb;border-radius:12px;overflow:hidden;">
<tr><td style="padding:24px 32px;border-bottom:1px solid #f1f2f4;">
<span style="font-size:13px;font-weight:600;letter-spacing:0.08em;color:#2874bb;text-transform:uppercase;">Windshift</span>
</td></tr>
<tr><td style="padding:32px;font-size:15px;line-height:1.6;color:#1f2937;">
`

// The footer band is split from its message so the do-not-reply and repliable
// variants stay structurally identical.
const emailShellFooterOpen = `</td></tr>
<tr><td style="padding:20px 32px;background:#fafbfc;border-top:1px solid #f1f2f4;font-size:12px;line-height:1.5;color:#6b7280;text-align:center;">
`

const emailShellFooterClose = `</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`

const emailShellClose = emailShellFooterOpen + `{{t "email.footer.automated"}}` + emailShellFooterClose

// emailShellCloseRepliable is used by templates whose body asks the customer to
// reply. A do-not-reply footer would contradict that instruction.
const emailShellCloseRepliable = emailShellFooterOpen + `{{t "email.footer.repliable"}}` + emailShellFooterClose

const buttonStyle = `display:inline-block;background:#2874bb;color:#ffffff;text-decoration:none;font-weight:600;font-size:14px;padding:12px 24px;border-radius:8px;line-height:1.2;`

const magicLinkHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">{{t "email.magicLink.heading"}}</h1>
<p style="margin:0 0 8px;">{{t "email.common.greeting" "name" .FirstName}}</p>
<p style="margin:0 0 24px;">{{t "email.magicLink.body" "expires" .ExpiresIn}}</p>
<p style="margin:0 0 24px;"><a href="{{.MagicLinkURL}}" style="` + buttonStyle + `">{{t "email.magicLink.button"}}</a></p>
<p style="margin:0 0 8px;font-size:13px;color:#6b7280;">{{t "email.common.manualLinkHint"}}</p>
<p style="margin:0 0 24px;font-size:13px;word-break:break-all;"><a href="{{.MagicLinkURL}}" style="color:#2874bb;text-decoration:underline;">{{.MagicLinkURL}}</a></p>
<p style="margin:0;font-size:13px;color:#6b7280;">{{t "email.magicLink.note"}}</p>
` + emailShellClose

const magicLinkText = `{{t "email.common.greeting" "name" .FirstName}}

{{t "email.magicLink.body" "expires" .ExpiresIn}}

{{.MagicLinkURL}}

{{t "email.magicLink.note"}}
`

const emailVerificationHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">{{t "email.verification.heading"}}</h1>
<p style="margin:0 0 8px;">{{t "email.common.greeting" "name" .FirstName}}</p>
<p style="margin:0 0 24px;">{{t "email.verification.body"}}</p>
<p style="margin:0 0 24px;"><a href="{{.VerificationURL}}" style="` + buttonStyle + `">{{t "email.verification.button"}}</a></p>
<p style="margin:0 0 8px;font-size:13px;color:#6b7280;">{{t "email.common.manualLinkHint"}}</p>
<p style="margin:0 0 24px;font-size:13px;word-break:break-all;"><a href="{{.VerificationURL}}" style="color:#2874bb;text-decoration:underline;">{{.VerificationURL}}</a></p>
<p style="margin:0;font-size:13px;color:#6b7280;">{{t "email.verification.note" "expires" .ExpiresIn}}</p>
` + emailShellClose

const emailVerificationText = `{{t "email.common.greeting" "name" .FirstName}}

{{t "email.verification.body"}}

{{.VerificationURL}}

{{t "email.verification.note" "expires" .ExpiresIn}}
`

const invitationHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">{{t "email.invitation.heading"}}</h1>
<p style="margin:0 0 8px;">{{t "email.common.greeting" "name" .FirstName}}</p>
<p style="margin:0 0 24px;">{{t "email.invitation.body"}}</p>
<p style="margin:0 0 24px;"><a href="{{.InvitationURL}}" style="` + buttonStyle + `">{{t "email.invitation.button"}}</a></p>
<p style="margin:0 0 8px;font-size:13px;color:#6b7280;">{{t "email.common.manualLinkHint"}}</p>
<p style="margin:0 0 24px;font-size:13px;word-break:break-all;"><a href="{{.InvitationURL}}" style="color:#2874bb;text-decoration:underline;">{{.InvitationURL}}</a></p>
<p style="margin:0;font-size:13px;color:#6b7280;">{{t "email.invitation.note" "expires" .ExpiresIn}}</p>
` + emailShellClose

const invitationText = `{{t "email.common.greeting" "name" .FirstName}}

{{t "email.invitation.body"}}

{{.InvitationURL}}

{{t "email.invitation.note" "expires" .ExpiresIn}}
`

const notificationBatchHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">{{if eq .NotificationCount 1}}{{t "email.notificationBatch.headingOne"}}{{else}}{{t "email.notificationBatch.headingOther" "count" .NotificationCount}}{{end}}</h1>
<p style="margin:0 0 24px;">{{t "email.notificationBatch.intro" "name" .UserName}}</p>
{{range .Notifications}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 12px;border:1px solid #eef0f3;border-left:3px solid {{.AccentColor}};border-radius:8px;background:#fafbfc;">
<tr><td style="padding:14px 16px;">
<div style="font-weight:600;font-size:14px;color:#0f172a;margin-bottom:4px;">{{if .ActionURL}}<a href="{{.ActionURL}}" style="color:#0f172a;text-decoration:none;">{{.Title}}</a>{{else}}{{.Title}}{{end}}</div>
<div style="font-size:13px;color:#374151;line-height:1.5;">{{.Message}}</div>
<div style="font-size:12px;color:#9ca3af;margin-top:8px;">{{.FormattedTime}}</div>
{{if .ActionURL}}<div style="font-size:13px;margin-top:8px;"><a href="{{.ActionURL}}" style="color:#2874bb;text-decoration:underline;">{{t "email.common.openInWindshift"}}</a></div>{{end}}
</td></tr>
</table>
{{end}}
<p style="margin:24px 0 0;font-size:13px;color:#6b7280;">{{t "email.notificationBatch.manage"}}</p>
` + emailShellClose

const notificationBatchText = `{{t "email.common.greeting" "name" .UserName}}

{{if eq .NotificationCount 1}}{{t "email.notificationBatch.headingOne"}}{{else}}{{t "email.notificationBatch.headingOther" "count" .NotificationCount}}{{end}}:

{{range .Notifications}}* {{.Title}}
  {{.Message}}
  {{.FormattedTime}}
{{if .ActionURL}}  {{t "email.common.openInWindshift"}}: {{.ActionURL}}
{{end}}
{{end}}
{{t "email.notificationBatch.manage"}}
`

const mentionHTML = emailShellOpen + `<p style="margin:0 0 8px;">{{t "email.common.greeting" "name" .RecipientName}}</p>
<p style="margin:0 0 16px;font-size:14px;color:#6b7280;">{{t "email.mention.body" "actor" .ActorName "source" .SourceLabel "itemKey" .ItemKey "itemTitle" .ItemTitle}}</p>
<div class="ws-md" style="font-size:15px;line-height:1.6;color:#1f2937;border-left:3px solid #a855f7;background:#fafbfc;border-radius:6px;padding:16px;">{{markdown .Content}}</div>
{{if .ActionURL}}<p style="margin:20px 0 0;"><a href="{{.ActionURL}}" style="` + buttonStyle + `">{{t "email.common.openInWindshift"}}</a></p>{{end}}
` + emailShellClose

const mentionText = `{{t "email.common.greeting" "name" .RecipientName}}

{{t "email.mention.body" "actor" .ActorName "source" .SourceLabel "itemKey" .ItemKey "itemTitle" .ItemTitle}}

─────────────────────────────
{{plainMentions .Content}}
─────────────────────────────
{{if .ActionURL}}
{{t "email.common.openInWindshift"}}: {{.ActionURL}}
{{end}}
`

const portalReplyHTML = emailShellOpen + `<p style="margin:0 0 16px;font-size:14px;color:#6b7280;">{{t "email.portalReply.body" "author" .AuthorName "itemKey" .ItemKey "itemTitle" .ItemTitle}}</p>
<div class="ws-md" style="font-size:15px;line-height:1.6;color:#1f2937;border-left:3px solid #2874bb;background:#fafbfc;border-radius:6px;padding:16px;">{{markdown .Content}}</div>
{{if .PortalURL}}<p style="margin:16px 0 0;font-size:13px;"><a href="{{.PortalURL}}" style="color:#2874bb;text-decoration:underline;">{{t "email.portalReply.portalLink"}}</a></p>{{end}}
<p style="margin:24px 0 0;font-size:13px;color:#6b7280;">{{t "email.portalReply.note"}}</p>
` + emailShellCloseRepliable

const portalReplyText = `{{t "email.portalReply.body" "author" .AuthorName "itemKey" .ItemKey "itemTitle" .ItemTitle}}

─────────────────────────────
{{plainMentions .Content}}
─────────────────────────────
{{if .PortalURL}}
{{t "email.portalReply.portalLink"}}: {{.PortalURL}}
{{end}}
{{t "email.portalReply.note"}}
`

const intakeConfirmationHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">{{t "email.intake.heading"}}</h1>
<p style="margin:0 0 8px;">{{t "email.common.greeting" "name" .FirstName}}</p>
<p style="margin:0 0 8px;">{{t "email.intake.body" "itemKey" .ItemKey "itemTitle" .ItemTitle}}</p>
<p style="margin:0 0 24px;">{{t "email.intake.body2"}}</p>
{{if .PortalURL}}<p style="margin:0 0 16px;">{{t "email.intake.body3"}}</p>
<p style="margin:0 0 24px;"><a href="{{.PortalURL}}" style="` + buttonStyle + `">{{t "email.intake.button"}}</a></p>{{end}}
<p style="margin:0;font-size:13px;color:#6b7280;">{{t "email.intake.note"}}</p>
` + emailShellCloseRepliable

const intakeConfirmationText = `{{t "email.common.greeting" "name" .FirstName}}

{{t "email.intake.body" "itemKey" .ItemKey "itemTitle" .ItemTitle}}

{{t "email.intake.body2"}}
{{if .PortalURL}}
{{t "email.intake.body3"}}
{{.PortalURL}}
{{end}}
{{t "email.intake.note"}}
`

const approvalRequestedHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">{{t "email.approval.heading"}}</h1>
<p style="margin:0 0 8px;">{{t "email.common.greeting" "name" .FirstName}}</p>
<p style="margin:0 0 8px;">{{t "email.approval.body" "itemKey" .ItemKey "itemTitle" .ItemTitle}}</p>
<p style="margin:0 0 24px;">{{t "email.approval.body2" "expires" .ExpiresIn}}</p>
<p style="margin:0 0 24px;"><a href="{{.ApprovalURL}}" style="` + buttonStyle + `">{{t "email.approval.button"}}</a></p>
<p style="margin:0 0 8px;font-size:13px;color:#6b7280;">{{t "email.common.manualLinkHint"}}</p>
<p style="margin:0 0 24px;font-size:13px;word-break:break-all;"><a href="{{.ApprovalURL}}" style="color:#2874bb;text-decoration:underline;">{{.ApprovalURL}}</a></p>
<p style="margin:0;font-size:13px;color:#6b7280;">{{t "email.approval.note"}}</p>
` + emailShellClose

const approvalRequestedText = `{{t "email.common.greeting" "name" .FirstName}}

{{t "email.approval.body" "itemKey" .ItemKey "itemTitle" .ItemTitle}}

{{t "email.approval.body2" "expires" .ExpiresIn}}

{{.ApprovalURL}}

{{t "email.approval.note"}}
`
