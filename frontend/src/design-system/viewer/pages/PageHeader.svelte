<script>
  import PageHeader from '../../../lib/layout/PageHeader.svelte'
  import Button from '../../../lib/components/Button.svelte'
  import { LayoutDashboard, Users, FileText, Settings, Plus, Download, Filter } from 'lucide-svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">PageHeader</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Page-level header component with icon, title, subtitle, optional count, and action buttons.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <PageHeader
        title="Dashboard"
        subtitle="Overview of your workspace"
      />
    </div>
  </section>

  <!-- With Icon -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Icon</h2>
    <div class="space-y-4">
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <PageHeader
          icon={LayoutDashboard}
          title="Dashboard"
          subtitle="Overview of your workspace"
        />
      </div>
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <PageHeader
          icon={Users}
          title="Team"
          subtitle="Manage team members and permissions"
        />
      </div>
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <PageHeader
          icon={FileText}
          title="Documents"
          subtitle="View and manage your files"
        />
      </div>
    </div>
  </section>

  <!-- With Count -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Count</h2>
    <div class="space-y-4">
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <PageHeader
          icon={Users}
          title="Team Members"
          subtitle="Manage your team"
          count="12 members"
        />
      </div>
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <PageHeader
          icon={FileText}
          title="Documents"
          subtitle="All uploaded files"
          count="48 files"
        />
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
        <PageHeader
          icon={Users}
          title="Team Members"
          subtitle="Manage your team"
          count="12 members"
        >
          <svelte:fragment slot="actions">
            <Button variant="primary" icon={Plus}>Invite Member</Button>
          </svelte:fragment>
        </PageHeader>
      </div>
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <PageHeader
          icon={FileText}
          title="Documents"
          subtitle="All uploaded files"
          count="48 files"
        >
          <svelte:fragment slot="actions">
            <Button variant="secondary" icon={Filter}>Filter</Button>
            <Button variant="secondary" icon={Download}>Export</Button>
            <Button variant="primary" icon={Plus}>Upload</Button>
          </svelte:fragment>
        </PageHeader>
      </div>
    </div>
  </section>

  <!-- Complete Example -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Complete Example</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <PageHeader
        icon={Settings}
        title="Settings"
        subtitle="Configure your workspace preferences"
      >
        <svelte:fragment slot="actions">
          <Button variant="ghost">Reset to Defaults</Button>
          <Button variant="primary">Save Changes</Button>
        </svelte:fragment>
      </PageHeader>
      <div class="h-48 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
        <span class="text-sm" style="color: var(--ds-text-subtle);">Page content area</span>
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
            <td class="p-2"><code>icon</code></td>
            <td class="p-2">Lucide icon component</td>
            <td class="p-2">null</td>
          </tr>
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
            <td class="p-2"><code>count</code></td>
            <td class="p-2">string | number | null</td>
            <td class="p-2">null</td>
          </tr>
          <tr>
            <td class="p-2"><code>slot="actions"</code></td>
            <td class="p-2">slot content</td>
            <td class="p-2">-</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
