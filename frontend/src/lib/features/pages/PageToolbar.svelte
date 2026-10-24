<script>
  import PageWorkItemsButton from './PageWorkItemsButton.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import IconArrowsMaximize from '@tabler/icons-svelte-runes/icons/arrows-maximize';
  import IconArrowsMinimize from '@tabler/icons-svelte-runes/icons/arrows-minimize';
  import Dots from '@tabler/icons-svelte-runes/icons/dots';
  import Pencil from '@tabler/icons-svelte-runes/icons/pencil';
  import Eye from '@tabler/icons-svelte-runes/icons/eye';
  import { autoGrow } from '../../utils/autoGrowTextarea.js';
  import { t } from '../../stores/i18n.svelte.js';

  /** Header of the page pane: save status, work-items popover, the
   * segmented edit/read toggle, canvas width, kebab menu, and title. */
  let {
    page,
    workspaceId,
    pageLinks = [],
    loadingPageLinks = false,
    pageLinkTypeId = null,
    statusLabel = '',
    saveStatus = 'idle',
    canEditPage = false,
    menuItems = [],
    mode = $bindable('edit'),
    canvasExpanded = $bindable(false),
    title = '',
    titleInputEl = $bindable(null),
    PageTitleIcon = null,
    pageTitleIconColor = '',
    onTitleInput,
    onTitleKeydown,
    onLinkCreated = () => {},
    onLinkRemoved = () => {},
  } = $props();

  /* Segmented Edit | Read toggle. Two buttons sharing a 1px border;
     active button uses --ds-surface-selected to read as pressed.
     All toolbar controls share a 2rem height so the row's top and
     bottom edges line up across the work-items button, this toggle,
     and the 2rem icon buttons next to it. */
  const modeToggleBtnClass = (active) =>
    `inline-flex h-8 items-center gap-1 border-0 bg-transparent px-2.5 text-xs ${
      active
        ? 'bg-ds-surface-selected text-ds-text'
        : 'text-ds-text-subtle not-first:border-l not-first:border-ds-border hover:bg-ds-background-neutral-hovered hover:text-ds-text'
    }`;
</script>

<div class="flex flex-col items-stretch gap-4 px-(--page-gutter) pt-3">
  <div class="-mr-(--page-gutter) ml-auto flex flex-wrap items-center justify-end gap-2">
    {#if statusLabel && mode === 'edit' && canEditPage}
      <span
        class="min-w-16 text-right text-xs {saveStatus === 'error'
          ? 'text-ds-text-danger'
          : 'text-ds-text-subtle'}"
        data-testid="page-save-status"
        data-status={saveStatus}
      >
        {statusLabel}
      </span>
    {/if}
    {#if page}
      <PageWorkItemsButton
        workspaceId={page.workspace_id ?? workspaceId}
        pageId={page.id}
        {pageLinks}
        loading={loadingPageLinks}
        {pageLinkTypeId}
        onlinkCreated={onLinkCreated}
        onlinkRemoved={onLinkRemoved}
      />
    {/if}
    <div
      class="inline-flex overflow-hidden rounded-md border border-ds-border"
      role="group"
      aria-label={t('pages.modeAria')}
      data-testid="page-mode-toggle"
    >
      {#if canEditPage}
        <button
          type="button"
          class={modeToggleBtnClass(mode === 'edit')}
          aria-pressed={mode === 'edit'}
          onclick={() => (mode = 'edit')}
          data-testid="page-mode-edit"
        >
          <Pencil size={14} />
          <span>{t('pages.modeEdit')}</span>
        </button>
      {/if}
      <button
        type="button"
        class={modeToggleBtnClass(mode === 'read')}
        aria-pressed={mode === 'read'}
        onclick={() => (mode = 'read')}
        data-testid="page-mode-read"
      >
        <Eye size={14} />
        <span>{t('pages.modeRead')}</span>
      </button>
    </div>
    <Tooltip
      content={t(canvasExpanded ? 'pages.canvasComfortable' : 'pages.canvasWide')}
      placement="bottom"
      class="inline-flex"
    >
      <button
        type="button"
        class="inline-flex h-8 w-8 items-center justify-center rounded-sm p-0 text-ds-text-subtle hover:bg-ds-background-neutral-hovered hover:text-ds-text aria-pressed:bg-ds-background-neutral-hovered aria-pressed:text-ds-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ds-border-focused @max-[52rem]/pages:hidden"
        aria-label={t(canvasExpanded ? 'pages.canvasComfortable' : 'pages.canvasWide')}
        aria-pressed={canvasExpanded}
        onclick={() => (canvasExpanded = !canvasExpanded)}
        data-testid="page-canvas-width-toggle"
      >
        {#if canvasExpanded}
          <IconArrowsMinimize size={16} aria-hidden="true" />
        {:else}
          <IconArrowsMaximize size={16} aria-hidden="true" />
        {/if}
      </button>
    </Tooltip>
    {#if menuItems.length > 0}
      <DropdownMenu
        triggerIcon={Dots}
        items={menuItems}
        showChevron={false}
        iconOnly={true}
        placement="bottom-end"
        triggerClass="h-8 w-8 justify-center border-none bg-transparent p-0 text-ds-text-subtle hover:bg-ds-background-neutral-hovered hover:text-ds-text"
        triggerTestid="page-toolbar-kebab"
      />
    {/if}
  </div>
  <div class="flex w-full min-w-0 items-start gap-2.5">
    {#if PageTitleIcon}
      <PageTitleIcon
        size={22}
        class="mt-[0.35rem] shrink-0"
        style="color: {pageTitleIconColor};"
        aria-hidden="true"
      />
    {/if}
    <!-- The title wraps instead of scrolling inside a single-line field, so
         long titles are never clipped in edit or read mode. autoGrow keeps
         the textarea sized to its content. -->
    <textarea
      id="page-title-input"
      bind:this={titleInputEl}
      class="block w-full min-w-0 flex-1 resize-none overflow-hidden bg-transparent p-0 text-[1.75rem] leading-[1.2] font-bold text-ds-text outline-none [overflow-wrap:anywhere]"
      rows={1}
      value={title}
      oninput={onTitleInput}
      onkeydown={onTitleKeydown}
      placeholder={t('pages.titlePlaceholder')}
      disabled={!canEditPage}
      use:autoGrow={title}
    ></textarea>
  </div>
</div>
