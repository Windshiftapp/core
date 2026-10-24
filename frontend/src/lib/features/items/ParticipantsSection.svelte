<script>
  import { onMount } from 'svelte';
  import Users from '@lucide/svelte/icons/users';
  import X from '@lucide/svelte/icons/x';
  import Plus from '@lucide/svelte/icons/plus';
  import Spinner from '../../components/Spinner.svelte';
  import Text from '../../components/Text.svelte';
  import Input from '../../components/Input.svelte';
  import Button from '../../components/Button.svelte';
  import PortalCustomerPicker from '../../pickers/PortalCustomerPicker.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { items } from '../../api/items.js';

  let { itemId, canEdit = true } = $props();

  let participants = $state([]);
  let loading = $state(true);
  let showAddForm = $state(false);
  let email = $state('');
  let name = $state('');
  let adding = $state(false);
  let emailInput = $state(null);

  async function load() {
    loading = true;
    try {
      participants = (await items.listParticipants(itemId)) ?? [];
    } catch {
      participants = [];
    } finally {
      loading = false;
    }
  }

  onMount(load);

  // Focus the first field when the add form is revealed, mirroring the
  // personal-tasks composer.
  $effect(() => {
    if (showAddForm && emailInput) emailInput.focus();
  });

  function openAddForm() {
    if (adding) return;
    showAddForm = true;
  }

  function closeAddForm() {
    showAddForm = false;
    email = '';
    name = '';
  }

  async function addByEmail() {
    const value = email.trim();
    if (!value || adding) return;
    adding = true;
    try {
      participants =
        (await items.addParticipant(itemId, {
          email: value,
          name: name.trim() || undefined
        })) ?? [];
      closeAddForm();
    } catch (error) {
      errorToast(error?.message || t('items.participantsAddFailed'));
    } finally {
      adding = false;
    }
  }

  async function addByCustomer(customer) {
    if (!customer?.id) return;
    try {
      participants = (await items.addParticipant(itemId, { portal_customer_id: customer.id })) ?? [];
      closeAddForm();
    } catch (error) {
      errorToast(error?.message || t('items.participantsAddFailed'));
    }
  }

  async function remove(participant) {
    try {
      await items.removeParticipant(itemId, participant.portal_customer_id);
      participants = participants.filter((p) => p.portal_customer_id !== participant.portal_customer_id);
    } catch (error) {
      errorToast(error?.message || t('items.participantsRemoveFailed'));
    }
  }
</script>

<div class="pt-4 mt-4 border-t" style="border-color: var(--ds-border);">
  <div class="flex items-center justify-between gap-2 group">
    <div class="flex items-center gap-2">
      <Users class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
      <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">
        {t('items.participants')}
      </Text>
    </div>
    {#if canEdit && !loading}
      <button
        type="button"
        class="p-1 rounded transition-colors opacity-40 group-hover:opacity-100 focus-visible:opacity-100 disabled:opacity-20 disabled:cursor-not-allowed"
        style="color: var(--ds-text-subtle);"
        title={t('items.participantAdd')}
        aria-label={t('items.participantAdd')}
        data-testid="item-participant-add-toggle"
        disabled={showAddForm}
        onclick={openAddForm}
      >
        <Plus class="w-4 h-4" />
      </button>
    {/if}
  </div>

  {#if loading}
    <div class="pt-3 flex justify-center">
      <Spinner size="small" />
    </div>
  {:else}
    <div class="mt-3 space-y-2" data-testid="item-participants">
      {#if participants.length === 0}
        <div data-testid="item-participants-empty">
          <Text variant="subtle" size="sm">{t('items.participantsEmpty')}</Text>
        </div>
      {/if}
      {#each participants as participant (participant.portal_customer_id)}
        <div
          class="flex items-center justify-between gap-2 px-2 py-1.5 rounded border"
          style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
          data-testid={`item-participant-${participant.portal_customer_id}`}
        >
          <div class="min-w-0">
            <div class="text-sm truncate" style="color: var(--ds-text);">
              {participant.customer_name || participant.customer_email}
            </div>
            {#if participant.customer_name && participant.customer_email}
              <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                {participant.customer_email}
              </div>
            {/if}
          </div>
          {#if canEdit}
            <button
              type="button"
              class="p-1 rounded hover-bg flex-shrink-0"
              title={t('items.participantRemove')}
              data-testid={`item-participant-remove-${participant.portal_customer_id}`}
              onclick={() => remove(participant)}
            >
              <X class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
            </button>
          {/if}
        </div>
      {/each}
    </div>

    {#if canEdit && showAddForm}
      <div class="mt-3 space-y-2">
        <form
          class="space-y-2"
          onsubmit={(event) => {
            event.preventDefault();
            addByEmail();
          }}
        >
          <Input
            type="email"
            bind:value={email}
            bind:inputRef={emailInput}
            placeholder={t('items.participantEmailPlaceholder')}
            dataTestid="item-participant-email"
            disabled={adding}
            size="small"
          />
          <Input
            type="text"
            bind:value={name}
            placeholder={t('items.participantNamePlaceholder')}
            dataTestid="item-participant-name"
            disabled={adding}
            size="small"
          />
          <div class="flex justify-end gap-2">
            <button
              type="button"
              class="px-2 py-1 text-xs rounded"
              style="color: var(--ds-text);"
              onclick={closeAddForm}
              disabled={adding}
            >
              {t('common.cancel')}
            </button>
            <Button
              type="submit"
              variant="primary"
              size="small"
              dataTestid="item-participant-add"
              disabled={adding || !email.trim()}
            >
              {t('items.participantAdd')}
            </Button>
          </div>
        </form>
        <div class="flex items-center gap-2">
          <div class="h-px flex-1" style="background-color: var(--ds-border);"></div>
          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('common.or')}</span>
          <div class="h-px flex-1" style="background-color: var(--ds-border);"></div>
        </div>
        <PortalCustomerPicker
          value={null}
          placeholder={t('items.participantPickExisting')}
          class="w-full"
          disabled={adding}
          onSelect={addByCustomer}
        />
      </div>
    {/if}
  {/if}
</div>
