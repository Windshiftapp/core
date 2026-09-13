<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import { IconTag as Tag, IconLoader as Loader2, IconSparkles as Sparkles } from '@tabler/icons-svelte-runes';
  import AlertBox from '../../components/AlertBox.svelte';
  import { loadMilestoneReleaseConnections } from './milestoneReleaseData.js';

  let { milestone, workspaceId = null, hasExistingRelease = false, onreleased, onclose } = $props();

  let loading = $state(true);
  let submitting = $state(false);
  let error = $state(null);
  let generatingNotes = $state(false);
  let generateError = $state(null);
  let aiAvailable = $state(false);

  // SCM connection list: raw objects from the API
  let connections = $state([]);
  let selectedConnectionId = $state(null);
  let repositories = $state([]);
  let selectedRepository = $state(''); // "owner/repo" full_name
  let loadingRepos = $state(false);
  let releaseMode = $state('create');
  let releaseCandidates = $state([]);
  let loadingCandidates = $state(false);
  const selectedProviderType = $derived(connections.find(connection => connection.id === selectedConnectionId)?.provider_type);

  // Release form fields (snapshot of milestone at open; user-edited thereafter)
  // svelte-ignore state_referenced_locally
  let tagName = $state(sanitizeTagName(milestone?.name ?? ''));
  // svelte-ignore state_referenced_locally
  let releaseName = $state(milestone?.name ?? '');
  // svelte-ignore state_referenced_locally
  let releaseBody = $state(milestone?.description ?? '');
  let targetCommitish = $state('');
  let isDraft = $state(false);
  let isPrerelease = $state(false);
  let idempotencyKey = $state(null);

  function sanitizeTagName(name) {
    return 'v' + (name ?? '')
      .toLowerCase()
      .replace(/[^a-z0-9._-]/g, '-')
      .replace(/-+/g, '-')
      .replace(/^-+|-+$/g, '')
      .substring(0, 50);
  }

  onMount(async () => {
    await Promise.all([
      loadConnections(),
      api.ai.status().then(s => { aiAvailable = s?.available ?? false; }).catch(() => {})
    ]);
  });

  async function loadConnections() {
    loading = true;
    error = null;
    try {
      connections = await loadMilestoneReleaseConnections(api, workspaceId);

      if (connections.length === 1) {
        selectedConnectionId = connections[0].id;
        await loadRepositories(selectedConnectionId);
      }
    } catch (err) {
      console.error('Failed to load SCM connections:', err);
      error = 'Failed to load SCM connections.';
    } finally {
      loading = false;
    }
  }

  async function loadRepositories(connectionId) {
    if (!connectionId) {
      repositories = [];
      selectedRepository = '';
      return;
    }
    loadingRepos = true;
    const conn = connections.find(c => c.id === connectionId);
    const wsId = conn?._workspaceId ?? workspaceId;
    if (!wsId) {
      repositories = [];
      loadingRepos = false;
      return;
    }
    try {
      const repos = await api.workspaceSCM.getLinkedRepos(wsId, connectionId) || [];
      repositories = repos;
      if (repos.length === 1) {
        selectedRepository = repos[0].repository_name ?? '';
        await loadReleaseCandidates(selectedRepository);
      } else {
        selectedRepository = '';
      }
    } catch (err) {
      console.error('Failed to load repositories:', err);
      repositories = [];
    } finally {
      loadingRepos = false;
    }
  }

  async function loadReleaseCandidates(repositoryName) {
    releaseCandidates = [];
    if (releaseMode !== 'attach' || !repositoryName) return;
    const repository = repositories.find(repo => repo.repository_name === repositoryName);
    if (!repository?.id) return;
    loadingCandidates = true;
    try {
      releaseCandidates = await api.workspaceSCM.getReleaseCandidates(repository.id) || [];
      if (releaseCandidates.length === 1) tagName = releaseCandidates[0].tag_name;
    } catch (err) {
      console.error('Failed to load release candidates:', err);
      error = err.message || 'Failed to load existing tags and releases.';
    } finally {
      loadingCandidates = false;
    }
  }

  async function handleReleaseModeChange(mode) {
    if (mode === 'attach') {
      await loadReleaseCandidates(selectedRepository);
    } else {
      releaseCandidates = [];
    }
  }

  async function handleConnectionChange(connection) {
    await loadRepositories(connection?.id ?? null);
  }

  function getConnectionLabel(conn) {
    const base = conn.name ?? conn.provider_name ?? `Connection ${conn.id}`;
    return conn._workspaceName ? `${base} (${conn._workspaceName})` : base;
  }

  const canSubmit = $derived(
    !submitting &&
    tagName.trim().length > 0 &&
    (!selectedConnectionId || selectedRepository.length > 0) &&
    (releaseMode !== 'attach' || releaseCandidates.some(candidate => candidate.tag_name === tagName))
  );

  async function generateNotes() {
    if (generatingNotes) return;
    generatingNotes = true;
    generateError = null;
    try {
      const result = await api.ai.generateReleaseNotes(milestone.id);
      if (result?.tag_name) tagName = result.tag_name;
      if (result?.name) releaseName = result.name;
      if (result?.notes) releaseBody = result.notes;
    } catch (err) {
      generateError = err.message || 'Failed to generate release notes.';
    } finally {
      generatingNotes = false;
    }
  }

  async function submit() {
    if (!canSubmit) return;
    submitting = true;
    error = null;

    try {
      const payload = {
        mode: releaseMode,
        tag_name: tagName.trim(),
        name: releaseName.trim(),
        body: releaseBody.trim(),
        is_draft: selectedProviderType === 'gitlab' ? false : isDraft,
        is_prerelease: selectedProviderType === 'gitlab' ? false : isPrerelease,
        target_commitish: targetCommitish.trim()
      };

      if (selectedConnectionId) {
        payload.connection_id = selectedConnectionId;
        payload.repository = selectedRepository;
        payload.repository_id = repositories.find(repo => repo.repository_name === selectedRepository)?.id;
      }

      idempotencyKey ??= crypto.randomUUID();
      const updatedMilestone = await api.milestones.release(milestone.id, payload, idempotencyKey);
      successToast(
        releaseMode === 'attach'
          ? `Attached ${tagName.trim()} to the milestone.`
          : updatedMilestone.latest_release?.scm_release_url
          ? `Release created at ${updatedMilestone.latest_release.scm_release_url}`
          : 'Milestone marked as completed.',
        'Released'
      );
      onreleased?.(updatedMilestone);
    } catch (err) {
      console.error('Failed to release milestone:', err);
      error = err.message || 'Failed to create release.';
      errorToast(error, 'Release failed');
    } finally {
      submitting = false;
    }
  }

  function cancel() {
    onclose?.();
  }
</script>

<div class="flex flex-col min-h-0 flex-1 overflow-y-auto p-6 space-y-4">
  <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
    <Tag class="w-5 h-5" style="color: var(--ds-text-subtle);" />
    <h2 class="text-base font-semibold" style="color: var(--ds-text);">Release Milestone</h2>
  </div>

  {#if hasExistingRelease}
    <AlertBox variant="warning" message="This milestone already has a release. Creating another will add a new release to the SCM provider." />
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-8 gap-2" style="color: var(--ds-text-subtle);">
      <Loader2 class="w-5 h-5 animate-spin" />
      <span>Loading SCM connections…</span>
    </div>
  {:else}
    {#if error}
      <AlertBox variant="error" message={error} />
    {/if}

    <!-- SCM Connection -->
    {#if connections.length > 0}
      <div>
        <Label>SCM Connection <span class="font-normal" style="color: var(--ds-text-subtle);">(optional)</span></Label>
        <BasePicker
          bind:value={selectedConnectionId}
          items={connections}
          placeholder="Select a connection"
          allowClear={true}
          getValue={(c) => c.id}
          getLabel={getConnectionLabel}
          onSelect={handleConnectionChange}
        />
      </div>

      <!-- Repository -->
      {#if selectedConnectionId}
        <div>
          <Label>Repository</Label>
          {#if loadingRepos}
            <div class="flex items-center gap-2 text-sm py-2" style="color: var(--ds-text-subtle);">
              <Loader2 class="w-4 h-4 animate-spin" />
              Loading repositories…
            </div>
          {:else if repositories.length === 0}
            <p class="text-sm py-2" style="color: var(--ds-text-subtle);">No linked repositories found for this connection.</p>
          {:else}
            <BasePicker
              bind:value={selectedRepository}
              items={repositories}
              placeholder="Select a repository"
              getValue={(r) => r.repository_name ?? ''}
              getLabel={(r) => r.repository_name ?? ''}
              onSelect={(repository) => loadReleaseCandidates(repository.repository_name)}
            />
          {/if}
        </div>
      {/if}

      {#if selectedConnectionId && selectedRepository}
        <div>
          <Label>Release action</Label>
          <BasePicker
            bind:value={releaseMode}
            items={[
              { value: 'create', label: 'Create a new SCM release' },
              { value: 'attach', label: 'Attach an existing tag or release' }
            ]}
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
            onSelect={(item) => handleReleaseModeChange(item.value)}
          />
        </div>
      {/if}
    {:else}
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        No SCM connections available. The milestone will be marked as completed without creating a release.
      </p>
    {/if}

    <!-- Tag Name -->
    <div>
      <Label required>{releaseMode === 'attach' ? 'Existing tag or release' : 'Tag Name'}</Label>
      {#if releaseMode === 'attach'}
        {#if loadingCandidates}
          <div class="flex items-center gap-2 text-sm py-2" style="color: var(--ds-text-subtle);">
            <Loader2 class="w-4 h-4 animate-spin" />
            Loading tags and releases…
          </div>
        {:else if releaseCandidates.length === 0}
          <p class="text-sm py-2" style="color: var(--ds-text-subtle);">No tags or releases were found in this repository.</p>
        {:else}
          <BasePicker
            bind:value={tagName}
            items={releaseCandidates}
            placeholder="Select a tag or release"
            getValue={(candidate) => candidate.tag_name}
            getLabel={(candidate) => `${candidate.tag_name} · ${candidate.release ? 'release' : 'tag'}`}
          />
        {/if}
      {:else}
        <Input
          type="text"
          bind:value={tagName}
          placeholder="v1.0.0"
          size="small"
        />
      {/if}
    </div>

    <!-- Release Title -->
    {#if releaseMode === 'create'}
    <div>
      <Label>Release Title</Label>
      <Input
        type="text"
        bind:value={releaseName}
        placeholder="Release title"
        size="small"
      />
    </div>

    <!-- Release Notes -->
    <div>
      <div class="flex items-center gap-2 mb-1">
        <Label class="mb-0">Release Notes</Label>
        {#if aiAvailable}
          <button
            type="button"
            onclick={generateNotes}
            disabled={generatingNotes}
            class="flex items-center gap-1 px-2 py-0.5 text-xs rounded border transition-opacity"
            style="color: var(--ds-text-subtle); border-color: var(--ds-border); background: var(--ds-background); opacity: {generatingNotes ? 0.5 : 1};"
            title="Generate release notes with AI"
          >
            {#if generatingNotes}
              <Loader2 class="w-3 h-3 animate-spin" />
            {:else}
              <Sparkles class="w-3 h-3" />
            {/if}
            <span>{generatingNotes ? 'Generating…' : 'Generate'}</span>
          </button>
        {/if}
      </div>
      {#if generateError}
        <p class="text-xs mb-1" style="color: var(--ds-text-danger, #dc2626);">{generateError}</p>
      {/if}
      <Textarea
        bind:value={releaseBody}
        placeholder="Describe the changes in this release…"
        rows={20}
      />
    </div>

    <!-- Target Branch (optional) -->
    <div>
      <Label>Target Branch / Commit <span class="font-normal" style="color: var(--ds-text-subtle);">(optional)</span></Label>
      <Input
        type="text"
        bind:value={targetCommitish}
        placeholder="main"
        size="small"
      />
    </div>

    <!-- Draft / Pre-release checkboxes -->
    {#if selectedProviderType === 'gitlab'}
      <p class="text-xs" style="color: var(--ds-text-subtle);">GitLab Releases are published directly and do not expose draft or pre-release states.</p>
    {:else}
      <div class="flex items-center gap-6">
        <Checkbox bind:checked={isDraft} label="Draft" size="small" />
        <Checkbox bind:checked={isPrerelease} label="Pre-release" size="small" />
      </div>
    {/if}
    {/if}
  {/if}
</div>

<DialogFooter
  onCancel={cancel}
  onConfirm={submit}
  confirmLabel={submitting ? 'Releasing…' : 'Release'}
  cancelLabel="Cancel"
  loading={submitting}
  disabled={!canSubmit || loading}
/>
