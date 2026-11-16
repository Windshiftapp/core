<script>
  import X from '@lucide/svelte/icons/x';
  import { t } from '../stores/i18n.svelte.js';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import Button from '../components/Button.svelte';

  let {
    isOpen = $bindable(false),
    isDarkMode = false,
    maxWidth = 'max-w-2xl',
    bodyClass = 'px-6 py-4 max-h-[60vh] overflow-y-auto',
    headerPaddingClass = 'px-6 py-4',
    id = undefined,
    title = '',
    subtitle = '',
    showHeader = true,
    backdropOpacity = 0.4,
    backdropBlur = 'blur(2px)',
    onClose = null,
    ariaLabelledBy = undefined,
    ariaLabel = undefined,
    dataTestid = undefined,
    header = undefined,
    children = undefined
  } = $props();
  const titleId = $props.id();

  function close() {
    isOpen = false;
    onClose?.();
  }

</script>

<ModalBackdrop bind:show={isOpen} {id} {dataTestid} class="p-4" align=""
  opacity={backdropOpacity} blur={0} extraFilter={backdropBlur} onclose={close}
  ariaLabelledBy={ariaLabelledBy || (showHeader && !header && title && !ariaLabel ? titleId : undefined)}
  ariaLabel={ariaLabel || (header || !showHeader ? title || undefined : undefined)}>
    <!-- Modal content -->
    <div class="relative flex items-center justify-center w-full h-full pointer-events-none">
      <div
        data-testid={dataTestid ? `${dataTestid}-panel` : undefined}
        class={`w-full ${maxWidth} rounded-lg border border-[var(--ds-border)] bg-[var(--ds-surface-raised)] text-[var(--ds-text)] shadow-xl overflow-hidden pointer-events-auto`}
      >
      {#if showHeader}
        <div
          class={`${headerPaddingClass} border-b border-[var(--ds-border)] bg-[var(--ds-surface)] flex items-center justify-between`}
        >
          {#if header}
            {@render header()}
          {:else}
            <div>
              {#if title}
                <h2 id={titleId} class="text-lg font-semibold">
                  {title}
                </h2>
              {/if}
              {#if subtitle}
                <p class="text-sm mt-1 text-[var(--ds-text-subtle)]">
                  {subtitle}
                </p>
              {/if}
            </div>
          {/if}
          <Button variant="ghost" icon={X} onclick={close} ariaLabel={t('aria.close')} dataTestid={dataTestid ? `${dataTestid}-close` : undefined} />
        </div>
      {/if}

      <div class={bodyClass}>
        {@render children?.()}
      </div>
      </div>
    </div>
</ModalBackdrop>
