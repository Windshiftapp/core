<script>
  import Select from '../../../lib/components/Select.svelte'

  let basicValue = $state('')
  let sizeValue = $state('')
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Select</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Dropdown select component with size variants.
  </p>

  <!-- Basic Select -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Select</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
          Choose an option
        </label>
        <Select bind:value={basicValue}>
          <option value="">Select...</option>
          <option value="option1">Option 1</option>
          <option value="option2">Option 2</option>
          <option value="option3">Option 3</option>
        </Select>
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
        <Select size="small" bind:value={sizeValue}>
          <option value="">Select...</option>
          <option value="a">Option A</option>
          <option value="b">Option B</option>
        </Select>
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Medium (default)</label>
        <Select size="medium">
          <option value="">Select...</option>
          <option value="a">Option A</option>
          <option value="b">Option B</option>
        </Select>
      </div>
    </div>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Normal</label>
        <Select>
          <option value="">Select...</option>
          <option value="a">Option A</option>
        </Select>
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Disabled</label>
        <Select disabled>
          <option value="">Disabled select</option>
        </Select>
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Required</label>
        <Select required>
          <option value="">Required field</option>
          <option value="a">Option A</option>
        </Select>
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
            <td class="p-2">string (bindable)</td>
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
          <tr>
            <td class="p-2"><code>children</code></td>
            <td class="p-2">snippet (option elements)</td>
            <td class="p-2">-</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
