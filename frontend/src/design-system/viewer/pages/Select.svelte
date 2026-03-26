<script>
  import Select from '../../../lib/components/Select.svelte'

  let basicValue = $state('')
  let sizeValue = $state('')
  let disabledValue = $state('a')
  let onchangeValue = $state('')

  const basicOptions = [
    { value: '', label: 'Select...' },
    { value: 'option1', label: 'Option 1' },
    { value: 'option2', label: 'Option 2' },
    { value: 'option3', label: 'Option 3' }
  ]

  const abOptions = [
    { value: '', label: 'Select...' },
    { value: 'a', label: 'Option A' },
    { value: 'b', label: 'Option B' }
  ]

  const disabledOptionDemo = [
    { value: '', label: 'Select...' },
    { value: 'a', label: 'Option A' },
    { value: 'b', label: 'Option B (disabled)', disabled: true },
    { value: 'c', label: 'Option C' }
  ]
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Select</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Custom popover-based select component with keyboard navigation, consistent cross-browser rendering, and size variants.
  </p>

  <!-- Basic Select -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Select</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
          Choose an option
        </label>
        <Select bind:value={basicValue} options={basicOptions} placeholder="Select..." />
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          Selected: {basicValue || '(none)'}
        </p>
      </div>
    </div>
  </section>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Small</label>
        <Select size="small" bind:value={sizeValue} options={abOptions} />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Medium (default)</label>
        <Select size="medium" options={abOptions} />
      </div>
    </div>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Normal</label>
        <Select options={abOptions} placeholder="Select..." />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Disabled</label>
        <Select disabled value={disabledValue} options={abOptions} />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Required</label>
        <Select required options={abOptions} placeholder="Required field" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Disabled Options</label>
        <Select options={disabledOptionDemo} placeholder="Select..." />
      </div>
    </div>
  </section>

  <!-- Onchange callback -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Onchange Callback</h2>
    <div class="max-w-md space-y-4">
      <div>
        <Select
          options={basicOptions}
          onchange={(v) => onchangeValue = v}
          placeholder="Select..."
        />
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          Last onchange value: {onchangeValue || '(none)'}
        </p>
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
            <td class="p-2"><code>options</code></td>
            <td class="p-2">{'{ value, label, disabled? }[]'}</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">string (bindable)</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onchange</code></td>
            <td class="p-2">(value) => void</td>
            <td class="p-2">undefined</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'small' | 'medium'</td>
            <td class="p-2">'medium'</td>
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
            <td class="p-2"><code>id</code></td>
            <td class="p-2">string</td>
            <td class="p-2">undefined</td>
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
