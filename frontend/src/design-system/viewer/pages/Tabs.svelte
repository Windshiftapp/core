<script>
  import Tabs from '../../../lib/Tabs.svelte'
  import { FileText, Settings, Users, BarChart } from 'lucide-svelte'

  let basicActiveTab = 'tab1'
  let iconActiveTab = 'overview'
  let badgeActiveTab = 'all'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Tabs</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Tab navigation component for organizing content into switchable panels.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <Tabs
      tabs={[
        { id: 'tab1', label: 'First Tab' },
        { id: 'tab2', label: 'Second Tab' },
        { id: 'tab3', label: 'Third Tab' }
      ]}
      bind:activeTab={basicActiveTab}
    >
      {#if basicActiveTab === 'tab1'}
        <p style="color: var(--ds-text);">Content for the first tab. This is where you would put your tab-specific content.</p>
      {:else if basicActiveTab === 'tab2'}
        <p style="color: var(--ds-text);">Content for the second tab. Each tab can have completely different content.</p>
      {:else if basicActiveTab === 'tab3'}
        <p style="color: var(--ds-text);">Content for the third tab. Switch between tabs to see different content.</p>
      {/if}
    </Tabs>
  </section>

  <!-- With Icons -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Icons</h2>
    <Tabs
      tabs={[
        { id: 'overview', label: 'Overview', icon: BarChart },
        { id: 'documents', label: 'Documents', icon: FileText },
        { id: 'team', label: 'Team', icon: Users },
        { id: 'settings', label: 'Settings', icon: Settings }
      ]}
      bind:activeTab={iconActiveTab}
    >
      {#if iconActiveTab === 'overview'}
        <div class="space-y-2">
          <p style="color: var(--ds-text);">Overview Dashboard</p>
          <p class="text-sm" style="color: var(--ds-text-subtle);">View key metrics and statistics at a glance.</p>
        </div>
      {:else if iconActiveTab === 'documents'}
        <div class="space-y-2">
          <p style="color: var(--ds-text);">Documents Library</p>
          <p class="text-sm" style="color: var(--ds-text-subtle);">Browse and manage your files and documents.</p>
        </div>
      {:else if iconActiveTab === 'team'}
        <div class="space-y-2">
          <p style="color: var(--ds-text);">Team Members</p>
          <p class="text-sm" style="color: var(--ds-text-subtle);">Manage your team and their permissions.</p>
        </div>
      {:else if iconActiveTab === 'settings'}
        <div class="space-y-2">
          <p style="color: var(--ds-text);">Settings</p>
          <p class="text-sm" style="color: var(--ds-text-subtle);">Configure your workspace preferences.</p>
        </div>
      {/if}
    </Tabs>
  </section>

  <!-- With Badges -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Badges</h2>
    <Tabs
      tabs={[
        { id: 'all', label: 'All Items', badge: '24' },
        { id: 'active', label: 'Active', badge: '12' },
        { id: 'pending', label: 'Pending', badge: '8' },
        { id: 'completed', label: 'Completed', badge: '4' }
      ]}
      bind:activeTab={badgeActiveTab}
    >
      {#if badgeActiveTab === 'all'}
        <p style="color: var(--ds-text);">Showing all 24 items</p>
      {:else if badgeActiveTab === 'active'}
        <p style="color: var(--ds-text);">Showing 12 active items</p>
      {:else if badgeActiveTab === 'pending'}
        <p style="color: var(--ds-text);">Showing 8 pending items</p>
      {:else if badgeActiveTab === 'completed'}
        <p style="color: var(--ds-text);">Showing 4 completed items</p>
      {/if}
    </Tabs>
  </section>

  <!-- Tab Data Structure -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Tab Data Structure</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <pre class="text-sm overflow-x-auto" style="color: var(--ds-text);">{`const tabs = [
  { id: 'overview', label: 'Overview' },
  { id: 'details', label: 'Details', icon: FileText },
  { id: 'activity', label: 'Activity', badge: '5' },
  { id: 'settings', label: 'Settings', icon: Settings, badge: '!' }
]`}</pre>
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
            <td class="p-2"><code>tabs</code></td>
            <td class="p-2">Tab[]</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>activeTab</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'' (auto: first tab)</td>
          </tr>
          <tr>
            <td class="p-2"><code>on:tab-change</code></td>
            <td class="p-2">event</td>
            <td class="p-2">-</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Tab Interface -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Tab Interface</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Property</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Required</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>id</code></td>
            <td class="p-2">string</td>
            <td class="p-2">Yes</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">Yes</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>icon</code></td>
            <td class="p-2">Lucide icon component</td>
            <td class="p-2">No</td>
          </tr>
          <tr>
            <td class="p-2"><code>badge</code></td>
            <td class="p-2">string</td>
            <td class="p-2">No</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
