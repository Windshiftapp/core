<script>
  import Progress from '../../../lib/Progress.svelte'

  let barValue = $state(65)
  let circularValue = $state(75)
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Progress</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Progress indicator component for showing completion status, available in bar and circular variants.
  </p>

  <!-- Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Variants</h2>
    <div class="space-y-6">
      <!-- Bar -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Bar (default)</h3>
        <div class="max-w-md">
          <Progress value={barValue} variant="bar" />
        </div>
      </div>

      <!-- Circular -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Circular</h3>
        <div class="flex gap-6">
          <Progress value={circularValue} variant="circular" size="sm" showLabel />
          <Progress value={circularValue} variant="circular" size="md" showLabel />
          <Progress value={circularValue} variant="circular" size="lg" showLabel />
        </div>
      </div>
    </div>
  </section>

  <!-- Sizes (Bar) -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Bar Sizes</h2>
    <div class="space-y-4 max-w-md">
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Small (sm)</p>
        <Progress value={60} size="sm" />
      </div>
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Medium (md) - default</p>
        <Progress value={60} size="md" />
      </div>
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Large (lg)</p>
        <Progress value={60} size="lg" />
      </div>
    </div>
  </section>

  <!-- Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Colors</h2>
    <div class="space-y-4 max-w-md">
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Primary (default)</p>
        <Progress value={70} color="primary" />
      </div>
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Success</p>
        <Progress value={100} color="success" />
      </div>
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Warning</p>
        <Progress value={45} color="warning" />
      </div>
      <div>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">Danger</p>
        <Progress value={25} color="danger" />
      </div>
    </div>
  </section>

  <!-- With Labels -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Labels</h2>
    <div class="space-y-4 max-w-md">
      <div>
        <Progress value={65} showLabel />
      </div>
      <div>
        <Progress value={80} showLabel label="Upload progress" />
      </div>
      <div>
        <Progress value={42} showLabel label="Tasks completed" />
      </div>
    </div>
  </section>

  <!-- Interactive Demo -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Interactive Demo</h2>
    <div
      class="p-6 rounded-lg max-w-md"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="mb-4">
        <label class="block text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">
          Bar Progress: {barValue}%
        </label>
        <input
          type="range"
          min="0"
          max="100"
          bind:value={barValue}
          class="w-full"
        />
        <Progress value={barValue} showLabel class="mt-3" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">
          Circular Progress: {circularValue}%
        </label>
        <input
          type="range"
          min="0"
          max="100"
          bind:value={circularValue}
          class="w-full"
        />
        <div class="mt-3 flex justify-center">
          <Progress value={circularValue} variant="circular" size="lg" showLabel />
        </div>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div class="space-y-6">
      <!-- File Upload -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">File Upload</h3>
        <div
          class="p-4 rounded-lg max-w-md"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="flex items-center gap-3 mb-3">
            <div class="w-10 h-10 rounded flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
              <span style="color: var(--ds-text-subtle);">PDF</span>
            </div>
            <div class="flex-1">
              <p class="text-sm font-medium" style="color: var(--ds-text);">document.pdf</p>
              <p class="text-xs" style="color: var(--ds-text-subtle);">2.4 MB</p>
            </div>
          </div>
          <Progress value={73} showLabel size="sm" />
        </div>
      </div>

      <!-- Task Completion -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Task Completion</h3>
        <div
          class="p-4 rounded-lg max-w-md"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="flex items-center justify-between mb-2">
            <span class="text-sm font-medium" style="color: var(--ds-text);">Sprint Progress</span>
            <span class="text-sm" style="color: var(--ds-text-subtle);">8 of 12 tasks</span>
          </div>
          <Progress value={67} color="success" />
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
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number</td>
            <td class="p-2">0</td>
            <td class="p-2">Current progress value</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>max</code></td>
            <td class="p-2">number</td>
            <td class="p-2">100</td>
            <td class="p-2">Maximum value</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>variant</code></td>
            <td class="p-2">'bar' | 'circular'</td>
            <td class="p-2">'bar'</td>
            <td class="p-2">Progress indicator style</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'sm' | 'md' | 'lg'</td>
            <td class="p-2">'md'</td>
            <td class="p-2">Size of the progress indicator</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>color</code></td>
            <td class="p-2">'primary' | 'success' | 'warning' | 'danger'</td>
            <td class="p-2">'primary'</td>
            <td class="p-2">Color of the progress fill</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showLabel</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
            <td class="p-2">Show percentage label</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
            <td class="p-2">Custom label text (bar variant)</td>
          </tr>
          <tr>
            <td class="p-2"><code>class</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
            <td class="p-2">Additional CSS classes</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
