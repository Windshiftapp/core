// Display order for milestone comments: newest first, so a new comment
// lands right below the composer. The API returns pages newest first; this
// keeps that order once older pages and newly posted comments are merged in.

function createdTime(comment) {
  const time = Date.parse(comment?.created_at ?? '');
  return Number.isNaN(time) ? 0 : time;
}

/**
 * A newest-first copy of the comments: by creation time, then id, both
 * descending, the API's feed order. Editing a comment
 * leaves its creation time alone, so it keeps its place.
 */
export function newestCommentsFirst(comments) {
  return [...(comments ?? [])].sort(
    (a, b) => createdTime(b) - createdTime(a) || (b.id ?? 0) - (a.id ?? 0)
  );
}
