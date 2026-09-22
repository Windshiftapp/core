<script>
  import { Share, SquarePlus } from '@lucide/svelte';
  import Modal from '../dialogs/Modal.svelte';
  import { iosInstallOpen } from './installClient.js';
  import { t } from '../stores/i18n.svelte.js';

  function close() {
    iosInstallOpen.set(false);
  }
</script>

<Modal bind:isOpen={$iosInstallOpen} maxWidth="max-w-sm" zIndexClass="z-[600]" onclose={close}>
  <div class="sheet" data-testid="ios-install-sheet">
    <h2>{t('mobile.install.title')}</h2>
    <p class="lead">{t('mobile.install.description')}</p>
    <ol class="steps">
      <li><span class="n">1</span> {t('mobile.install.openSafari')}</li>
      <li><span class="n">2</span> {t('mobile.install.share')} <Share size={15} aria-hidden="true" /></li>
      <li><span class="n">3</span> {t('mobile.install.addToHome')} <SquarePlus size={15} aria-hidden="true" /></li>
      <li><span class="n">4</span> {t('mobile.install.launch')}</li>
    </ol>
    <button class="ok" onclick={close} data-testid="ios-install-dismiss" type="button">{t('mobile.install.dismiss')}</button>
  </div>
</Modal>

<style>
  .sheet { display: flex; flex-direction: column; gap: 0.85rem; padding: 1.1rem; }
  h2 { margin: 0; font-size: 1.0625rem; font-weight: var(--font-semibold, 600); color: var(--ds-text); }
  .lead { margin: 0; font-size: 0.875rem; color: var(--ds-text-subtle); line-height: 1.5; }
  .steps { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.7rem; }
  .steps li {
    display: flex; align-items: center; flex-wrap: wrap; gap: 0.4rem;
    font-size: 0.9375rem; color: var(--ds-text); line-height: 1.4;
  }
  .n {
    flex-shrink: 0; width: 22px; height: 22px; display: inline-flex; align-items: center; justify-content: center;
    border-radius: var(--radius-full, 9999px); background-color: var(--ds-background-neutral);
    color: var(--ds-text-subtle); font-size: 0.75rem; font-weight: var(--font-semibold, 600);
  }
  .ok {
    margin-top: 0.25rem; min-height: 44px; border: none; border-radius: var(--radius-md, 6px);
    background-color: var(--ds-interactive); color: var(--ds-text-inverse, #fff);
    font-weight: var(--font-semibold, 600); cursor: pointer;
  }
</style>
