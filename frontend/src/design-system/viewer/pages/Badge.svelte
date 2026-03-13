<script>
  import Badge from '../../../lib/components/Badge.svelte';
  import { Info, CheckCircle, AlertTriangle, XCircle, Star, Bell } from 'lucide-svelte';
</script>

<div class="space-y-8">
  <div>
    <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Badge</h1>
    <p style="color: var(--ds-text-subtle);">
      Status pill for informational badges, counts, and small status indicators.
      Different from Lozenge which is for semantic colored tags.
    </p>
  </div>

  <!-- Variants -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Variants</h2>
    <div class="flex flex-wrap gap-3">
      <Badge variant="neutral">Neutral</Badge>
      <Badge variant="info">Info</Badge>
      <Badge variant="success">Success</Badge>
      <Badge variant="warning">Warning</Badge>
      <Badge variant="danger">Danger</Badge>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<Badge variant="neutral">Neutral</Badge>
<Badge variant="info">Info</Badge>
<Badge variant="success">Success</Badge>
<Badge variant="warning">Warning</Badge>
<Badge variant="danger">Danger</Badge>`}
    </pre>
  </section>

  <!-- Sizes -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="flex items-center gap-3">
      <Badge size="xs">xs</Badge>
      <Badge size="sm">sm</Badge>
      <Badge size="md">md</Badge>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<Badge size="xs">xs</Badge>
<Badge size="sm">sm</Badge>
<Badge size="md">md</Badge>`}
    </pre>
  </section>

  <!-- With Icons -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Icons</h2>
    <div class="flex flex-wrap gap-3">
      <Badge variant="info" icon={Info}>Information</Badge>
      <Badge variant="success" icon={CheckCircle}>Completed</Badge>
      <Badge variant="warning" icon={AlertTriangle}>Warning</Badge>
      <Badge variant="danger" icon={XCircle}>Error</Badge>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<Badge variant="info" icon={Info}>Information</Badge>
<Badge variant="success" icon={CheckCircle}>Completed</Badge>
<Badge variant="warning" icon={AlertTriangle}>Warning</Badge>
<Badge variant="danger" icon={XCircle}>Error</Badge>`}
    </pre>
  </section>

  <!-- Use Cases -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Common Use Cases</h2>
    <div class="space-y-4">
      <div class="flex items-center gap-2">
        <span style="color: var(--ds-text);">Notifications</span>
        <Badge variant="danger" size="xs">3</Badge>
      </div>
      <div class="flex items-center gap-2">
        <span style="color: var(--ds-text);">Status</span>
        <Badge variant="success" icon={CheckCircle}>Active</Badge>
      </div>
      <div class="flex items-center gap-2">
        <span style="color: var(--ds-text);">New Feature</span>
        <Badge variant="info" icon={Star}>New</Badge>
      </div>
      <div class="flex items-center gap-2">
        <span style="color: var(--ds-text);">Alerts</span>
        <Badge variant="warning" icon={Bell}>5 pending</Badge>
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
            <td class="py-2 pr-4 font-mono text-xs">variant</td>
            <td class="py-2 pr-4 font-mono text-xs">'neutral' | 'info' | 'success' | 'warning' | 'danger'</td>
            <td class="py-2 pr-4 font-mono text-xs">'neutral'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Color variant</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">size</td>
            <td class="py-2 pr-4 font-mono text-xs">'xs' | 'sm' | 'md'</td>
            <td class="py-2 pr-4 font-mono text-xs">'sm'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Badge size</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">icon</td>
            <td class="py-2 pr-4 font-mono text-xs">Component</td>
            <td class="py-2 pr-4 font-mono text-xs">null</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Optional leading icon</td>
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
