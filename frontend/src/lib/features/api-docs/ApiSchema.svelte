<script>
  import { resolveRef } from './openapi-store.svelte.js';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import Self from './ApiSchema.svelte';

  /**
   * Recursive schema viewer. Handles $ref, primitives, arrays, objects,
   * oneOf/anyOf/allOf. Object children are collapsed by default at any
   * depth > 0 so the page doesn't dump everything at once.
   */
  let {
    spec,
    schema,
    name = null,
    required = false,
    depth = 0,
    defaultOpen = false,
  } = $props();

  // Initial expansion is intentionally captured once; user toggles own the state after mount.
  // svelte-ignore state_referenced_locally
  let open = $state(defaultOpen || depth === 0);

  // $ref pointer: resolve once, then render that target. The user-visible
  // type label keeps the original ref name when available so they know
  // the contract.
  const refName = $derived(refLeaf(schema?.$ref));
  const resolved = $derived(schema?.$ref ? resolveRef(spec, schema.$ref) ?? schema : schema);

  function refLeaf(ref) {
    if (!ref) return null;
    const parts = String(ref).split('/');
    return parts[parts.length - 1] || null;
  }

  const typeLabel = $derived(formatType(resolved, refName));

  function formatType(s, refName) {
    if (!s) return '—';
    if (refName) return refName;
    if (s.enum && Array.isArray(s.enum)) return 'enum';
    if (s.type === 'array') {
      const item = s.items || {};
      const inner = item.$ref ? refLeaf(item.$ref) : (item.type || 'any');
      return `${inner}[]`;
    }
    if (s.oneOf) return 'oneOf';
    if (s.anyOf) return 'anyOf';
    if (s.allOf) return 'allOf';
    return s.format ? `${s.type || 'any'} (${s.format})` : (s.type || 'any');
  }

  // The fields shown inline under an object schema row.
  const objectEntries = $derived(
    resolved && resolved.type === 'object' && resolved.properties
      ? Object.entries(resolved.properties)
      : []
  );

  // Bug-be-gone: required-ness for nested children comes from the parent
  // schema's `required: []` list, not the child schema itself.
  const requiredSet = $derived(new Set(resolved?.required ?? []));

  const composition = $derived(resolved?.oneOf || resolved?.anyOf || resolved?.allOf || null);
  const compositionLabel = $derived(resolved?.oneOf ? 'oneOf' : resolved?.anyOf ? 'anyOf' : resolved?.allOf ? 'allOf' : null);

  const hasExpandableChildren = $derived(
    objectEntries.length > 0 ||
    !!composition ||
    (resolved?.type === 'array' && resolved.items && (resolved.items.$ref || resolved.items.type === 'object'))
  );
</script>

<div class="schema-row" style="--depth: {depth}">
  <button
    type="button"
    class="schema-head"
    class:schema-head--expandable={hasExpandableChildren}
    onclick={() => { if (hasExpandableChildren) open = !open; }}
    aria-expanded={hasExpandableChildren ? open : undefined}
  >
    {#if hasExpandableChildren}
      <span class="caret">
        {#if open}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
      </span>
    {:else}
      <span class="caret caret--placeholder"></span>
    {/if}

    {#if name}
      <code class="schema-name">{name}</code>
    {/if}
    <span class="schema-type">{typeLabel}</span>
    {#if required}
      <span class="schema-required">required</span>
    {/if}
    {#if resolved?.format && !refName}
      <span class="schema-meta">{resolved.format}</span>
    {/if}
  </button>

  {#if resolved?.description}
    <div class="schema-desc">{resolved.description}</div>
  {/if}
  {#if resolved?.enum}
    <div class="schema-enum">
      {#each resolved.enum as v}
        <code>{JSON.stringify(v)}</code>
      {/each}
    </div>
  {/if}

  {#if open && hasExpandableChildren}
    <div class="schema-children">
      {#if objectEntries.length > 0}
        {#each objectEntries as [propName, propSchema] (propName)}
          <Self
            {spec}
            schema={propSchema}
            name={propName}
            required={requiredSet.has(propName)}
            depth={depth + 1}
          />
        {/each}
      {:else if resolved?.type === 'array' && resolved.items}
        <Self
          {spec}
          schema={resolved.items}
          name="(item)"
          depth={depth + 1}
          defaultOpen
        />
      {:else if composition}
        <div class="composition-label">{compositionLabel}</div>
        {#each composition as variant, i}
          <Self
            {spec}
            schema={variant}
            name={`option ${i + 1}`}
            depth={depth + 1}
            defaultOpen
          />
        {/each}
      {/if}
    </div>
  {/if}
</div>

<style>
  .schema-row {
    --indent: calc(var(--depth, 0) * 14px);
    padding: 4px 0 4px var(--indent);
    border-bottom: 1px dashed var(--ds-border);
  }
  .schema-row:last-child {
    border-bottom: none;
  }
  .schema-head {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    background: transparent;
    border: none;
    padding: 2px 0;
    color: var(--ds-text);
    font-size: 13px;
    cursor: default;
    text-align: left;
  }
  .schema-head--expandable {
    cursor: pointer;
  }
  .schema-head--expandable:hover .schema-name,
  .schema-head--expandable:hover .schema-type {
    color: var(--ds-text-link);
  }
  .caret {
    display: inline-flex;
    align-items: center;
    width: 12px;
    color: var(--ds-text-subtle);
  }
  .caret--placeholder { width: 12px; }
  .schema-name {
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    font-size: 12.5px;
    color: var(--ds-text);
  }
  .schema-type {
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    font-size: 12px;
    color: var(--ds-text-subtle);
  }
  .schema-required {
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.03em;
    color: var(--ds-text-danger);
    text-transform: uppercase;
  }
  .schema-meta {
    font-size: 11px;
    color: var(--ds-text-subtlest, var(--ds-text-subtle));
  }
  .schema-desc {
    padding-left: 20px;
    margin-top: 2px;
    color: var(--ds-text-subtle);
    font-size: 12.5px;
    line-height: 1.45;
  }
  .schema-enum {
    padding-left: 20px;
    margin-top: 4px;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .schema-enum code {
    background: var(--ds-background-neutral);
    color: var(--ds-text);
    font-size: 11.5px;
    padding: 1px 6px;
    border-radius: 3px;
  }
  .schema-children {
    margin-top: 4px;
  }
  .composition-label {
    padding-left: 20px;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ds-text-subtle);
    margin: 6px 0 2px;
  }
</style>
