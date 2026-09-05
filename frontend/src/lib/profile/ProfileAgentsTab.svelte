<script>
  import { Bot, Code, Copy, Key, MoreHorizontal, Pencil, Plus, Trash2 } from '@lucide/svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';
  import Input from '../components/Input.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { copyToClipboard } from '../utils/clipboard.js';
  import { formatAuthenticatedInstant } from '../utils/authenticatedDateFormatter.js';

  let { userId = null } = $props();

  let agents = $state([]);
  let loading = $state(false);
  let createError = $state('');
  let newAgent = $state({ username: '', first_name: '', last_name: '', email: '' });
  let creating = $state(false);
  let featureDisabled = $state(false);
  let tokensByAgent = $state({});
  let expandedAgent = $state(null);
  let mintState = $state({});
  let editingAgent = $state(null);
  let editingName = $state('');
  let renameError = $state('');
  let renaming = $state(false);
  let loadedUserId = null;
  let loadVersion = 0;

  $effect(() => {
    if (!userId || userId === loadedUserId) return;
    loadedUserId = userId;
    void loadAgents();
  });

  function ensureMintState(agentId) {
    if (!mintState[agentId]) {
      mintState[agentId] = { name: '', expiresAt: '', minting: false, error: '', token: '' };
    }
  }

  async function loadAgents() {
    const version = ++loadVersion;
    loading = true;
    try {
      const result = await api.getMyAgents();
      if (version === loadVersion) agents = result || [];
    } catch (err) {
      if (version === loadVersion) agents = [];
      console.error('Failed to load agents:', err);
    } finally {
      if (version === loadVersion) loading = false;
    }
  }

  async function toggleTokens(agentId) {
    if (expandedAgent === agentId) {
      expandedAgent = null;
      return;
    }
    expandedAgent = agentId;
    ensureMintState(agentId);
    try {
      tokensByAgent[agentId] = await api.getApiTokens(agentId);
    } catch (err) {
      tokensByAgent[agentId] = [];
      console.error('Failed to load agent tokens:', err);
    }
  }

  async function mintToken(agentId) {
    ensureMintState(agentId);
    const state = mintState[agentId];
    if (!state.name.trim()) {
      state.error = t('security.enterTokenName');
      return;
    }
    state.error = '';
    state.minting = true;
    try {
      const result = await api.createApiToken({
        name: state.name,
        user_id: agentId,
        expires_on: state.expiresAt || null,
      });
      state.token = result?.token || result?.api_token?.token || '';
      state.name = '';
      state.expiresAt = '';
      tokensByAgent[agentId] = await api.getApiTokens(agentId);
    } catch (err) {
      state.error = err?.message || t('users.agents.tokenCreateFailed');
    } finally {
      state.minting = false;
    }
  }

  async function revokeToken(agentId, tokenId, tokenName) {
    const confirmed = await confirm({
      title: t('security.revokeToken'),
      message: tokenName
        ? t('security.confirmRevokeToken', { name: tokenName })
        : t('users.agents.confirmRevokeUnnamedToken'),
      confirmText: t('security.revokeToken'),
    });
    if (!confirmed) return;
    try {
      await api.revokeApiToken(tokenId);
      tokensByAgent[agentId] = await api.getApiTokens(agentId);
    } catch (err) {
      console.error('Failed to revoke token:', err);
    }
  }

  async function createAgent() {
    createError = '';
    featureDisabled = false;
    if (!newAgent.username || !newAgent.first_name || !newAgent.last_name) {
      createError = t('users.agents.requiredFields');
      return;
    }
    creating = true;
    try {
      const created = await api.createMyAgent({
        username: newAgent.username,
        first_name: newAgent.first_name,
        last_name: newAgent.last_name,
        email: newAgent.email || undefined,
      });
      agents = [created, ...agents];
      newAgent = { username: '', first_name: '', last_name: '', email: '' };
    } catch (err) {
      createError = err?.message || '';
      featureDisabled = err?.status === 403 && !createError;
      if (!createError && !featureDisabled) createError = t('users.agents.createFailed');
    } finally {
      creating = false;
    }
  }

  async function deleteAgent(agentId) {
    const confirmed = await confirm({
      title: t('users.agents.deleteTitle'),
      message: t('users.agents.deleteMessage'),
      confirmText: t('common.delete'),
    });
    if (!confirmed) return;
    try {
      await api.deleteMyAgent(agentId);
      agents = agents.filter((agent) => agent.id !== agentId);
    } catch (err) {
      console.error('Failed to delete agent:', err);
    }
  }

  function openRename(agent) {
    editingAgent = agent;
    editingName = agent.full_name || `${agent.first_name} ${agent.last_name}`.trim();
    renameError = '';
  }

  function closeRename() {
    if (renaming) return;
    editingAgent = null;
    editingName = '';
    renameError = '';
  }

  async function renameAgent() {
    const name = editingName.trim();
    if (!editingAgent || !name) {
      renameError = t('users.agents.nameRequired');
      return;
    }
    renaming = true;
    renameError = '';
    try {
      const updated = await api.updateMyAgent(editingAgent.id, { name });
      agents = agents.map((agent) => (agent.id === updated.id ? updated : agent));
      editingAgent = null;
      editingName = '';
    } catch (err) {
      renameError = err?.message || t('users.agents.renameFailed');
    } finally {
      renaming = false;
    }
  }

  function actionItems(agent) {
    return [
      { id: 'rename', title: t('users.agents.rename'), icon: Pencil, onClick: () => openRename(agent) },
      {
        id: 'tokens',
        testid: `agent-manage-tokens-${agent.id}`,
        title: expandedAgent === agent.id ? t('users.agents.hideTokens') : t('users.agents.manageTokens'),
        icon: Key,
        onClick: () => toggleTokens(agent.id),
      },
      { type: 'divider' },
      {
        id: 'delete',
        title: t('common.delete'),
        icon: Trash2,
        color: 'var(--ds-text-danger)',
        onClick: () => deleteAgent(agent.id),
      },
    ];
  }
</script>

<div class="mb-6">
  <h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
    <Bot class="h-5 w-5" style="color: var(--ds-text-subtle);" />
    {t('users.agents.title')}
  </h2>
  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.agents.description')}</p>
</div>

<div
  class="border rounded-lg p-6 mb-4"
  style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
>
  <h3 class="text-base font-medium mb-3" style="color: var(--ds-text);">{t('users.agents.createTitle')}</h3>
  <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
    <Input placeholder={t('common.username')} bind:value={newAgent.username} />
    <Input type="email" placeholder={t('common.email')} bind:value={newAgent.email} />
    <Input placeholder={t('users.firstName')} bind:value={newAgent.first_name} />
    <Input placeholder={t('users.lastName')} bind:value={newAgent.last_name} />
  </div>
  {#if createError}
    <p class="text-sm mt-2" style="color: var(--ds-text-danger);">{createError}</p>
  {/if}
  {#if featureDisabled}
    <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">{t('users.agents.creationUnavailable')}</p>
  {/if}
  <div class="mt-3">
    <!-- shortcut-guard-exempt: this submits the inline agent form rather than opening a create surface. -->
    <Button variant="primary" onclick={createAgent} disabled={creating} loading={creating}>
      {creating ? t('users.agents.creating') : t('users.agents.create')}
    </Button>
  </div>
</div>

<div
  class="border rounded-lg p-6"
  style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
>
  <h3 class="text-base font-medium mb-3" style="color: var(--ds-text);">{t('users.agents.yourAgents')}</h3>
  {#if loading}
    <p class="text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
  {:else if agents.length === 0}
    <p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.agents.empty')}</p>
  {:else}
    <ul class="divide-y divide-[var(--ds-border)]">
      {#each agents as agent (agent.id)}
        <li data-testid={`agent-row-${agent.id}`} class="py-3">
          <div class="flex items-center justify-between">
            <div>
              <div class="font-medium" style="color: var(--ds-text);">
                {agent.full_name || `${agent.first_name} ${agent.last_name}`}
              </div>
              <div class="text-sm" style="color: var(--ds-text-subtle);">
                @{agent.username} · {agent.is_active ? t('users.active') : t('users.inactive')}
              </div>
            </div>
            <DropdownMenu
              triggerClass="p-2 rounded hover-bg transition-colors"
              showChevron={false}
              iconOnly
              items={actionItems(agent)}
              placement="bottom-end"
              triggerTestid={`agent-actions-${agent.id}`}
            >
              {#snippet children()}
                <MoreHorizontal class="w-5 h-5" aria-hidden="true" />
                <span class="sr-only">
                  {t('users.agents.actionsFor', { name: agent.full_name || agent.username })}
                </span>
              {/snippet}
            </DropdownMenu>
          </div>

          {#if expandedAgent === agent.id && mintState[agent.id]}
            <div class="mt-3 p-4 rounded" style="background-color: var(--ds-background-neutral);">
              {#if mintState[agent.id].token}
                <div
                  class="p-4 rounded mb-4"
                  style="background-color: var(--ds-background-success-subtle); border: 1px solid var(--ds-border-success);"
                >
                  <h5 class="text-sm font-semibold mb-2" style="color: var(--ds-text-success);">
                    {t('security.tokenCreated')}
                  </h5>
                  <p class="text-sm mb-3" style="color: var(--ds-text);">{t('security.tokenWarning')}</p>
                  <div class="flex items-center space-x-2">
                    <Input type="text" value={mintState[agent.id].token} readonly class="flex-1 font-mono border-[var(--ds-border-success)]" />
                    <Button
                      variant="default"
                      size="small"
                      icon={Copy}
                      onclick={() => copyToClipboard(mintState[agent.id].token)}
                    >{t('common.copy')}</Button>
                  </div>
                  <div class="mt-3">
                    <Button variant="default" size="small" onclick={() => (mintState[agent.id].token = '')}>
                      {t('common.done')}
                    </Button>
                  </div>
                </div>
              {/if}

              <div class="mb-4">
                <h5 class="text-sm font-medium mb-2" style="color: var(--ds-text);">{t('security.createToken')}</h5>
                <div class="grid grid-cols-1 md:grid-cols-2 gap-2 mb-2">
                  <Input placeholder={t('security.tokenName')} bind:value={mintState[agent.id].name} />
                  <Input type="date" bind:value={mintState[agent.id].expiresAt} />
                </div>
                <DescriptionText>{t('users.agents.tokenExpirationHint')}</DescriptionText>
                {#if mintState[agent.id].error}
                  <p class="text-sm mt-2" style="color: var(--ds-text-danger);">{mintState[agent.id].error}</p>
                {/if}
                <div class="mt-3">
                  <!-- shortcut-guard-exempt: this submits the expanded agent's token form. -->
                  <Button
                    variant="primary"
                    icon={Plus}
                    onclick={() => mintToken(agent.id)}
                    disabled={mintState[agent.id].minting || !mintState[agent.id].name.trim()}
                    loading={mintState[agent.id].minting}
                  >{t('security.createToken')}</Button>
                </div>
              </div>

              <h5 class="text-sm font-medium mb-2" style="color: var(--ds-text);">{t('users.agents.existingTokens')}</h5>
              <div class="space-y-2">
                {#each tokensByAgent[agent.id] || [] as token (token.id)}
                  <div
                    data-testid={`agent-token-row-${token.id}`}
                    class="flex items-center justify-between p-3 border rounded"
                    style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
                  >
                    <div class="flex items-center space-x-3">
                      <Code class="h-5 w-5" style="color: var(--ds-icon-subtle);" />
                      <div>
                        <div class="font-medium text-sm" style="color: var(--ds-text);">{token.name}</div>
                        <div class="text-xs" style="color: var(--ds-text-subtle);">
                          {t('users.agents.tokenDates', {
                            created: formatAuthenticatedInstant(token.created_at, { year: 'numeric', month: 'short', day: 'numeric' }) || '-',
                            expires: formatAuthenticatedInstant(token.expires_at, { year: 'numeric', month: 'short', day: 'numeric' }) || t('users.agents.neverExpires'),
                          })}
                        </div>
                      </div>
                    </div>
                    <Button
                      variant="default"
                      size="small"
                      icon={Trash2}
                      dataTestid={`agent-token-revoke-${token.id}`}
                      onclick={() => revokeToken(agent.id, token.id, token.name)}
                    >{t('security.revokeToken')}</Button>
                  </div>
                {:else}
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.agents.noTokens')}</p>
                {/each}
              </div>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<Modal
  isOpen={editingAgent !== null}
  preventClose={renaming}
  closeOnBackdropClick={false}
  onclose={closeRename}
  onSubmit={renameAgent}
  submitDisabled={renaming || !editingName.trim()}
  maxWidth="max-w-md"
>
  <ModalHeader title={t('users.agents.rename')} onClose={closeRename} />
  <div class="px-6 py-4">
    <label for="agent-display-name" class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
      {t('users.displayName')}
    </label>
    <Input id="agent-display-name" bind:value={editingName} maxlength={100} />
    <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
      {t('users.agents.renameHint', { username: editingAgent?.username ?? '' })}
    </p>
    {#if renameError}
      <p class="text-sm mt-2" style="color: var(--ds-text-danger);">{renameError}</p>
    {/if}
  </div>
  <DialogFooter
    confirmLabel={t('users.agents.rename')}
    onCancel={closeRename}
    onConfirm={renameAgent}
    disabled={!editingName.trim()}
    loading={renaming}
    showKeyboardHint
    confirmTestid="agent-rename-confirm"
  />
</Modal>
