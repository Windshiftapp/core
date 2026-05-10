<script>
  import PortalModal from './PortalModal.svelte';
  import RequestTypeFieldsBuilder from './RequestTypeFieldsBuilder.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Thin modal wrapper around RequestTypeFieldsBuilder. Used by the
  // asset-reports flow (Portal.svelte) which still wants a centered modal.
  // The portal customize sidebar renders the builder inline instead.
  let {
    isOpen = $bindable(false),
    requestTypeId = null,
    requestTypeName = '',
    resourceId = null,
    resourceName = '',
    channelId = null,
    apiHandlers = null,
    isDarkMode = false,
    onsaved = undefined,
    onclose = undefined
  } = $props();

  function handleClose() {
    isOpen = false;
    onclose?.();
  }
</script>

{#if isOpen}
  <PortalModal
    isOpen={isOpen}
    isDarkMode={isDarkMode}
    maxWidth="max-w-2xl"
    showHeader={false}
    bodyClass="p-0 max-h-[80vh] overflow-hidden"
    onClose={handleClose}
  >
    <div class="h-[70vh] flex flex-col">
      <RequestTypeFieldsBuilder
        {requestTypeId}
        {requestTypeName}
        {resourceId}
        {resourceName}
        {channelId}
        {apiHandlers}
        {isDarkMode}
        {onsaved}
        onclose={handleClose}
      />
    </div>
  </PortalModal>
{/if}
