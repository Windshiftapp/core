<script>
  import FieldSelector from '../../../lib/FieldSelector.svelte'
  import Label from '../../../lib/Label.svelte'

  let selectedField = $state(null)
  let selectedField2 = $state(null)

  function handleSelect(event) {
    console.log('Selected field:', event.detail)
  }
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">FieldSelector</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    A dropdown selector for standard and custom fields, grouped by category with type badges. Used in configuration forms like filters, sorting, and column selection.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Click to open the dropdown. Fields are grouped by category with searchable list.
    </p>
    <div class="w-80">
      <FieldSelector
        bind:selectedField
        placeholder="Select a field..."
        on:select={handleSelect}
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">
      Selected: {selectedField?.name || 'None'}
    </p>
  </section>

  <!-- With Selection -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Selection</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Shows the selected field with type badge and clear button.
    </p>
    <div class="w-80">
      <FieldSelector
        selectedField={{ id: 'status', name: 'Status', type: 'enum', description: 'Current status' }}
      />
    </div>
  </section>

  <!-- Disabled State -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Disabled State</h2>
    <div class="w-80">
      <FieldSelector
        selectedField={{ id: 'title', name: 'Title', type: 'text', description: 'Item title' }}
        disabled={true}
      />
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context: Sort Configuration</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Example of FieldSelector used in a sort configuration form.
    </p>
    <div
      class="p-6 rounded-lg max-w-md"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sort Items By</h3>

      <div class="space-y-4">
        <div>
          <Label class="mb-2">
            Primary Sort Field
          </Label>
          <FieldSelector
            bind:selectedField
            placeholder="Select field..."
          />
        </div>

        <div>
          <Label class="mb-2">
            Secondary Sort Field
          </Label>
          <FieldSelector
            bind:selectedField={selectedField2}
            placeholder="Select field..."
          />
        </div>
      </div>

      <div class="mt-6 pt-4 border-t flex gap-2" style="border-color: var(--ds-border);">
        <button
          class="px-4 py-2 rounded text-sm font-medium text-white"
          style="background-color: var(--ds-background-brand-bold);"
        >
          Apply Sort
        </button>
        <button
          class="px-4 py-2 rounded text-sm"
          style="color: var(--ds-text-subtle);"
        >
          Reset
        </button>
      </div>
    </div>
  </section>

  <!-- Field Categories -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Standard Field Categories</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="space-y-3 text-sm">
        <div>
          <span class="font-medium" style="color: var(--ds-text);">Basic:</span>
          <span style="color: var(--ds-text-subtle);"> Title, Description, Key, ID</span>
        </div>
        <div>
          <span class="font-medium" style="color: var(--ds-text);">Status & Priority:</span>
          <span style="color: var(--ds-text-subtle);"> Status, Priority</span>
        </div>
        <div>
          <span class="font-medium" style="color: var(--ds-text);">Assignments:</span>
          <span style="color: var(--ds-text-subtle);"> Assignee, Creator</span>
        </div>
        <div>
          <span class="font-medium" style="color: var(--ds-text);">Projects & Milestones:</span>
          <span style="color: var(--ds-text-subtle);"> Milestone, Project, Item Type</span>
        </div>
        <div>
          <span class="font-medium" style="color: var(--ds-text);">Dates:</span>
          <span style="color: var(--ds-text-subtle);"> Created Date, Updated Date</span>
        </div>
        <div>
          <span class="font-medium" style="color: var(--ds-text);">Hierarchy:</span>
          <span style="color: var(--ds-text-subtle);"> Parent ID, Is Task</span>
        </div>
        <div>
          <span class="font-medium" style="color: var(--ds-text);">Workspace:</span>
          <span style="color: var(--ds-text-subtle);"> Workspace Name, Workspace ID, Workspace Key</span>
        </div>
        <div>
          <span class="font-medium" style="color: var(--ds-text);">Custom Fields:</span>
          <span style="color: var(--ds-text-subtle);"> (loaded from API with "Custom" badge)</span>
        </div>
      </div>
    </div>
  </section>

  <!-- Field Type Badges -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Field Type Badges</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Each field displays a colored badge indicating its type.
    </p>
    <div class="flex flex-wrap gap-2">
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-information); color: var(--ds-text-information);">Text</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-success); color: var(--ds-text-success);">Number</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-discovery); color: var(--ds-text-discovery);">Date</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-warning); color: var(--ds-text-warning);">Select</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-neutral); color: var(--ds-text);">Boolean</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-information); color: var(--ds-text-information);">User</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-danger); color: var(--ds-text-danger);">Reference</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-discovery); color: var(--ds-text-discovery);">Custom</span>
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
            <td class="p-2"><code>selectedField</code></td>
            <td class="p-2">{`{id, name, type, description, isCustom?} | null`}</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select field...'</td>
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
            <td class="p-2">Fired when a field is selected. Contains the full field object.</td>
          </tr>
          <tr>
            <td class="p-2"><code>on:clear</code></td>
            <td class="p-2">Fired when the selection is cleared.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
