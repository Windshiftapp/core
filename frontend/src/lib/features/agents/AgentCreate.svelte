<script>
  import { onMount } from 'svelte';
  import {
    IconArrowLeft as ArrowLeft,
    IconBolt as Bolt,
    IconCode as Code,
    IconFile as BlankIcon,
    IconGitPullRequest as ReviewIcon,
    IconListCheck as TriageIcon,
    IconMap2 as WorkspaceGuideIcon,
    IconRocket as ReleaseIcon,
    IconRoute as DeliveryIcon,
    IconTestPipe as QAIcon,
    IconUserStar as AgentIcon,
  } from '@tabler/icons-svelte-runes';
  import { agentBindings, api } from '../../api.js';
  import { navigate } from '../../router.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import FormField from '../../components/FormField.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { moduleSettings } from '../../stores/moduleSettings.js';
  import { getShortcutDisplay, toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import AgentRunnerSetup from './AgentRunnerSetup.svelte';

  let { workspaceId } = $props();

  let templates = $state([]);
  let llmConnections = $state([]);
  let candidates = $state([]);
  let capabilityCatalog = $state([]);
  let repositoryOptions = $state([]);
  let runnerPools = $state([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let submitError = $state('');

  let templateKey = $state('');
  let profileType = $state('standard');
  let name = $state('');
  let handle = $state('');
  let purpose = $state('');
  let instructions = $state('');
  let llmConnectionId = $state(null);
  let actingUserId = $state(0);
  let capabilityGroups = $state([]);
  let selectedRepository = $state('');
  let repoBaseRef = $state('');
  let targetPoolId = $state(null);
  let runnerSetupMode = $state('existing');
  let pendingRunnerTokenId = $state(null);
  let pendingRunnerTokenPoolId = $state(null);
  let knownRunnerInstanceIds = $state([]);
  let draftReady = $state(false);

  const templateIcons = {
    workspace_guide: WorkspaceGuideIcon,
    work_item_triage: TriageIcon,
    delivery_coordinator: DeliveryIcon,
    software_engineer: Code,
    code_reviewer: ReviewIcon,
    qa_test_engineer: QAIcon,
    release_manager: ReleaseIcon,
    blank: BlankIcon,
  };

  const selectedTemplate = $derived(templates.find((template) => template.key === templateKey));
  const draftStorageKey = $derived(`agent-studio-create:${workspaceId}`);
  const canSubmit = $derived(
    !saving
      && templateKey
      && name.trim()
      && /^[a-z0-9][a-z0-9._-]{2,31}$/.test(handle.trim())
      && llmConnectionId
  );
  const llmOptions = $derived([
    { value: null, label: 'Select an LLM connection', disabled: true },
    ...llmConnections.map((connection) => ({
      value: connection.id,
      label: [connection.name, connection.model].filter(Boolean).join(' · '),
    })),
  ]);
  const identityOptions = $derived([
    { value: 0, label: 'Workspace-managed identity' },
    ...candidates.map((candidate) => ({
      value: candidate.user_id,
      label: candidate.name || candidate.username || `User #${candidate.user_id}`,
    })),
  ]);
  const visibleCapabilityCatalog = $derived(
    capabilityCatalog.filter(
      (group) => group.key !== 'tests' || $moduleSettings.test_management_enabled
    )
  );
  onMount(load);

  $effect(() => {
    if (!draftReady) return;
    try {
      window.localStorage.setItem(
        draftStorageKey,
        JSON.stringify({
          templateKey,
          profileType,
          name,
          handle,
          purpose,
          instructions,
          llmConnectionId,
          actingUserId,
          capabilityGroups,
          selectedRepository,
          repoBaseRef,
          targetPoolId,
          runnerSetupMode,
          pendingRunnerTokenId,
          pendingRunnerTokenPoolId,
          knownRunnerInstanceIds,
        })
      );
    } catch {
      // Draft persistence is a convenience; browser storage policy must not
      // block the administrator from creating an agent.
    }
  });

  async function load() {
    loading = true;
    error = '';
    try {
      const [
        templateResult,
        connectionResult,
        candidateResult,
        capabilityResult,
        scmConnections,
        pools,
      ] = await Promise.all([
        agentBindings.listTemplates(workspaceId),
        api.llmProviders.getEnabled(),
        agentBindings.getCandidates(workspaceId).catch(() => []),
        agentBindings.listToolCapabilities(workspaceId).catch(() => []),
        api.workspaceSCM
          .getConnectionsOverview(workspaceId, { includeRepositories: true })
          .catch(() => []),
        api.actionCapabilities.getForWorkspace(workspaceId, 'runner_pool').catch(() => []),
      ]);
      templates = templateResult || [];
      llmConnections = connectionResult || [];
      candidates = candidateResult || [];
      capabilityCatalog = capabilityResult || [];
      runnerPools = pools || [];
      repositoryOptions = [
        { value: '', label: 'Select a centrally configured repository', disabled: true },
        ...(scmConnections || []).flatMap((connection) =>
          (connection.repositories || []).map((repository) => ({
            value: `${connection.id}:${repository.id}`,
            label:
              repository.repository_name
              || repository.repository_url
              || `Repository #${repository.id}`,
            connectionId: connection.id,
            repoSlug: repository.repository_name || '',
            defaultBranch: repository.default_branch || '',
          }))
        ),
      ];
      if (templates.length) selectTemplate(templates[0]);
      if (llmConnections.length === 1) llmConnectionId = llmConnections[0].id;
      restoreDraft();
      draftReady = true;
    } catch (err) {
      error = err.message || 'Agent Studio could not be loaded.';
    } finally {
      loading = false;
    }
  }

  function selectTemplate(template) {
    if (template.default_type !== 'coding' && pendingRunnerTokenId) {
      cancelPendingRunnerSetup();
    }
    templateKey = template.key;
    profileType = template.default_type;
    instructions = template.instructions || '';
    if (!name) name = template.name;
    if (!handle) {
      handle = template.key.replaceAll('_', '-').slice(0, 32);
    }
  }

  function templateIcon(template) {
    return templateIcons[template.key] || (template.default_type === 'coding' ? Code : Bolt);
  }

  function updateHandle() {
    handle = handle.toLowerCase().replace(/[^a-z0-9._-]/g, '').slice(0, 32);
  }

  function restoreDraft() {
    try {
      const saved = JSON.parse(window.localStorage.getItem(draftStorageKey) || 'null');
      if (!saved || typeof saved !== 'object') return;
      const template = templates.find((entry) => entry.key === saved.templateKey);
      if (template) {
        templateKey = template.key;
        profileType = saved.profileType === 'coding' ? 'coding' : 'standard';
      }
      if (typeof saved.name === 'string') name = saved.name;
      if (typeof saved.handle === 'string') handle = saved.handle;
      if (typeof saved.purpose === 'string') purpose = saved.purpose;
      if (typeof saved.instructions === 'string') instructions = saved.instructions;
      if (llmConnections.some((entry) => entry.id === saved.llmConnectionId)) {
        llmConnectionId = saved.llmConnectionId;
      }
      if (candidates.some((entry) => entry.user_id === saved.actingUserId)) {
        actingUserId = saved.actingUserId;
      }
      capabilityGroups = Array.isArray(saved.capabilityGroups)
        ? saved.capabilityGroups.filter((key) =>
            capabilityCatalog.some((group) => group.key === key && !group.required)
          )
        : [];
      if (repositoryOptions.some((entry) => entry.value === saved.selectedRepository)) {
        selectedRepository = saved.selectedRepository;
      }
      if (typeof saved.repoBaseRef === 'string') repoBaseRef = saved.repoBaseRef;
      if (runnerPools.some((entry) => entry.id === saved.targetPoolId)) {
        targetPoolId = saved.targetPoolId;
      }
      if (saved.runnerSetupMode === 'existing') {
        runnerSetupMode = 'existing';
      } else if (['new', 'this_machine', 'another_machine'].includes(saved.runnerSetupMode)) {
        runnerSetupMode = 'new';
      }
      if (Number(saved.pendingRunnerTokenId) > 0) {
        pendingRunnerTokenId = Number(saved.pendingRunnerTokenId);
      }
      if (Number(saved.pendingRunnerTokenPoolId) > 0) {
        pendingRunnerTokenPoolId = Number(saved.pendingRunnerTokenPoolId);
      }
      knownRunnerInstanceIds = Array.isArray(saved.knownRunnerInstanceIds)
        ? saved.knownRunnerInstanceIds.map(Number).filter((id) => id > 0)
        : [];
    } catch {
      try {
        window.localStorage.removeItem(draftStorageKey);
      } catch {
        // Ignore unavailable browser storage.
      }
    }
  }

  async function cancelPendingRunnerSetup() {
    if (pendingRunnerTokenId && pendingRunnerTokenPoolId) {
      await api.runnerPools
        .revokeWorkspaceToken(
          workspaceId,
          pendingRunnerTokenPoolId,
          pendingRunnerTokenId
        )
        .catch(() => {});
    }
    pendingRunnerTokenId = null;
    pendingRunnerTokenPoolId = null;
    knownRunnerInstanceIds = [];
  }

  async function leaveCreate() {
    draftReady = false;
    await cancelPendingRunnerSetup();
    try {
      window.localStorage.removeItem(draftStorageKey);
    } catch {
      // Ignore unavailable browser storage.
    }
    navigate(`/workspaces/${workspaceId}/agents`);
  }

  async function changeProfileType(value) {
    if (value !== 'coding' && pendingRunnerTokenId) {
      await cancelPendingRunnerSetup();
    }
    profileType = value;
  }

  function toggleCapability(group, checked) {
    capabilityGroups = checked
      ? [...new Set([...capabilityGroups, group])]
      : capabilityGroups.filter((value) => value !== group);
  }

  async function createProfile() {
    if (!canSubmit) return;
    saving = true;
    submitError = '';
    try {
      const body = {
        template_key: templateKey,
        profile_type: profileType,
        acting_user_id: Number(actingUserId) || 0,
        name: name.trim(),
        handle: handle.trim(),
        purpose: purpose.trim(),
        instructions,
        llm_connection_id: Number(llmConnectionId),
      };
      if (profileType === 'standard') {
        body.capability_groups = capabilityGroups;
      }
      const repository = repositoryOptions.find((option) => option.value === selectedRepository);
      if (profileType === 'coding' && repository?.repoSlug) {
        body.repos = [
          {
            repo_slug: repository.repoSlug,
            repo_base_ref: repoBaseRef.trim() || repository.defaultBranch,
            scm_connection_id: repository.connectionId,
            is_primary: true,
          },
        ];
      }
      if (profileType === 'coding' && targetPoolId) {
        body.target_pool_id = Number(targetPoolId);
      }
      const created = await agentBindings.createProfile(workspaceId, body);
      try {
        if (pendingRunnerTokenId && pendingRunnerTokenPoolId) {
          window.localStorage.setItem(
            `agent-studio-runner:${workspaceId}:${created.id}`,
            JSON.stringify({
              setupMode: runnerSetupMode,
              pendingTokenId: pendingRunnerTokenId,
              pendingTokenPoolId: pendingRunnerTokenPoolId,
              knownInstanceIds: knownRunnerInstanceIds,
            })
          );
        }
        window.localStorage.removeItem(draftStorageKey);
      } catch {
        // Ignore unavailable browser storage.
      }
      draftReady = false;
      navigate(`/workspaces/${workspaceId}/agents/${created.id}`);
    } catch (err) {
      submitError = err.message || 'The agent could not be created.';
    } finally {
      saving = false;
    }
  }
</script>

<section
  class="min-h-full px-4 py-6 sm:px-6 lg:px-8"
  style="background-color: var(--ds-surface);"
  data-testid="agent-create"
>
  <div class="mx-auto max-w-5xl space-y-6">
    <Button
      variant="subtle"
      size="small"
      icon={ArrowLeft}
      onclick={leaveCreate}
      dataTestid="agent-create-back"
    >
      Back to agents
    </Button>

    <PageHeader
      icon={AgentIcon}
      title="Create an agent"
      subtitle="Start from an approved specialist template and save it as a Draft"
    />

    {#if loading}
      <StateDisplay type="loading" message="Loading Agent Studio…" class="py-20" />
    {:else if error}
      <StateDisplay
        type="error"
        title="Agent Studio unavailable"
        message={error}
        onRetry={load}
        class="py-20"
      />
    {:else}
      <div>
        <h2 class="text-base font-semibold" style="color: var(--ds-text);">
          Choose a specialist
        </h2>
        <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
          Templates provide reviewed starting instructions; you can tailor them before saving.
        </p>
        <div class="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
          {#each templates as template (template.key)}
            {@const TemplateIcon = templateIcon(template)}
            <button
              type="button"
              class="rounded-lg text-left focus:outline-none focus:ring-2"
              style="--tw-ring-color: var(--ds-border-focused);"
              onclick={() => selectTemplate(template)}
              data-testid="agent-template"
              data-template-key={template.key}
              aria-pressed={templateKey === template.key}
            >
              <Card
                variant={templateKey === template.key ? 'raised' : 'outlined'}
                padding="default"
                hoverable
                class="h-full"
                style={templateKey === template.key ? 'border-color: var(--ds-border-selected);' : ''}
              >
                <div class="flex items-center gap-3">
                  <span
                    class="template-icon"
                    class:template-icon--coding={template.default_type === 'coding'}
                  >
                    <TemplateIcon class="h-5 w-5" aria-hidden="true" />
                  </span>
                  <span class="font-medium" style="color: var(--ds-text);">{template.name}</span>
                </div>
                <p
                  class="mt-3 text-xs leading-5"
                  style="color: var(--ds-text-subtle);"
                  data-testid="agent-template-description"
                >
                  {template.default_type === 'coding'
                    ? 'Coding agent that works in connected repositories and opens pull requests.'
                    : 'Windshift agent for workspace planning and coordination.'}
                </p>
              </Card>
            </button>
          {/each}
        </div>
      </div>

      <Card variant="raised" padding="spacious">
        <div class="grid grid-cols-1 gap-x-5 md:grid-cols-2">
          <FormField
            id="agent-name"
            label="Name"
            required
            helper="The display name workspace members will see."
          >
            <Input id="agent-name" bind:value={name} required dataTestid="agent-create-name" />
          </FormField>

          <FormField
            id="agent-handle"
            label="Handle"
            required
            helper="3–32 lowercase letters, numbers, dots, underscores, or hyphens."
          >
            <Input
              id="agent-handle"
              bind:value={handle}
              required
              dataTestid="agent-create-handle"
              oninput={updateHandle}
            />
          </FormField>

          <FormField id="agent-type" label="Profile type" required>
            <Select
              id="agent-create-type"
              bind:value={profileType}
              options={[
                { value: 'standard', label: 'Standard · Windshift runtime' },
                { value: 'coding', label: 'Coding · authorized runner' },
              ]}
              onchange={changeProfileType}
            />
          </FormField>

          <FormField
            id="agent-llm"
            label="LLM connection"
            required
            helper="The selected model is fixed for this profile."
          >
            <Select
              id="agent-create-llm"
              bind:value={llmConnectionId}
              options={llmOptions}
              placeholder="Select an LLM connection"
              required
            />
          </FormField>

          {#if candidates.length}
            <FormField
              id="agent-identity"
              label="Identity"
              helper="Workspace-managed is preferred; centralized identities remain available when required."
              class="md:col-span-2"
            >
              <Select
                id="agent-create-identity"
                bind:value={actingUserId}
                options={identityOptions}
              />
            </FormField>
          {/if}

          <FormField
            id="agent-purpose"
            label="Purpose"
            helper="A short explanation shown in the workspace catalog."
            class="md:col-span-2"
          >
            <Textarea
              id="agent-purpose"
              bind:value={purpose}
              rows={3}
              data-testid="agent-create-purpose"
            />
          </FormField>

          {#if profileType === 'coding'}
            <FormField
              id="agent-repository"
              label="Primary repository"
              helper="Optional while Draft; required before the agent can become Ready."
            >
              <Select
                id="agent-create-repository"
                bind:value={selectedRepository}
                options={repositoryOptions}
                placeholder="Select a centrally configured repository"
                onchange={(value) => {
                  const selected = repositoryOptions.find((option) => option.value === value);
                  repoBaseRef = selected?.defaultBranch || '';
                }}
              />
            </FormField>
            <FormField id="agent-base-ref" label="Base branch">
              <Input
                id="agent-base-ref"
                bind:value={repoBaseRef}
                placeholder="main"
                dataTestid="agent-create-base-ref"
              />
            </FormField>
            <div class="md:col-span-2 mb-4">
              <AgentRunnerSetup
                {workspaceId}
                pools={runnerPools}
                bind:selectedPoolId={targetPoolId}
                bind:setupMode={runnerSetupMode}
                bind:pendingTokenId={pendingRunnerTokenId}
                bind:pendingTokenPoolId={pendingRunnerTokenPoolId}
                bind:knownInstanceIds={knownRunnerInstanceIds}
              />
            </div>
          {:else}
            <div class="md:col-span-2 mb-4">
              <h3 class="text-sm font-semibold" style="color: var(--ds-text);">
                Tools and access
              </h3>
              <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
                Read and comment is mandatory. Optional groups come from the executable tool registry.
              </p>
              <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
                {#each visibleCapabilityCatalog as group (group.key)}
                  <div class="rounded-lg border p-3" style="border-color: var(--ds-border);">
                    <Checkbox
                      checked={group.required || capabilityGroups.includes(group.key)}
                      disabled={group.required}
                      label={group.label}
                      hint={`${group.tools?.length || 0} available tool(s)${group.required ? ' · Required' : ''}`}
                      onchange={(checked) => toggleCapability(group.key, checked)}
                      dataTestid={`agent-capability-${group.key}`}
                    />
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          <FormField
            id="agent-instructions"
            label="Instructions"
            helper="Copied into this profile; future template changes will not overwrite it."
            class="md:col-span-2"
          >
            <Textarea
              id="agent-instructions"
              bind:value={instructions}
              rows={12}
              data-testid="agent-create-instructions"
            />
          </FormField>
        </div>

        {#if llmConnections.length === 0}
          <AlertBox
            variant="warning"
            message="No enabled LLM connection is available. Ask a system administrator to configure one."
          />
        {/if}
        {#if submitError}
          <AlertBox variant="error" message={submitError} class="mt-4" />
        {/if}

        <div class="mt-6 flex justify-end gap-3">
          <Button href={`/workspaces/${workspaceId}/agents`}>Cancel</Button>
          <Button
            variant="primary"
            disabled={!canSubmit}
            loading={saving}
            onclick={createProfile}
            keyboardHint={getShortcutDisplay('agents', 'create')}
            hotkeyConfig={{
              key: toHotkeyString('agents', 'create'),
              guard: () => canSubmit,
            }}
            dataTestid="agent-create-submit"
          >
            Create draft
          </Button>
        </div>
      </Card>
    {/if}
  </div>
</section>

<style>
  .template-icon {
    display: inline-flex;
    width: 2.25rem;
    height: 2.25rem;
    flex: 0 0 auto;
    align-items: center;
    justify-content: center;
    border-radius: 0.625rem;
    color: var(--ds-icon);
    background: var(--ds-background-neutral);
  }

  .template-icon--coding {
    color: var(--ds-icon-accent-blue);
    background: var(--ds-accent-blue-subtle);
  }
</style>
