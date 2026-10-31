import { itemUrl } from '../../utils/urls.js';

// Milestone Activity: turns one feed entry from
// GET /milestones/{id}/activity into display parts. The component renders
// `actor lead [target link] trail`.

const MILESTONE_STATUS_KEYS = {
  planning: 'milestones.status.planning',
  'in-progress': 'milestones.status.inProgress',
  completed: 'milestones.status.completed',
  cancelled: 'milestones.status.cancelled',
};

/**
 * @param {any} entry - one activity entry
 * @param {{ t: (key: string, params?: Record<string, any>) => string, formatDate: (value: string) => string }} helpers
 * @returns {{ actor: string, lead: string, target: { label: string, href: string } | null, trail: string }}
 */
export function describeMilestoneActivity(entry, { t, formatDate }) {
  const actor =
    entry?.actor_kind === 'system'
      ? t('milestones.activity.system')
      : entry?.actor_name || t('milestones.activity.someone');
  const none = t('milestones.activity.none');
  const milestoneStatus = (value) => {
    if (!value) return none;
    return MILESTONE_STATUS_KEYS[value] ? t(MILESTONE_STATUS_KEYS[value]) : value;
  };
  const date = (value) => (value ? formatDate(value) || value : none);
  const itemTarget = entry?.item
    ? {
        label: [entry.item.key, entry.item.title].filter(Boolean).join(' '),
        href: itemUrl({ workspaceId: entry.item.workspace_id, itemId: entry.item.id }),
      }
    : null;
  const pageTarget = entry?.page
    ? {
        label: entry.page.title || t('pages.untitled'),
        href: `/workspaces/${entry.page.workspace_id}/pages/${entry.page.id}`,
      }
    : null;

  switch (entry?.type) {
    case 'milestone_comment_added':
      return {
        actor,
        lead: t('milestones.activity.commentedOnMilestone'),
        target: null,
        trail: '',
      };
    case 'milestone_description_changed':
      return { actor, lead: t('milestones.activity.changedDescription'), target: null, trail: '' };
    case 'milestone_status_changed':
      return {
        actor,
        lead: t('milestones.activity.changedStatus', {
          from: milestoneStatus(entry.old_value),
          to: milestoneStatus(entry.new_value),
        }),
        target: null,
        trail: '',
      };
    case 'milestone_target_date_changed':
      return {
        actor,
        lead: t('milestones.activity.changedTargetDate', {
          from: date(entry.old_value),
          to: date(entry.new_value),
        }),
        target: null,
        trail: '',
      };
    case 'milestone_page_linked':
      return { actor, lead: t('milestones.activity.linkedPage'), target: pageTarget, trail: '' };
    case 'milestone_page_unlinked':
      return { actor, lead: t('milestones.activity.unlinkedPage'), target: pageTarget, trail: '' };
    case 'item_comment_added':
      return { actor, lead: t('milestones.activity.commentedOn'), target: itemTarget, trail: '' };
    case 'item_status_changed':
      return {
        actor,
        lead: t('milestones.activity.changedItemStatus'),
        target: itemTarget,
        trail: t('milestones.activity.fromTo', {
          from: entry.old_value || none,
          to: entry.new_value || none,
        }),
      };
    case 'item_added':
      return {
        actor,
        lead: t('milestones.activity.added'),
        target: itemTarget,
        trail: t('milestones.activity.toMilestone'),
      };
    case 'item_removed':
      return {
        actor,
        lead: t('milestones.activity.removed'),
        target: itemTarget,
        trail: t('milestones.activity.fromMilestone'),
      };
    default:
      return { actor, lead: entry?.type ?? '', target: itemTarget, trail: '' };
  }
}
