<script>
  import SectionHeader from '../../../lib/layout/SectionHeader.svelte'
  import Button from '../../../lib/components/Button.svelte'
  import { Plus, Settings, Download, Filter } from 'lucide-svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">SectionHeader</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Section header component with title, optional subtitle, and action buttons.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <SectionHeader title="Recent Activity" />
      <div class="h-24 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
        <span class="text-sm" style="color: var(--ds-text-subtle);">Content area</span>
      </div>
    </div>
  </section>

  <!-- With Subtitle -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Subtitle</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <SectionHeader
        title="Team Members"
        subtitle="Manage your team and their permissions"
      />
      <div class="h-24 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
        <span class="text-sm" style="color: var(--ds-text-subtle);">Content area</span>
      </div>
    </div>
  </section>

  <!-- With Actions -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Actions</h2>
    <div class="space-y-4">
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <SectionHeader title="Projects">
          {#snippet actions()}
            <Button variant="primary" size="small" icon={Plus}>New Project</Button>
          {/snippet}
        </SectionHeader>
        <div class="h-24 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
          <span class="text-sm" style="color: var(--ds-text-subtle);">Content area</span>
        </div>
      </div>

      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <SectionHeader
          title="Documents"
          subtitle="View and manage your files"
        >
          {#snippet actions()}
            <Button variant="secondary" size="small" icon={Filter}>Filter</Button>
            <Button variant="primary" size="small" icon={Plus}>Upload</Button>
          {/snippet}
        </SectionHeader>
        <div class="h-24 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
          <span class="text-sm" style="color: var(--ds-text-subtle);">Content area</span>
        </div>
      </div>
    </div>
  </section>

  <!-- Common Use Cases -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Common Use Cases</h2>
    <div class="space-y-4">
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <SectionHeader
          title="API Keys"
          subtitle="Manage your API keys for external integrations"
        >
          {#snippet actions()}
            <Button variant="primary" size="small" icon={Plus}>Create Key</Button>
          {/snippet}
        </SectionHeader>
      </div>

      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <SectionHeader
          title="Settings"
          subtitle="Configure your workspace preferences"
        >
          {#snippet actions()}
            <Button variant="ghost" size="small" icon={Settings}>Advanced</Button>
          {/snippet}
        </SectionHeader>
      </div>

      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <SectionHeader
          title="Export Data"
          subtitle="Download your data in various formats"
        >
          {#snippet actions()}
            <Button variant="secondary" size="small" icon={Download}>Export CSV</Button>
            <Button variant="secondary" size="small" icon={Download}>Export JSON</Button>
          {/snippet}
        </SectionHeader>
      </div>
    </div>
  </section>

  <!-- Props Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Props Reference</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>title</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>subtitle</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>actions</code></td>
            <td class="p-2">snippet</td>
            <td class="p-2">null</td>
          </tr>
          <tr>
            <td class="p-2"><code>class</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
