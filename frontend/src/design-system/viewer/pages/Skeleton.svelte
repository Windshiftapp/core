<script>
  import Skeleton from '../../../lib/components/Skeleton.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Skeleton</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Loading placeholder component for content that is still loading, with support for text, circle, and rectangle variants.
  </p>

  <!-- Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Variants</h2>
    <div class="space-y-6">
      <!-- Text -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Text (default)</h3>
        <div class="max-w-md">
          <Skeleton variant="text" />
        </div>
      </div>

      <!-- Multi-line Text -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Multi-line Text</h3>
        <div class="max-w-md">
          <Skeleton variant="text" lines={3} />
        </div>
      </div>

      <!-- Circle -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Circle (avatars)</h3>
        <div class="flex gap-4">
          <Skeleton variant="circle" width="2rem" />
          <Skeleton variant="circle" width="2.5rem" />
          <Skeleton variant="circle" width="3rem" />
          <Skeleton variant="circle" width="4rem" />
        </div>
      </div>

      <!-- Rectangle -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Rectangle (images, cards)</h3>
        <div class="flex gap-4">
          <Skeleton variant="rect" width="100px" height="100px" />
          <Skeleton variant="rect" width="200px" height="100px" />
        </div>
      </div>
    </div>
  </section>

  <!-- Animation -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Animation</h2>
    <div class="flex gap-8">
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Animated (default)</h3>
        <Skeleton variant="rect" width="150px" height="80px" />
      </div>
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Static</h3>
        <Skeleton variant="rect" width="150px" height="80px" animate={false} />
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div class="space-y-6">
      <!-- Card Loading -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Card Loading State</h3>
        <div
          class="p-4 rounded-lg max-w-sm"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="flex gap-3 items-start">
            <Skeleton variant="circle" width="2.5rem" />
            <div class="flex-1">
              <Skeleton variant="text" width="60%" class="mb-2" />
              <Skeleton variant="text" lines={2} />
            </div>
          </div>
        </div>
      </div>

      <!-- List Loading -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">List Loading State</h3>
        <div
          class="rounded-lg overflow-hidden max-w-md"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          {#each [1, 2, 3] as _}
            <div class="p-4 flex gap-3 items-center" style="border-bottom: 1px solid var(--ds-border);">
              <Skeleton variant="circle" width="2rem" />
              <div class="flex-1">
                <Skeleton variant="text" width="40%" />
              </div>
              <Skeleton variant="rect" width="60px" height="24px" />
            </div>
          {/each}
        </div>
      </div>

      <!-- Table Loading -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Table Loading State</h3>
        <div
          class="rounded-lg overflow-hidden"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="p-3 flex gap-4" style="border-bottom: 1px solid var(--ds-border);">
            <Skeleton variant="text" width="20%" />
            <Skeleton variant="text" width="30%" />
            <Skeleton variant="text" width="25%" />
            <Skeleton variant="text" width="15%" />
          </div>
          {#each [1, 2, 3, 4] as _}
            <div class="p-3 flex gap-4" style="border-bottom: 1px solid var(--ds-border);">
              <Skeleton variant="text" width="20%" />
              <Skeleton variant="text" width="30%" />
              <Skeleton variant="text" width="25%" />
              <Skeleton variant="text" width="15%" />
            </div>
          {/each}
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
            <td class="p-2"><code>variant</code></td>
            <td class="p-2">'text' | 'circle' | 'rect'</td>
            <td class="p-2">'text'</td>
            <td class="p-2">Shape of the skeleton</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>width</code></td>
            <td class="p-2">string | null</td>
            <td class="p-2">null</td>
            <td class="p-2">CSS width value</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>height</code></td>
            <td class="p-2">string | null</td>
            <td class="p-2">null</td>
            <td class="p-2">CSS height value</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>lines</code></td>
            <td class="p-2">number</td>
            <td class="p-2">1</td>
            <td class="p-2">Number of lines (text variant only)</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>animate</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
            <td class="p-2">Enable pulse animation</td>
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
