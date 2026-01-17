<script>
  import Button from '../components/Button.svelte';
  import UserPicker from './UserPicker.svelte';
  import GroupPicker from './GroupPicker.svelte';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    type = $bindable('user'), // 'user' or 'group'
    userId = $bindable(null),
    groupId = $bindable(null),
    userLabel = '',
    groupLabel = '',
    userPlaceholder = '',
    groupPlaceholder = '',
    confirmText = '',
    cancelText = '',
    disabled = false,
    on_confirm = () => {},
    on_cancel = () => {},
    class: className = ''
  } = $props();

  const resolvedUserLabel = $derived(userLabel || t('pickers.selectUser'));
  const resolvedGroupLabel = $derived(groupLabel || t('pickers.selectGroup'));
  const resolvedUserPlaceholder = $derived(userPlaceholder || t('pickers.searchUser'));
  const resolvedGroupPlaceholder = $derived(groupPlaceholder || t('pickers.searchGroup'));
  const resolvedConfirmText = $derived(confirmText || t('common.add'));
  const resolvedCancelText = $derived(cancelText || t('common.cancel'));

  function handleConfirm() {
    const result = type === 'user' ? { userId } : { groupId };
    on_confirm(result);
  }

  function handleCancel() {
    on_cancel();
  }

  const isValid = $derived.by(() => type === 'user' ? userId : groupId);
</script>

<div class="space-y-4 {className}">
  <!-- Type Selection -->
  <div>
    <Label color="default" class="mb-2">{t('pickers.assignTo')}</Label>
    <div class="flex gap-4">
      <label class="flex items-center">
        <input
          type="radio"
          bind:group={type}
          value="user"
          class="mr-2"
        />
        <span style="color: var(--ds-text);">{t('common.user')}</span>
      </label>
      <label class="flex items-center">
        <input
          type="radio"
          bind:group={type}
          value="group"
          class="mr-2"
        />
        <span style="color: var(--ds-text);">{t('common.group')}</span>
      </label>
    </div>
  </div>

  <!-- User/Group Selection -->
  {#if type === 'user'}
    <div>
      <UserPicker
        bind:value={userId}
        label={resolvedUserLabel}
        placeholder={resolvedUserPlaceholder}
      />
    </div>
  {:else}
    <div>
      <GroupPicker
        bind:value={groupId}
        label={resolvedGroupLabel}
        placeholder={resolvedGroupPlaceholder}
      />
    </div>
  {/if}

  <!-- Action Buttons -->
  <div class="flex gap-2 justify-end">
    <Button
      onclick={handleCancel}
      variant="secondary"
      size="medium"
    >
      {resolvedCancelText}
    </Button>
    <Button
      onclick={handleConfirm}
      variant="primary"
      size="medium"
      disabled={disabled || !isValid}
    >
      {resolvedConfirmText}
    </Button>
  </div>
</div>