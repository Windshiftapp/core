<script>
  import ItemPicker from '../../../lib/pickers/ItemPicker.svelte'
  import { User, Folder, Tag, Calendar } from 'lucide-svelte'

  // Mock data for demos
  const simpleItems = [
    { id: 1, name: 'Option 1' },
    { id: 2, name: 'Option 2' },
    { id: 3, name: 'Option 3' },
    { id: 4, name: 'Option 4' },
  ]

  const users = [
    { id: 1, name: 'Alice Johnson', email: 'alice@example.com', role: 'Admin' },
    { id: 2, name: 'Bob Smith', email: 'bob@example.com', role: 'Developer' },
    { id: 3, name: 'Carol White', email: 'carol@example.com', role: 'Designer' },
    { id: 4, name: 'David Brown', email: 'david@example.com', role: 'Manager' },
  ]

  const projects = [
    { id: 1, name: 'Project Alpha', description: 'Main product development', color: '#3b82f6', status: 'Active' },
    { id: 2, name: 'Project Beta', description: 'Internal tools', color: '#10b981', status: 'Active' },
    { id: 3, name: 'Project Gamma', description: 'Customer portal', color: '#f59e0b', status: 'Paused' },
    { id: 4, name: 'Project Delta', description: 'Mobile app', color: '#8b5cf6', status: 'Planning' },
  ]

  const milestones = [
    { id: 1, name: 'Q1 Release', startDate: '2024-01-01', endDate: '2024-03-31', category: 'Release' },
    { id: 2, name: 'Q2 Release', startDate: '2024-04-01', endDate: '2024-06-30', category: 'Release' },
    { id: 3, name: 'Beta Launch', startDate: '2024-02-15', endDate: '2024-02-28', category: 'Launch' },
  ]

  let simpleValue = $state(null)
  let userValue = $state(null)
  let projectValue = $state(null)
  let milestoneValue = $state(null)
  let withUnassigned = $state(null)

  // Config for user picker with icon and secondary text
  const userConfig = {
    icon: { type: 'component', source: () => User },
    primary: { text: (item) => item.name },
    secondary: { text: (item) => item.email },
    badges: [
      { text: (item) => item.role, bgColor: () => 'var(--ds-background-neutral)' }
    ],
    searchFields: ['name', 'email', 'role']
  }

  // Config for project picker with color dots
  const projectConfig = {
    icon: { type: 'color-dot', source: (item) => item.color, size: 'w-3 h-3' },
    primary: { text: (item) => item.name },
    secondary: { text: (item) => item.description },
    badges: [
      {
        text: (item) => item.status,
        bgColor: (item) => item.status === 'Active' ? '#dcfce7' : item.status === 'Paused' ? '#fef3c7' : '#e0e7ff',
        textColor: (item) => item.status === 'Active' ? '#166534' : item.status === 'Paused' ? '#92400e' : '#3730a3'
      }
    ],
    searchFields: ['name', 'description', 'status']
  }

  // Config for milestone picker with date range
  const milestoneConfig = {
    icon: { type: 'component', source: () => Calendar },
    primary: { text: (item) => item.name },
    metadata: [
      {
        type: 'date-range',
        startDate: (item) => item.startDate,
        endDate: (item) => item.endDate
      },
      {
        type: 'badge',
        text: (item) => item.category,
        bgColor: () => 'var(--ds-background-neutral)'
      }
    ],
    searchFields: ['name', 'category']
  }
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">ItemPicker</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    A highly configurable generic picker component built on Melt UI. Serves as the foundation for many domain-specific pickers with support for icons, badges, metadata, and custom item rendering.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Simple picker with default configuration. Items need an <code>id</code> and <code>name</code> or <code>label</code>.
    </p>
    <div class="w-64">
      <ItemPicker
        bind:value={simpleValue}
        items={simpleItems}
        placeholder="Select an option..."
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Selected: {simpleValue || 'None'}</p>
  </section>

  <!-- With Icon and Secondary Text -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Icon and Secondary Text</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Configure icons, secondary text, and badges via the <code>config</code> prop.
    </p>
    <div class="w-80">
      <ItemPicker
        bind:value={userValue}
        items={users}
        config={userConfig}
        placeholder="Select a user..."
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Selected: {userValue || 'None'}</p>
  </section>

  <!-- With Color Dots -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Color Dots</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Use <code>icon.type: 'color-dot'</code> to show colored indicators.
    </p>
    <div class="w-80">
      <ItemPicker
        bind:value={projectValue}
        items={projects}
        config={projectConfig}
        placeholder="Select a project..."
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Selected: {projectValue || 'None'}</p>
  </section>

  <!-- With Date Range Metadata -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Date Range Metadata</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Display date ranges and additional metadata using the <code>metadata</code> config.
    </p>
    <div class="w-80">
      <ItemPicker
        bind:value={milestoneValue}
        items={milestones}
        config={milestoneConfig}
        placeholder="Select a milestone..."
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Selected: {milestoneValue || 'None'}</p>
  </section>

  <!-- With Unassigned Option -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Unassigned Option</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Enable <code>showUnassigned</code> to add a "None" option at the top.
    </p>
    <div class="w-64">
      <ItemPicker
        bind:value={withUnassigned}
        items={simpleItems}
        placeholder="Select an option..."
        showUnassigned={true}
        unassignedLabel="No selection"
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Selected: {withUnassigned || 'None'}</p>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="flex flex-wrap gap-4">
      <div class="w-64">
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Disabled</p>
        <ItemPicker
          items={simpleItems}
          placeholder="Disabled..."
          disabled={true}
        />
      </div>
      <div class="w-64">
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Loading</p>
        <ItemPicker
          items={[]}
          placeholder="Loading..."
          loading={true}
        />
      </div>
    </div>
  </section>

  <!-- Config Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Config Object</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <pre class="text-sm overflow-x-auto" style="color: var(--ds-text);">{`const config = {
  // Icon configuration
  icon: {
    type: 'component' | 'color-dot',
    source: (item) => IconComponent | '#hexcolor',
    size: 'w-2 h-2' // for color-dot
  },

  // Primary text (required)
  primary: {
    text: (item) => string
  },

  // Secondary text (optional)
  secondary: {
    text: (item) => string
  },

  // Badges array (optional)
  badges: [{
    text: (item) => string,
    bgColor: (item) => string,
    textColor: (item) => string
  }],

  // Metadata array (optional)
  metadata: [{
    type: 'date-range' | 'badge' | 'text',
    // for date-range:
    startDate: (item) => string,
    endDate: (item) => string,
    // for badge/text:
    text: (item) => string,
    icon: IconComponent
  }],

  // Fields to search
  searchFields: ['name', 'label', ...],

  // Value accessors
  getValue: (item) => item.id,
  getLabel: (item) => item.name
}`}</pre>
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
            <td class="p-2">any (bindable)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>items</code></td>
            <td class="p-2">array</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>config</code></td>
            <td class="p-2">object</td>
            <td class="p-2">{'{}'}</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showUnassigned</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>unassignedLabel</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'None'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>allowClear</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>loading</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>autoOpen</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>children</code></td>
            <td class="p-2">snippet</td>
            <td class="p-2">null (custom trigger)</td>
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
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Callback Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onSelect</code></td>
            <td class="p-2">Called when an item is selected, receives the item object directly</td>
          </tr>
          <tr>
            <td class="p-2"><code>onCancel</code></td>
            <td class="p-2">Called when the popover closes without a selection</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
