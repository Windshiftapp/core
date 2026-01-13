<script>
  import { createEventDispatcher } from 'svelte';
  import Button from '../../components/Button.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { getShortcut, matchesShortcut, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';

  const dispatch = createEventDispatcher();

  // Get QL shortcut configuration
  const qlExecuteShortcut = getShortcut('ql', 'execute');

  // Props
  export let query = '';
  export let isEditing = false;
  export let error = null;

  function handleToggleEdit() {
    dispatch('toggle-edit');
  }

  function handleExecute() {
    dispatch('execute');
  }

  function handleClear() {
    dispatch('clear');
  }

  function handleQueryChange(event) {
    dispatch('query-change', event.target.value);
  }

  function handleKeydown(event) {
    if (matchesShortcut(event, qlExecuteShortcut)) {
      event.preventDefault();
      handleExecute();
    }
  }
</script>

<div class="mb-4">
  <!-- Query display - subtle inline style -->
  <div class="flex items-center gap-3 text-xs" style="color: var(--ds-text-subtle);">
    <div class="flex items-center gap-2 min-w-0">
      <span class="font-medium shrink-0">Query:</span>
      <code class="font-mono truncate" title={query || 'No query'}>
        {query || 'No filters applied'}
      </code>
      <Button
        variant="ghost"
        size="sm"
        onclick={handleToggleEdit}
      >
        {isEditing ? 'Hide' : 'Edit'}
      </Button>
    </div>
    {#if error && !isEditing}
      <span style="color: var(--ds-text-danger);">Error</span>
    {/if}
  </div>

  <!-- Expandable editor -->
  {#if isEditing}
    <div class="mt-3 p-3 rounded-lg border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <label for="ql-editor" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">
        Query Language
      </label>
      <Textarea
        id="ql-editor"
        value={query}
        oninput={handleQueryChange}
        placeholder='Example: workspace = "My Project" AND status = "open"'
        class="font-mono text-sm"
        rows={2}
        onkeydown={handleKeydown}
      />
      {#if error}
        <div class="mt-1 text-xs font-mono" style="color: var(--ds-text-danger);">
          {error}
        </div>
      {/if}
      <div class="mt-2 flex items-center justify-between">
        <span class="text-xs" style="color: var(--ds-text-subtlest);">
          {getShortcutDisplay('ql', 'execute')} to execute
        </span>
        <div class="flex gap-2">
          <Button variant="ghost" size="sm" onclick={handleClear}>Clear</Button>
          <Button variant="primary" size="sm" onclick={handleExecute}>Execute</Button>
        </div>
      </div>
    </div>
  {/if}
</div>
