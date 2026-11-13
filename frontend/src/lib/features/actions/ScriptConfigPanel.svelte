<script>
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';

  let {
    selectedNode,
    flowStore,
  } = $props();

  const cfg = $derived(selectedNode?.data?.config || {});

  const sourceExample = `// The effective executor's identity and permissions apply.
// Call the v2 REST API synchronously:
const res = windshift.api.request({
  method: 'POST',
  path: '/items/' + context.item_id + '/comments',
  body: { content: 'Handled by automation', is_private: true }
});
console.log('comment status', res.status);
return { comment_id: res.body?.id ?? null };`;
</script>

<div class="script-panel">
  <div>
    <label for="execute-script-source" class="block text-xs font-medium mb-1">JavaScript (function body)</label>
    <Textarea
      id="execute-script-source"
      data-testid="execute-script-source"
      rows={14}
      class="script-source"
      value={cfg.source || ''}
      placeholder={sourceExample}
      oninput={(e) => flowStore.updateNodeConfig(selectedNode.id, { source: e.currentTarget.value })}
    />
    <p class="hint">
      Saving validates syntax only; the script never runs while editing. The
      return value becomes JSON output. There is no network, filesystem, shell,
      module, or async access — only <code>windshift.api.request</code> and
      <code>console</code>.
    </p>
  </div>

  <div>
    <label for="execute-script-output-variable" class="block text-xs font-medium mb-1">Output variable</label>
    <Input
      id="execute-script-output-variable"
      dataTestid="execute-script-output-variable"
      type="text"
      size="small"
      value={cfg.output_variable || ''}
      placeholder="script_result"
      oninput={(e) => flowStore.updateNodeConfig(selectedNode.id, { output_variable: e.currentTarget.value })}
    />
    <p class="hint">Downstream nodes read this variable. Reserved names such as
      <code>actor_user_id</code> are rejected.</p>
  </div>

  <div class="script-grid">
    <div>
      <label for="execute-script-timeout" class="block text-xs font-medium mb-1">Timeout (ms)</label>
      <Input
        id="execute-script-timeout"
        dataTestid="execute-script-timeout"
        type="number"
        size="small"
        value={cfg.timeout_ms ? String(cfg.timeout_ms) : ''}
        placeholder="5000"
        oninput={(e) => flowStore.updateNodeConfig(selectedNode.id, { timeout_ms: e.currentTarget.value ? parseInt(e.currentTarget.value, 10) : 0 })}
      />
    </div>
    <div>
      <label for="execute-script-max-calls" class="block text-xs font-medium mb-1">Max API calls</label>
      <Input
        id="execute-script-max-calls"
        dataTestid="execute-script-max-calls"
        type="number"
        size="small"
        value={cfg.max_api_calls ? String(cfg.max_api_calls) : ''}
        placeholder="50"
        oninput={(e) => flowStore.updateNodeConfig(selectedNode.id, { max_api_calls: e.currentTarget.value ? parseInt(e.currentTarget.value, 10) : 0 })}
      />
    </div>
  </div>

  <div class="reference">
    <p class="reference-title">windshift.api.request(&#123; method, path, body &#125;)</p>
    <p class="hint">
      <code>path</code> is relative to the v2 REST root (for example
      <code>/items/123</code>). Alternate hosts, redirects, path escapes, and
      unsupported routes are rejected. Requests run with a short-lived token
      scoped to this executor and are audited with the action, execution, and
      node that produced them.
    </p>
    <p class="hint">
      <code>console.log/info/warn/error/debug</code> write bounded lines to the
      execution trace.
    </p>
  </div>
</div>

<style>
  .script-panel {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .script-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  :global(textarea.script-source) {
    font-family: var(--ds-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 12px;
    line-height: 1.5;
  }
  .hint {
    margin-top: 4px;
    color: var(--ds-text-subtle);
    font-size: 11px;
    line-height: 1.4;
  }
  .hint code,
  .reference code {
    font-family: var(--ds-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  }
  .reference {
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    padding: 8px;
    background: var(--ds-surface-sunken, transparent);
  }
  .reference-title {
    font-size: 11px;
    font-weight: 600;
    margin-bottom: 4px;
  }
</style>
