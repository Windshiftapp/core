// Mention identity helpers.
//
// New mentions embed the user id as `@{Display Name#123}`. The id is
// authoritative, so a mention resolves even when a display name is duplicated,
// and the content never has to contain a username (which may be an email
// address). Legacy `@username` and `@"Display Name"` mentions still resolve
// against a roster; a display name only resolves when exactly one entry
// matches.

export function displayNameOf(user) {
  return `${user?.first_name || ''} ${user?.last_name || ''}`.trim();
}

function normalize(value) {
  return String(value ?? '')
    .trim()
    .toLowerCase();
}

/**
 * Resolve a legacy mention identifier against a user roster.
 *
 * Usernames win outright because they are unique. A display name only resolves
 * when exactly one roster entry matches; otherwise every match is returned so
 * the caller can show the candidates rather than notify the wrong person.
 *
 * @returns {{ user: object | null, matches: object[] }}
 */
export function resolveMentionUser(users, identifier) {
  const needle = normalize(identifier);
  if (!needle) return { user: null, matches: [] };

  const roster = users || [];
  const byUsername = roster.find((user) => normalize(user?.username) === needle);
  if (byUsername) return { user: byUsername, matches: [byUsername] };

  const matches = roster.filter((user) => normalize(displayNameOf(user)) === needle);
  return { user: matches.length === 1 ? matches[0] : null, matches };
}

/**
 * Build the canonical mention token for a selected user.
 *
 * The id is authoritative; the display name is only a readable hint. A user
 * with no display name falls back to their username, the only label available.
 * Users without an id (should not happen for a roster entry) fall back to the
 * legacy name syntax.
 */
export function mentionTextFor(user) {
  const id = user?.id;
  const displayName = displayNameOf(user);
  if (!Number.isInteger(id) || id <= 0) {
    if (displayName) return displayName.includes(' ') ? `@"${displayName}"` : `@${displayName}`;
    return `@${user?.username || 'user'}`;
  }
  return `@{${displayName || user?.username || 'user'}#${id}}`;
}
