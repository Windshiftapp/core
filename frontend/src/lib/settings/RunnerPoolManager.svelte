<script>
  // Admin UI for runner pools (WI-237). A runner_pool is an ActionCapability;
  // here admins mint/revoke its registration tokens and view/revoke the runner
  // instances registered against it. Backend lifecycle: WI-177.
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Trash2, Copy, Server, KeyRound } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Input from '../components/Input.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { formatAuthenticatedDateTime } from '../utils/authenticatedDateFormatter.js';
  import { t } from '../stores/i18n.svelte.js';

  let pools = $state([]);
  let loadingPools = $state(true);
  let selectedPool = $state(null);

  let tokens = $state([]);
  let loadingTokens = $state(false);
  let instances = $state([]);
  let loadingInstances = $state(false);

  // Create-pool modal
  let showCreatePool = $state(false);
  let creating = $state(false);
  let poolForm = $state({ name: '', maxConcurrent: 0, appliesToAll: true, enabled: true });

  // Mint-token modal + one-time reveal. mintedToken carries the plaintext
  // token AND the server-generated copy-paste install command (WI-309) —
  // public WS_API_URL, version-matched images, and the token baked in.
  let showMint = $state(false);
  let minting = $state(false);
  let mintForm = $state({ description: '', ttlHours: 0 });
  let mintedToken = $state(null); // { token, installCommand }

  const canCreate = $derived(poolForm.name.trim().length > 0 && Number(poolForm.maxConcurrent) >= 0);
  const REGISTRATION_TOKEN_ENV = 'WSRUNNER_REGISTRATION_TOKEN';

  onMount(loadPools);

  async function loadPools() {
    loadingPools = true;
    try {
      const all = await api.actionCapabilities.getAll();
      pools = (all || []).filter((c) => c.capability_type === 'runner_pool');
      // Keep the selection valid across reloads.
      if (selectedPool) {
        selectedPool = pools.find((p) => p.id === selectedPool.id) || null;
      }
    } catch (e) {
      errorToast(e?.message || t('settings.adminOperations.runnerPools.loadFailed'));
    } finally {
      loadingPools = false;
    }
  }

  async function selectPool(pool) {
    selectedPool = pool;
    await Promise.all([loadTokens(pool.id), loadInstances(pool.id)]);
  }

  async function loadTokens(poolId) {
    loadingTokens = true;
    try {
      tokens = (await api.runnerPools.listTokens(poolId)) || [];
    } catch (e) {
      errorToast(e?.message || t('settings.adminOperations.runnerPools.tokensLoadFailed'));
    } finally {
      loadingTokens = false;
    }
  }

  async function loadInstances(poolId) {
    loadingInstances = true;
    try {
      instances = (await api.runnerPools.listInstances(poolId)) || [];
    } catch (e) {
      errorToast(e?.message || t('settings.adminOperations.runnerPools.runnersLoadFailed'));
    } finally {
      loadingInstances = false;
    }
  }

  function openCreate() {
    poolForm = { name: '', maxConcurrent: 0, appliesToAll: true, enabled: true };
    showCreatePool = true;
  }

  async function createPool() {
    if (!canCreate) return;
    creating = true;
    try {
      await api.actionCapabilities.create({
        name: poolForm.name.trim(),
        capability_type: 'runner_pool',
        config: JSON.stringify({ max_concurrent_runs: Number(poolForm.maxConcurrent) || 0 }),
        is_enabled: poolForm.enabled,
        applies_to_all_workspaces: poolForm.appliesToAll,
        workspace_ids: [],
      });
      successToast(t('settings.adminOperations.runnerPools.created'));
      showCreatePool = false;
      await loadPools();
    } catch (e) {
      errorToast(e?.message || t('settings.adminOperations.runnerPools.createFailed'));
    } finally {
      creating = false;
    }
  }

  function openMint() {
    mintForm = { description: '', ttlHours: 0 };
    showMint = true;
  }

  async function mintToken() {
    if (!selectedPool) return;
    minting = true;
    try {
      const res = await api.runnerPools.mintToken(selectedPool.id, {
        description: mintForm.description.trim(),
        ttl_hours: Number(mintForm.ttlHours) || 0,
      });
      showMint = false;
      // Plaintext, shown once.
      mintedToken = res?.token ? { token: res.token, installCommand: res.install_command || '' } : null;
      await loadTokens(selectedPool.id);
    } catch (e) {
      errorToast(e?.message || t('settings.adminOperations.runnerPools.mintFailed'));
    } finally {
      minting = false;
    }
  }

  async function revokeToken(tok) {
    const ok = await confirm({
      title: t('settings.adminOperations.runnerPools.revokeTokenTitle'),
      message: t('settings.adminOperations.runnerPools.revokeTokenMessage', { prefix: tok.token_prefix }),
      confirmText: t('settings.adminOperations.runnerPools.revoke'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.runnerPools.revokeToken(selectedPool.id, tok.id);
      successToast(t('settings.adminOperations.runnerPools.tokenRevoked'));
      await loadTokens(selectedPool.id);
    } catch (e) {
      errorToast(e?.message || t('settings.adminOperations.runnerPools.revokeTokenFailed'));
    }
  }

  async function revokeInstance(inst) {
    const ok = await confirm({
      title: t('settings.adminOperations.runnerPools.revokeRunnerTitle'),
      message: t('settings.adminOperations.runnerPools.revokeRunnerMessage', { name: inst.name || `#${inst.id}` }),
      confirmText: t('settings.adminOperations.runnerPools.revoke'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.runnerPools.revokeInstance(selectedPool.id, inst.id);
      successToast(t('settings.adminOperations.runnerPools.runnerRevoked'));
      await loadInstances(selectedPool.id);
    } catch (e) {
      errorToast(e?.message || t('settings.adminOperations.runnerPools.revokeRunnerFailed'));
    }
  }

  async function copy(text) {
    try {
      await navigator.clipboard.writeText(text);
      successToast(t('settings.adminOperations.runnerPools.copied'));
    } catch {
      errorToast(t('settings.adminOperations.runnerPools.copyFailed'));
    }
  }

  function fmtDate(d) {
    return d ? formatAuthenticatedDateTime(d) : '—';
  }

  function tokenStatus(tok) {
    if (tok.revoked_at) return { label: t('settings.adminOperations.runnerPools.revoked'), appearance: 'removed' };
    if (tok.expires_at && new Date(tok.expires_at) < new Date()) {
      return { label: t('settings.adminOperations.runnerPools.expired'), appearance: 'default' };
    }
    return { label: t('settings.adminOperations.runnerPools.active'), appearance: 'success' };
  }

  // Mirrors the server's RunnerLivenessWindow (90s, ~3 missed heartbeats):
  // past it the lease reaper treats the runner as dead, so the UI must not
  // keep calling it active.
  const HEARTBEAT_FRESH_MS = 90_000;

  function instanceStatus(inst) {
    if (inst.revoked_at) return { label: t('settings.adminOperations.runnerPools.revoked'), appearance: 'removed' };
    if (inst.status === 'active') {
      const lastSeen = inst.last_heartbeat_at || inst.registered_at;
      if (lastSeen && Date.now() - new Date(lastSeen).getTime() <= HEARTBEAT_FRESH_MS) {
        return { label: t('settings.adminOperations.runnerPools.online'), appearance: 'success' };
      }
      return { label: t('settings.adminOperations.runnerPools.stale'), appearance: 'warning' };
    }
    return { label: inst.status || t('settings.adminOperations.runnerPools.unknown'), appearance: 'default' };
  }

  const poolColumns = $derived([
    { key: 'name', label: t('settings.adminOperations.runnerPools.pool'), slot: 'name' },
    { key: 'status', label: t('common.status'), slot: 'status' },
    { key: 'concurrency', label: t('settings.adminOperations.runnerPools.maxConcurrent'), slot: 'concurrency' },
  ]);
  const tokenColumns = $derived([
    { key: 'token_prefix', label: t('settings.adminOperations.runnerPools.token'), slot: 'prefix' },
    { key: 'description', label: t('common.description'), slot: 'description' },
    { key: 'created_at', label: t('common.created'), slot: 'created' },
    { key: 'expires_at', label: t('settings.adminOperations.runnerPools.expires'), slot: 'expires' },
    { key: 'status', label: t('common.status'), slot: 'status' },
    { key: 'actions', label: '', slot: 'actions', align: 'text-right', width: 'w-20' },
  ]);
  const instanceColumns = $derived([
    { key: 'name', label: t('settings.adminOperations.runnerPools.runner'), slot: 'name' },
    { key: 'status', label: t('common.status'), slot: 'status' },
    { key: 'registered_at', label: t('settings.adminOperations.runnerPools.registered'), slot: 'registered' },
    { key: 'last_heartbeat_at', label: t('settings.adminOperations.runnerPools.lastHeartbeat'), slot: 'heartbeat' },
    { key: 'actions', label: '', slot: 'actions', align: 'text-right', width: 'w-20' },
  ]);

  function maxConcurrentLabel(pool) {
    try {
      const n = JSON.parse(pool.config || '{}').max_concurrent_runs ?? 0;
      return Number(n) === 0 ? t('settings.adminOperations.runnerPools.unlimited') : String(n);
    } catch {
      return t('settings.adminOperations.runnerPools.unlimited');
    }
  }
</script>

<div class="space-y-6">
  <PageHeader title={t('settings.adminOperations.runnerPools.title')} subtitle={t('settings.adminOperations.runnerPools.subtitle')}>
    {#snippet actions()}
      <!-- shortcut-guard-exempt: admin settings tab action, not a primary global-create surface -->
      <Button variant="primary" onclick={openCreate} icon={Plus}>
        {t('settings.adminOperations.runnerPools.newPool')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if loadingPools}
    <div class="flex justify-center py-10"><Spinner /></div>
  {:else}
    <DataTable
      columns={poolColumns}
      data={pools}
      keyField="id"
      onRowClick={selectPool}
      selectedItemId={selectedPool?.id ?? null}
      emptyIcon={Server}
      emptyMessage={t('settings.adminOperations.runnerPools.empty')}
      emptyDescription={t('settings.adminOperations.runnerPools.emptyDescription')}
    >
      {#snippet name(pool)}
        <span class="font-medium" style="color: var(--ds-text);">{pool.name}</span>
      {/snippet}
      {#snippet status(pool)}
        <Lozenge appearance={pool.is_enabled ? 'success' : 'default'} size="sm">
          {pool.is_enabled ? t('common.enabled') : t('common.disabled')}
        </Lozenge>
      {/snippet}
      {#snippet concurrency(pool)}
        <span class="text-sm" style="color: var(--ds-text-subtle);">{maxConcurrentLabel(pool)}</span>
      {/snippet}
    </DataTable>
  {/if}

  {#if selectedPool}
    <!-- Registration tokens -->
    <section class="space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-semibold flex items-center gap-2" style="color: var(--ds-text);">
          <KeyRound size={16} /> {t('settings.adminOperations.runnerPools.registrationTokens', { name: selectedPool.name })}
        </h3>
        <Button variant="secondary" size="sm" onclick={openMint}>
          <Plus size={14} /> {t('settings.adminOperations.runnerPools.mintToken')}
        </Button>
      </div>
      {#if loadingTokens}
        <div class="flex justify-center py-6"><Spinner /></div>
      {:else}
        <DataTable columns={tokenColumns} data={tokens} keyField="id" emptyMessage={t('settings.adminOperations.runnerPools.noTokens')}>
          {#snippet prefix(tok)}
            <code class="text-xs" style="color: var(--ds-text);">{tok.token_prefix}…</code>
          {/snippet}
          {#snippet description(tok)}
            <span class="text-sm" style="color: var(--ds-text-subtle);">{tok.description || '—'}</span>
          {/snippet}
          {#snippet created(tok)}
            <span class="text-xs" style="color: var(--ds-text-subtle);">{fmtDate(tok.created_at)}</span>
          {/snippet}
          {#snippet expires(tok)}
            <span class="text-xs" style="color: var(--ds-text-subtle);">{tok.expires_at ? fmtDate(tok.expires_at) : t('settings.adminOperations.runnerPools.never')}</span>
          {/snippet}
          {#snippet status(tok)}
            {@const s = tokenStatus(tok)}
            <Lozenge appearance={s.appearance} size="sm">{s.label}</Lozenge>
          {/snippet}
          {#snippet actions(tok)}
            {#if !tok.revoked_at}
              <Button variant="danger-ghost" size="small" icon={Trash2} title={t('settings.adminOperations.runnerPools.revokeToken')} onclick={() => revokeToken(tok)}></Button>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </section>

    <!-- Runner instances -->
    <section class="space-y-3">
      <h3 class="text-sm font-semibold flex items-center gap-2" style="color: var(--ds-text);">
        <Server size={16} /> {t('settings.adminOperations.runnerPools.runnersForPool', { name: selectedPool.name })}
      </h3>
      {#if loadingInstances}
        <div class="flex justify-center py-6"><Spinner /></div>
      {:else}
        <DataTable columns={instanceColumns} data={instances} keyField="id" emptyMessage={t('settings.adminOperations.runnerPools.noRunners')}>
          {#snippet name(inst)}
            <span class="font-medium text-sm" style="color: var(--ds-text);">{inst.name || `#${inst.id}`}</span>
          {/snippet}
          {#snippet status(inst)}
            {@const s = instanceStatus(inst)}
            <Lozenge appearance={s.appearance} size="sm">{s.label}</Lozenge>
          {/snippet}
          {#snippet registered(inst)}
            <span class="text-xs" style="color: var(--ds-text-subtle);">{fmtDate(inst.registered_at)}</span>
          {/snippet}
          {#snippet heartbeat(inst)}
            <span class="text-xs" style="color: var(--ds-text-subtle);">{fmtDate(inst.last_heartbeat_at)}</span>
          {/snippet}
          {#snippet actions(inst)}
            {#if !inst.revoked_at}
              <Button variant="danger-ghost" size="small" icon={Trash2} title={t('settings.adminOperations.runnerPools.revokeRunner')} onclick={() => revokeInstance(inst)}></Button>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </section>
  {/if}
</div>

{#if showCreatePool}
  <Modal isOpen={true} onclose={() => (showCreatePool = false)} onSubmit={createPool} submitDisabled={!canCreate || creating}>
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.adminOperations.runnerPools.newPoolTitle')} onclose={() => (showCreatePool = false)} />
      <div class="space-y-4 p-4">
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('common.name')}</span>
          <Input
            class="mt-1"
            size="small"
            bind:value={poolForm.name}
            placeholder="e.g. default-runners"
          />
        </label>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.runnerPools.maxConcurrentRuns')}</span>
          <Input
            type="number"
            min="0"
            class="mt-1"
            size="small"
            bind:value={poolForm.maxConcurrent}
          />
          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.runnerPools.zeroUnlimited')}</span>
        </label>
        <Checkbox bind:checked={poolForm.appliesToAll} label={t('settings.adminOperations.runnerPools.availableAll')} size="small" />
        <Checkbox bind:checked={poolForm.enabled} label={t('common.enabled')} size="small" />
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => (showCreatePool = false)} keyboardHint="Esc">{t('common.cancel')}</Button>
          <Button variant="primary" onclick={createPool} loading={creating} disabled={!canCreate} keyboardHint={submitHint}>{t('settings.adminOperations.runnerPools.createPool')}</Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}

{#if showMint}
  <Modal isOpen={true} onclose={() => (showMint = false)} onSubmit={mintToken} submitDisabled={minting}>
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.adminOperations.runnerPools.mintTitle')} onclose={() => (showMint = false)} />
      <div class="space-y-4 p-4">
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.runnerPools.descriptionOptional')}</span>
          <Input
            class="mt-1"
            size="small"
            bind:value={mintForm.description}
            placeholder="e.g. eu-west runner box"
          />
        </label>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.runnerPools.expiresHours')}</span>
          <Input
            type="number"
            min="0"
            class="mt-1"
            size="small"
            bind:value={mintForm.ttlHours}
          />
          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.runnerPools.zeroNeverExpires')}</span>
        </label>
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => (showMint = false)} keyboardHint="Esc">{t('common.cancel')}</Button>
          <Button variant="primary" onclick={mintToken} loading={minting} keyboardHint={submitHint}>{t('settings.adminOperations.runnerPools.mintToken')}</Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}

{#if mintedToken}
  <Modal isOpen={true} onclose={() => (mintedToken = null)}>
    {#snippet children()}
      <ModalHeader title={t('settings.adminOperations.runnerPools.addHostTitle')} onclose={() => (mintedToken = null)} />
      <div class="space-y-4 p-4">
        {#if mintedToken.installCommand}
          <div class="space-y-2">
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('settings.adminOperations.runnerPools.installHelp')}
            </p>
            <div class="flex items-start gap-2">
              <code
                data-testid="runner-install-command"
                class="flex-1 overflow-x-auto whitespace-pre rounded-md border px-3 py-2 text-xs"
                style="background: var(--ds-surface-sunken); color: var(--ds-text); border-color: var(--ds-border);"
              >{mintedToken.installCommand}</code>
              <Button variant="primary" size="sm" dataTestid="copy-install-command" onclick={() => copy(mintedToken.installCommand)}>
                <Copy size={14} /> {t('common.copy')}
              </Button>
            </div>
          </div>
        {/if}
        <div class="space-y-2">
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {#if mintedToken.installCommand}{t('settings.adminOperations.runnerPools.manualPrefix')}{:else}{t('settings.adminOperations.runnerPools.setTokenPrefix')}{/if}
            <code>{REGISTRATION_TOKEN_ENV}</code> {t('settings.adminOperations.runnerPools.tokenOnce')}
          </p>
          <div class="flex items-center gap-2">
            <code
              data-testid="runner-registration-token"
              class="flex-1 overflow-x-auto rounded-md border px-3 py-2 text-xs"
              style="background: var(--ds-surface-sunken); color: var(--ds-text); border-color: var(--ds-border);"
            >{mintedToken.token}</code>
            <Button variant="secondary" size="sm" dataTestid="copy-registration-token" onclick={() => copy(mintedToken.token)}>
              <Copy size={14} /> {t('common.copy')}
            </Button>
          </div>
        </div>
        <div class="flex justify-end pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="primary" dataTestid="runner-token-done" onclick={() => (mintedToken = null)}>{t('common.done')}</Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}
