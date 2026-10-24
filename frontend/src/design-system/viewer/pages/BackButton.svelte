<script>
  import BackButton from '../../../lib/components/BackButton.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">BackButton</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Standard back navigation affordance. Renders a router-aware link with
    <code>href</code>, otherwise a button with <code>onclick</code>. Use it for
    every "Back to …" control so arrow, label, spacing, and hover match.
  </p>

  <!-- Inline link variant -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Inline link</h2>
    <div
      class="p-4 rounded-lg space-y-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <BackButton href="#backbutton" label="Back to agents" />
      <BackButton href="#backbutton" label="Back to Workspace" size="small" />
    </div>
  </section>

  <!-- Button variant -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Button</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <BackButton onclick={() => {}} label="Back to iterations" />
    </div>
  </section>

  <!-- Icon only -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Icon only</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Omit <code>label</code> for a compact control. Always pass
      <code>tooltip</code> (or <code>label</code>) for the accessible name.
    </p>
    <div
      class="p-4 rounded-lg flex items-center gap-6"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <BackButton onclick={() => {}} tooltip="Back" />
      <BackButton onclick={() => {}} tooltip="Back" size="small" class="p-1" />
    </div>
  </section>

  <!-- Disabled -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Disabled</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <BackButton href="#backbutton" label="Back to agents" disabled />
    </div>
  </section>

  <!-- Props -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Props</h2>
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
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>href</code></td>
            <td class="p-2">string | null</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onclick</code></td>
            <td class="p-2">function | null</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'small' | 'medium'</td>
            <td class="p-2">'medium'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>tooltip</code></td>
            <td class="p-2">string | null</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>icon</code></td>
            <td class="p-2">Component</td>
            <td class="p-2">ArrowLeft</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>testid</code></td>
            <td class="p-2">string</td>
            <td class="p-2">undefined</td>
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
</div>
