<script>
  // Global coding-agent allowlist editor with named catalogs and audit-reasoned
  // removal.

  import { onMount } from 'svelte';
  import { Loader2, Plus, Trash2 } from '@lucide/svelte';
  import { agentSecurity, api } from '../api.js';
  import UserPicker from '../pickers/UserPicker.svelte';
  import WorkspacePicker from '../pickers/WorkspacePicker.svelte';
  import Input from '../components/Input.svelte';
  import Button from '../components/Button.svelte';
  import ConfirmWithReasonDialog from '../dialogs/ConfirmWithReasonDialog.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';

  // flagEnabled is informational: when false, we hint that grants are
  // staged but inert. Loading + state are otherwise self-contained.
  let { flagEnabled = false } = $props();

  let entries = $state([]);
  let loading = $state(true);
  let usersById = $state({});
  // Subset of usersById restricted to centralized service users (is_agent
  // true + no owner). The backend rejects anything else from the
  // allowlist; the UserPicker only ever offers these so admins can't
  // even pick something the server is going to refuse.
  let serviceUsers = $state([]);
  let workspacesById = $state({});

  // Add-form state. addWorkspaceIds is empty when the admin hasn't
  // picked any specific workspace — interpreted as a single "any
  // workspace" grant. Each picked id becomes its own grant on submit.
  let addUserId = $state(null);
  let addWorkspaceIds = $state([]);
  let addReason = $state('');
  let adding = $state(false);

  // Remove-dialog state.
  let removeDialogOpen = $state(false);
  let pendingRemove = $state(null); // { userId, workspaceId? }
  let pendingRemoveLabel = $state('');

  onMount(load);

  async function load() {
    loading = true;
    try {
      const [list, users, workspaces] = await Promise.all([
        agentSecurity.listAllowlist(),
        api.getUsers(),
        api.workspaces.getAll(),
      ]);
      entries = list ?? [];
      const um = {};
      const eligible = [];
      for (const u of users ?? []) {
        um[u.id] = u;
        // Centralized service users only: is_agent=true and no owner.
        // Owned agents (agent_owner_user_id set) reach bindings through
        // the WI-87 chokepoint directly; humans must never be impersonated.
        if (u.is_agent && !u.agent_owner_user_id) eligible.push(u);
      }
      usersById = um;
      serviceUsers = eligible;
      const wm = {};
      for (const w of workspaces ?? []) {
        wm[w.id] = w;
      }
      workspacesById = wm;
    } catch (err) {
      console.error('Failed to load agent-security allowlist:', err);
      errorToast(err?.message || t('securitySettings.allowlist.loadFailed'));
    } finally {
      loading = false;
    }
  }

  function displayUser(userId) {
    const u = usersById[userId];
    if (!u) return t('securitySettings.allowlist.userNumber', { id: userId });
    const full = `${u.first_name ?? ''} ${u.last_name ?? ''}`.trim();
    return full || u.username || u.email || t('securitySettings.allowlist.userNumber', { id: userId });
  }

  function displayWorkspace(workspaceId) {
    if (!workspaceId) return t('securitySettings.allowlist.anyWorkspace');
    const w = workspacesById[workspaceId];
    if (!w) return t('securitySettings.allowlist.workspaceNumber', { id: workspaceId });
    return w.name || w.key || t('securitySettings.allowlist.workspaceNumber', { id: workspaceId });
  }

  let canAdd = $derived(!!addUserId && addReason.trim().length > 0 && !adding);

  async function addEntry() {
    if (!canAdd) return;
    const body = {
      user_id: addUserId,
      workspace_ids: addWorkspaceIds, // [] = single "any workspace" grant
      reason: addReason.trim(),
    };
    adding = true;
    try {
      await agentSecurity.addAllowlist(body);
      addUserId = null;
      addWorkspaceIds = [];
      addReason = '';
      await load();
    } catch (err) {
      errorToast(err?.message || t('securitySettings.allowlist.addFailed'));
      console.error('Failed to add allowlist entry:', err);
    } finally {
      adding = false;
    }
  }

  function openRemoveDialog(entry) {
    pendingRemove = { userId: entry.user_id, workspaceId: entry.workspace_id ?? null };
    pendingRemoveLabel = `${displayUser(entry.user_id)} (${displayWorkspace(entry.workspace_id)})`;
    removeDialogOpen = true;
  }

  async function confirmRemove(reason) {
    if (!pendingRemove) return;
    try {
      await agentSecurity.removeAllowlist(pendingRemove.userId, {
        workspaceId: pendingRemove.workspaceId ?? undefined,
        reason,
      });
      pendingRemove = null;
      pendingRemoveLabel = '';
      await load();
    } catch (err) {
      errorToast(err?.message || t('securitySettings.allowlist.removeFailed'));
      console.error('Failed to remove allowlist entry:', err);
    }
  }

  function cancelRemove() {
    pendingRemove = null;
    pendingRemoveLabel = '';
  }
</script>

<div>
  <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">
    {t('securitySettings.allowlist.title')}
  </h4>

  {#if !flagEnabled}
    <div class="mb-3">
      <DescriptionText>
        {t('securitySettings.allowlist.disabledHint')}
      </DescriptionText>
    </div>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-4">
      <Loader2 class="w-4 h-4 animate-spin" style="color: var(--ds-icon-subtle);" />
    </div>
  {:else}
    {#if entries.length === 0}
      <p class="text-sm py-2" style="color: var(--ds-text-subtle);">
        {t('securitySettings.allowlist.empty')}
      </p>
    {:else}
      <div class="border rounded-md overflow-hidden" style="border-color: var(--ds-border);">
        <table class="w-full text-sm">
          <thead>
            <tr style="background-color: var(--ds-background-neutral);">
              <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">{t('securitySettings.allowlist.user')}</th>
              <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">{t('securitySettings.allowlist.scope')}</th>
              <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">{t('securitySettings.allowlist.reason')}</th>
              <th class="px-3 py-2"></th>
            </tr>
          </thead>
          <tbody>
            {#each entries as entry (`${entry.user_id}:${entry.workspace_id ?? 0}`)}
              <tr class="border-t" style="border-color: var(--ds-border);">
                <td class="px-3 py-2" style="color: var(--ds-text);">{displayUser(entry.user_id)}</td>
                <td class="px-3 py-2" style="color: var(--ds-text-subtle);">{displayWorkspace(entry.workspace_id)}</td>
                <td class="px-3 py-2" style="color: var(--ds-text-subtle);">{entry.reason || '—'}</td>
                <td class="px-3 py-2 text-right">
                  <button
                    type="button"
                    onclick={() => openRemoveDialog(entry)}
                    class="inline-flex items-center justify-center p-1 rounded hover:opacity-80"
                    style="color: var(--ds-icon-danger);"
                    title={t('securitySettings.allowlist.remove')}
                    aria-label={t('securitySettings.allowlist.removeFor', { name: displayUser(entry.user_id) })}
                  >
                    <Trash2 class="w-4 h-4" />
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    <div
      class="add-grant mt-4 p-3 rounded-md"
      style="background-color: var(--ds-background-neutral);"
      data-testid="agent-security-add-grant"
    >
      <p class="text-xs font-medium mb-2" style="color: var(--ds-text);">{t('securitySettings.allowlist.add')}</p>
      <div class="add-grant-fields grid gap-3 items-end">
        <div class="min-w-0">
          <label for="add-user" class="block text-xs mb-1" style="color: var(--ds-text-subtle);">{t('securitySettings.allowlist.user')}</label>
          <div id="add-user">
            <UserPicker bind:value={addUserId} users={serviceUsers} placeholder={t('securitySettings.allowlist.pickUser')} class="min-h-[38px]" />
          </div>
        </div>
        <div class="min-w-0">
          <label for="add-workspace" class="block text-xs mb-1" style="color: var(--ds-text-subtle);">{t('securitySettings.allowlist.workspace')}</label>
          <div id="add-workspace">
            <WorkspacePicker
              bind:value={addWorkspaceIds}
              allowClear
              placeholder={t('securitySettings.allowlist.anyWorkspace')}
            />
          </div>
        </div>
        <div class="min-w-0">
          <label for="add-reason" class="block text-xs mb-1" style="color: var(--ds-text-subtle);">{t('securitySettings.allowlist.reasonLogged')}</label>
          <Input id="add-reason" size="small" class="min-h-[38px]" bind:value={addReason} placeholder={t('securitySettings.allowlist.reasonExample')} />
        </div>
        <div class="min-w-0">
          <!-- shortcut-guard-exempt: admin settings tab action, not a primary global-create surface -->
          <Button
            variant="primary"
            fullWidth
            icon={Plus}
            loading={adding}
            disabled={!canAdd}
            onclick={addEntry}
            class="min-h-[38px]"
          >
            {t('common.add')}
          </Button>
        </div>
      </div>
    </div>
  {/if}
</div>

<ConfirmWithReasonDialog
  bind:show={removeDialogOpen}
  variant="danger"
  title={t('securitySettings.allowlist.removeTitle')}
  message={t('securitySettings.allowlist.removeMessage', { name: pendingRemoveLabel })}
  reasonLabel={t('securitySettings.allowlist.reasonLogged')}
  reasonPlaceholder={t('securitySettings.allowlist.removeReason')}
  confirmText={t('securitySettings.allowlist.remove')}
  onconfirm={confirmRemove}
  oncancel={cancelRemove}
/>

<style>
  .add-grant {
    container-type: inline-size;
  }

  .add-grant-fields {
    grid-template-columns: minmax(0, 1fr);
  }

  @container (min-width: 48rem) {
    .add-grant-fields {
      grid-template-columns: minmax(0, 4fr) minmax(0, 3fr) minmax(0, 4fr) max-content;
    }
  }
</style>
