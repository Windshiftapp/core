<script>
  import SearchInput from '../../../lib/SearchInput.svelte'

  let searchValue = ''
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">SearchInput</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Search input component with built-in search icon and size variants.
  </p>

  <!-- Basic Search -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Search</h2>
    <div class="max-w-md space-y-4">
      <SearchInput bind:value={searchValue} placeholder="Search items..." />
      <p class="text-xs" style="color: var(--ds-text-subtle);">
        Current value: {searchValue || '(empty)'}
      </p>
    </div>
  </section>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Small</label>
        <SearchInput size="small" placeholder="Small search..." />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Medium (default)</label>
        <SearchInput size="medium" placeholder="Medium search..." />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Large</label>
        <SearchInput size="large" placeholder="Large search..." />
      </div>
    </div>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Normal</label>
        <SearchInput placeholder="Search..." />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Disabled</label>
        <SearchInput placeholder="Disabled search..." disabled />
      </div>
    </div>
  </section>

  <!-- With Gradient Background -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Gradient Background</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      The <code>hasGradient</code> prop adjusts styling for use on gradient backgrounds.
    </p>
    <div
      class="max-w-md p-6 rounded-lg"
      style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);"
    >
      <SearchInput hasGradient placeholder="Search on gradient..." />
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
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Search...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'small' | 'medium' | 'large'</td>
            <td class="p-2">'medium'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>hasGradient</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>on_input</code></td>
            <td class="p-2">function</td>
            <td class="p-2">undefined</td>
          </tr>
          <tr>
            <td class="p-2"><code>on_keydown</code></td>
            <td class="p-2">function</td>
            <td class="p-2">undefined</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
