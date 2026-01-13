<script>
  import LoadingState from '../../../lib/LoadingState.svelte';
</script>

<div class="space-y-8">
  <div>
    <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">LoadingState</h1>
    <p style="color: var(--ds-text-subtle);">
      Centered loading spinner with optional message.
      Use for full-page or section loading states.
    </p>
  </div>

  <!-- Basic -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <LoadingState />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<LoadingState />`}
    </pre>
  </section>

  <!-- Custom Message -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Message</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <LoadingState message="Loading items..." />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<LoadingState message="Loading items..." />`}
    </pre>
  </section>

  <!-- No Message -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">No Message</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <LoadingState message="" />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<LoadingState message="" />`}
    </pre>
  </section>

  <!-- Sizes -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="grid grid-cols-3 gap-4">
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <LoadingState size="sm" message="Small" />
      </div>
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <LoadingState size="md" message="Medium" />
      </div>
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <LoadingState size="lg" message="Large" />
      </div>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<LoadingState size="sm" message="Small" />
<LoadingState size="md" message="Medium" />
<LoadingState size="lg" message="Large" />`}
    </pre>
  </section>

  <!-- Inline -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Inline Mode</h2>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      Use <code class="px-1 py-0.5 rounded" style="background-color: var(--ds-background-neutral);">inline</code> for horizontal layout in buttons or compact spaces.
    </p>
    <div class="p-4 rounded border space-y-4" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <LoadingState inline message="Saving..." size="sm" />
      <LoadingState inline message="Processing request..." />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<LoadingState inline message="Saving..." size="sm" />
<LoadingState inline message="Processing request..." />`}
    </pre>
  </section>

  <!-- Use Cases -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Common Use Cases</h2>
    <div class="space-y-4">
      <div>
        <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">Page loading:</p>
        <div class="p-4 rounded border h-48 flex items-center justify-center" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
          <LoadingState message="Loading workspace..." />
        </div>
      </div>
      <div>
        <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">Data fetching:</p>
        <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
          <LoadingState size="sm" message="Fetching items..." class="py-4" />
        </div>
      </div>
    </div>
  </section>

  <!-- Props -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Props</h2>
    <div class="overflow-x-auto">
      <table class="w-full text-sm" style="color: var(--ds-text);">
        <thead>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <th class="text-left py-2 pr-4">Prop</th>
            <th class="text-left py-2 pr-4">Type</th>
            <th class="text-left py-2 pr-4">Default</th>
            <th class="text-left py-2">Description</th>
          </tr>
        </thead>
        <tbody>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">message</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">'Loading...'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Loading message text</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">size</td>
            <td class="py-2 pr-4 font-mono text-xs">'sm' | 'md' | 'lg'</td>
            <td class="py-2 pr-4 font-mono text-xs">'md'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Spinner size</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">inline</td>
            <td class="py-2 pr-4 font-mono text-xs">boolean</td>
            <td class="py-2 pr-4 font-mono text-xs">false</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Horizontal layout mode</td>
          </tr>
          <tr>
            <td class="py-2 pr-4 font-mono text-xs">class</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">''</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Additional CSS classes</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
