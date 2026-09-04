<script>
  import MethodBadge from './MethodBadge.svelte';
  import ApiSchema from './ApiSchema.svelte';
  import { renderMarkdown } from '../../utils/render-markdown.js';
  import {
    operationRequiredScopes,
    resolveOperationParameters,
    resolveRef,
  } from './openapi-store.svelte.js';

  let { spec, entry } = $props();

  // entry = { tag, path, method, operation, id }
  const op = $derived(entry.operation);
  const params = $derived(resolveOperationParameters(spec, entry));
  const grouped = $derived(groupParams(params));
  const requiredScopes = $derived(operationRequiredScopes(op));

  function groupParams(list) {
    const buckets = { path: [], query: [], header: [], cookie: [] };
    for (const p of list) {
      const target = buckets[p.in] || (buckets[p.in] = []);
      target.push(p);
    }
    return buckets;
  }

  const requestBodySchema = $derived(extractRequestBody(op.requestBody, spec));
  function extractRequestBody(body, spec) {
    if (!body) return null;
    const resolved = body.$ref ? resolveRef(spec, body.$ref) : body;
    const content = resolved?.content || {};
    const jsonEntry = content['application/json'] || content[Object.keys(content)[0]];
    return jsonEntry ? { mediaType: Object.keys(content)[0], schema: jsonEntry.schema } : null;
  }

  const responses = $derived(Object.entries(op.responses || {}));
  function responseSchema(resp) {
    if (!resp) return null;
    const r = resp.$ref ? resolveRef(spec, resp.$ref) : resp;
    const content = r?.content || {};
    const jsonEntry = content['application/json'] || content[Object.keys(content)[0]];
    return jsonEntry?.schema ?? null;
  }
  function responseTone(code) {
    const n = Number(code);
    if (n >= 200 && n < 300) return 'success';
    if (n >= 300 && n < 400) return 'info';
    if (n >= 400 && n < 500) return 'warning';
    if (n >= 500) return 'danger';
    return 'neutral';
  }
  function toneColor(t) {
    switch (t) {
      case 'success': return 'var(--ds-text-accent-green)';
      case 'info':    return 'var(--ds-text-accent-blue)';
      case 'warning': return 'var(--ds-text-accent-yellow)';
      case 'danger':  return 'var(--ds-text-danger)';
      default:        return 'var(--ds-text-subtle)';
    }
  }

  function capitalize(s) { return s.charAt(0).toUpperCase() + s.slice(1); }
</script>

<article id={entry.id} class="op" data-testid="api-docs-operation" data-method={entry.method} data-path={entry.path}>
  <header class="op-head">
    <div class="op-title-row">
      <MethodBadge method={entry.method} size="md" />
      <code class="op-path">{entry.path}</code>
    </div>
    {#if op.summary}
      <h2 class="op-summary">{op.summary}</h2>
    {/if}
    {#if op.description}
      <div class="op-description prose">
        {@html renderMarkdown(op.description)}
      </div>
    {/if}
  </header>

  {#if op.security && op.security.length > 0}
    <section class="op-section">
      <h3>Security</h3>
      <div class="op-meta-list">
        {#each op.security as req}
          {#each Object.keys(req) as scheme}
            <span class="security-pill">
              {scheme}
            </span>
          {/each}
        {/each}
        {#if requiredScopes.length > 0}
          <span class="security-pill">
            Required scopes
            <span class="security-scopes">({requiredScopes.join(', ')})</span>
          </span>
        {/if}
      </div>
    </section>
  {/if}

  {#each ['path', 'query', 'header', 'cookie'] as where}
    {#if grouped[where] && grouped[where].length > 0}
      <section class="op-section">
        <h3>{capitalize(where)} parameters</h3>
        <table class="params">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Required</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {#each grouped[where] as p}
              <tr>
                <td><code>{p.name}</code></td>
                <td><code>{p.schema?.type || 'any'}{p.schema?.format ? ` (${p.schema.format})` : ''}</code></td>
                <td>{p.required ? 'yes' : ''}</td>
                <td>{p.description || ''}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>
    {/if}
  {/each}

  {#if requestBodySchema}
    <section class="op-section">
      <h3>Request body <span class="hint">({requestBodySchema.mediaType})</span></h3>
      <ApiSchema {spec} schema={requestBodySchema.schema} defaultOpen />
    </section>
  {/if}

  {#if responses.length > 0}
    <section class="op-section">
      <h3>Responses</h3>
      <div class="responses">
        {#each responses as [code, resp]}
          {@const r = resp.$ref ? resolveRef(spec, resp.$ref) : resp}
          {@const tone = responseTone(code)}
          {@const schema = responseSchema(resp)}
          <details class="response" open={tone === 'success'}>
            <summary>
              <code class="response-code" style="color: {toneColor(tone)};">{code}</code>
              <span class="response-desc">{r?.description || ''}</span>
            </summary>
            {#if schema}
              <div class="response-body">
                <ApiSchema {spec} {schema} defaultOpen />
              </div>
            {/if}
          </details>
        {/each}
      </div>
    </section>
  {/if}
</article>

<style>
  .op {
    padding: 24px 32px 48px;
    max-width: 920px;
    margin: 0 auto;
    color: var(--ds-text);
  }
  .op-head {
    margin-bottom: 24px;
    padding-bottom: 16px;
    border-bottom: 1px solid var(--ds-border);
  }
  .op-title-row {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .op-path {
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    font-size: 15px;
    color: var(--ds-text);
    background: var(--ds-background-neutral);
    padding: 3px 8px;
    border-radius: 4px;
  }
  .op-summary {
    margin: 12px 0 0;
    font-size: 20px;
    font-weight: 600;
    line-height: 1.3;
    color: var(--ds-text);
  }
  .op-description {
    margin-top: 10px;
    color: var(--ds-text-subtle);
    line-height: 1.55;
    font-size: 14px;
  }
  .op-description :global(p) { margin: 0 0 8px; }
  .op-description :global(code) {
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    background: var(--ds-background-neutral);
    padding: 1px 5px;
    border-radius: 3px;
    font-size: 12.5px;
  }
  .op-description :global(a) {
    color: var(--ds-text-link);
    text-decoration: underline;
  }
  .op-description :global(pre) {
    background: var(--ds-background-neutral);
    padding: 12px;
    border-radius: 6px;
    overflow-x: auto;
    font-size: 12.5px;
  }

  .op-section {
    margin-top: 24px;
  }
  .op-section h3 {
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--ds-text-subtle);
    margin: 0 0 10px;
  }
  .hint {
    text-transform: none;
    letter-spacing: 0;
    font-weight: 500;
    color: var(--ds-text-subtlest, var(--ds-text-subtle));
  }

  .params {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .params thead th {
    text-align: left;
    font-weight: 600;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--ds-text-subtle);
    padding: 6px 10px;
    border-bottom: 1px solid var(--ds-border);
  }
  .params tbody td {
    padding: 8px 10px;
    border-bottom: 1px solid var(--ds-border);
    vertical-align: top;
  }
  .params code {
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    font-size: 12.5px;
  }

  .responses {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .response {
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    background: var(--ds-surface);
  }
  .response > summary {
    cursor: pointer;
    padding: 8px 12px;
    list-style: none;
    display: flex;
    align-items: baseline;
    gap: 12px;
  }
  .response > summary::-webkit-details-marker { display: none; }
  .response-code {
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    font-size: 13px;
    font-weight: 600;
  }
  .response-desc {
    color: var(--ds-text-subtle);
    font-size: 13px;
  }
  .response-body {
    padding: 6px 12px 12px;
    border-top: 1px solid var(--ds-border);
  }

  .op-meta-list {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .security-pill {
    background: var(--ds-background-neutral);
    color: var(--ds-text);
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    font-size: 12px;
    padding: 3px 8px;
    border-radius: 4px;
  }
  .security-scopes {
    color: var(--ds-text-subtle);
    margin-left: 4px;
  }
</style>
