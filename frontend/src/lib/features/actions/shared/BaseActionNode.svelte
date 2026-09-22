<script>
  import { Handle } from '@xyflow/svelte';
  import { getHandlePositions } from '../nodes/flowDirection.js';
  import { t } from '../../../stores/i18n.svelte.js';
  import Badge from '../../../components/Badge.svelte';
  import StatusBadge from '../../../components/StatusBadge.svelte';

  function truncateContent(content, maxLength = 50) {
    if (!content) return '';
    return content.length > maxLength ? content.substring(0, maxLength) + '...' : content;
  }

  function getStatus(statusId) {
    if (!statusId || !data.statuses) return null;
    return data.statuses.find(s => s.id === statusId);
  }

  let {
    data = {},
    selected = false,
    flowStore,
    icon: Icon,
    title,
    accentColor = 'teal',
    colorVars = null,
    // Optional custom body snippet; falls back to the built-in body variants.
    body = null,
    // i18n key for the label next to capability_id (e.g. AI agent shows "Model")
    capabilityLabelKey = 'actions.config.capability',
    // i18n key overriding the unconfigured-state placeholder
    placeholderKey = '',
    // Common configuration patterns
    showCapabilityId = false,
    showToolsCount = false,
    showInputOutput = false,
    showHttpInfo = false,
    showConfigInfo = false,
    showFieldInfo = false,
    showAssetInfo = false,
    showCommentInfo = false,
    showStatusInfo = false,
    showMilestoneInfo = false,
    showAssignmentInfo = false,
  } = $props();

  let status = $derived(showStatusInfo && data.config?.status_id ? getStatus(data.config.status_id) : null);

  let positions = $derived(getHandlePositions(flowStore.direction));

  let colors = $derived(colorVars || {
    accent: `var(--ds-accent-${accentColor})`,
    subtle: `var(--ds-accent-${accentColor}-subtle)`,
    subtler: `var(--ds-accent-${accentColor}-subtler)`,
  });

  // Get appropriate placeholder text based on configuration
  function getPlaceholderText() {
    if (placeholderKey) return t(placeholderKey);
    if (showCapabilityId) return t('actions.config.selectModelAndTools');
    if (showInputOutput) return t('actions.config.configureExtract');
    if (showHttpInfo) return t('actions.config.configureRequest');
    if (showConfigInfo) return t('actions.config.selectConfig');
    if (showFieldInfo) return t('actions.config.configureAssetUpdate');
    if (showAssetInfo) return t('actions.config.configureAssetCreation');
    if (showCommentInfo) return t('actions.config.enterComment');
    if (showStatusInfo) return t('actions.config.selectStatus');
    if (showMilestoneInfo) return 'Configure milestone upsert';
    if (showAssignmentInfo) return 'Select team';
    return t('actions.config.configure');
  }

  // Common body rendering logic
</script>

{#snippet defaultBody()}
    {#if showCapabilityId && data.config?.capability_id}
      <div class="cap-info">
        <span class="cap-label">{t(capabilityLabelKey)}:</span>
        <span class="cap-value">#{data.config.capability_id}</span>
        {#if showToolsCount}
          <span class="tools-count">
            {data.config.tools?.length || 0} {t('actions.config.tools')}
          </span>
        {/if}
      </div>
    {:else if showInputOutput && data.config?.input_field && data.config?.output_field}
      <div class="ai-info">
        <span class="ai-label">{data.config.input_field}</span>
        <span class="ai-arrow">&rarr;</span>
        <span class="ai-label">{data.config.output_field}</span>
      </div>
    {:else if showHttpInfo && data.config?.url_template}
      <div class="http-info">
        <span class="method">{data.config.method || 'GET'}</span>
        <span class="url">{data.config.url_template}</span>
      </div>
    {:else if showConfigInfo && data.config?.config_name}
      <div class="config-info">
        <span class="config-label">{t('actions.config.config')}:</span>
        <span class="config-value">{data.config.config_name}</span>
      </div>
    {:else if showFieldInfo && data.config?.source_field_id}
      <div class="field-info">
        <span class="field-name">{data.config.source_field_id}</span>
        <span class="field-arrow">&rarr;</span>
        <span class="field-value">{t('actions.config.fieldMappings', { count: data.config.field_mappings?.length || 0 })}</span>
      </div>
    {:else if showAssetInfo && data.config?.asset_type_id && data.config?.title}
      <div class="asset-info">
        <span class="asset-name">{data.config.title}</span>
      </div>
    {:else if showCommentInfo && data.config?.content}
      <div class="comment-preview">
        {#if data.config.is_private}
          <Badge variant="warning" size="xs">{t('actions.config.private')}</Badge>
        {/if}
        <span class="comment-text">{truncateContent(data.config.content)}</span>
      </div>
    {:else if showStatusInfo && data.config?.status_id}
      {#if status}
        <StatusBadge {status} />
      {:else}
        <div class="status-id">ID: {data.config.status_id}</div>
      {/if}
    {:else if showMilestoneInfo && data.config?.upsert_key_template}
      <div class="cm-info">
        <span class="cm-label">key</span>
        <span class="cm-template">{data.config.upsert_key_template}</span>
      </div>
      {#if data.config?.name_template}
        <div class="cm-info">
          <span class="cm-label">name</span>
          <span class="cm-template">{data.config.name_template}</span>
        </div>
      {/if}
    {:else if showAssignmentInfo && data.config?.team_id}
      <div class="assignment-info">
        <span class="team-name">{data.config.team_name || `Team #${data.config.team_id}`}</span>
      </div>
    {:else}
      <div class="placeholder">{getPlaceholderText()}</div>
    {/if}
  {/snippet}

<div
  class="base-action-node action-flow-node"
  class:selected
  data-testid={`action-node-${data.nodeType || 'unknown'}`}
  style="--_accent: {colors.accent}; --_accent-subtle: {colors.subtle}; --_accent-subtler: {colors.subtler};"
>
  <Handle type="target" position={positions.input} id="input" />

  <div class="node-header">
    <Icon size={16} />
    <span class="node-title">{title}</span>
  </div>
  <div class="node-body">
    {#if body}
      {@render body()}
    {:else}
      {@render defaultBody()}
    {/if}
  </div>

  <Handle type="source" position={positions.output} id="output" />
</div>

<style>
  .base-action-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--_accent);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--_accent-subtle);
    border-bottom: 1px solid var(--_accent-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--_accent);
  }

  .node-body {
    padding: 10px 12px;
  }

  /* Common info patterns */
  .cap-info,
  .ai-info,
  .http-info,
  .config-info,
  .field-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    flex-wrap: wrap;
  }

  .ai-info {
    font-size: 12px;
  }

  .http-info {
    font-size: 11px;
  }

  .cap-label,
  .ai-label,
  .config-label {
    color: var(--ds-text-subtlest);
  }

  .cap-value {
    color: var(--ds-text);
    font-family: monospace;
    font-size: 11px;
  }

  .ai-label {
    color: var(--ds-text);
    font-family: monospace;
    font-size: 11px;
    background: var(--ds-surface-sunken);
    padding: 1px 5px;
    border-radius: 3px;
  }

  .ai-arrow {
    color: var(--ds-text-subtlest);
  }

  .method {
    color: var(--ds-text);
    font-family: monospace;
    font-weight: 600;
    background: var(--ds-surface-sunken);
    padding: 1px 5px;
    border-radius: 3px;
  }

  .url {
    color: var(--ds-text-subtle);
    font-family: monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 140px;
  }

  .config-value {
    color: var(--ds-text);
    font-family: monospace;
    font-size: 11px;
  }

  .tools-count {
    color: var(--ds-text-subtle);
    font-size: 11px;
    background: var(--ds-surface-sunken);
    padding: 1px 5px;
    border-radius: 3px;
  }

  .field-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }

  .field-name {
    color: var(--ds-text);
    font-weight: 500;
  }

  .field-arrow {
    color: var(--ds-text-subtlest);
  }

  .field-value {
    color: var(--ds-text-subtle);
    font-size: 11px;
    background-color: var(--ds-surface-sunken);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .asset-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }

  .asset-name {
    color: var(--ds-text);
    font-weight: 500;
    max-width: 160px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .comment-preview {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .comment-text {
    font-size: 12px;
    color: var(--ds-text-subtle);
    line-height: 1.4;
  }

  .status-id {
    font-size: 12px;
    color: var(--ds-text-subtle);
    font-family: monospace;
  }

  .cm-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
  }

  .cm-label {
    color: var(--ds-text-subtle);
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .cm-template {
    color: var(--ds-text);
    font-family: monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 140px;
  }

  .assignment-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }

  .team-name {
    color: var(--ds-text);
    font-weight: 500;
  }

  /* Covers the built-in placeholder and placeholders inside caller-supplied body snippets. */
  .base-action-node :global(.placeholder) {
    color: var(--ds-text-subtle);
    font-size: 12px;
    font-style: italic;
  }

  :global(.base-action-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--_accent);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
