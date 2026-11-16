import Bell from '@lucide/svelte/icons/bell';
import Bot from '@lucide/svelte/icons/bot';
import Box from '@lucide/svelte/icons/box';
import Code from '@lucide/svelte/icons/code';
import Database from '@lucide/svelte/icons/database';
import FileText from '@lucide/svelte/icons/file-text';
import GitFork from '@lucide/svelte/icons/git-fork';
import Globe from '@lucide/svelte/icons/globe';
import HelpCircle from '@lucide/svelte/icons/help-circle';
import MessageSquare from '@lucide/svelte/icons/message-square';
import Milestone from '@lucide/svelte/icons/milestone';
import Pencil from '@lucide/svelte/icons/pencil';
import PlusSquare from '@lucide/svelte/icons/plus-square';
import RefreshCw from '@lucide/svelte/icons/refresh-cw';
import ScanText from '@lucide/svelte/icons/scan-text';
import Send from '@lucide/svelte/icons/send';
import Tag from '@lucide/svelte/icons/tag';
import UsersRound from '@lucide/svelte/icons/users-round';
import Zap from '@lucide/svelte/icons/zap';

export const actionNodeDefinitions = {
  trigger: { icon: Zap, accent: 'blue', layout: 'trigger', labelKey: 'trigger' },
  set_field: { icon: Pencil, accent: 'purple', layout: 'step', labelKey: 'setField' },
  set_status: { icon: RefreshCw, accent: 'teal', layout: 'step', labelKey: 'setStatus' },
  add_comment: { icon: MessageSquare, accent: 'orange', layout: 'step', labelKey: 'addComment' },
  notify_user: { icon: Bell, accent: 'purple', layout: 'step', labelKey: 'notifyUser' },
  condition: { icon: HelpCircle, accent: 'yellow', layout: 'condition', labelKey: 'condition' },
  update_asset: { icon: Database, accent: 'teal', layout: 'step', labelKey: 'updateAsset' },
  create_asset: { icon: PlusSquare, accent: 'green', layout: 'step', labelKey: 'createAsset' },
  create_item: { icon: FileText, accent: 'green', layout: 'step', labelKey: 'createItem' },
  related_items: { icon: GitFork, accent: 'blue', layout: 'step', labelKey: 'relatedItems' },
  transition_item: { icon: RefreshCw, accent: 'teal', layout: 'step', labelKey: 'transitionItem' },
  round_robin_assign: {
    icon: UsersRound,
    accent: 'purple',
    layout: 'step',
    labelKey: 'roundRobinAssign',
  },
  container_run: { icon: Box, accent: 'blue', layout: 'step', labelKey: 'containerRun' },
  http_request: { icon: Globe, accent: 'teal', layout: 'step', labelKey: 'httpRequest' },
  ai_extract: { icon: ScanText, accent: 'purple', layout: 'step', labelKey: 'aiExtract' },
  ai_agent: { icon: Bot, accent: 'purple', layout: 'step', labelKey: 'aiAgent' },
  create_milestone: {
    icon: Milestone,
    accent: 'green',
    layout: 'step',
    labelKey: 'createMilestone',
  },
  notify_customer: { icon: Send, accent: 'purple', layout: 'step', labelKey: 'notifyCustomer' },
  insert_canned_response: {
    icon: FileText,
    accent: 'orange',
    layout: 'step',
    labelKey: 'insertCannedResponse',
  },
  adjust_labels: { icon: Tag, accent: 'purple', layout: 'step', labelKey: 'adjustLabels' },
  execute_script: { icon: Code, accent: 'teal', layout: 'step', labelKey: 'executeScript' },
};

const triggerLabelKeys = {
  status_transition: 'statusTransition',
  status_loop: 'statusLoop',
  item_created: 'itemCreated',
  item_updated: 'itemUpdated',
  item_linked: 'itemLinked',
  sla_breached: 'slaBreached',
  sla_warning: 'slaWarning',
  manual: 'manual',
  comment_created: 'commentCreated',
  item_inactive: 'itemInactive',
  scm_tag_created: 'scmTagCreated',
  scm_release_branch_created: 'scmReleaseBranchCreated',
  scm_pr_linked: 'scmPrLinked',
  scm_pr_merged: 'scmPrMerged',
  asset_created: 'assetCreated',
  asset_updated: 'assetUpdated',
  asset_status_changed: 'assetStatusChanged',
};

export function nodePresentation(type, t, catalogLabel = type) {
  const definition = Object.hasOwn(actionNodeDefinitions, type)
    ? actionNodeDefinitions[type]
    : null;
  if (!definition) return { icon: HelpCircle, accent: 'gray', layout: null, label: catalogLabel };
  const key = `actions.nodes.${definition.labelKey}`;
  const label = t(key);
  return { ...definition, label: label === key ? catalogLabel : label };
}

export function triggerLabel(type, t, catalogLabel = type) {
  const key = Object.hasOwn(triggerLabelKeys, type) && `actions.trigger.${triggerLabelKeys[type]}`;
  if (!key) return catalogLabel;
  const label = t(key);
  return label === key ? catalogLabel : label;
}

export function minimapNodeColor(node) {
  const accent = Object.hasOwn(actionNodeDefinitions, node.type)
    ? actionNodeDefinitions[node.type].accent
    : null;
  return accent ? `var(--ds-accent-${accent}-subtle)` : 'var(--ds-surface-sunken)';
}

export function minimapNodeStroke(node) {
  const accent = Object.hasOwn(actionNodeDefinitions, node.type)
    ? actionNodeDefinitions[node.type].accent
    : null;
  return accent ? `var(--ds-accent-${accent})` : 'var(--ds-border)';
}
