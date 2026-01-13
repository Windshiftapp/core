<script>
  import CategoryMultiSelect from '../../../lib/CategoryMultiSelect.svelte'

  // Mock category data
  const categories = [
    { id: 1, name: 'Bug', color: 'red' },
    { id: 2, name: 'Feature', color: 'blue' },
    { id: 3, name: 'Enhancement', color: 'green' },
    { id: 4, name: 'Documentation', color: 'purple' },
    { id: 5, name: 'Performance', color: 'orange' },
    { id: 6, name: 'Security', color: 'pink' },
    { id: 7, name: 'Testing', color: 'indigo' },
    { id: 8, name: 'Maintenance', color: 'gray' },
  ]

  const statusCategories = [
    { id: 1, name: 'To Do', color: '#6b7280' },
    { id: 2, name: 'In Progress', color: '#3b82f6' },
    { id: 3, name: 'In Review', color: '#f59e0b' },
    { id: 4, name: 'Done', color: '#22c55e' },
    { id: 5, name: 'Blocked', color: '#ef4444' },
  ]

  let selectedIds = $state([1, 3])
  let statusIds = $state([2])
  let emptyIds = $state([])

  function handleChange(event) {
    console.log('Changed:', event)
  }
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">CategoryMultiSelect</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    A multi-select component for categories with color indicators and chip display. Selected items appear as colored chips above the dropdown trigger.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Click to open dropdown. Selected categories appear as colored chips. Click the X on a chip to remove it.
    </p>
    <div class="w-80">
      <CategoryMultiSelect
        bind:selectedIds
        {categories}
        placeholder="Select categories..."
        on_change={handleChange}
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">
      Selected IDs: [{selectedIds.join(', ')}]
    </p>
  </section>

  <!-- With Label -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Label</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Add a label using the <code>label</code> prop.
    </p>
    <div class="w-80">
      <CategoryMultiSelect
        bind:selectedIds={statusIds}
        categories={statusCategories}
        label="Status Categories"
        placeholder="Select statuses..."
      />
    </div>
  </section>

  <!-- With Helper Text -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Helper Text</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Add helper text below the component.
    </p>
    <div class="w-80">
      <CategoryMultiSelect
        bind:selectedIds={emptyIds}
        {categories}
        label="Item Types"
        placeholder="Select item types..."
        helperText="Select one or more item types to filter by"
      />
    </div>
  </section>

  <!-- Hex Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Hex Colors</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Categories can use either color names ('red', 'blue', etc.) or hex colors.
    </p>
    <div class="w-80">
      <CategoryMultiSelect
        selectedIds={[1, 2, 3]}
        categories={statusCategories}
        placeholder="Select statuses..."
      />
    </div>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="space-y-4">
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Disabled</p>
        <div class="w-80">
          <CategoryMultiSelect
            selectedIds={[1, 2]}
            {categories}
            disabled={true}
          />
        </div>
      </div>
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Empty</p>
        <div class="w-80">
          <CategoryMultiSelect
            selectedIds={[]}
            {categories}
            placeholder="No categories selected"
          />
        </div>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context: Filter Form</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Example of CategoryMultiSelect in a filter panel.
    </p>
    <div
      class="p-6 rounded-lg max-w-md"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Filter Items</h3>

      <div class="space-y-4">
        <CategoryMultiSelect
          bind:selectedIds
          {categories}
          label="Item Types"
          placeholder="All types"
        />

        <CategoryMultiSelect
          bind:selectedIds={statusIds}
          categories={statusCategories}
          label="Statuses"
          placeholder="All statuses"
        />
      </div>

      <div class="mt-6 pt-4 border-t flex gap-2" style="border-color: var(--ds-border);">
        <button
          class="px-4 py-2 rounded text-sm font-medium text-white"
          style="background-color: var(--ds-background-brand-bold);"
        >
          Apply Filters
        </button>
        <button
          class="px-4 py-2 rounded text-sm"
          style="color: var(--ds-text-subtle);"
          onclick={() => { selectedIds = []; statusIds = []; }}
        >
          Clear All
        </button>
      </div>
    </div>
  </section>

  <!-- Color Names Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Supported Color Names</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="flex flex-wrap gap-3">
        {#each ['red', 'orange', 'yellow', 'green', 'blue', 'indigo', 'purple', 'pink', 'gray', 'slate'] as color}
          {@const hexMap = {
            red: '#ef4444', orange: '#f97316', yellow: '#eab308', green: '#22c55e',
            blue: '#3b82f6', indigo: '#6366f1', purple: '#a855f7', pink: '#ec4899',
            gray: '#6b7280', slate: '#64748b'
          }}
          <div class="flex items-center gap-2">
            <div class="w-4 h-4 rounded-full" style="background-color: {hexMap[color]};"></div>
            <span class="text-sm" style="color: var(--ds-text);">{color}</span>
          </div>
        {/each}
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
            <td class="p-2"><code>categories</code></td>
            <td class="p-2">{`{id, name, color}[]`}</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>selectedIds</code></td>
            <td class="p-2">number[] (bindable)</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select categories...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>helperText</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Callback Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Callbacks</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Signature</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr>
            <td class="p-2"><code>on_change</code></td>
            <td class="p-2">{`({ selectedIds, added?, removed? }) => void`}</td>
            <td class="p-2">Called when selection changes. Includes which ID was added or removed.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
