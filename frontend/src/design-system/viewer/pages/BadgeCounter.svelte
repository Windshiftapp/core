<script>
  import BadgeCounter from '../../../lib/BadgeCounter.svelte'
  import { Bell, Mail, MessageSquare } from 'lucide-svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">BadgeCounter</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Notification badge component for displaying counts on icons and buttons. Automatically shows "99+" for large numbers.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div class="flex items-center gap-8">
      <div class="relative inline-flex">
        <Bell class="w-6 h-6" style="color: var(--ds-text);" />
        <BadgeCounter count={3} />
      </div>
      <div class="relative inline-flex">
        <Mail class="w-6 h-6" style="color: var(--ds-text);" />
        <BadgeCounter count={12} />
      </div>
      <div class="relative inline-flex">
        <MessageSquare class="w-6 h-6" style="color: var(--ds-text);" />
        <BadgeCounter count={42} />
      </div>
    </div>
  </section>

  <!-- Count Overflow -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Count Overflow</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Numbers above the <code>max</code> prop (default: 99) display as "99+".
    </p>
    <div class="flex items-center gap-8">
      <div class="text-center">
        <div class="relative inline-flex mb-2">
          <Bell class="w-6 h-6" style="color: var(--ds-text);" />
          <BadgeCounter count={50} />
        </div>
        <p class="text-xs" style="color: var(--ds-text-subtle);">count=50</p>
      </div>
      <div class="text-center">
        <div class="relative inline-flex mb-2">
          <Bell class="w-6 h-6" style="color: var(--ds-text);" />
          <BadgeCounter count={99} />
        </div>
        <p class="text-xs" style="color: var(--ds-text-subtle);">count=99</p>
      </div>
      <div class="text-center">
        <div class="relative inline-flex mb-2">
          <Bell class="w-6 h-6" style="color: var(--ds-text);" />
          <BadgeCounter count={100} />
        </div>
        <p class="text-xs" style="color: var(--ds-text-subtle);">count=100</p>
      </div>
      <div class="text-center">
        <div class="relative inline-flex mb-2">
          <Bell class="w-6 h-6" style="color: var(--ds-text);" />
          <BadgeCounter count={999} />
        </div>
        <p class="text-xs" style="color: var(--ds-text-subtle);">count=999</p>
      </div>
    </div>
  </section>

  <!-- Custom Max -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Max</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Customize the overflow threshold with the <code>max</code> prop.
    </p>
    <div class="flex items-center gap-8">
      <div class="text-center">
        <div class="relative inline-flex mb-2">
          <Mail class="w-6 h-6" style="color: var(--ds-text);" />
          <BadgeCounter count={15} max={9} />
        </div>
        <p class="text-xs" style="color: var(--ds-text-subtle);">max=9, count=15</p>
      </div>
      <div class="text-center">
        <div class="relative inline-flex mb-2">
          <Mail class="w-6 h-6" style="color: var(--ds-text);" />
          <BadgeCounter count={50} max={49} />
        </div>
        <p class="text-xs" style="color: var(--ds-text-subtle);">max=49, count=50</p>
      </div>
    </div>
  </section>

  <!-- Zero Count -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Zero Count (Hidden)</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      The badge is automatically hidden when count is 0.
    </p>
    <div class="flex items-center gap-8">
      <div class="text-center">
        <div class="relative inline-flex mb-2">
          <Bell class="w-6 h-6" style="color: var(--ds-text);" />
          <BadgeCounter count={0} />
        </div>
        <p class="text-xs" style="color: var(--ds-text-subtle);">count=0 (hidden)</p>
      </div>
      <div class="text-center">
        <div class="relative inline-flex mb-2">
          <Bell class="w-6 h-6" style="color: var(--ds-text);" />
          <BadgeCounter count={1} />
        </div>
        <p class="text-xs" style="color: var(--ds-text-subtle);">count=1 (visible)</p>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="flex items-center gap-4">
        <button
          class="relative p-2 rounded-lg hover:bg-gray-100 transition-colors"
          title="Notifications"
        >
          <Bell class="w-5 h-5" style="color: var(--ds-text);" />
          <BadgeCounter count={5} />
        </button>
        <button
          class="relative p-2 rounded-lg hover:bg-gray-100 transition-colors"
          title="Messages"
        >
          <MessageSquare class="w-5 h-5" style="color: var(--ds-text);" />
          <BadgeCounter count={23} />
        </button>
        <button
          class="relative p-2 rounded-lg hover:bg-gray-100 transition-colors"
          title="Inbox"
        >
          <Mail class="w-5 h-5" style="color: var(--ds-text);" />
          <BadgeCounter count={127} />
        </button>
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
            <td class="p-2"><code>count</code></td>
            <td class="p-2">number</td>
            <td class="p-2">0</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>max</code></td>
            <td class="p-2">number</td>
            <td class="p-2">99</td>
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

  <!-- Implementation Note -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Implementation Note</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        The parent element must have <code>position: relative</code> for the badge to position correctly.
        Use <code>relative inline-flex</code> on the parent for proper alignment with icons.
      </p>
    </div>
  </section>
</div>
