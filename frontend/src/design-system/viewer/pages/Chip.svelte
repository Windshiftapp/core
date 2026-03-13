<script>
  import Chip from '../../../lib/components/Chip.svelte';
  import { Tag, User, Folder, Star, Flag, Circle } from 'lucide-svelte';

  let chips = $state(['Frontend', 'Backend', 'Design']);

  function removeChip(chip) {
    chips = chips.filter(c => c !== chip);
  }

  function resetChips() {
    chips = ['Frontend', 'Backend', 'Design'];
  }
</script>

<div class="space-y-8">
  <div>
    <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Chip</h1>
    <p style="color: var(--ds-text-subtle);">
      Metadata container with optional icon and remove button.
      Use for tags, labels, filters, and removable metadata items.
    </p>
  </div>

  <!-- Colors -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Colors</h2>
    <div class="flex flex-wrap gap-2">
      <Chip color="blue">Blue</Chip>
      <Chip color="green">Green</Chip>
      <Chip color="purple">Purple</Chip>
      <Chip color="teal">Teal</Chip>
      <Chip color="gray">Gray</Chip>
      <Chip color="red">Red</Chip>
      <Chip color="yellow">Yellow</Chip>
      <Chip color="orange">Orange</Chip>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<Chip color="blue">Blue</Chip>
<Chip color="green">Green</Chip>
<Chip color="purple">Purple</Chip>
<Chip color="teal">Teal</Chip>
<Chip color="gray">Gray</Chip>
<Chip color="red">Red</Chip>
<Chip color="yellow">Yellow</Chip>
<Chip color="orange">Orange</Chip>`}
    </pre>
  </section>

  <!-- With Icons -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Icons</h2>
    <div class="flex flex-wrap gap-2">
      <Chip color="blue" icon={Tag}>Label</Chip>
      <Chip color="green" icon={User}>Assignee</Chip>
      <Chip color="purple" icon={Folder}>Category</Chip>
      <Chip color="yellow" icon={Star}>Featured</Chip>
      <Chip color="red" icon={Flag}>Priority</Chip>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<Chip color="blue" icon={Tag}>Label</Chip>
<Chip color="green" icon={User}>Assignee</Chip>
<Chip color="purple" icon={Folder}>Category</Chip>`}
    </pre>
  </section>

  <!-- Removable -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Removable Chips</h2>
    <div class="flex flex-wrap gap-2 items-center">
      {#each chips as chip}
        <Chip color="blue" removable onRemove={() => removeChip(chip)}>{chip}</Chip>
      {/each}
      {#if chips.length === 0}
        <button onclick={resetChips} class="text-sm underline" style="color: var(--ds-text-subtle);">
          Reset chips
        </button>
      {/if}
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`{#each chips as chip}
  <Chip color="blue" removable onRemove={() => removeChip(chip)}>
    {chip}
  </Chip>
{/each}`}
    </pre>
  </section>

  <!-- Use Cases -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Common Use Cases</h2>
    <div class="space-y-4">
      <div>
        <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">Tags/Labels:</p>
        <div class="flex flex-wrap gap-2">
          <Chip color="blue" icon={Tag}>bug</Chip>
          <Chip color="green" icon={Tag}>feature</Chip>
          <Chip color="purple" icon={Tag}>documentation</Chip>
        </div>
      </div>
      <div>
        <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">Filters:</p>
        <div class="flex flex-wrap gap-2">
          <Chip color="gray" removable onRemove={() => {}}>Status: Open</Chip>
          <Chip color="gray" removable onRemove={() => {}}>Priority: High</Chip>
          <Chip color="gray" removable onRemove={() => {}}>Type: Bug</Chip>
        </div>
      </div>
      <div>
        <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">Status indicators:</p>
        <div class="flex flex-wrap gap-2">
          <Chip color="green" icon={Circle}>Online</Chip>
          <Chip color="yellow" icon={Circle}>Away</Chip>
          <Chip color="gray" icon={Circle}>Offline</Chip>
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
            <td class="py-2 pr-4 font-mono text-xs">color</td>
            <td class="py-2 pr-4 font-mono text-xs">'blue' | 'green' | 'purple' | 'teal' | 'gray' | 'red' | 'yellow' | 'orange'</td>
            <td class="py-2 pr-4 font-mono text-xs">'blue'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Chip color</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">icon</td>
            <td class="py-2 pr-4 font-mono text-xs">Component</td>
            <td class="py-2 pr-4 font-mono text-xs">null</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Optional leading icon</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">removable</td>
            <td class="py-2 pr-4 font-mono text-xs">boolean</td>
            <td class="py-2 pr-4 font-mono text-xs">false</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Show remove button</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">onRemove</td>
            <td class="py-2 pr-4 font-mono text-xs">() => void</td>
            <td class="py-2 pr-4 font-mono text-xs">null</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Callback when remove is clicked</td>
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
