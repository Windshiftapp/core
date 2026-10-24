package emailutil

// DefaultTemplate is a built-in email template used as the fallback when the
// admin-editable DB row is missing or inactive. The same content is used to
// seed the notification_templates table on a fresh install.
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
func DefaultTemplates() []DefaultTemplate {
	return []DefaultTemplate{
		{
			Name:        TemplateMagicLink,
			Description: "Sent when a portal customer requests a magic-link sign-in.",
			Subject:     "Sign in to your portal",
			HTMLBody:    magicLinkHTML,
			TextBody:    magicLinkText,
		},
		{
			Name:        TemplateEmailVerification,
			Description: "Sent to users to verify their email address.",
			Subject:     "Verify your email address",
			HTMLBody:    emailVerificationHTML,
			TextBody:    emailVerificationText,
		},
		{
			Name:        TemplateInvitation,
			Description: "Sent to invite a new user and prompt them to set a password.",
			Subject:     "You've been invited to Windshift",
			HTMLBody:    invitationHTML,
			TextBody:    invitationText,
		},
		{
			Name:        TemplateNotificationBatch,
			Description: "Sent when a user has unread notifications batched for delivery.",
			Subject:     `Windshift — {{if eq .NotificationCount 1}}You have 1 new notification{{else}}You have {{.NotificationCount}} new notifications{{end}}`,
			HTMLBody:    notificationBatchHTML,
			TextBody:    notificationBatchText,
		},
		{
			Name:        TemplateMention,
			Description: "Sent to a user who was @mentioned in a comment or item description. Includes the message body.",
			Subject:     `{{.ActorName}} mentioned you in {{.ItemKey}}`,
			HTMLBody:    mentionHTML,
			TextBody:    mentionText,
		},
		{
			Name:        TemplatePortalReply,
			Description: "Threaded reply sent to a portal customer when an internal user comments on their ticket (email or portal origin).",
			Subject:     `Re: {{.OriginalSubject}}`,
			HTMLBody:    portalReplyHTML,
			TextBody:    portalReplyText,
		},
		{
			Name:        TemplateApprovalRequested,
			Description: "Sent to a portal customer when an approval step opens that requires their decision. Includes a magic-link to the portal approval page.",
			Subject:     `Approval requested: {{.ItemKey}} — {{.ItemTitle}}`,
			HTMLBody:    approvalRequestedHTML,
			TextBody:    approvalRequestedText,
		},
		{
			Name:        TemplateIntakeConfirmation,
			Description: "Receipt sent to the sender when an email intake creates a new request. Includes a portal link when the intake exposes the request to a portal.",
			Subject:     `We've received your submission ({{.ItemKey}})`,
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
<html lang="en">
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

const emailShellClose = emailShellFooterOpen + `This is an automated email — please do not reply.` + emailShellFooterClose

// emailShellCloseRepliable is used by templates whose body asks the customer to
// reply. A do-not-reply footer would contradict that instruction.
const emailShellCloseRepliable = emailShellFooterOpen + `You're receiving this because you have an open request with us.` + emailShellFooterClose

const buttonStyle = `display:inline-block;background:#2874bb;color:#ffffff;text-decoration:none;font-weight:600;font-size:14px;padding:12px 24px;border-radius:8px;line-height:1.2;`

const magicLinkHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">Sign in to your portal</h1>
<p style="margin:0 0 8px;">Hi {{.FirstName}},</p>
<p style="margin:0 0 24px;">Click the button below to sign in. The link is valid for {{.ExpiresIn}}.</p>
<p style="margin:0 0 24px;"><a href="{{.MagicLinkURL}}" style="` + buttonStyle + `">Sign in</a></p>
<p style="margin:0 0 8px;font-size:13px;color:#6b7280;">If the button doesn't work, copy and paste this URL into your browser:</p>
<p style="margin:0 0 24px;font-size:13px;word-break:break-all;"><a href="{{.MagicLinkURL}}" style="color:#2874bb;text-decoration:underline;">{{.MagicLinkURL}}</a></p>
<p style="margin:0;font-size:13px;color:#6b7280;">If you didn't request this link, you can safely ignore this email — no one can sign in without it.</p>
` + emailShellClose

const magicLinkText = `Hi {{.FirstName}},

Click the link below to sign in to your portal. The link is valid for {{.ExpiresIn}}:

{{.MagicLinkURL}}

If you didn't request this link, you can safely ignore this email.
`

const emailVerificationHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">Verify your email</h1>
<p style="margin:0 0 8px;">Hi {{.FirstName}},</p>
<p style="margin:0 0 24px;">Please confirm your email address to finish setting up your account.</p>
<p style="margin:0 0 24px;"><a href="{{.VerificationURL}}" style="` + buttonStyle + `">Verify email</a></p>
<p style="margin:0 0 8px;font-size:13px;color:#6b7280;">If the button doesn't work, copy and paste this URL into your browser:</p>
<p style="margin:0 0 24px;font-size:13px;word-break:break-all;"><a href="{{.VerificationURL}}" style="color:#2874bb;text-decoration:underline;">{{.VerificationURL}}</a></p>
<p style="margin:0;font-size:13px;color:#6b7280;">This link expires in {{.ExpiresIn}}. If you didn't create a Windshift account, you can ignore this email.</p>
` + emailShellClose

const emailVerificationText = `Hi {{.FirstName}},

Please confirm your email address to finish setting up your account:

{{.VerificationURL}}

This link expires in {{.ExpiresIn}}. If you didn't create a Windshift account, you can ignore this email.
`

const invitationHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">You've been invited to Windshift</h1>
<p style="margin:0 0 8px;">Hi {{.FirstName}},</p>
<p style="margin:0 0 24px;">Set a password to activate your account and start collaborating with your team.</p>
<p style="margin:0 0 24px;"><a href="{{.InvitationURL}}" style="` + buttonStyle + `">Set your password</a></p>
<p style="margin:0 0 8px;font-size:13px;color:#6b7280;">If the button doesn't work, copy and paste this URL into your browser:</p>
<p style="margin:0 0 24px;font-size:13px;word-break:break-all;"><a href="{{.InvitationURL}}" style="color:#2874bb;text-decoration:underline;">{{.InvitationURL}}</a></p>
<p style="margin:0;font-size:13px;color:#6b7280;">This invitation expires in {{.ExpiresIn}}. If you weren't expecting it, you can safely ignore this email.</p>
` + emailShellClose

const invitationText = `Hi {{.FirstName}},

You've been invited to join Windshift. Set a password to activate your account:

{{.InvitationURL}}

This invitation expires in {{.ExpiresIn}}. If you weren't expecting it, you can safely ignore this email.
`

const notificationBatchHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">{{if eq .NotificationCount 1}}1 new notification{{else}}{{.NotificationCount}} new notifications{{end}}</h1>
<p style="margin:0 0 24px;">Hi {{.UserName}}, here's what's new:</p>
{{range .Notifications}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 12px;border:1px solid #eef0f3;border-left:3px solid {{.AccentColor}};border-radius:8px;background:#fafbfc;">
<tr><td style="padding:14px 16px;">
<div style="font-weight:600;font-size:14px;color:#0f172a;margin-bottom:4px;">{{if .ActionURL}}<a href="{{.ActionURL}}" style="color:#0f172a;text-decoration:none;">{{.Title}}</a>{{else}}{{.Title}}{{end}}</div>
<div style="font-size:13px;color:#374151;line-height:1.5;">{{.Message}}</div>
<div style="font-size:12px;color:#9ca3af;margin-top:8px;">{{.FormattedTime}}</div>
{{if .ActionURL}}<div style="font-size:13px;margin-top:8px;"><a href="{{.ActionURL}}" style="color:#2874bb;text-decoration:underline;">View in Windshift</a></div>{{end}}
</td></tr>
</table>
{{end}}
<p style="margin:24px 0 0;font-size:13px;color:#6b7280;">Manage how often you receive these emails in your notification preferences.</p>
` + emailShellClose

const notificationBatchText = `Hi {{.UserName}},

You have {{.NotificationCount}} new notification{{if ne .NotificationCount 1}}s{{end}} from Windshift:

{{range .Notifications}}* {{.Title}}
  {{.Message}}
  {{.FormattedTime}}
{{if .ActionURL}}  View: {{.ActionURL}}
{{end}}
{{end}}
Manage your notification preferences in Windshift.
`

const mentionHTML = emailShellOpen + `<p style="margin:0 0 8px;">Hi {{.RecipientName}},</p>
<p style="margin:0 0 16px;font-size:14px;color:#6b7280;">
<strong style="color:#0f172a;">{{.ActorName}}</strong> mentioned you in {{.SourceLabel}} on
<span style="font-family:'JetBrains Mono',ui-monospace,monospace;color:#2874bb;">{{.ItemKey}}</span>
&nbsp;·&nbsp; {{.ItemTitle}}
</p>
<div class="ws-md" style="font-size:15px;line-height:1.6;color:#1f2937;border-left:3px solid #a855f7;background:#fafbfc;border-radius:6px;padding:16px;">{{markdown .Content}}</div>
{{if .ActionURL}}<p style="margin:20px 0 0;"><a href="{{.ActionURL}}" style="` + buttonStyle + `">View in Windshift</a></p>{{end}}
` + emailShellClose

const mentionText = `Hi {{.RecipientName}},

{{.ActorName}} mentioned you in {{.SourceLabel}} on {{.ItemKey}} · {{.ItemTitle}}

─────────────────────────────
{{.Content}}
─────────────────────────────
{{if .ActionURL}}
View in Windshift: {{.ActionURL}}
{{end}}
`

const portalReplyHTML = emailShellOpen + `<p style="margin:0 0 16px;font-size:14px;color:#6b7280;">
<strong style="color:#0f172a;">{{.AuthorName}}</strong> replied on
<span style="font-family:'JetBrains Mono',ui-monospace,monospace;color:#2874bb;">{{.ItemKey}}</span>
&nbsp;·&nbsp; {{.ItemTitle}}
</p>
<div class="ws-md" style="font-size:15px;line-height:1.6;color:#1f2937;border-left:3px solid #2874bb;background:#fafbfc;border-radius:6px;padding:16px;">{{markdown .Content}}</div>
{{if .PortalURL}}<p style="margin:16px 0 0;font-size:13px;"><a href="{{.PortalURL}}" style="color:#2874bb;text-decoration:underline;">View this request in your portal</a></p>{{end}}
<p style="margin:24px 0 0;font-size:13px;color:#6b7280;">To continue the conversation, simply reply to this email.</p>
` + emailShellCloseRepliable

const portalReplyText = `{{.AuthorName}} replied on {{.ItemKey}} · {{.ItemTitle}}

─────────────────────────────
{{.Content}}
─────────────────────────────
{{if .PortalURL}}
View this request in your portal: {{.PortalURL}}
{{end}}
To continue the conversation, simply reply to this email.
`

const intakeConfirmationHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">We've received your submission</h1>
<p style="margin:0 0 8px;">Hi {{.FirstName}},</p>
<p style="margin:0 0 8px;">Thanks for contacting us. We've logged your request as
<span style="font-family:'JetBrains Mono',ui-monospace,monospace;color:#2874bb;">{{.ItemKey}}</span>
&nbsp;·&nbsp; {{.ItemTitle}}.</p>
<p style="margin:0 0 24px;">Our team will review it and get back to you as soon as possible.</p>
{{if .PortalURL}}<p style="margin:0 0 16px;">You can follow progress and add details in your portal:</p>
<p style="margin:0 0 24px;"><a href="{{.PortalURL}}" style="` + buttonStyle + `">View your request</a></p>{{end}}
<p style="margin:0;font-size:13px;color:#6b7280;">To add more information, simply reply to this email.</p>
` + emailShellCloseRepliable

const intakeConfirmationText = `Hi {{.FirstName}},

Thanks for contacting us. We've received your submission and logged it as {{.ItemKey}} · {{.ItemTitle}}.

Our team will review it and get back to you as soon as possible.
{{if .PortalURL}}
You can follow progress and add details in your portal:
{{.PortalURL}}
{{end}}
To add more information, simply reply to this email.
`

const approvalRequestedHTML = emailShellOpen + `<h1 style="margin:0 0 16px;font-size:22px;font-weight:600;color:#0f172a;letter-spacing:-0.01em;">Approval requested</h1>
<p style="margin:0 0 8px;">Hi {{.FirstName}},</p>
<p style="margin:0 0 8px;">Your approval is required on
<span style="font-family:'JetBrains Mono',ui-monospace,monospace;color:#2874bb;">{{.ItemKey}}</span>
&nbsp;·&nbsp; {{.ItemTitle}}.</p>
<p style="margin:0 0 24px;">Click the button below to review and decide. The link is valid for {{.ExpiresIn}}.</p>
<p style="margin:0 0 24px;"><a href="{{.ApprovalURL}}" style="` + buttonStyle + `">Review approval</a></p>
<p style="margin:0 0 8px;font-size:13px;color:#6b7280;">If the button doesn't work, copy and paste this URL into your browser:</p>
<p style="margin:0 0 24px;font-size:13px;word-break:break-all;"><a href="{{.ApprovalURL}}" style="color:#2874bb;text-decoration:underline;">{{.ApprovalURL}}</a></p>
<p style="margin:0;font-size:13px;color:#6b7280;">If you weren't expecting this request, you can safely ignore this email.</p>
` + emailShellClose

const approvalRequestedText = `Hi {{.FirstName}},

Your approval is required on {{.ItemKey}} · {{.ItemTitle}}.

Click the link below to review and decide. The link is valid for {{.ExpiresIn}}:

{{.ApprovalURL}}

If you weren't expecting this request, you can safely ignore this email.
`
