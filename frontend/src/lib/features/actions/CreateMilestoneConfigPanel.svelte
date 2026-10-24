<script>
  import HelpCircle from '@lucide/svelte/icons/help-circle';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';
  import Select from '../../components/Select.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';

  let {
    selectedNode,
    flowStore = actionFlowStore,
    showPlaceholderModal = $bindable(false),
  } = $props();

  const cfg = $derived(selectedNode?.data?.config || {});

  // Default-true for the two attach flags so the panel reflects the
  // executor's runtime defaults — undefined here means "use default = on".
  function attachReleaseOnTag() {
    return cfg.attach_release_on_tag === undefined ? true : !!cfg.attach_release_on_tag;
  }
  function attachCommitIssues() {
    return cfg.attach_commit_issues === undefined ? true : !!cfg.attach_commit_issues;
  }

  // Literal placeholders rendered into the inputs. Built as strings so
  // Svelte doesn't try to interpret the double-braces as expressions.
  const refShortLiteral = '{{ref.short}}';
  const refNameLiteral = '{{ref.name}}';
  const namePlaceholder = `Release ${refShortLiteral}`;
  const descPlaceholder = `What shipped in ${refNameLiteral}`;

  const statusOptions = [
    { value: 'planning', label: 'planning' },
    { value: 'in-progress', label: 'in-progress' },
    { value: 'completed', label: 'completed' },
    { value: 'cancelled', label: 'cancelled' }
  ];
</script>

<div class="cm-panel">
  <div>
    <div class="flex items-center gap-1 mb-1">
      <label for="cm-upsert-key" class="block text-xs font-medium">Upsert key (required)</label>
      <button
        onclick={() => (showPlaceholderModal = true)}
        class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
        title="Show placeholder reference"
      >
        <HelpCircle class="w-3.5 h-3.5" />
      </button>
    </div>
    <Input
      id="cm-upsert-key"
      type="text"
      size="small"
      value={cfg.upsert_key_template || ''}
      placeholder={refShortLiteral}
      oninput={(e) => flowStore.updateNodeConfig(selectedNode.id, { upsert_key_template: e.currentTarget.value })}
    />
    <p class="hint">Stable identifier used to upsert. The same rendered value on a tag and its
      release/* branch pairs them on one milestone.</p>
  </div>

  <div>
    <label for="cm-name" class="block text-xs font-medium mb-1">Milestone name template</label>
    <Input
      id="cm-name"
      type="text"
      size="small"
      value={cfg.name_template || ''}
      placeholder={namePlaceholder}
      oninput={(e) => flowStore.updateNodeConfig(selectedNode.id, { name_template: e.currentTarget.value })}
    />
  </div>

  <div class="cm-grid">
    <div>
      <label for="cm-status-branch" class="block text-xs font-medium mb-1">Status on branch event</label>
      <Select
        id="cm-status-branch"
        options={statusOptions}
        value={cfg.status_on_branch || 'planning'}
        onchange={(v) => flowStore.updateNodeConfig(selectedNode.id, { status_on_branch: v })}
        size="small"
      />
    </div>
    <div>
      <label for="cm-status-tag" class="block text-xs font-medium mb-1">Status on tag event</label>
      <Select
        id="cm-status-tag"
        options={statusOptions}
        value={cfg.status_on_tag || 'in-progress'}
        onchange={(v) => flowStore.updateNodeConfig(selectedNode.id, { status_on_tag: v })}
        size="small"
      />
    </div>
  </div>

  <Checkbox
    checked={attachReleaseOnTag()}
    onchange={(checked) => flowStore.updateNodeConfig(selectedNode.id, { attach_release_on_tag: checked })}
    label="Attach a milestone_releases row on tag events"
    size="small"
  />

  <Checkbox
    checked={attachCommitIssues()}
    onchange={(checked) => flowStore.updateNodeConfig(selectedNode.id, { attach_commit_issues: checked })}
    label="Auto-attach items mentioned in commits since previous tag"
    size="small"
  />

  <div>
    <label for="cm-description" class="block text-xs font-medium mb-1">Description template (insert only)</label>
    <Textarea
      id="cm-description"
      rows={3}
      value={cfg.description_template || ''}
      placeholder={descPlaceholder}
      oninput={(e) => flowStore.updateNodeConfig(selectedNode.id, { description_template: e.currentTarget.value })}
    />
  </div>
</div>

<style>
  .cm-panel {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .cm-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .hint {
    margin-top: 4px;
    color: var(--ds-text-subtle);
    font-size: 11px;
    line-height: 1.4;
  }
</style>
