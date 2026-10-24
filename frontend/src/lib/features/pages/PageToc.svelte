<script>
  import { t } from '../../stores/i18n.svelte.js';

  /** Supporting rail listing the document headings of the open page. */
  let { headings = [], onselect } = $props();
</script>

<aside
  class="sticky top-0 mr-(--page-gutter) max-h-[calc(100vh-8rem)] max-[1100px]:hidden shrink-0 grow-0 basis-[220px] self-start overflow-y-auto border-l border-ds-border pl-4 text-[0.8125rem]"
  data-testid="page-toc"
  aria-label={t('pages.tocAriaLabel')}
>
  <h3 class="mb-2 text-xs font-semibold uppercase text-ds-text-subtle">
    {t('pages.tocHeading')}
  </h3>
  <ul class="m-0 flex list-none flex-col gap-0.5 p-0">
    {#each headings as heading (heading.line)}
      <li data-testid="page-toc-entry" style="padding-left: {(heading.level - 1) * 0.75}rem">
        <button
          type="button"
          class="w-full cursor-pointer overflow-hidden text-ellipsis whitespace-nowrap border-0 bg-transparent px-0 py-1 text-left text-ds-text-subtle hover:text-ds-text"
          onclick={() => onselect?.(heading)}
          title={heading.text}
        >
          {heading.text}
        </button>
      </li>
    {/each}
  </ul>
</aside>
