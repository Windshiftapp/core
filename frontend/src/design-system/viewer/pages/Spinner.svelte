<script>
  import Spinner from '../../../lib/Spinner.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Spinner</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Loading spinner component with size variants for indicating async operations.
  </p>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="flex items-center gap-8">
      <div class="text-center">
        <div class="flex items-center justify-center h-16">
          <Spinner size="sm" />
        </div>
        <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">Small (sm)</p>
      </div>
      <div class="text-center">
        <div class="flex items-center justify-center h-16">
          <Spinner size="md" />
        </div>
        <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">Medium (md)</p>
      </div>
      <div class="text-center">
        <div class="flex items-center justify-center h-16">
          <Spinner size="lg" />
        </div>
        <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">Large (lg)</p>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div class="space-y-6">
      <!-- Button loading -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Button Loading State</h3>
        <button
          class="px-4 py-2 rounded text-white flex items-center gap-2"
          style="background-color: var(--ds-interactive);"
          disabled
        >
          <Spinner size="sm" class="border-white" />
          Loading...
        </button>
      </div>

      <!-- Card loading -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Card Loading State</h3>
        <div
          class="p-8 rounded-lg flex items-center justify-center"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="text-center">
            <Spinner size="md" class="mx-auto" />
            <p class="mt-3 text-sm" style="color: var(--ds-text-subtle);">Loading data...</p>
          </div>
        </div>
      </div>

      <!-- Full page loading -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Full Page Loading</h3>
        <div
          class="h-64 rounded-lg flex items-center justify-center"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="text-center">
            <Spinner size="lg" class="mx-auto" />
            <p class="mt-4 text-sm" style="color: var(--ds-text-subtle);">Please wait...</p>
          </div>
        </div>
      </div>
    </div>
  </section>

  <!-- Branded Loading -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Branded Loading</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Full-page loading states using the Windshift logo with pulse animation.
    </p>

    <div class="space-y-6">
      <!-- Active Loading State -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">App Loading State</h3>
        <div
          class="h-64 rounded-lg flex items-center justify-center"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="text-center">
            <div class="w-16 h-16 mx-auto mb-4">
              <img src="/windshift-3.svg" alt="Windshift" class="w-16 h-16 animate-pulse" />
            </div>
            <p class="text-sm" style="color: var(--ds-text-subtle);">Loading...</p>
          </div>
        </div>
      </div>

      <!-- Background State (behind login) -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Background State</h3>
        <div
          class="h-64 rounded-lg flex items-center justify-center"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="text-center">
            <img src="/windshift-3.svg" alt="Windshift" class="w-16 h-16 mx-auto mb-4 opacity-50" />
            <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text-disabled);">Windshift</h1>
            <p style="color: var(--ds-text-subtle);">Work Management</p>
          </div>
        </div>
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
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'sm' | 'md' | 'lg'</td>
            <td class="p-2">'md'</td>
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

  <!-- Size Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Size Reference</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Size</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Dimensions</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Use Case</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>sm</code></td>
            <td class="p-2">16x16px (h-4 w-4)</td>
            <td class="p-2">Inline, buttons</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>md</code></td>
            <td class="p-2">32x32px (h-8 w-8)</td>
            <td class="p-2">Cards, sections</td>
          </tr>
          <tr>
            <td class="p-2"><code>lg</code></td>
            <td class="p-2">48x48px (h-12 w-12)</td>
            <td class="p-2">Full page loading</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
