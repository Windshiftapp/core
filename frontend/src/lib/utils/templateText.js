/**
 * True when a description is empty or still holds `appliedBody` exactly as a
 * template applied it, i.e. it is safe to replace with another template.
 *
 * The comparison is exact (ignoring only leading/trailing whitespace): the
 * description editor only writes back after a real document change, so an
 * untouched template keeps the applied text verbatim. Anything else counts as
 * the user's own text and is never replaced.
 */
export function isUntouchedTemplateText(description, appliedBody) {
  if (!description?.trim()) return true;
  return appliedBody != null && description.trim() === appliedBody.trim();
}
