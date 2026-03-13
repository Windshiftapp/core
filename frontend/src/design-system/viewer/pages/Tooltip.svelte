<script>
  import Tooltip from '../../../lib/components/Tooltip.svelte'
  import Button from '../../../lib/components/Button.svelte'
  import { Info, HelpCircle, Settings } from 'lucide-svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Tooltip</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Tooltip component built on Melt UI for displaying helpful information on hover.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div class="flex items-center gap-4">
      <Tooltip content="This is a helpful tooltip">
        <span class="underline decoration-dotted cursor-help" style="color: var(--ds-text);">
          Hover over me
        </span>
      </Tooltip>

      <Tooltip content="Click for more information">
        <Info class="w-5 h-5 cursor-pointer" style="color: var(--ds-text-subtle);" />
      </Tooltip>
    </div>
  </section>

  <!-- Placements -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Placements</h2>
    <div class="flex flex-wrap items-center gap-6 py-8">
      <Tooltip content="Tooltip on top" placement="top">
        <Button variant="default">Top</Button>
      </Tooltip>

      <Tooltip content="Tooltip on bottom" placement="bottom">
        <Button variant="default">Bottom</Button>
      </Tooltip>

      <Tooltip content="Tooltip on left" placement="left">
        <Button variant="default">Left</Button>
      </Tooltip>

      <Tooltip content="Tooltip on right" placement="right">
        <Button variant="default">Right</Button>
      </Tooltip>
    </div>
  </section>

  <!-- With Icons -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Icons</h2>
    <div class="flex items-center gap-6">
      <Tooltip content="Need help? Click for documentation">
        <HelpCircle class="w-5 h-5 cursor-pointer" style="color: var(--ds-text-subtle);" />
      </Tooltip>

      <Tooltip content="Configure your settings">
        <Settings class="w-5 h-5 cursor-pointer" style="color: var(--ds-text-subtle);" />
      </Tooltip>

      <Tooltip content="Additional information about this field">
        <Info class="w-5 h-5 cursor-pointer" style="color: var(--ds-interactive);" />
      </Tooltip>
    </div>
  </section>

  <!-- Custom Delay -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Delay</h2>
    <div class="flex items-center gap-6">
      <Tooltip content="Instant tooltip" delay={{ open: 0, close: 0 }}>
        <Button variant="secondary">No Delay</Button>
      </Tooltip>

      <Tooltip content="Default delay (300ms)" delay={{ open: 300, close: 0 }}>
        <Button variant="secondary">Default</Button>
      </Tooltip>

      <Tooltip content="Slow tooltip (1000ms)" delay={{ open: 1000, close: 0 }}>
        <Button variant="secondary">1s Delay</Button>
      </Tooltip>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="flex items-center gap-2 mb-2">
        <label class="text-sm font-medium" style="color: var(--ds-text);">API Key</label>
        <Tooltip content="Your API key is used for authentication. Keep it secure and never share it publicly.">
          <Info class="w-4 h-4 cursor-help" style="color: var(--ds-text-subtle);" />
        </Tooltip>
      </div>
      <input
        type="password"
        value="sk-xxxxxxxxxxxx"
        class="w-full max-w-md px-3 py-2 text-sm border rounded"
        style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
        disabled
      />
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
            <td class="p-2"><code>content</code></td>
            <td class="p-2">string</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placement</code></td>
            <td class="p-2">'top' | 'bottom' | 'left' | 'right'</td>
            <td class="p-2">'bottom'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>delay</code></td>
            <td class="p-2">{`{ open: number, close: number }`}</td>
            <td class="p-2">{`{ open: 300, close: 0 }`}</td>
          </tr>
          <tr>
            <td class="p-2"><code>children</code></td>
            <td class="p-2">snippet</td>
            <td class="p-2">-</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
