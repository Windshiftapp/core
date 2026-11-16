import { getActionNotificationRecipients } from '../../../stores/createActionFlowStore.svelte.js';
import { conditionOperatorLabel } from '../conditionOperators.js';
import { triggerLabel } from './actionNodeDefinitions.js';
import {
  conditionFieldScope,
  getFieldSelectorValue,
  stripConditionScope,
} from './fieldNameMapping.js';

export const PREVIEW_CARD_WIDTH = 280;
export const PREVIEW_VALUE_LINES = 2;
export const PREVIEW_LIST_ENTRIES = 3;
export const PREVIEW_DETAIL_LIMIT = 1000;
const COMPACT_LIMIT = 120;

function text(value) {
  if (value == null) return '';
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch {
      return '';
    }
  }
  return String(value);
}

function list(value) {
  return Array.isArray(value) ? value : [];
}
function object(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
}

function reference(id, lookup, kind, t, hint = '') {
  if (id == null || id === '' || id === 0) return t('actions.preview.notSelected');
  const entity = list(lookup?.state === 'ready' ? lookup.items : []).find(
    (entry) => String(entry.customFieldId ?? entry.custom_field_id ?? entry.id) === String(id)
  );
  const name =
    entity?.name ||
    entity?.display_name ||
    `${entity?.first_name || ''} ${entity?.last_name || ''}`.trim() ||
    entity?.username;
  if (name) return name;
  const state = lookup?.state === 'loading' ? 'loadingName' : 'unavailableName';
  return `${hint || `${t(`actions.preview.${kind}`)} #${text(id)}`} (${t(`actions.preview.${state}`)})`;
}

function safeURL(value, t) {
  if (!value) return t('actions.preview.notSelected');
  try {
    if (typeof value !== 'string' || /[{}]/.test(value)) throw new Error('template');
    const url = new URL(value);
    if (!['http:', 'https:'].includes(url.protocol)) throw new Error('scheme');
    url.username = '';
    url.password = '';
    url.hash = '';
    for (const key of [...url.searchParams.keys()]) url.searchParams.set(key, '…');
    return url.toString();
  } catch {
    return t('actions.preview.configuredURL');
  }
}

export function buildNodePreview({ type, config, triggerType }, editorData = {}, t) {
  const c = object(config);
  const d = editorData;
  const p = (key, params = {}) => t(`actions.preview.${key}`, params);
  const rows = [];
  function safeDetail(full) {
    const suffix = `… ${p('truncated')}`;
    return full.length > PREVIEW_DETAIL_LIMIT
      ? `${full.slice(0, PREVIEW_DETAIL_LIMIT - suffix.length)}${suffix}`
      : full;
  }
  function row(key, value, labelKey = key) {
    const full = text(value);
    const compact = full.length > COMPACT_LIMIT ? `${full.slice(0, COMPACT_LIMIT - 1)}…` : full;
    const detail = safeDetail(full);
    rows.push({ key, label: p(labelKey), value: compact, ...(full !== compact ? { detail } : {}) });
  }
  function collection(key, values) {
    const entries = list(values).map(text);
    const compact = entries
      .slice(0, PREVIEW_LIST_ENTRIES)
      .map((entry) => (entry.length > 28 ? `${entry.slice(0, 27)}…` : entry))
      .join(', ');
    const more = entries.length - PREVIEW_LIST_ENTRIES;
    row(
      key,
      entries.length ? `${compact}${more > 0 ? `; ${p('more', { count: more })}` : ''}` : p('none')
    );
    const full = entries.join(', ');
    if (entries.length && full !== compact) rows[rows.length - 1].detail = safeDetail(full);
  }
  const ref = (id, scope, kind, hint = '') => reference(id, scope, kind, t, hint);
  const status = (id) => ref(id, d.statuses, 'status');
  function field(name, hint = '', fields = d.customFields) {
    if (!name) return hint || p('notSelected');
    const key = text(name);
    const customId = key.replace(/^(custom_field_|cf_)/, '');
    const custom = list(fields?.state === 'ready' ? fields.items : []).find(
      (entry) =>
        String(entry.customFieldId ?? entry.custom_field_id ?? entry.id) === customId ||
        entry.field_name === key
    );
    if (custom) return custom.name || custom.display_name || custom.field_name;
    if (/^(custom_field_|cf_)\d+$/.test(key) || /^\d+$/.test(key))
      return ref(customId, fields, 'customField', hint);
    if (
      d.domain === 'asset' &&
      ['asset_title', 'asset_tag', 'asset_type_name', 'asset_status_name'].includes(key)
    )
      return p(key);
    const resolved = getFieldSelectorValue({ field_name: key }, t)?.name;
    return resolved === key ? hint || key : resolved || hint || key;
  }
  const value = (v) => (v == null || v === '' ? p('clearValue') : text(v));
  const boolean = (v) =>
    v == null || typeof v === 'boolean'
      ? t(`actions.condition.${v === true}`)
      : p('unsupportedValue', { value: text(v) });
  const sourceAssetField = c.source_field_id
    ? list(d.assetReferenceFields?.state === 'ready' ? d.assetReferenceFields.items : []).find(
        (entry) =>
          String(entry.id) === String(c.source_field_id) || entry.field_name === c.source_field_id
      )
    : undefined;
  const assetSetScope =
    c.asset_set_id ||
    (type === 'update_asset' ? object(sourceAssetField?.field_config).asset_set_id : 0);
  const assetScope = d.assetDestinations?.[String(assetSetScope)] || {};
  const assetFields = d.assetFieldsByType?.[`${assetSetScope}:${c.asset_type_id}`];
  function mappings() {
    collection(
      'mappings',
      list(c.field_mappings).map((mapping) => {
        const entry = object(mapping);
        const source =
          entry.source_type === 'item_field'
            ? field(entry.source_value)
            : value(entry.source_value);
        const sourceType = ['item_field', 'literal', 'variable'].includes(entry.source_type)
          ? p(entry.source_type)
          : p('unsupportedValue', { value: text(entry.source_type) });
        return `${field(entry.target_field_id, '', assetFields)} ← ${sourceType}: ${source}`;
      })
    );
  }
  function assetDestination() {
    row(
      'assetSet',
      type === 'update_asset' && !c.asset_set_id
        ? p('anyAssetSet')
        : ref(c.asset_set_id, d.assetSets, 'assetSet')
    );
    row(
      'assetType',
      type === 'update_asset' && !c.asset_type_id
        ? p('anyAssetType')
        : ref(c.asset_type_id, assetScope.types, 'assetType')
    );
  }
  function model() {
    row('model', ref(c.capability_id, d.capabilitiesByType?.llm_connection, 'capability'));
  }
  function output() {
    row('output', c.output_field || p('notSelected'));
  }

  switch (type) {
    case 'trigger':
      row('trigger', triggerLabel(triggerType, t));
      if (triggerType === 'status_transition' || triggerType === 'asset_status_changed') {
        row('from', c.from_status_id ? status(c.from_status_id) : p('anyStatus'));
        let destination = c.to_status_id ? status(c.to_status_id) : p('anyStatus');
        if (d.domain !== 'asset' && typeof c.to_status_category_completed === 'boolean') {
          const category = p(
            c.to_status_category_completed ? 'anyCompletedStatus' : 'anyNonCompletedStatus'
          );
          destination = c.to_status_id ? `${destination}; ${category}` : category;
        }
        row('to', destination);
      } else if (triggerType === 'status_loop') {
        row('to', c.to_status_id ? status(c.to_status_id) : p('anyStatus'));
      } else if (triggerType === 'item_updated') {
        row('field', c.field_name ? field(c.field_name) : p('anyField'));
      }
      if (d.domain === 'asset') {
        row(
          triggerType === 'manual' ? 'fieldScope' : 'assetType',
          c.asset_type_id ? ref(c.asset_type_id, d.assetTypes, 'assetType') : p('anyAssetType')
        );
      } else if (
        ['status_loop', 'item_created', 'item_updated', 'item_inactive'].includes(triggerType)
      ) {
        row(
          'itemType',
          c.item_type_id ? ref(c.item_type_id, d.itemTypes, 'itemType') : p('anyItemType')
        );
      }
      if (triggerType === 'item_linked')
        row(
          'linkType',
          c.link_type_id ? ref(c.link_type_id, d.linkTypes, 'linkType') : p('anyLinkType')
        );
      if (triggerType === 'comment_created')
        row(
          'author',
          p(
            c.from_customer === true
              ? 'customer'
              : c.from_customer === false
                ? 'agent'
                : 'anyAuthor'
          )
        );
      if (triggerType === 'item_inactive')
        row(
          'inactiveHours',
          Number(c.inactive_hours) > 0
            ? p('hours', { count: c.inactive_hours })
            : p('invalidThreshold')
        );
      if (triggerType === 'manual') {
        row('invocation', p('humanInvoked'));
        if (d.domain !== 'asset') {
          const roles = list(d.allowedRoleIds);
          if (roles.length)
            collection(
              'access',
              roles.map((id) => ref(id, d.roles, 'role'))
            );
          else row('access', t('actions.manualAccess.allEditors'));
        }
      } else if (text(triggerType).startsWith('scm_')) {
        row(
          'repository',
          c.workspace_repository_id
            ? ref(c.workspace_repository_id, d.repositories, 'repository')
            : p('anyRepository')
        );
        if (c.repository_full_name) row('repositoryName', c.repository_full_name);
      } else if (
        [
          'status_transition',
          'status_loop',
          'item_created',
          'item_updated',
          'item_linked',
          'sla_breached',
          'sla_warning',
          'comment_created',
          'item_inactive',
          'asset_created',
          'asset_updated',
          'asset_status_changed',
        ].includes(triggerType)
      ) {
        row('cascades', boolean(c.respond_to_cascades));
      }
      break;
    case 'set_field':
      row(
        'field',
        c.target === 'custom_field' && c.custom_field_id
          ? ref(c.custom_field_id, d.customFields, 'customField', c.field_display_name)
          : field(c.field_name, c.field_display_name)
      );
      if (['milestone_id', 'milestone_ids'].includes(c.field_name)) {
        let ids = c.value;
        if (typeof ids === 'string') {
          try {
            ids = JSON.parse(ids);
          } catch {
            /* Templates remain text. */
          }
        }
        if (Array.isArray(ids))
          collection(
            'value',
            ids.map((id) => ref(id, d.milestones, 'milestone'))
          );
        else row('value', value(c.value));
        if (Array.isArray(ids) && ids.length === 0)
          rows[rows.length - 1].value = p('clearMilestones');
      } else if (['assignee_id', 'creator_id'].includes(c.field_name) || c.field_type === 'user') {
        row(
          'value',
          c.value == null || c.value === '' || c.value === 0 || c.value === '0'
            ? p('unassigned')
            : /^\d+$/.test(text(c.value))
              ? ref(c.value, d.users, 'user', c.value_display_name)
              : value(c.value)
        );
      } else if (c.field_name === 'status_id' && /^\d+$/.test(text(c.value)))
        row('value', status(c.value));
      else if (c.field_name === 'item_type_id' && /^\d+$/.test(text(c.value)))
        row('value', ref(c.value, d.itemTypes, 'itemType', c.value_display_name));
      else row('value', c.value_display_name || value(c.value));
      break;
    case 'set_status':
      row('target', status(c.status_id));
      break;
    case 'transition_item': {
      const target = object(c.target);
      if (!target.mode || target.mode === 'explicit') row('target', status(target.status_id));
      else if (target.mode === 'category_name')
        row('target', `${p('category')}: ${target.category_name || p('notSelected')}`);
      else if (target.mode === 'matching_terminal') row('target', p('matchingTerminal'));
      else row('target', p('unsupportedValue', { value: text(target.mode) }));
      if (['category_name', 'matching_terminal'].includes(target.mode))
        row('fallback', p('firstTerminal'));
      row('skipMatching', boolean(c.skip_if_already_matching ?? true));
      break;
    }
    case 'add_comment':
      row('content', c.content || p('notConfigured'));
      row('visibility', c.is_private === true ? p('private') : p('publicWithCaveat'));
      break;
    case 'notify_user': {
      const recipients = list(getActionNotificationRecipients(c));
      collection(
        'recipients',
        recipients.map((id) =>
          ['assignee', 'creator'].includes(id)
            ? t(`actions.recipients.${id}`)
            : ref(id, d.users, 'user')
        )
      );
      row('message', c.message || p('notConfigured'));
      if (c.title) row('title', c.title);
      row('includeLink', boolean(c.include_link));
      if (d.domain !== 'asset') row('linkBehavior', p('workspaceLinkCaveat'));
      break;
    }
    case 'condition':
      row('field', field(stripConditionScope(text(c.field_name))));
      row(
        'scope',
        d.domain === 'asset'
          ? p('thisAsset')
          : t(
              `actions.config.${conditionFieldScope(text(c.field_name)) === 'parent' ? 'scopeParentItem' : 'scopeThisItem'}`
            )
      );
      row('operator', conditionOperatorLabel(c.operator, t) || c.operator || p('notSelected'));
      if (!['is_empty', 'is_not_empty'].includes(c.operator)) row('value', value(c.value));
      break;
    case 'related_items':
      row(
        'relation',
        ['descendants', 'direct_children', 'ancestors', 'linked'].includes(c.relation)
          ? p(c.relation)
          : c.relation || p('notSelected')
      );
      if (c.relation === 'linked') {
        row(
          'linkType',
          c.link_type_id ? ref(c.link_type_id, d.linkTypes, 'linkType') : p('anyLinkType')
        );
        row(
          'direction',
          ['outgoing', 'incoming', 'both', undefined, ''].includes(c.link_direction)
            ? p(c.link_direction || 'both')
            : p('unsupportedValue', { value: text(c.link_direction) })
        );
      }
      row('scope', p(c.cross_workspace === true ? 'crossWorkspace' : 'sameWorkspace'));
      row('maxItems', String(Number(c.max_items) > 0 ? c.max_items : 1000));
      break;
    case 'round_robin_assign':
      row('team', ref(c.team_id, d.teams, 'team', c.team_name));
      row('skipOnLeave', boolean(c.skip_on_leave_members));
      row('useSubstitutes', boolean(c.use_leave_substitutes));
      break;
    case 'update_asset':
      row('field', field(c.source_field_id, '', d.assetReferenceFields));
      assetDestination();
      mappings();
      break;
    case 'create_asset': {
      assetDestination();
      row('title', c.title || p('notConfigured'));
      row('description', c.description || p('none'));
      row('assetTag', c.asset_tag || p('automaticTag'));
      row(
        'category',
        c.category_id ? ref(c.category_id, assetScope.categories, 'category') : p('none')
      );
      const defaultStatus = list(
        assetScope.statuses?.state === 'ready' ? assetScope.statuses.items : []
      ).find((entry) => entry.is_default);
      row(
        'initialStatus',
        c.status_id
          ? ref(c.status_id, assetScope.statuses, 'status')
          : `${p('setDefaultStatus')}${defaultStatus ? `: ${defaultStatus.name}` : ''}`
      );
      mappings();
      break;
    }
    case 'create_item':
      row(
        'workspace',
        ref(
          c.workspace_id,
          d.workspaceDestinations?.[String(c.workspace_id)]?.workspace,
          'workspace'
        )
      );
      row(
        'itemType',
        ref(c.item_type_id, d.workspaceDestinations?.[String(c.workspace_id)]?.types, 'itemType')
      );
      row('title', c.title || p('notConfigured'));
      row('description', c.description || p('none'));
      break;
    case 'create_milestone':
      row('upsertKey', c.upsert_key_template || p('notConfigured'));
      row('name', c.name_template || p('upsertNameDefault'));
      row('description', c.description_template || p('none'));
      row('branchStatus', c.status_on_branch || 'planning');
      row('tagStatus', c.status_on_tag || 'in-progress');
      row('attachRelease', boolean(c.attach_release_on_tag ?? true));
      row('attachCommitItems', boolean(c.attach_commit_issues ?? true));
      row(
        'category',
        c.category_id ? ref(c.category_id, d.milestoneCategories, 'category') : p('none')
      );
      break;
    case 'container_run':
      row(
        'capability',
        ref(c.capability_id, d.capabilitiesByType?.docker_environment, 'capability')
      );
      output();
      row(
        'timeout',
        c.pool_capability_id
          ? p('localTimeoutOnly', { count: Number(c.timeout_secs) > 0 ? c.timeout_secs : 300 })
          : p('seconds', { count: Number(c.timeout_secs) > 0 ? c.timeout_secs : 300 })
      );
      row(
        'routing',
        c.pool_capability_id
          ? ref(c.pool_capability_id, d.capabilitiesByType?.runner_pool, 'capability')
          : p('localRunner')
      );
      break;
    case 'http_request': {
      row(
        'method',
        typeof c.method === 'string' ? c.method.trim().toUpperCase() || 'GET' : c.method || 'GET'
      );
      row('url', safeURL(c.url_template, t));
      row(
        'capability',
        c.capability_id
          ? ref(c.capability_id, d.capabilitiesByType?.http_client, 'capability')
          : p('none')
      );
      output();
      let bodyType = String(typeof c.body);
      if (typeof c.body === 'string') {
        try {
          const body = JSON.parse(c.body);
          bodyType = Array.isArray(body) ? 'array' : body === null ? 'null' : typeof body;
        } catch {
          bodyType = 'text';
        }
      }
      row(
        'body',
        c.body == null || c.body === '' ? p('none') : p('configuredBody', { type: bodyType })
      );
      const headers = Object.keys(object(c.headers));
      collection('headers', headers);
      rows[rows.length - 1].label = p('headersCount', { count: headers.length });
      break;
    }
    case 'ai_extract': {
      model();
      row('input', c.input_field || p('notSelected'));
      output();
      row('prompt', c.prompt || p('notConfigured'));
      let schema = c.output_schema;
      if (typeof schema === 'string') {
        try {
          schema = JSON.parse(schema);
        } catch {
          schema = null;
        }
      }
      const properties = Object.entries(object(object(schema).properties));
      if (properties.length)
        collection(
          'schema',
          properties.map(
            ([key, definition]) => `${key}: ${object(definition).type || p('anyType')}`
          )
        );
      else
        row(
          'schema',
          schema
            ? p('schemaType', { type: object(schema).type || p('anyType') })
            : c.output_schema
              ? p('schemaConfigured')
              : p('notConfigured')
        );
      break;
    }
    case 'ai_agent':
      model();
      row('prompt', c.prompt || p('notConfigured'));
      collection('input', c.input_fields);
      collection(
        'tools',
        list(c.tools).map((id) => ref(id, d.capabilitiesByType?.http_client, 'capability'))
      );
      row('maxSteps', String(Number(c.max_steps) > 0 ? Math.min(Number(c.max_steps), 50) : 10));
      output();
      break;
    case 'notify_customer':
      row('message', c.message || p('notConfigured'));
      row('subject', c.subject || p('ticketSubject'));
      row('destination', p('customerEmail'));
      row('visibility', p('skipPrivateTrigger'));
      break;
    case 'insert_canned_response': {
      row('response', ref(c.canned_response_id, d.cannedResponses, 'response'));
      const response = list(d.cannedResponses?.items).find(
        (entry) => String(entry.id) === String(c.canned_response_id)
      );
      row(
        'visibility',
        response
          ? response.is_private
            ? p('private')
            : p('publicWithCaveat')
          : p('unavailableName')
      );
      if (response?.is_active === false) row('availability', p('inactiveResponse'));
      break;
    }
    case 'adjust_labels':
      collection(
        'addLabels',
        list(c.add_label_ids).map((id) => ref(id, d.labels, 'label'))
      );
      collection(
        'removeLabels',
        list(c.remove_label_ids).map((id) => ref(id, d.labels, 'label'))
      );
      break;
    case 'execute_script':
      row(
        'source',
        typeof c.source === 'string' && c.source.trim()
          ? p('sourceLines', { count: c.source.trim().split(/\r?\n/).length })
          : p('notConfigured')
      );
      row(
        'output',
        typeof c.output_variable === 'string' && c.output_variable.trim()
          ? c.output_variable.trim()
          : 'script_result'
      );
      row(
        'timeout',
        p('milliseconds', {
          count: Number(c.timeout_ms) > 0 && Number(c.timeout_ms) <= 30000 ? c.timeout_ms : 5000,
        })
      );
      row(
        'maxAPICalls',
        String(Number(c.max_api_calls) > 0 && Number(c.max_api_calls) <= 50 ? c.max_api_calls : 50)
      );
      break;
    default:
      row('unsupported', p('unsupportedNode', { type: text(type) }));
  }
  return { rows };
}
