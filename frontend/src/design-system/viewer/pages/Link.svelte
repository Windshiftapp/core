<script>
  import Link from '../../../lib/Link.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Link</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Semantic link component that uses native anchor tags for proper accessibility and browser behavior.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div class="space-y-4">
      <p style="color: var(--ds-text);">
        Check out our
        <Link href="#" style="color: var(--ds-link);" class="hover:underline">documentation</Link>
        for more information.
      </p>
      <p style="color: var(--ds-text);">
        Visit the
        <Link href="#" style="color: var(--ds-link);" class="hover:underline">settings page</Link>
        to configure your preferences.
      </p>
    </div>
  </section>

  <!-- Link Styles -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Link Styles</h2>
    <div class="space-y-4">
      <div>
        <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">Default link</p>
        <Link href="#" style="color: var(--ds-link);" class="hover:underline">
          Click me
        </Link>
      </div>
      <div>
        <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">Subtle link</p>
        <Link href="#" style="color: var(--ds-text-subtle);" class="underline">
          Subtle link
        </Link>
      </div>
      <div>
        <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">Bold link</p>
        <Link href="#" style="color: var(--ds-link);" class="font-semibold hover:underline">
          Important link
        </Link>
      </div>
    </div>
  </section>

  <!-- Active State -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Active State</h2>
    <div class="flex gap-4">
      <Link href="#" style="color: var(--ds-link);" class="px-3 py-2 rounded">
        Inactive Link
      </Link>
      <Link href="#" active style="background: var(--ds-background-selected); color: var(--ds-text-selected);" class="px-3 py-2 rounded font-medium">
        Active Link
      </Link>
    </div>
  </section>

  <!-- Disabled State -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Disabled State</h2>
    <div class="flex gap-4">
      <Link href="#" style="color: var(--ds-link);" class="hover:underline">
        Enabled Link
      </Link>
      <Link href="#" disabled style="color: var(--ds-text-disabled);" class="cursor-not-allowed">
        Disabled Link
      </Link>
    </div>
  </section>

  <!-- With Custom Click Handler -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Custom Click Handler</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Use the <code>onClick</code> prop for custom behavior like opening modals while preserving link semantics.
    </p>
    <Link
      href="#item-123"
      onClick={(e) => {
        e.preventDefault()
        alert('Custom click handler - could open a modal!')
      }}
      style="color: var(--ds-link);"
      class="hover:underline"
    >
      Open in Modal
    </Link>
  </section>

  <!-- In Navigation -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Navigation</h2>
    <nav
      class="flex gap-1 p-2 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <Link href="#" style="background: var(--ds-background-selected); color: var(--ds-text-selected);" class="px-4 py-2 rounded-md font-medium">
        Dashboard
      </Link>
      <Link href="#" style="color: var(--ds-text-subtle);" class="px-4 py-2 rounded-md">
        Projects
      </Link>
      <Link href="#" style="color: var(--ds-text-subtle);" class="px-4 py-2 rounded-md">
        Team
      </Link>
      <Link href="#" style="color: var(--ds-text-subtle);" class="px-4 py-2 rounded-md">
        Settings
      </Link>
    </nav>
  </section>

  <!-- Benefits -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Benefits</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <ul class="text-sm space-y-2" style="color: var(--ds-text-subtle);">
        <li>• <strong>Right-click "Open in New Tab"</strong> - Standard browser behavior</li>
        <li>• <strong>Ctrl/Cmd+click</strong> - Opens in new tab automatically</li>
        <li>• <strong>URL preview</strong> - Shows link destination in browser status bar</li>
        <li>• <strong>Accessibility</strong> - Proper keyboard navigation and screen reader support</li>
        <li>• <strong>SEO friendly</strong> - Search engines can follow the links</li>
      </ul>
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
            <td class="p-2"><code>href</code></td>
            <td class="p-2">string</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>active</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onClick</code></td>
            <td class="p-2">function</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>class</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr>
            <td class="p-2"><code>element</code></td>
            <td class="p-2">HTMLAnchorElement (export)</td>
            <td class="p-2">-</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
