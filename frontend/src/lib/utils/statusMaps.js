/**
 * Domain status → display color maps shared across views.
 *
 * These are deliberately separate from `statusColors.js`, which operates on
 * workflow status objects/categories. The maps here cover small, fixed status
 * vocabularies (document processing jobs, action log runs) that would
 * otherwise be re-declared per view and drift apart.
 */

/** Document / processing-job status → `Lozenge` color name. */
export function getDocumentStatusColor(status) {
  switch (status) {
    case 'pending':
      return 'grey';
    case 'processing':
      return 'blue';
    case 'ready':
      return 'green';
    case 'error':
      return 'red';
    default:
      return 'grey';
  }
}

/** Action log run status → CSS custom-property color. */
export function getActionLogStatusColor(status) {
  switch (status) {
    case 'completed':
      return 'var(--ds-success)';
    case 'failed':
      return 'var(--ds-danger)';
    case 'running':
      return 'var(--ds-accent-blue)';
    default:
      return 'var(--ds-text-subtlest)';
  }
}
