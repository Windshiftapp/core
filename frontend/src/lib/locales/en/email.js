/**
 * Transactional and notification email copy.
 *
 * Rendered by the Go backend (internal/i18n) via the `t` template function, so
 * the copy lives beside the UI catalogs and is covered by the same key and
 * placeholder parity checks. Placeholders use {name} syntax.
 */
export default {
  email: {
    footer: {
      automated: 'This is an automated email — please do not reply.',
      repliable: "You're receiving this because you have an open request with us.",
    },
    common: {
      greeting: 'Hi {name},',
      openInWindshift: 'View in Windshift',
      manualLinkHint: "If the button doesn't work, copy and paste this URL into your browser:",
    },
    duration: {
      minute: '{count} minute',
      minutes: '{count} minutes',
      hour: '{count} hour',
      hours: '{count} hours',
      day: '{count} day',
      days: '{count} days',
    },
    magicLink: {
      subject: 'Sign in to your portal',
      heading: 'Sign in to your portal',
      body: 'Click the button below to sign in. The link is valid for {expires}.',
      button: 'Sign in',
      note: "If you didn't request this link, you can safely ignore this email — no one can sign in without it.",
    },
    verification: {
      subject: 'Verify your email address',
      heading: 'Verify your email',
      body: 'Please confirm your email address to finish setting up your account.',
      button: 'Verify email',
      note: "This link expires in {expires}. If you didn't create a Windshift account, you can ignore this email.",
    },
    invitation: {
      subject: "You've been invited to Windshift",
      heading: "You've been invited to Windshift",
      body: 'Set a password to activate your account and start collaborating with your team.',
      button: 'Set your password',
      note: "This invitation expires in {expires}. If you weren't expecting it, you can safely ignore this email.",
    },
    notificationBatch: {
      subjectOne: 'Windshift — You have 1 new notification',
      subjectOther: 'Windshift — You have {count} new notifications',
      headingOne: '1 new notification',
      headingOther: '{count} new notifications',
      intro: "Hi {name}, here's what's new:",
      manage: 'Manage how often you receive these emails in your notification preferences.',
    },
    mention: {
      subject: '{actor} mentioned you in {itemKey}',
      body: '{actor} mentioned you in {source} on {itemKey} · {itemTitle}',
      source: {
        comment: 'a comment',
        description: 'the description',
        item: 'an item',
      },
    },
    portalReply: {
      subject: 'Re: {subject}',
      body: '{author} replied on {itemKey} · {itemTitle}',
      portalLink: 'View this request in your portal',
      note: 'To continue the conversation, simply reply to this email.',
    },
    approval: {
      subject: 'Approval requested: {itemKey} — {itemTitle}',
      heading: 'Approval requested',
      body: 'Your approval is required on {itemKey} · {itemTitle}.',
      body2: 'Click the button below to review and decide. The link is valid for {expires}.',
      button: 'Review approval',
      note: "If you weren't expecting this request, you can safely ignore this email.",
    },
    intake: {
      subject: "We've received your submission ({itemKey})",
      heading: "We've received your submission",
      body: "Thanks for contacting us. We've logged your request as {itemKey} · {itemTitle}.",
      body2: 'Our team will review it and get back to you as soon as possible.',
      body3: 'You can follow progress and add details in your portal:',
      button: 'View your request',
      note: 'To add more information, simply reply to this email.',
    },
  },
};
