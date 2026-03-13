<script>
  import InlineSelectEditor from '../../../lib/editors/InlineSelectEditor.svelte'

  // Mock options for demos
  const statusOptions = [
    { value: 'active', label: 'Active', color: '#22c55e' },
    { value: 'pending', label: 'Pending', color: '#f59e0b' },
    { value: 'paused', label: 'Paused', color: '#6b7280' },
    { value: 'completed', label: 'Completed', color: '#3b82f6' },
  ]

  const priorityOptions = [
    { value: 'critical', label: 'Critical' },
    { value: 'high', label: 'High' },
    { value: 'medium', label: 'Medium' },
    { value: 'low', label: 'Low' },
  ]

  const categoryOptions = [
    { value: 'bug', label: 'Bug' },
    { value: 'feature', label: 'Feature' },
    { value: 'task', label: 'Task' },
    { value: 'improvement', label: 'Improvement' },
  ]

  let statusValue = $state('active')
  let priorityValue = $state('medium')
  let categoryValue = $state(null)
  let requiredValue = $state('bug')

  let statusEditor
  let priorityEditor
  let categoryEditor

  function handleSave(event, editorRef, valueSetter) {
    // Simulate async save
    setTimeout(() => {
      valueSetter(event.detail.value)
      editorRef.confirmSave(event.detail.value)
    }, 500)
  }

  function handleSaveWithError(event, editorRef) {
    // Simulate failed save
    setTimeout(() => {
      editorRef.rejectSave('Permission denied')
    }, 500)
  }
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">InlineSelectEditor</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    An inline editable dropdown that toggles between display and edit mode. Includes save/cancel actions with async save support and error handling.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Click the value to edit. The component auto-saves on selection change.
    </p>
    <div class="flex items-center gap-2">
      <span class="text-sm" style="color: var(--ds-text);">Status:</span>
      <InlineSelectEditor
        bind:this={statusEditor}
        value={statusValue}
        options={statusOptions}
        placeholder="Select status..."
        on:save={(e) => handleSave(e, statusEditor, v => statusValue = v)}
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Current value: {statusValue}</p>
  </section>

  <!-- With Color Indicators -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Color Indicators</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Options with a <code>color</code> property display a color dot in the display mode.
    </p>
    <div
      class="p-4 rounded-lg inline-block"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="flex items-center gap-4">
        <span class="text-sm font-medium" style="color: var(--ds-text);">Project Status:</span>
        <InlineSelectEditor
          bind:this={statusEditor}
          value={statusValue}
          options={statusOptions}
          on:save={(e) => handleSave(e, statusEditor, v => statusValue = v)}
        />
      </div>
    </div>
  </section>

  <!-- Without Color -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Without Color</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Options without a <code>color</code> property display as plain text.
    </p>
    <div class="flex items-center gap-2">
      <span class="text-sm" style="color: var(--ds-text);">Priority:</span>
      <InlineSelectEditor
        bind:this={priorityEditor}
        value={priorityValue}
        options={priorityOptions}
        on:save={(e) => handleSave(e, priorityEditor, v => priorityValue = v)}
      />
    </div>
  </section>

  <!-- With Allow Clear -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Allow Clear</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Enable <code>allowClear</code> to show a "None" option for clearing the value.
    </p>
    <div class="flex items-center gap-2">
      <span class="text-sm" style="color: var(--ds-text);">Category:</span>
      <InlineSelectEditor
        bind:this={categoryEditor}
        value={categoryValue}
        options={categoryOptions}
        placeholder="Select category..."
        allowClear={true}
        on:save={(e) => handleSave(e, categoryEditor, v => categoryValue = v)}
      />
    </div>
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Current value: {categoryValue || 'null'}</p>
  </section>

  <!-- Required Field -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Required Field</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Set <code>required</code> to show validation error when no value is selected.
    </p>
    <div class="flex items-center gap-2">
      <span class="text-sm" style="color: var(--ds-text);">Type:</span>
      <InlineSelectEditor
        value={requiredValue}
        options={categoryOptions}
        placeholder="Required..."
        required={true}
        on:save={(e) => requiredValue = e.detail.value}
      />
    </div>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="space-y-4">
      <div class="flex items-center gap-4">
        <span class="text-sm w-20" style="color: var(--ds-text);">Disabled:</span>
        <InlineSelectEditor
          value="medium"
          options={priorityOptions}
          disabled={true}
        />
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      InlineSelectEditor is ideal for inline editing in tables, lists, or detail views.
    </p>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2" style="color: var(--ds-text);">Task</th>
            <th class="text-left p-2" style="color: var(--ds-text);">Status</th>
            <th class="text-left p-2" style="color: var(--ds-text);">Priority</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2">Implement user auth</td>
            <td class="p-2">
              <InlineSelectEditor value="active" options={statusOptions} />
            </td>
            <td class="p-2">
              <InlineSelectEditor value="high" options={priorityOptions} />
            </td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2">Fix navigation bug</td>
            <td class="p-2">
              <InlineSelectEditor value="pending" options={statusOptions} />
            </td>
            <td class="p-2">
              <InlineSelectEditor value="critical" options={priorityOptions} />
            </td>
          </tr>
          <tr>
            <td class="p-2">Update documentation</td>
            <td class="p-2">
              <InlineSelectEditor value="completed" options={statusOptions} />
            </td>
            <td class="p-2">
              <InlineSelectEditor value="low" options={priorityOptions} />
            </td>
          </tr>
        </tbody>
      </table>
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
            <td class="p-2">any</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>options</code></td>
            <td class="p-2">{`{value, label, color?}[]`}</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>required</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>allowClear</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>className</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr>
            <td class="p-2"><code>displayClass</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'hover:bg-gray-50 cursor-pointer'</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Methods Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Methods</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Method</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>confirmSave(newValue)</code></td>
            <td class="p-2">Call to confirm async save succeeded. Updates internal value and closes edit mode.</td>
          </tr>
          <tr>
            <td class="p-2"><code>rejectSave(errorMsg)</code></td>
            <td class="p-2">Call to indicate save failed. Shows error message and keeps edit mode open.</td>
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
          <tr>
            <td class="p-2"><code>on:save</code></td>
            <td class="p-2">Fired when user selects a value. Contains <code>detail.value</code>.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
