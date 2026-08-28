<script>
  import Button from './Button.svelte';
  import Spinner from './Spinner.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { loader, componentProps = {}, label = 'view' } = $props();
  let retryVersion = $state(0);

  const loadPromise = $derived.by(() => {
    retryVersion;
    return loader();
  });
</script>

{#await loadPromise}
  <div
    class="flex flex-1 min-h-[40vh] items-center justify-center"
    style="color: var(--ds-text);"
    role="status"
    data-testid="root-view-loading"
    data-root-label={label}
  >
    <div class="text-center">
      <Spinner class="mx-auto mb-3" />
      <p class="text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
    </div>
  </div>
{:then loadedModule}
  {@const Component = loadedModule.default}
  <Component {...componentProps} />
{:catch}
  <div
    class="flex flex-1 min-h-[40vh] items-center justify-center px-6"
    style="color: var(--ds-text);"
    role="alert"
    data-testid="root-view-error"
    data-root-label={label}
  >
    <div class="text-center max-w-sm">
      <h1 class="text-lg font-semibold mb-2">{t('errors.failedToLoad')}</h1>
      <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
        {t('errors.NETWORK_ERROR')}
      </p>
      <!-- shortcut-guard-exempt: retrying a failed lazy import is a recovery action, not a form submission. -->
      <Button
        variant="primary"
        size="large"
        dataTestid="root-view-retry"
        onclick={() => retryVersion++}
      >{t('common.retry')}</Button>
    </div>
  </div>
{/await}
