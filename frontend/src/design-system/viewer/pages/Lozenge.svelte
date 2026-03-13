<script>
  import Lozenge from '../../../lib/components/Lozenge.svelte'

  const colors = [
    'red', 'orange', 'amber', 'yellow', 'lime', 'green', 'emerald', 'teal',
    'cyan', 'sky', 'blue', 'indigo', 'violet', 'purple', 'fuchsia', 'pink',
    'rose', 'zinc', 'gray'
  ];
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Lozenge</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Colored badge component for status indicators, tags, and labels. Supports 19 color variants.
  </p>

  <!-- All Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">All Colors</h2>
    <div class="flex flex-wrap gap-2">
      {#each colors as color}
        <Lozenge {color} text={color} />
      {/each}
    </div>
  </section>

  <!-- Rounded Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Rounded Variants</h2>
    <div class="flex flex-wrap gap-4">
      <div class="space-y-2">
        <p class="text-xs font-medium" style="color: var(--ds-text-subtle);">rounded-md (default)</p>
        <div class="flex gap-2">
          <Lozenge color="blue" text="Default" rounded="rounded-md" />
          <Lozenge color="green" text="Active" rounded="rounded-md" />
          <Lozenge color="red" text="Error" rounded="rounded-md" />
        </div>
      </div>
      <div class="space-y-2">
        <p class="text-xs font-medium" style="color: var(--ds-text-subtle);">rounded</p>
        <div class="flex gap-2">
          <Lozenge color="blue" text="Default" rounded="rounded" />
          <Lozenge color="green" text="Active" rounded="rounded" />
          <Lozenge color="red" text="Error" rounded="rounded" />
        </div>
      </div>
    </div>
  </section>

  <!-- Common Use Cases -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Common Use Cases</h2>
    <div class="space-y-6">
      <!-- Status indicators -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Status Indicators</h3>
        <div class="flex gap-2">
          <Lozenge color="green" text="Active" />
          <Lozenge color="yellow" text="Pending" />
          <Lozenge color="red" text="Inactive" />
          <Lozenge color="zinc" text="Archived" />
        </div>
      </div>

      <!-- Priority levels -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Priority Levels</h3>
        <div class="flex gap-2">
          <Lozenge color="red" text="Critical" />
          <Lozenge color="orange" text="High" />
          <Lozenge color="amber" text="Medium" />
          <Lozenge color="blue" text="Low" />
        </div>
      </div>

      <!-- Categories -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Categories</h3>
        <div class="flex gap-2">
          <Lozenge color="purple" text="Feature" />
          <Lozenge color="red" text="Bug" />
          <Lozenge color="cyan" text="Enhancement" />
          <Lozenge color="gray" text="Documentation" />
        </div>
      </div>

      <!-- Workflow states -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Workflow States</h3>
        <div class="flex gap-2">
          <Lozenge color="zinc" text="Backlog" />
          <Lozenge color="blue" text="In Progress" />
          <Lozenge color="amber" text="Review" />
          <Lozenge color="green" text="Done" />
        </div>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div
      class="p-4 rounded-lg space-y-3"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      {#each [
        { title: 'Implement user authentication', status: 'In Progress', statusColor: 'blue', priority: 'High', priorityColor: 'orange' },
        { title: 'Fix navigation bug', status: 'Done', statusColor: 'green', priority: 'Critical', priorityColor: 'red' },
        { title: 'Update documentation', status: 'Backlog', statusColor: 'zinc', priority: 'Low', priorityColor: 'blue' }
      ] as item}
        <div class="flex items-center justify-between py-2 border-b last:border-b-0" style="border-color: var(--ds-border);">
          <span class="text-sm" style="color: var(--ds-text);">{item.title}</span>
          <div class="flex gap-2">
            <Lozenge color={item.statusColor} text={item.status} />
            <Lozenge color={item.priorityColor} text={item.priority} />
          </div>
        </div>
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
            <td class="p-2"><code>color</code></td>
            <td class="p-2">string (see color list)</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>text</code></td>
            <td class="p-2">string</td>
            <td class="p-2">-</td>
          </tr>
          <tr>
            <td class="p-2"><code>rounded</code></td>
            <td class="p-2">'rounded' | 'rounded-md'</td>
            <td class="p-2">'rounded-md'</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Color Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Available Colors</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        <code>red</code>, <code>orange</code>, <code>amber</code>, <code>yellow</code>, <code>lime</code>,
        <code>green</code>, <code>emerald</code>, <code>teal</code>, <code>cyan</code>, <code>sky</code>,
        <code>blue</code>, <code>indigo</code>, <code>violet</code>, <code>purple</code>, <code>fuchsia</code>,
        <code>pink</code>, <code>rose</code>, <code>zinc</code>, <code>gray</code> (alias: <code>grey</code>)
      </p>
    </div>
  </section>
</div>
