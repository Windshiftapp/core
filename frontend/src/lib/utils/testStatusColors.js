/**
 * Centralized test status colors using design system tokens
 * All colors adapt to light/dark mode via CSS variables
 */

export const TEST_STATUS = {
  NOT_RUN: 'not_run',
  PASSED: 'passed',
  FAILED: 'failed',
  BLOCKED: 'blocked',
  SKIPPED: 'skipped',
  IN_PROGRESS: 'in_progress',
  COMPLETED: 'completed'
};

/**
 * Get status badge styles (inline CSS for theme support)
 */
export function getStatusBadgeStyle(status) {
  const styles = {
    not_run: {
      background: 'var(--ds-status-neutral-bg)',
      color: 'var(--ds-status-neutral-text)',
      border: 'var(--ds-status-neutral-border)'
    },
    passed: {
      background: 'var(--ds-status-success-bg)',
      color: 'var(--ds-status-success-text)',
      border: 'var(--ds-status-success-border)'
    },
    failed: {
      background: 'var(--ds-status-danger-bg)',
      color: 'var(--ds-status-danger-text)',
      border: 'var(--ds-status-danger-border)'
    },
    blocked: {
      background: 'var(--ds-status-warning-bg)',
      color: 'var(--ds-status-warning-text)',
      border: 'var(--ds-status-warning-border)'
    },
    skipped: {
      background: 'var(--ds-status-neutral-bg)',
      color: 'var(--ds-status-neutral-text)',
      border: 'var(--ds-status-neutral-border)'
    },
    in_progress: {
      background: 'var(--ds-status-info-bg)',
      color: 'var(--ds-status-info-text)',
      border: 'var(--ds-status-info-border)'
    },
    completed: {
      background: 'var(--ds-status-success-bg)',
      color: 'var(--ds-status-success-text)',
      border: 'var(--ds-status-success-border)'
    }
  };
  return styles[status] || styles.not_run;
}

/**
 * Get CSS string for status badge
 */
export function getStatusBadgeCSS(status) {
  const style = getStatusBadgeStyle(status);
  return `background-color: ${style.background}; color: ${style.color}; border: 1px solid ${style.border};`;
}

/**
 * Get status button styles (for Pass/Fail/Blocked/Skip buttons)
 */
export function getStatusButtonStyle(status, isSelected) {
  const colors = {
    passed: { active: 'var(--ds-status-success-solid)', border: 'var(--ds-status-success-border)', text: 'var(--ds-status-success-text)' },
    failed: { active: 'var(--ds-status-danger-solid)', border: 'var(--ds-status-danger-border)', text: 'var(--ds-status-danger-text)' },
    blocked: { active: 'var(--ds-status-warning-solid)', border: 'var(--ds-status-warning-border)', text: 'var(--ds-status-warning-text)' },
    skipped: { active: 'var(--ds-status-neutral-solid)', border: 'var(--ds-status-neutral-border)', text: 'var(--ds-status-neutral-text)' }
  };

  const color = colors[status] || colors.skipped;

  if (isSelected) {
    return `background-color: ${color.active}; color: white; border: 1px solid ${color.active};`;
  }
  return `background-color: transparent; color: ${color.text}; border: 1px solid ${color.border};`;
}

/**
 * Get hover style for status buttons
 */
export function getStatusButtonHoverStyle(status) {
  const colors = {
    passed: 'var(--ds-status-success-bg)',
    failed: 'var(--ds-status-danger-bg)',
    blocked: 'var(--ds-status-warning-bg)',
    skipped: 'var(--ds-status-neutral-bg)'
  };
  return `background-color: ${colors[status] || colors.skipped};`;
}

/**
 * Get human-readable status label
 */
export function getStatusLabel(status) {
  const labels = {
    not_run: 'Not Run',
    passed: 'Passed',
    failed: 'Failed',
    blocked: 'Blocked',
    skipped: 'Skipped',
    in_progress: 'In Progress',
    completed: 'Completed'
  };
  return labels[status] || status;
}

/**
 * Generate status badge HTML for DataTable render functions
 */
export function renderStatusBadge(status) {
  const style = getStatusBadgeCSS(status);
  const label = getStatusLabel(status);
  return `<span class="inline-flex px-2 py-1 text-xs font-semibold rounded-full" style="${style}">${label}</span>`;
}

/**
 * Generate milestone badge HTML for DataTable render functions
 */
export function renderMilestoneBadge(name) {
  if (!name) {
    return `<span style="color: var(--ds-text-subtle);">No milestone</span>`;
  }
  return `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium" style="background-color: var(--ds-status-info-bg); color: var(--ds-status-info-text);">${name}</span>`;
}
