<script>
  import WorkspaceSelector from '../../../lib/workspaces/WorkspaceSelector.svelte'
  import Label from '../../../lib/components/Label.svelte'
  import { Building } from 'lucide-svelte'

  // Mock workspace data
  const workspaces = [
    {
      id: 1,
      name: 'Engineering',
      key: 'ENG',
      description: 'Core product development team',
      color: '#3b82f6'
    },
    {
      id: 2,
      name: 'Marketing',
      key: 'MKT',
      description: 'Brand and marketing initiatives',
      color: '#10b981'
    },
    {
      id: 3,
      name: 'Design',
      key: 'DSN',
      description: 'User experience and visual design',
      color: '#f59e0b'
    },
    {
      id: 4,
      name: 'Operations',
      key: 'OPS',
      description: 'Internal operations and tooling',
      color: '#8b5cf6'
    },
    {
      id: 5,
      name: 'Customer Success',
      key: 'CS',
      description: 'Customer support and success programs',
      color: '#ef4444'
    },
  ]

  let selectedValue = $state(null)
  let withDefault = $state(1)
  let withClear = $state(2)
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">WorkspaceSelector</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    A specialized picker for selecting a single workspace. Displays workspace avatar/icon, key badge, and description with searchable dropdown.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Click to open the dropdown. Type to search workspaces by name, key, or description.
    </p>
    <div class="w-80">
      <WorkspaceSelector
        bind:value={selectedValue}
        workspaces={workspaces}
        placeholder="Select a workspace..."
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Selected ID: {selectedValue || 'None'}</p>
  </section>

  <!-- With Default Value -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Default Value</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Pre-select a workspace by setting the <code>value</code> prop.
    </p>
    <div class="w-80">
      <WorkspaceSelector
        bind:value={withDefault}
        workspaces={workspaces}
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Selected ID: {withDefault}</p>
  </section>

  <!-- With Allow Clear -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Allow Clear</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Enable <code>allowClear</code> to show an X button for clearing the selection.
    </p>
    <div class="w-80">
      <WorkspaceSelector
        bind:value={withClear}
        workspaces={workspaces}
        allowClear={true}
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Selected ID: {withClear || 'None'}</p>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="space-y-4">
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Disabled</p>
        <div class="w-80">
          <WorkspaceSelector
            value={1}
            workspaces={workspaces}
            disabled={true}
          />
        </div>
      </div>
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Loading</p>
        <div class="w-80">
          <WorkspaceSelector
            workspaces={[]}
            loading={true}
          />
        </div>
      </div>
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Empty</p>
        <div class="w-80">
          <WorkspaceSelector
            workspaces={[]}
            placeholder="No workspaces available"
          />
        </div>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context: Move Item</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Example of workspace selector in a form context.
    </p>
    <div
      class="p-6 rounded-lg max-w-md"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Move Item</h3>
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
        Select the destination workspace for this item.
      </p>

      <div class="mb-4">
        <Label class="mb-2">
          Current Workspace
        </Label>
        <div class="flex items-center gap-2 px-4 py-3 rounded" style="background-color: var(--ds-surface-sunken);">
          <Building size={16} style="color: var(--ds-text-subtle);" />
          <span style="color: var(--ds-text);">Engineering</span>
          <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">ENG</span>
        </div>
      </div>

      <div class="mb-6">
        <Label class="mb-2">
          Move To
        </Label>
        <WorkspaceSelector
          bind:value={selectedValue}
          workspaces={workspaces.filter(w => w.id !== 1)}
          placeholder="Select destination..."
        />
      </div>

      <div class="flex gap-2">
        <button
          class="px-4 py-2 rounded text-sm font-medium text-white"
          style="background-color: var(--ds-background-brand-bold);"
          disabled={!selectedValue}
        >
          Move Item
        </button>
        <button
          class="px-4 py-2 rounded text-sm"
          style="color: var(--ds-text-subtle);"
        >
          Cancel
        </button>
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
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number | null (bindable)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>workspaces</code></td>
            <td class="p-2">{`{id, name, key, description?, avatar_url?, color?}[]`}</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select workspace...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>allowClear</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>loading</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Events Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Events</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Event</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>on:select</code></td>
            <td class="p-2">Fired when a workspace is selected. Contains the full workspace object.</td>
          </tr>
          <tr>
            <td class="p-2"><code>on:cancel</code></td>
            <td class="p-2">Fired when the dropdown closes without a selection.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
