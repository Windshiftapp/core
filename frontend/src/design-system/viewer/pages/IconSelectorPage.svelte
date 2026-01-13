<script>
  import IconSelector from '../../../lib/IconSelector.svelte'
  import Label from '../../../lib/Label.svelte'

  let selectedIcon = $state('Target')
  let selectedColor = $state('#3b82f6')

  let icon2 = $state('Rocket')
  let color2 = $state('#8b5cf6')

  function handleChange(event) {
    selectedIcon = event.detail.icon
    selectedColor = event.detail.color
  }
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">IconSelector</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    A grid-based selector for choosing from 70+ Lucide icons with search functionality and a color picker. Ideal for workspace/project branding configuration.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      The component shows a preview, searchable icon grid, and color palette with custom color input.
    </p>
    <div class="max-w-md">
      <IconSelector
        bind:selectedIcon
        bind:selectedColor
        label="Select Icon & Color"
        on:change={handleChange}
      />
    </div>
    <p class="mt-4 text-xs" style="color: var(--ds-text-subtle);">
      Selected: {selectedIcon} with color {selectedColor}
    </p>
  </section>

  <!-- Side by Side -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Multiple Selectors</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Each selector maintains its own state independently.
    </p>
    <div class="grid grid-cols-2 gap-8">
      <div>
        <IconSelector
          bind:selectedIcon
          bind:selectedColor
          label="Primary Icon"
        />
      </div>
      <div>
        <IconSelector
          bind:selectedIcon={icon2}
          bind:selectedColor={color2}
          label="Secondary Icon"
        />
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context: Workspace Settings</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Example of how IconSelector might be used in a workspace configuration form.
    </p>
    <div
      class="p-6 rounded-lg max-w-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Workspace Appearance</h3>

      <div class="mb-4">
        <Label class="mb-2">
          Workspace Name
        </Label>
        <input
          type="text"
          value="Project Alpha"
          class="w-full px-3 py-2 rounded text-sm"
          style="background-color: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        />
      </div>

      <IconSelector
        bind:selectedIcon
        bind:selectedColor
        label="Workspace Icon"
      />

      <div class="mt-6 pt-4 border-t flex gap-2" style="border-color: var(--ds-border);">
        <button
          class="px-4 py-2 rounded text-sm font-medium text-white"
          style="background-color: var(--ds-background-brand-bold);"
        >
          Save Changes
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

  <!-- Preview Examples -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Preview Examples</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      The selected icon and color can be used to create branded workspace badges.
    </p>
    <div class="flex flex-wrap gap-4">
      {#each [
        { icon: 'Target', color: '#3b82f6', name: 'Marketing' },
        { icon: 'Code', color: '#10b981', name: 'Engineering' },
        { icon: 'Palette', color: '#f59e0b', name: 'Design' },
        { icon: 'Shield', color: '#ef4444', name: 'Security' },
        { icon: 'Rocket', color: '#8b5cf6', name: 'Product' },
      ] as item}
        <div
          class="flex items-center gap-3 px-4 py-3 rounded-lg"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div
            class="w-10 h-10 rounded-lg flex items-center justify-center"
            style="background-color: {item.color};"
          >
            <!-- Icon would render here in real usage -->
            <span class="text-white text-xs">{item.icon.slice(0, 2)}</span>
          </div>
          <span class="font-medium" style="color: var(--ds-text);">{item.name}</span>
        </div>
      {/each}
    </div>
  </section>

  <!-- Available Icons -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Available Icons</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      The selector includes 70+ carefully curated Lucide icons covering common use cases.
    </p>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <p class="text-sm" style="color: var(--ds-text);">
        <strong>Categories include:</strong> Actions (Edit, Copy, Save), Objects (Folder, File, Database),
        People (User, Users), Communication (Mail, Phone, MessageSquare), Status (CheckCircle, AlertCircle),
        Navigation (Home, Map, Compass), and many more.
      </p>
    </div>
  </section>

  <!-- Color Palette -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Color Palette</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      25 pre-selected colors with good contrast and variety, plus a custom color picker.
    </p>
    <div class="flex flex-wrap gap-2">
      {#each [
        '#7c3aed', '#2563eb', '#059669', '#dc2626', '#ea580c',
        '#6b7280', '#8b5cf6', '#3b82f6', '#10b981', '#ef4444',
        '#f59e0b', '#84cc16', '#06b6d4', '#ec4899', '#f97316',
        '#64748b', '#7c2d12', '#1e40af', '#065f46', '#991b1b',
        '#92400e', '#365314', '#0e7490', '#be185d', '#9a3412'
      ] as color}
        <div
          class="w-8 h-8 rounded"
          style="background-color: {color};"
          title={color}
        ></div>
      {/each}
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
            <td class="p-2"><code>selectedIcon</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Package'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>selectedColor</code></td>
            <td class="p-2">string (hex)</td>
            <td class="p-2">'#3b82f6'</td>
          </tr>
          <tr>
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Icon & Color'</td>
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
            <td class="p-2"><code>on:change</code></td>
            <td class="p-2">Fired when icon or color changes. Contains <code>detail.icon</code> and <code>detail.color</code>.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
