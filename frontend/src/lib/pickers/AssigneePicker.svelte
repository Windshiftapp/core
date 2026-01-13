<script>
  import Button from '../components/Button.svelte';
  import UserPicker from './UserPicker.svelte';
  import GroupPicker from './GroupPicker.svelte';
  import Label from '../components/Label.svelte';

  let {
    type = $bindable('user'), // 'user' or 'group'
    userId = $bindable(null),
    groupId = $bindable(null),
    userLabel = 'Select User',
    groupLabel = 'Select Group',
    userPlaceholder = 'Search for a user...',
    groupPlaceholder = 'Search for a group...',
    confirmText = 'Add',
    cancelText = 'Cancel',
    disabled = false,
    on_confirm = () => {},
    on_cancel = () => {},
    class: className = ''
  } = $props();

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
    <Label color="default" class="mb-2">Assign to:</Label>
    <div class="flex gap-4">
      <label class="flex items-center">
        <input
          type="radio"
          bind:group={type}
          value="user"
          class="mr-2"
        />
        <span style="color: var(--ds-text);">User</span>
      </label>
      <label class="flex items-center">
        <input
          type="radio"
          bind:group={type}
          value="group"
          class="mr-2"
        />
        <span style="color: var(--ds-text);">Group</span>
      </label>
    </div>
  </div>

  <!-- User/Group Selection -->
  {#if type === 'user'}
    <div>
      <UserPicker
        bind:value={userId}
        label={userLabel}
        placeholder={userPlaceholder}
      />
    </div>
  {:else}
    <div>
      <GroupPicker
        bind:value={groupId}
        label={groupLabel}
        placeholder={groupPlaceholder}
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
      {cancelText}
    </Button>
    <Button
      onclick={handleConfirm}
      variant="primary"
      size="medium"
      disabled={disabled || !isValid}
    >
      {confirmText}
    </Button>
  </div>
</div>