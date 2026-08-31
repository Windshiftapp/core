<script>
  import { AlertTriangle, Download, ExternalLink, FileText, Search, ShieldCheck } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Input from '../components/Input.svelte';
  import NativeSelect from '../components/NativeSelect.svelte';
  import manifest from '../generated/license-manifest.json';

  const dependencies = manifest.dependencies;
  const allOption = 'all';

  let search = $state('');
  let ecosystem = $state(allOption);
  let project = $state(allOption);
  let license = $state(allOption);
  let scope = $state(allOption);
  let unknownOnly = $state(false);

  const ecosystems = $derived(uniqueValues(dependencies, 'ecosystem'));
  const projects = $derived(uniqueValues(dependencies, 'project'));
  const scopes = $derived(uniqueValues(dependencies, 'scope'));
  const licenses = $derived(uniqueValues(dependencies, 'license'));

  const unknownCount = $derived(dependencies.filter((entry) => isUnknown(entry.license)).length);
  const licenseCount = $derived(licenses.filter((entry) => !isUnknown(entry)).length);

  const filteredDependencies = $derived.by(() => {
    const term = search.trim().toLowerCase();

    return dependencies.filter((entry) => {
      if (ecosystem !== allOption && entry.ecosystem !== ecosystem) return false;
      if (project !== allOption && entry.project !== project) return false;
      if (license !== allOption && entry.license !== license) return false;
      if (scope !== allOption && entry.scope !== scope) return false;
      if (unknownOnly && !isUnknown(entry.license)) return false;
      if (!term) return true;

      return [entry.name, entry.version, entry.license, entry.project, entry.ecosystem, entry.scope]
        .filter(Boolean)
        .some((value) => value.toLowerCase().includes(term));
    });
  });

  function uniqueValues(entries, key) {
    return [...new Set(entries.map((entry) => entry[key]).filter(Boolean))].sort((a, b) =>
      a.localeCompare(b)
    );
  }

  function isUnknown(value) {
    return !value || value === 'Unknown';
  }

  function licenseTone(value) {
    if (isUnknown(value)) return 'unknown';
    if (/gpl|agpl|lgpl|mpl/i.test(value)) return 'review';
    return 'known';
  }

  function sourceUrl(entry) {
    const url = entry.repository || entry.homepage;
    if (!url) return null;

    const normalized = url
      .replace(/^git\+/, '')
      .replace(/^git:\/\//, 'https://')
      .replace(/^github:/, 'https://github.com/')
      .replace(/^git@github\.com:/, 'https://github.com/')
      .replace(/\.git$/, '');

    if (/^https?:\/\//.test(normalized)) return normalized;
    if (/^[\w.-]+\/[\w./-]+$/.test(normalized)) return `https://github.com/${normalized}`;

    return entry.homepage && /^https?:\/\//.test(entry.homepage) ? entry.homepage : null;
  }

  function exportCsv() {
    const header = ['ecosystem', 'project', 'name', 'version', 'license', 'scope', 'licenseFile', 'source'];
    const rows = filteredDependencies.map((entry) => [
      entry.ecosystem,
      entry.project,
      entry.name,
      entry.version,
      entry.license,
      entry.scope,
      entry.licenseFile || '',
      sourceUrl(entry) || '',
    ]);
    const csv = [header, ...rows]
      .map((row) => row.map((value) => `"${String(value).replaceAll('"', '""')}"`).join(','))
      .join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'windshift-license-manifest.csv';
    link.click();
    URL.revokeObjectURL(url);
  }
</script>

<div class="min-h-full license-page">
  <div class="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
    <header class="mb-8 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <div class="mb-3 flex items-center gap-2 text-sm font-medium license-kicker">
          <ShieldCheck class="h-4 w-4" />
          Dependency compliance
        </div>
        <h1 class="text-3xl font-semibold tracking-normal license-title">License review</h1>
        <p class="mt-2 max-w-3xl text-sm license-subtitle">
          Third-party dependency license metadata for the Go module, app frontend, and demo plugin frontend.
        </p>
      </div>
      <Button variant="default" icon={Download} onclick={exportCsv}>Export CSV</Button>
    </header>

    <section class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div class="summary-tile">
        <div class="summary-label">Dependencies</div>
        <div class="summary-value">{dependencies.length}</div>
      </div>
      <div class="summary-tile">
        <div class="summary-label">License expressions</div>
        <div class="summary-value">{licenseCount}</div>
      </div>
      <div class="summary-tile">
        <div class="summary-label">Projects</div>
        <div class="summary-value">{projects.length}</div>
      </div>
      <div class="summary-tile" class:summary-tile-warning={unknownCount > 0}>
        <div class="summary-label">Unknown licenses</div>
        <div class="summary-value">{unknownCount}</div>
      </div>
    </section>

    <section class="filters mb-6">
      <label class="search-box">
        <Search class="h-4 w-4" />
        <Input bind:value={search} type="search" placeholder="Search dependencies, licenses, projects" class="license-search" size="small" />
      </label>

      <NativeSelect bind:value={ecosystem} ariaLabel="Ecosystem filter" class="license-filter" size="small" options={[{ value: allOption, label: 'All ecosystems' }, ...ecosystems.map((option) => ({ value: option, label: option }))]} />

      <NativeSelect bind:value={project} ariaLabel="Project filter" class="license-filter" size="small" options={[{ value: allOption, label: 'All projects' }, ...projects.map((option) => ({ value: option, label: option }))]} />

      <NativeSelect bind:value={license} ariaLabel="License filter" class="license-filter" size="small" options={[{ value: allOption, label: 'All licenses' }, ...licenses.map((option) => ({ value: option, label: option }))]} />

      <NativeSelect bind:value={scope} ariaLabel="Scope filter" class="license-filter" size="small" options={[{ value: allOption, label: 'All scopes' }, ...scopes.map((option) => ({ value: option, label: option }))]} />

      <Checkbox bind:checked={unknownOnly} label="Unknown only" class="unknown-toggle" size="small" />
    </section>

    <div class="table-shell">
      <div class="table-header">
        <div>
          <span class="font-semibold">{filteredDependencies.length}</span>
          <span class="table-header-muted">shown</span>
        </div>
        <div class="table-header-muted">Generated from {manifest.generatedFrom.join(', ')}</div>
      </div>

      <div class="overflow-x-auto">
        <table>
          <thead>
            <tr>
              <th>Dependency</th>
              <th>License</th>
              <th>Project</th>
              <th>Scope</th>
              <th>Source</th>
              <th>License file</th>
            </tr>
          </thead>
          <tbody>
            {#each filteredDependencies as entry (`${entry.ecosystem}:${entry.project}:${entry.name}:${entry.version}`)}
              {@const url = sourceUrl(entry)}
              <tr class:row-warning={isUnknown(entry.license)}>
                <td>
                  <div class="dependency-name">{entry.name}</div>
                  <div class="dependency-meta">{entry.ecosystem} · {entry.version}</div>
                </td>
                <td>
                  <span class="license-pill license-pill-{licenseTone(entry.license)}">
                    {#if isUnknown(entry.license)}
                      <AlertTriangle class="h-3.5 w-3.5" />
                    {/if}
                    {entry.license}
                  </span>
                </td>
                <td>{entry.project}</td>
                <td>
                  <span
                    class="scope-pill"
                    class:scope-pill-runtime={entry.scope === 'runtime'}
                    class:scope-pill-transitive={entry.scope === 'transitive'}
                  >{entry.scope}</span>
                </td>
                <td>
                  {#if url}
                    <a class="source-link" href={url} target="_blank" rel="noreferrer">
                      Open <ExternalLink class="h-3.5 w-3.5" />
                    </a>
                  {:else}
                    <span class="muted">None</span>
                  {/if}
                </td>
                <td>
                  {#if entry.licenseFile}
                    <span class="license-file"><FileText class="h-3.5 w-3.5" /> {entry.licenseFile}</span>
                  {:else}
                    <span class="muted">Not found</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  </div>
</div>

<style>
  .license-page {
    background: var(--ds-surface);
    color: var(--ds-text);
  }

  .license-kicker {
    color: var(--ds-interactive);
  }

  .license-title {
    color: var(--ds-text);
  }

  .license-subtitle,
  .table-header-muted,
  .dependency-meta,
  .muted {
    color: var(--ds-text-subtle);
  }

  .summary-tile,
  .filters,
  .table-shell {
    border: 1px solid var(--ds-border);
    background: var(--ds-surface-raised);
  }

  .summary-tile {
    min-height: 92px;
    border-radius: 8px;
    padding: 16px;
  }

  .summary-tile-warning {
    border-color: color-mix(in srgb, var(--ds-warning, #f59e0b) 60%, var(--ds-border));
    background: color-mix(in srgb, var(--ds-warning, #f59e0b) 10%, var(--ds-surface-raised));
  }

  .summary-label {
    color: var(--ds-text-subtle);
    font-size: 0.8125rem;
    font-weight: 500;
  }

  .summary-value {
    margin-top: 8px;
    color: var(--ds-text);
    font-size: 1.75rem;
    font-weight: 650;
    line-height: 1;
  }

  .filters {
    display: grid;
    grid-template-columns: minmax(240px, 1fr);
    gap: 12px;
    border-radius: 8px;
    padding: 12px;
  }

  .search-box,
  .unknown-toggle {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .search-box {
    min-height: 38px;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    padding: 0 10px;
    background: var(--ds-surface);
    color: var(--ds-text-subtle);
  }

  .search-box :global(.license-search),
  .filters :global(.license-filter) {
    width: 100%;
    min-height: 38px;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    background: var(--ds-surface);
    color: var(--ds-text);
    font-size: 0.875rem;
  }

  .search-box :global(.license-search) {
    min-height: 34px;
    border: 0;
    outline: 0;
  }

  .filters :global(.license-filter) {
    padding: 0 10px;
  }

  .unknown-toggle {
    min-height: 38px;
    color: var(--ds-text);
    font-size: 0.875rem;
    white-space: nowrap;
  }

  .table-shell {
    border-radius: 8px;
    overflow: hidden;
  }

  .table-header {
    display: flex;
    flex-direction: column;
    gap: 6px;
    justify-content: space-between;
    border-bottom: 1px solid var(--ds-border);
    padding: 14px 16px;
    font-size: 0.875rem;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.875rem;
  }

  th {
    background: var(--ds-background-neutral, #f3f4f6);
    color: var(--ds-text-subtle);
    font-size: 0.75rem;
    font-weight: 650;
    letter-spacing: 0;
    padding: 10px 14px;
    text-align: left;
    text-transform: uppercase;
    white-space: nowrap;
  }

  td {
    border-top: 1px solid var(--ds-border);
    color: var(--ds-text);
    padding: 12px 14px;
    vertical-align: top;
  }

  tbody tr:first-child td {
    border-top: 0;
  }

  tbody tr:hover {
    background: var(--ds-background-neutral-hovered, #f9fafb);
  }

  .row-warning {
    background: color-mix(in srgb, var(--ds-warning, #f59e0b) 7%, transparent);
  }

  .dependency-name {
    max-width: 360px;
    overflow-wrap: anywhere;
    font-weight: 600;
  }

  .dependency-meta {
    margin-top: 2px;
    font-size: 0.75rem;
  }

  .license-pill,
  .scope-pill {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    border-radius: 999px;
    padding: 3px 8px;
    font-size: 0.75rem;
    font-weight: 600;
    white-space: nowrap;
  }

  .license-pill-known {
    background: color-mix(in srgb, #22c55e 12%, var(--ds-surface-raised));
    color: color-mix(in srgb, #15803d 85%, var(--ds-text));
  }

  .license-pill-review {
    background: color-mix(in srgb, #3b82f6 12%, var(--ds-surface-raised));
    color: color-mix(in srgb, #1d4ed8 85%, var(--ds-text));
  }

  .license-pill-unknown {
    background: color-mix(in srgb, #f59e0b 16%, var(--ds-surface-raised));
    color: color-mix(in srgb, #b45309 85%, var(--ds-text));
  }

  .scope-pill {
    background: var(--ds-background-neutral, #f3f4f6);
    color: var(--ds-text-subtle);
  }

  .scope-pill-runtime {
    background: var(--ds-accent-blue-subtle, #deebff);
    color: var(--ds-icon-accent-blue, #0052cc);
  }

  .source-link,
  .license-file {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }

  .source-link {
    color: var(--ds-text-link);
    font-weight: 500;
  }

  .source-link:hover {
    color: var(--ds-text-link-hovered);
    text-decoration: underline;
  }

  .license-file {
    max-width: 420px;
    color: var(--ds-text-subtle);
    overflow-wrap: anywhere;
  }

  @media (min-width: 900px) {
    .filters {
      grid-template-columns: minmax(260px, 2fr) repeat(4, minmax(150px, 1fr)) auto;
      align-items: center;
    }

    .table-header {
      flex-direction: row;
      align-items: center;
    }
  }
</style>
