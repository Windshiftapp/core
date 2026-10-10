<script>
  import { onMount } from 'svelte';
  import { IconPlus, IconTrash, IconPencil } from '@tabler/icons-svelte-runes';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import Button from '../../components/Button.svelte';
  import TextField from '../../components/TextField.svelte';
  import SelectField from '../../components/SelectField.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';

  let { channelId, workspaces = [], portals = [] } = $props();

  let intakes = $state([]);
  let loading = $state(false);
  let editing = $state(null);
  // Item types belong to the intake's workspace, so this component owns the
  // load rather than relying on the mailbox's legacy email_workspace_id.
  let itemTypes = $state([]);
  let itemTypeLoadSequence = 0;

  async function loadItemTypesForWorkspace(workspaceId) {
    const requestSequence = ++itemTypeLoadSequence;
    itemTypes = [];
    if (!workspaceId) {
      return;
    }
    try {
      const loaded = await api.workspaces.getItemTypes(workspaceId);
      if (requestSequence === itemTypeLoadSequence) {
        itemTypes = loaded;
      }
    } catch (err) {
      console.error('Failed to load item types:', err);
      if (requestSequence === itemTypeLoadSequence) itemTypes = [];
    }
  }

  // A portal scopes the routing choices: its served workspaces are the only
  // valid workspace targets, which keeps intake and portal visibility
  // consistent by construction.
  function portalServedWorkspaceIds(portalId) {
    const portal = portals.find((p) => p.id === portalId);
    if (!portal?.config) return [];
    try {
      const config = typeof portal.config === 'string' ? JSON.parse(portal.config) : portal.config;
      return Array.isArray(config.portal_workspace_ids) ? config.portal_workspace_ids : [];
    } catch {
      return [];
    }
  }

  function blankIntake() {
    return {
      id: 0,
      folder: 'INBOX',
      portal_channel_id: null,
      workspace_id: null,
      item_type_id: null,
      rate_limit_per_hour: '',
      processing_disposition: '',
      status: 'enabled'
    };
  }

  async function load() {
    if (!channelId) return;
    loading = true;
    try {
      intakes = (await api.channelIntakes.list(channelId)) ?? [];
    } catch (err) {
      console.error('Failed to load intakes:', err);
      errorToast(t('channel.intakeLoadFailed'));
    } finally {
      loading = false;
    }
  }

  onMount(load);

  function startCreate() {
    editing = blankIntake();
    loadItemTypesForWorkspace(null);
  }

  function startEdit(intake) {
    // The API omits empty optionals via `omitempty`; start from the blank
    // defaults so every bound field is defined (Svelte 5 rejects binding
    // undefined into a prop with a fallback value). needs_attention is
    // system-set, so the form maps it to disabled until the admin re-enables.
    editing = { ...blankIntake(), ...intake };
    if (editing.status === 'needs_attention') {
      editing.status = 'disabled';
    }
    loadItemTypesForWorkspace(editing.workspace_id);
  }

  function onPortalChange() {
    if (!editing) return;
    if (editing.portal_channel_id) {
      const served = portalServedWorkspaceIds(editing.portal_channel_id);
      if (!served.includes(editing.workspace_id)) {
        editing.workspace_id = null;
        editing.item_type_id = null;
        loadItemTypesForWorkspace(null);
      }
    }
  }

  function onWorkspaceChange() {
    if (!editing) return;
    editing.item_type_id = null;
    loadItemTypesForWorkspace(editing.workspace_id);
  }

  async function save() {
    if (!editing.folder?.trim()) {
      errorToast(t('channel.intakeFolderRequired'));
      return;
    }
    if (!editing.workspace_id) {
      errorToast(t('channel.intakeWorkspaceRequired'));
      return;
    }
    if (!editing.item_type_id) {
      errorToast(t('channel.itemTypeRequired'));
      return;
    }
    const payload = {
      folder: editing.folder.trim(),
      portal_channel_id: editing.portal_channel_id || null,
      workspace_id: Number(editing.workspace_id),
      item_type_id: editing.item_type_id,
      rate_limit_per_hour:
        editing.rate_limit_per_hour === '' || editing.rate_limit_per_hour === null
          ? null
          : Number(editing.rate_limit_per_hour),
      processing_disposition: editing.processing_disposition || '',
      status: editing.status || 'enabled'
    };
    try {
      if (editing.id) {
        await api.channelIntakes.update(channelId, editing.id, payload);
      } else {
        await api.channelIntakes.create(channelId, payload);
      }
      editing = null;
      await load();
      successToast(t('channel.intakeSaved'));
    } catch (err) {
      errorToast(err?.message || t('channel.intakeSaveFailed'));
    }
  }

  async function remove(intake) {
    try {
      await api.channelIntakes.delete(channelId, intake.id);
      await load();
      successToast(t('channel.intakeDeleted'));
    } catch (err) {
      errorToast(err?.message || t('channel.intakeDeleteFailed'));
    }
  }

  function portalLabel(portalId) {
    const portal = portals.find((p) => p.id === portalId);
    return portal ? portal.name : `Portal #${portalId}`;
  }

  function workspaceLabel(workspaceId) {
    const workspace = workspaces.find((w) => w.id === workspaceId);
    return workspace ? workspace.name : `Workspace #${workspaceId}`;
  }

  const portalOptions = $derived([
    { value: null, label: t('channel.intakeNoPortal') },
    ...portals.map((portal) => ({ value: portal.id, label: portal.name }))
  ]);

  const workspaceOptions = $derived.by(() => {
    if (!editing) return [];
    let list = workspaces;
    if (editing.portal_channel_id) {
      const served = portalServedWorkspaceIds(editing.portal_channel_id);
      list = workspaces.filter((workspace) => served.includes(workspace.id));
    }
    return list.map((workspace) => ({ value: workspace.id, label: workspace.name }));
  });

  const dispositionOptions = [
    { value: '', label: t('channel.dispositionInherit') },
    { value: 'leave', label: t('channel.dispositionLeave') },
    { value: 'mark_read', label: t('channel.dispositionMarkRead') },
    { value: 'delete', label: t('channel.dispositionDelete') }
  ];

  const statusOptions = [
    { value: 'enabled', label: t('channel.intakeStatusEnabled') },
    { value: 'disabled', label: t('channel.intakeStatusDisabled') }
  ];

  // The monitored address is a mailbox property; show it once above the list.
  let mailboxAddress = $derived(intakes.find((intake) => intake.mailbox_address)?.mailbox_address || '');
</script>

<div class="pt-6 border-t space-y-4" style="border-color: var(--ds-border);">
  <div>
    <h4 class="text-sm font-semibold" style="color: var(--ds-text);">{t('channel.intakesTitle')}</h4>
    <DescriptionText>{t('channel.intakesHelp')}</DescriptionText>
    {#if mailboxAddress}
      <DescriptionText>
        <span data-testid="channel-intake-mailbox-address">
          {t('channel.intakesMailboxAddress', { address: mailboxAddress })}
        </span>
      </DescriptionText>
    {/if}
  </div>

  {#if loading}
    <StateDisplay type="loading" />
  {:else}
    <div class="space-y-2" data-testid="channel-intake-list">
      {#each intakes as intake (intake.id)}
        <div
          class="p-3 rounded border flex items-start justify-between gap-3"
          style="background: var(--ds-surface-raised); border-color: var(--ds-border);"
          data-testid="channel-intake-{intake.id}"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="font-medium text-sm" style="color: var(--ds-text);">{intake.folder}</span>
              <Lozenge
                color={intake.status === 'enabled'
                  ? 'green'
                  : intake.status === 'needs_attention'
                    ? 'orange'
                    : 'gray'}
              >
                {intake.status === 'enabled'
                  ? t('channel.intakeStatusEnabled')
                  : intake.status === 'needs_attention'
                    ? t('channel.intakeStatusNeedsAttention')
                    : t('channel.intakeStatusDisabled')}
              </Lozenge>
            </div>
            <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              {intake.portal_channel_id
                ? `${t('channel.intakeFeedsPortal')}: ${portalLabel(intake.portal_channel_id)}`
                : `${t('channel.intakeFeedsWorkspace')}: ${workspaceLabel(intake.workspace_id)}`}
            </div>
            {#if intake.status === 'needs_attention' && intake.status_reason}
              <div
                class="text-xs mt-1"
                style="color: var(--ds-text-warning);"
                data-testid="channel-intake-attention-{intake.id}"
              >
                {intake.status_reason}
              </div>
            {/if}
            <div class="text-xs mt-1 flex items-center gap-2" style="color: var(--ds-text-subtle);">
              <span data-testid="channel-intake-watermark-{intake.id}">
                {t('channel.intakeLastUID', { uid: intake.last_uid ?? 0 })}
              </span>
              {#if intake.rate_limited_count > 0}
                <Lozenge color="orange" dataTestid="channel-intake-rate-limited-{intake.id}">
                  {t('channel.intakeRateLimited', { count: intake.rate_limited_count })}
                </Lozenge>
              {/if}
            </div>
          </div>
          <div class="flex items-center gap-1 flex-shrink-0">
            <Button
              variant="ghost"
              size="small"
              dataTestid="channel-intake-edit-{intake.id}"
              onclick={() => startEdit(intake)}
            >
              <IconPencil size={14} />
            </Button>
            <Button
              variant="ghost"
              size="small"
              dataTestid="channel-intake-delete-{intake.id}"
              onclick={() => remove(intake)}
            >
              <IconTrash size={14} />
            </Button>
          </div>
        </div>
      {/each}

      {#if intakes.length === 0}
        <DescriptionText>{t('channel.intakesEmpty')}</DescriptionText>
      {/if}
    </div>

    {#if editing}
      <div class="p-4 rounded border space-y-4" style="background: var(--ds-surface); border-color: var(--ds-border);">
        <div class="grid grid-cols-2 gap-4">
          <TextField
            label={t('channel.intakeFolder')}
            labelColor="default"
            placeholder="INBOX"
            id="intake-folder"
            dataTestid="channel-intake-folder"
            bind:value={editing.folder}
          />
          <SelectField
            label={t('channel.intakeTargetPortal')}
            labelColor="default"
            id="intake-portal"
            options={portalOptions}
            bind:value={editing.portal_channel_id}
            onchange={onPortalChange}
          />
        </div>

        <div class="grid grid-cols-2 gap-4">
          <SelectField
            label={t('channel.intakeTargetWorkspace')}
            labelColor="default"
            id="intake-workspace"
            options={workspaceOptions}
            placeholder={t('channel.selectWorkspace')}
            bind:value={editing.workspace_id}
            onchange={onWorkspaceChange}
          />
          <SelectField
            label={t('channel.itemType')}
            labelColor="default"
            id="intake-item-type"
            disabled={!editing.workspace_id}
            options={[
              { value: null, label: t('channel.selectItemType') },
              ...itemTypes.map((type) => ({ value: type.id, label: type.name }))
            ]}
            bind:value={editing.item_type_id}
          />
        </div>

        <div class="grid grid-cols-3 gap-4">
          <TextField
            label={t('channel.rateLimitPerHour')}
            labelColor="default"
            type="number"
            min="0"
            placeholder="100"
            dataTestid="channel-intake-rate-limit"
            bind:value={editing.rate_limit_per_hour}
          />
          <SelectField
            label={t('channel.processingDisposition')}
            labelColor="default"
            id="intake-disposition"
            options={dispositionOptions}
            bind:value={editing.processing_disposition}
          />
          <SelectField
            label={t('channel.intakeStatus')}
            labelColor="default"
            id="intake-status"
            options={statusOptions}
            bind:value={editing.status}
          />
        </div>

        <div class="flex justify-end gap-2">
          <Button variant="ghost" onclick={() => (editing = null)}>{t('common.cancel')}</Button>
          <Button variant="primary" dataTestid="channel-intake-save" onclick={save}>{t('common.save')}</Button>
        </div>
      </div>
    {:else}
      <button
        type="button"
        onclick={startCreate}
        class="w-full flex items-center justify-center gap-2 px-4 py-3 rounded border-2 border-dashed transition-all"
        style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
        data-testid="channel-intake-add"
      >
        <IconPlus size={16} />
        <span class="font-medium">{t('channel.intakeAdd')}</span>
      </button>
    {/if}
  {/if}
</div>
