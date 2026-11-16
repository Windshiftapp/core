<script>
  import ModalBackdrop from '../../../lib/components/ModalBackdrop.svelte';
  import ModalHeader from '../../../lib/dialogs/ModalHeader.svelte';
  import Button from '../../../lib/components/Button.svelte';
  import Select from '../../../lib/components/Select.svelte';
  let open = $state(false);
  let child = $state(false);
  let value = $state('one');
</script>

<div class="p-8 max-w-4xl text-ds-text">
  <h1 class="text-2xl font-bold mb-4">ModalBackdrop</h1>
  <p class="mb-4">Bare overlay mechanics: positioning, scrim, optional portal, unique dialog naming, active-scope focus containment, background isolation and non-scrolling focus restoration. No panel or submission policy.</p>
  <Button onclick={() => open = true}>Open container-focused overlay</Button>
  <ModalBackdrop bind:show={open} portal initialFocus={false} dataTestid="ds-backdrop">
    <div class="max-w-lg w-full rounded-lg bg-ds-surface-raised border border-ds-border">
      <ModalHeader title="Shared overlay" onclose={() => open = false} />
      <div class="p-6 space-y-4">
        <Select id="ds-backdrop-select" bind:value options={[{ value: 'one', label: 'One' }, { value: 'two', label: 'Two' }]} />
        <Button onclick={() => child = true}>Open child overlay</Button>
        <Button onclick={() => open = false}>Close</Button>
      </div>
    </div>
    <ModalBackdrop bind:show={child} zIndex={70} portal ariaLabel="Child overlay">
      <div class="p-6 rounded-lg bg-ds-surface-raised">
        <p class="mb-4">The parent suspends until this child closes. Escape dismisses only the active scope.</p>
        <Button onclick={() => child = false}>Return to parent</Button>
      </div>
    </ModalBackdrop>
  </ModalBackdrop>
  <h2 class="text-lg font-semibold mt-8 mb-3">Focus and integration APIs</h2>
  <ul class="list-disc pl-5 space-y-2 text-ds-text-subtle">
    <li><code>initialFocus</code>: preferred CSS selector; false means container-only. <code>focusOnOpen=false</code> suppresses automatic initial focus, not containment, isolation or restoration.</li>
    <li><code>ariaLabelledBy</code>, then <code>ariaLabel</code>, then a registered/existing heading supply the name. Heading references are emitted only when their targets exist.</li>
    <li><code>backdropElement</code> is bindable; <code>class</code>, <code>id</code> and <code>dataTestid</code> reach the dialog. Numeric <code>zIndex=50</code> remains the default; null lets CSS classes set stacking.</li>
    <li><code>onKeydown</code> runs before primitive Escape handling, only for the active dialog and non-popup events. preventDefault vetoes dismissal. <code>onBackdropClick</code> runs before closeOnClick policy.</li>
    <li><code>createOwnedPopup()</code> captures component scope; apply its action to every portaled root and supply its trigger, close callback and keepTabStops policy. Disable competing popup traps inside a modal. Core pickers already integrate this contract.</li>
  </ul>
</div>
