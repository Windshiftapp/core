<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { api } from '../../api.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import Label from '../../components/Label.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import { Tag, Loader2, Sparkles } from 'lucide-svelte';

  let { milestone, workspaceId = null } = $props();

  const dispatch = createEventDispatcher();

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

  // Release form fields
  let tagName = $state(sanitizeTagName(milestone?.name ?? ''));
  let releaseName = $state(milestone?.name ?? '');
  let releaseBody = $state(milestone?.description ?? '');
  let targetCommitish = $state('');
  let isDraft = $state(false);
  let isPrerelease = $state(false);

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
      if (workspaceId) {
        // Workspace-scoped milestone: load connections for this workspace only
        const conns = await api.workspaceSCM.getConnections(workspaceId) || [];
        connections = conns.map(c => ({ ...c, _workspaceName: null, _workspaceId: workspaceId }));
      } else {
        // Global milestone: load connections across all accessible workspaces
        const allWorkspaces = await api.workspaces.getAll() || [];
        const allConns = [];
        await Promise.all(
          allWorkspaces.map(async (ws) => {
            try {
              const conns = await api.workspaceSCM.getConnections(ws.id) || [];
              conns.forEach(c => allConns.push({ ...c, _workspaceName: ws.name, _workspaceId: ws.id }));
            } catch {
              // skip workspaces where connections can't be loaded
            }
          })
        );
        connections = allConns;
      }

      if (connections.length === 1) {
        selectedConnectionId = connections[0].id;
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
        selectedRepository = repos[0].full_name ?? repos[0].name ?? '';
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

  $effect(() => {
    loadRepositories(selectedConnectionId);
  });

  function getConnectionLabel(conn) {
    const base = conn.name ?? conn.provider_name ?? `Connection ${conn.id}`;
    return conn._workspaceName ? `${base} (${conn._workspaceName})` : base;
  }

  const canSubmit = $derived(
    !submitting &&
    tagName.trim().length > 0 &&
    (!selectedConnectionId || selectedRepository.length > 0)
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
        tag_name: tagName.trim(),
        name: releaseName.trim(),
        body: releaseBody.trim(),
        is_draft: isDraft,
        is_prerelease: isPrerelease,
        target_commitish: targetCommitish.trim()
      };

      if (selectedConnectionId) {
        payload.connection_id = selectedConnectionId;
        payload.repository = selectedRepository;
      }

      const updatedMilestone = await api.milestones.release(milestone.id, payload);
      successToast(
        updatedMilestone.latest_release?.scm_release_url
          ? `Release created at ${updatedMilestone.latest_release.scm_release_url}`
          : 'Milestone marked as completed.',
        'Released'
      );
      dispatch('released', updatedMilestone);
    } catch (err) {
      console.error('Failed to release milestone:', err);
      error = err.message || 'Failed to create release.';
      errorToast(error, 'Release failed');
    } finally {
      submitting = false;
    }
  }

  function cancel() {
    dispatch('close');
  }
</script>

<div class="flex flex-col min-h-0 flex-1 overflow-y-auto p-6 space-y-4">
  <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
    <Tag class="w-5 h-5" style="color: var(--ds-text-subtle);" />
    <h2 class="text-base font-semibold" style="color: var(--ds-text);">Release Milestone</h2>
  </div>

  {#if loading}
    <div class="flex items-center justify-center py-8 gap-2" style="color: var(--ds-text-subtle);">
      <Loader2 class="w-5 h-5 animate-spin" />
      <span>Loading SCM connections…</span>
    </div>
  {:else}
    {#if error}
      <div class="text-sm px-3 py-2 rounded" style="background: var(--ds-background-danger, #fee2e2); color: var(--ds-text-danger, #dc2626);">
        {error}
      </div>
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
              getValue={(r) => r.full_name ?? r.name ?? ''}
              getLabel={(r) => r.full_name ?? r.name ?? ''}
            />
          {/if}
        </div>
      {/if}
    {:else}
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        No SCM connections available. The milestone will be marked as completed without creating a release.
      </p>
    {/if}

    <!-- Tag Name -->
    <div>
      <Label required>Tag Name</Label>
      <input
        type="text"
        bind:value={tagName}
        placeholder="v1.0.0"
        class="w-full px-3 py-2 text-sm rounded border focus:outline-none"
        style="background: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border);"
      />
    </div>

    <!-- Release Title -->
    <div>
      <Label>Release Title</Label>
      <input
        type="text"
        bind:value={releaseName}
        placeholder="Release title"
        class="w-full px-3 py-2 text-sm rounded border focus:outline-none"
        style="background: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border);"
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
      <input
        type="text"
        bind:value={targetCommitish}
        placeholder="main"
        class="w-full px-3 py-2 text-sm rounded border focus:outline-none"
        style="background: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border);"
      />
    </div>

    <!-- Draft / Pre-release checkboxes -->
    <div class="flex items-center gap-6">
      <label class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
        <input type="checkbox" bind:checked={isDraft} />
        Draft
      </label>
      <label class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
        <input type="checkbox" bind:checked={isPrerelease} />
        Pre-release
      </label>
    </div>
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
