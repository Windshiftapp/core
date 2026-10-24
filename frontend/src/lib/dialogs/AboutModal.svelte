<script>
  import X from '@lucide/svelte/icons/x';
  import Button from '../components/Button.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { versionLabel } from '../version.js';

  let {
    show = $bindable(false),
    onclose = null,
  } = $props();

  let shellVersion = $state(null);

  $effect(() => {
    if (!show || shellVersion !== null) return;
    loadShellVersion();
  });

  async function loadShellVersion() {
    try {
      const { getVersion } = await import('@tauri-apps/api/app');
      shellVersion = await getVersion();
    } catch (err) {
      console.warn('[desktop-about] failed to load shell version:', err);
      shellVersion = 'Unavailable';
    }
  }

  function close() {
    onclose?.();
    show = false;
  }
</script>

<!-- shortcut-guard-exempt: informational dialog with no form; the primary button only closes, so there is nothing to submit. -->
<ModalBackdrop bind:show onclose={close} ariaLabelledBy="desktop-about-title" zIndex={70}>
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div
    role="presentation"
    class="w-full max-w-md rounded shadow-xl"
    style="background-color: var(--ds-surface-raised); color: var(--ds-text);"
    onclick={(e) => e.stopPropagation()}
  >
    <div class="flex items-center justify-between border-b px-6 py-4" style="border-color: var(--ds-border);">
      <h2 id="desktop-about-title" class="text-lg font-semibold">About Windshift</h2>
      <Button variant="ghost" icon={X} onclick={close} title={t('common.close')} />
    </div>

    <div class="px-6 py-6">
      <div class="flex items-center gap-4">
        <img src="windshift-3.svg" alt="" class="h-14 w-14 shrink-0" />
        <div class="min-w-0">
          <div class="text-xl font-semibold">Windshift</div>
          <div class="text-sm" style="color: var(--ds-text-subtle);">Desktop workspace companion</div>
        </div>
      </div>

      <dl class="mt-6 grid grid-cols-[9rem_1fr] gap-x-4 gap-y-3 text-sm">
        <dt style="color: var(--ds-text-subtle);">Webapp version</dt>
        <dd>{versionLabel}</dd>

        <dt style="color: var(--ds-text-subtle);">Desktop version</dt>
        <dd>{shellVersion ?? 'Loading...'}</dd>

        <dt style="color: var(--ds-text-subtle);">Copyright</dt>
        <dd>Copyright 2026 Windshift</dd>
      </dl>
    </div>

    <div class="flex justify-end border-t px-6 py-4" style="border-color: var(--ds-border);">
      <Button variant="primary" size="small" onclick={close}>Done</Button>
    </div>
  </div>
</ModalBackdrop>
