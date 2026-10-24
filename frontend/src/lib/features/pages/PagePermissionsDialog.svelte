<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Select from '../../components/Select.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import GroupPicker from '../../pickers/GroupPicker.svelte';
  import RolePicker from '../../pickers/RolePicker.svelte';
  import Shield from '@tabler/icons-svelte-runes/icons/shield';
  import { pages } from '../../api/pages.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { t } from '../../stores/i18n.svelte.js';

  /**
   * Page permissions dialog. Shows the inherit_permissions flag, the
   * effective level for the current user, and the page's own ACL rows.
   * Admins (effective_level === 'admin') can flip inheritance and add or
   * remove ACL rows; lower-tier viewers see the data read-only.
   */
  let {
    isOpen = $bindable(false),
    workspaceId,
    pageId,
    onUpdated = null,
  } = $props();

  let data = $state(null);
  let loading = $state(false);
  let error = $state('');
  let saving = $state(false);

  let newPrincipalType = $state('user');
  /** @type {number | null} principal id bound to whichever picker is active */
  let newPrincipalId = $state(null);
  let newLevel = $state('view');

  // $derived so option/column labels follow live locale changes.
  const principalTypeOptions = $derived([
    { value: 'user', label: t('pages.permsPrincipalUser') },
    { value: 'group', label: t('pages.permsPrincipalGroup') },
    { value: 'role', label: t('pages.permsPrincipalRole') },
  ]);

  const levelOptions = $derived([
    { value: 'view', label: t('pages.permsLevelView') },
    { value: 'edit', label: t('pages.permsLevelEdit') },
    { value: 'admin', label: t('pages.permsLevelAdmin') },
  ]);

  const aclColumns = $derived([
    { key: 'principal', label: t('pages.permsColumnPrincipal'), slot: 'principal' },
    { key: 'permission_level', label: t('pages.permsColumnLevel'), slot: 'level' },
    { key: 'remove', label: '', slot: 'remove', width: '6rem', align: 'text-right' },
  ]);

  // Reset the principal selection when the type changes — a user id makes
  // no sense once the user has switched to picking a group or role.
  function onPrincipalTypeChange() {
    newPrincipalId = null;
  }

  $effect(() => {
    if (isOpen && workspaceId && pageId) {
      load();
    }
    if (!isOpen) {
      data = null;
      error = '';
      newPrincipalId = null;
      newPrincipalType = 'user';
      newLevel = 'view';
    }
  });

  async function load() {
    loading = true;
    error = '';
    try {
      const resp = await pages.getPermissions(workspaceId, pageId);
      // The Go handler returns `acl: nil` when there are no grants —
      // serialized as JSON `null` — and DataTable does `data.length`
      // unconditionally, which crashes on null. Normalize at the
      // boundary so every downstream consumer can assume an array.
      data = { ...resp, acl: resp?.acl ?? [] };
    } catch (err) {
      error = err?.message || t('pages.permsErrorLoad');
    } finally {
      loading = false;
    }
  }

  const isAdmin = $derived(data?.effective_level === 'admin');

  async function toggleInheritance() {
    if (!isAdmin) return;
    saving = true;
    error = '';
    try {
      await pages.setInheritance(workspaceId, pageId, !data.inherit_permissions);
      await load();
      onUpdated?.();
    } catch (err) {
      error = err?.message || t('pages.permsErrorInherit');
    } finally {
      saving = false;
    }
  }

  async function addGrant() {
    if (!isAdmin) return;
    if (typeof newPrincipalId !== 'number' || newPrincipalId <= 0) {
      error = t('pages.permsErrorNoPrincipal');
      return;
    }
    saving = true;
    error = '';
    try {
      await pages.grantPermission(workspaceId, pageId, {
        principalType: newPrincipalType,
        principalId: newPrincipalId,
        permissionLevel: newLevel,
      });
      newPrincipalId = null;
      await load();
      onUpdated?.();
    } catch (err) {
      error = err?.message || t('pages.permsErrorGrant');
    } finally {
      saving = false;
    }
  }

  async function revoke(permissionId) {
    if (!isAdmin) return;
    const ok = await confirm({
      title: t('pages.permsRemoveTitle'),
      message: t('pages.permsRemoveMessage'),
      confirmText: t('pages.permsRemoveConfirm'),
      cancelText: t('pages.permsRemoveCancel'),
      variant: 'danger',
    });
    if (!ok) return;
    saving = true;
    error = '';
    try {
      await pages.revokePermission(workspaceId, pageId, permissionId);
      await load();
      onUpdated?.();
    } catch (err) {
      error = err?.message || t('pages.permsErrorRevoke');
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:isOpen maxWidth="max-w-2xl">
  <ModalHeader
    title={t('pages.permsTitle')}
    subtitle={data
      ? t('pages.permsEffectiveAccess', {
          level: data.effective_level || t('pages.permsEffectiveAccessNone'),
        })
      : ''}
    onClose={() => (isOpen = false)}
  />
  <div class="dialog">
    {#if error}
      <div class="error" role="alert">{error}</div>
    {/if}

    {#if loading || !data}
      <p class="status">{t('pages.permsLoading')}</p>
    {:else}
      <section class="inheritance">
        <Checkbox
          id="page-perms-inherit-toggle"
          checked={data.inherit_permissions}
          disabled={!isAdmin || saving}
          onchange={toggleInheritance}
          label={t('pages.permsInheritLabel')}
          size="small"
        />
        <p class="hint">{t('pages.permsInheritHint')}</p>
      </section>

      <section class="acl">
        <h3>{t('pages.permsExplicitGrants')}</h3>
        <DataTable
          columns={aclColumns}
          data={data.acl}
          keyField="id"
          emptyMessage={t('pages.permsEmptyGrantsTitle')}
          emptyDescription={t('pages.permsEmptyGrantsDescription')}
          emptyIcon={Shield}
          rowAttrs={() => ({ 'data-testid': 'page-acl-row' })}
        >
          {#snippet principal(row)}
            <span style="color: var(--ds-text);">{row.principal_type} #{row.principal_id}</span>
          {/snippet}
          {#snippet level(row)}
            <span style="color: var(--ds-text);">{row.permission_level}</span>
          {/snippet}
          {#snippet remove(row)}
            {#if isAdmin}
              <Button
                variant="link"
                size="small"
                onclick={() => revoke(row.id)}
                disabled={saving}
              >
                {t('pages.permsRemove')}
              </Button>
            {/if}
          {/snippet}
        </DataTable>

        {#if isAdmin}
          <form
            class="add-grant"
            onsubmit={(e) => {
              e.preventDefault();
              addGrant();
            }}
          >
            <Select
              id="page-perms-new-principal-type"
              bind:value={newPrincipalType}
              options={principalTypeOptions}
              disabled={saving}
              onchange={onPrincipalTypeChange}
            />
            <div class="principal-picker">
              {#if newPrincipalType === 'user'}
                <UserPicker
                  bind:value={newPrincipalId}
                  {workspaceId}
                  placeholder={t('pages.permsPickUser')}
                  disabled={saving}
                />
              {:else if newPrincipalType === 'group'}
                <GroupPicker
                  bind:value={newPrincipalId}
                  placeholder={t('pages.permsPickGroup')}
                  disabled={saving}
                />
              {:else}
                <RolePicker
                  bind:value={newPrincipalId}
                  placeholder={t('pages.permsPickRole')}
                  disabled={saving}
                />
              {/if}
            </div>
            <Select
              id="page-perms-new-level"
              bind:value={newLevel}
              options={levelOptions}
              disabled={saving}
            />
            <Button
              id="page-perms-add-grant"
              type="submit"
              variant="primary"
              size="small"
              disabled={saving || typeof newPrincipalId !== 'number' || newPrincipalId <= 0}
            >
              {t('pages.permsAdd')}
            </Button>
          </form>
        {/if}
      </section>
    {/if}
  </div>
  <DialogFooter
    cancelLabel={t('pages.permsClose')}
    cancelTestid="page-perms-close"
    onCancel={() => (isOpen = false)}
  />
</Modal>

<style>
  .dialog {
    padding: 1.25rem 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .inheritance {
    border-top: 1px solid var(--ds-border, #e5e7eb);
    padding-top: 0.75rem;
  }

  .acl {
    border-top: 1px solid var(--ds-border, #e5e7eb);
    padding-top: 0.75rem;
  }

  .acl h3 {
    margin: 0 0 0.5rem 0;
    font-size: 0.875rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--ds-text-subtle, #6b7280);
  }

  .add-grant {
    display: grid;
    grid-template-columns: minmax(8rem, 1fr) minmax(12rem, 1.5fr) minmax(7rem, 1fr) auto;
    gap: 0.5rem;
    margin-top: 0.75rem;
    align-items: start;
  }

  .principal-picker {
    min-width: 0;
  }

  .error {
    padding: 0.625rem 0.875rem;
    background: var(--ds-status-danger-bg, #fef2f2);
    color: var(--ds-text-danger, #b91c1c);
    border-radius: 0.25rem;
    font-size: 0.875rem;
  }

  .status {
    color: var(--ds-text-subtle, #6b7280);
    font-size: 0.875rem;
  }
</style>
