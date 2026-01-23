<script>
  import StatusCircle from '../../../lib/components/StatusCircle.svelte';
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">StatusCircle</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Circular status indicator for showing step progress, task completion, or workflow states.
    Supports completed, active, and pending states with customizable sizes and colors.
  </p>

  <!-- Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Status States</h2>
    <div class="space-y-4">
      <div class="flex items-center gap-3">
        <StatusCircle status="completed" />
        <span style="color: var(--ds-text);">Completed - Blue filled circle with checkmark</span>
      </div>
      <div class="flex items-center gap-3">
        <StatusCircle status="active" />
        <span style="color: var(--ds-text);">Active - Circle with border and inner dot</span>
      </div>
      <div class="flex items-center gap-3">
        <StatusCircle status="pending" />
        <span style="color: var(--ds-text);">Pending - Empty circle with border</span>
      </div>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<StatusCircle status="completed" />
<StatusCircle status="active" />
<StatusCircle status="pending" />`}
    </pre>
  </section>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="space-y-4">
      <div class="flex items-center gap-3">
        <StatusCircle status="completed" size="sm" />
        <span style="color: var(--ds-text);">Small (sm) - 20px</span>
      </div>
      <div class="flex items-center gap-3">
        <StatusCircle status="active" size="md" />
        <span style="color: var(--ds-text);">Medium (md) - 24px (default)</span>
      </div>
      <div class="flex items-center gap-3">
        <StatusCircle status="pending" size="lg" />
        <span style="color: var(--ds-text);">Large (lg) - 28px</span>
      </div>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<StatusCircle status="completed" size="sm" />
<StatusCircle status="active" size="md" />
<StatusCircle status="pending" size="lg" />`}
    </pre>
  </section>

  <!-- Custom Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Colors</h2>
    <div class="space-y-4">
      <div class="flex items-center gap-3">
        <StatusCircle status="completed" color="#1388E7" />
        <span style="color: var(--ds-text);">Blue (#1388E7) - default</span>
      </div>
      <div class="flex items-center gap-3">
        <StatusCircle status="active" color="#10B981" />
        <span style="color: var(--ds-text);">Green (#10B981)</span>
      </div>
      <div class="flex items-center gap-3">
        <StatusCircle status="completed" color="#F59E0B" />
        <span style="color: var(--ds-text);">Amber (#F59E0B)</span>
      </div>
      <div class="flex items-center gap-3">
        <StatusCircle status="active" color="#EF4444" />
        <span style="color: var(--ds-text);">Red (#EF4444)</span>
      </div>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<StatusCircle status="completed" color="#1388E7" />
<StatusCircle status="active" color="#10B981" />
<StatusCircle status="completed" color="#F59E0B" />
<StatusCircle status="active" color="#EF4444" />`}
    </pre>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div class="space-y-6">
      <!-- Onboarding Steps -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Onboarding Steps</h3>
        <div
          class="p-4 rounded-lg max-w-md"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="space-y-4">
            <div class="flex items-start gap-3">
              <StatusCircle status="completed" />
              <div class="flex-1">
                <h4 class="text-sm font-medium" style="color: var(--ds-text);">Create Workspace</h4>
                <p class="text-sm" style="color: var(--ds-text-subtle);">Your workspace is ready</p>
              </div>
            </div>
            <div class="flex items-start gap-3">
              <StatusCircle status="active" />
              <div class="flex-1">
                <h4 class="text-sm font-medium" style="color: var(--ds-text);">Add Team Members</h4>
                <p class="text-sm" style="color: var(--ds-text-subtle);">Invite your team to collaborate</p>
              </div>
            </div>
            <div class="flex items-start gap-3">
              <StatusCircle status="pending" />
              <div class="flex-1">
                <h4 class="text-sm font-medium" style="color: var(--ds-text-subtle);">Create First Project</h4>
                <p class="text-sm" style="color: var(--ds-text-subtle);">Start organizing your work</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Task Progress -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Task Progress</h3>
        <div
          class="p-4 rounded-lg max-w-md"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div class="flex items-center justify-between mb-3">
            <span class="text-sm font-medium" style="color: var(--ds-text);">Sprint Progress</span>
            <span class="text-sm" style="color: var(--ds-text-subtle);">3 of 5 tasks</span>
          </div>
          <div class="flex gap-2">
            <StatusCircle status="completed" size="sm" />
            <StatusCircle status="completed" size="sm" />
            <StatusCircle status="completed" size="sm" />
            <StatusCircle status="active" size="sm" />
            <StatusCircle status="pending" size="sm" />
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
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>status</code></td>
            <td class="p-2">'completed' | 'active' | 'pending'</td>
            <td class="p-2">'pending'</td>
            <td class="p-2">Status state of the circle</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'sm' | 'md' | 'lg'</td>
            <td class="p-2">'md'</td>
            <td class="p-2">Size of the indicator (sm: 20px, md: 24px, lg: 28px)</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>color</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'#1388E7'</td>
            <td class="p-2">Custom color for completed and active states</td>
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
