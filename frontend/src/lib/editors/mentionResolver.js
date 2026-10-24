// Mention identity helpers.
//
// Display names are not unique, so a mention that only carries a display name
// cannot be resolved to a single person. Usernames are unique; these helpers
// keep new mentions unambiguous and make the hover card show the right user
// (or every candidate) instead of guessing.

export function displayNameOf(user) {
  return `${user?.first_name || ''} ${user?.last_name || ''}`.trim();
}

function normalize(value) {
  return String(value ?? '')
    .trim()
    .toLowerCase();
}

/**
 * Resolve a mention identifier against a user roster.
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
 * Pick the text to insert for a selected mention.
 *
 * A multi-word display name reads better in prose, but only the username is
 * unique. Fall back to the username when the roster holds another user with the
 * same display name.
 */
export function mentionTextFor(user, roster) {
  const handle = `@${user?.username || ''}`;
  const displayName = displayNameOf(user);
  if (!displayName) return handle;

  const isSameUser = (other) =>
    other === user || (other?.id != null && user?.id != null && other.id === user.id);
  const duplicated = (roster || []).some(
    (other) => !isSameUser(other) && normalize(displayNameOf(other)) === normalize(displayName)
  );
  if (duplicated) return handle;

  return displayName.includes(' ') ? `@"${displayName}"` : handle;
}
