<script>
  import ActionButton from '../../../lib/layout/ActionButton.svelte';
  import { Edit, Trash2, Copy, Download, Upload, RefreshCw, Settings, Plus, Eye, EyeOff, Star, Heart } from 'lucide-svelte';
</script>

<div class="space-y-8">
  <div>
    <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">ActionButton</h1>
    <p style="color: var(--ds-text-subtle);">
      Small action button with icon and optional label.
      Use in toolbars, action groups, and compact UI areas.
    </p>
  </div>

  <!-- Variants -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Variants</h2>
    <div class="flex flex-wrap gap-3">
      <ActionButton icon={Edit} label="Ghost" variant="ghost" />
      <ActionButton icon={Edit} label="Subtle" variant="subtle" />
      <ActionButton icon={Trash2} label="Danger" variant="danger" />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ActionButton icon={Edit} label="Ghost" variant="ghost" />
<ActionButton icon={Edit} label="Subtle" variant="subtle" />
<ActionButton icon={Trash2} label="Danger" variant="danger" />`}
    </pre>
  </section>

  <!-- Sizes -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="flex items-center gap-3">
      <ActionButton icon={Edit} label="Extra Small" size="xs" />
      <ActionButton icon={Edit} label="Small" size="sm" />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ActionButton icon={Edit} label="Extra Small" size="xs" />
<ActionButton icon={Edit} label="Small" size="sm" />`}
    </pre>
  </section>

  <!-- Icon Only -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Icon Only</h2>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      Use without label or with <code class="px-1 py-0.5 rounded" style="background-color: var(--ds-background-neutral);">showLabel={false}</code> for compact toolbars.
    </p>
    <div class="flex gap-2">
      <ActionButton icon={Edit} title="Edit" />
      <ActionButton icon={Copy} title="Copy" />
      <ActionButton icon={Download} title="Download" />
      <ActionButton icon={Trash2} title="Delete" variant="danger" />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ActionButton icon={Edit} title="Edit" />
<ActionButton icon={Copy} title="Copy" />
<ActionButton icon={Download} title="Download" />
<ActionButton icon={Trash2} title="Delete" variant="danger" />`}
    </pre>
  </section>

  <!-- Toolbar Example -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Toolbar Example</h2>
    <div class="p-4 rounded border flex items-center gap-1" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <ActionButton icon={Plus} label="Add" />
      <ActionButton icon={Edit} label="Edit" />
      <ActionButton icon={Copy} label="Duplicate" />
      <div class="w-px h-6 mx-2" style="background-color: var(--ds-border);"></div>
      <ActionButton icon={RefreshCw} title="Refresh" />
      <ActionButton icon={Settings} title="Settings" />
      <div class="flex-1"></div>
      <ActionButton icon={Trash2} variant="danger" title="Delete" />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<div class="flex items-center gap-1">
  <ActionButton icon={Plus} label="Add" />
  <ActionButton icon={Edit} label="Edit" />
  <ActionButton icon={Copy} label="Duplicate" />
  <div class="w-px h-6 mx-2" style="background-color: var(--ds-border);"></div>
  <ActionButton icon={RefreshCw} title="Refresh" />
  <ActionButton icon={Settings} title="Settings" />
  <div class="flex-1"></div>
  <ActionButton icon={Trash2} variant="danger" title="Delete" />
</div>`}
    </pre>
  </section>

  <!-- States -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="flex flex-wrap gap-3">
      <ActionButton icon={Edit} label="Normal" />
      <ActionButton icon={Edit} label="Disabled" disabled />
      <ActionButton icon={RefreshCw} label="Loading" loading />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ActionButton icon={Edit} label="Normal" />
<ActionButton icon={Edit} label="Disabled" disabled />
<ActionButton icon={RefreshCw} label="Loading" loading />`}
    </pre>
  </section>

  <!-- Toggle Example -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Toggle Patterns</h2>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      Use variant="subtle" for toggleable actions in "on" state.
    </p>
    <div class="flex gap-2">
      <ActionButton icon={Eye} label="Show" variant="subtle" />
      <ActionButton icon={EyeOff} label="Hide" variant="ghost" />
      <div class="w-px h-8 mx-2" style="background-color: var(--ds-border);"></div>
      <ActionButton icon={Star} variant="subtle" title="Starred" />
      <ActionButton icon={Heart} variant="ghost" title="Not favorited" />
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
            <td class="py-2 pr-4 font-mono text-xs">icon</td>
            <td class="py-2 pr-4 font-mono text-xs">Component</td>
            <td class="py-2 pr-4 font-mono text-xs">required</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Lucide icon component</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">label</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">''</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Button label text</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">showLabel</td>
            <td class="py-2 pr-4 font-mono text-xs">boolean</td>
            <td class="py-2 pr-4 font-mono text-xs">true</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Whether to show label</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">variant</td>
            <td class="py-2 pr-4 font-mono text-xs">'ghost' | 'subtle' | 'danger'</td>
            <td class="py-2 pr-4 font-mono text-xs">'ghost'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Visual variant</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">size</td>
            <td class="py-2 pr-4 font-mono text-xs">'xs' | 'sm'</td>
            <td class="py-2 pr-4 font-mono text-xs">'sm'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Button size</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">disabled</td>
            <td class="py-2 pr-4 font-mono text-xs">boolean</td>
            <td class="py-2 pr-4 font-mono text-xs">false</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Disable the button</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">loading</td>
            <td class="py-2 pr-4 font-mono text-xs">boolean</td>
            <td class="py-2 pr-4 font-mono text-xs">false</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Show loading spinner</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">title</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">''</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Tooltip text</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">onclick</td>
            <td class="py-2 pr-4 font-mono text-xs">() => void</td>
            <td class="py-2 pr-4 font-mono text-xs">null</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Click handler</td>
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
