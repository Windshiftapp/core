<script>
  import { onDestroy, onMount } from 'svelte';
  import Bolt from '@tabler/icons-svelte-runes/icons/bolt';
  import Check from '@tabler/icons-svelte-runes/icons/check';
  import Code from '@tabler/icons-svelte-runes/icons/code';
  import Flask from '@tabler/icons-svelte-runes/icons/flask';
  import History from '@tabler/icons-svelte-runes/icons/history';
  import Lock from '@tabler/icons-svelte-runes/icons/lock';
  import Message from '@tabler/icons-svelte-runes/icons/message';
  import Notebook from '@tabler/icons-svelte-runes/icons/notebook';
  import Refresh from '@tabler/icons-svelte-runes/icons/refresh';
  import AgentIcon from '@tabler/icons-svelte-runes/icons/user-star';
  import Save from '@tabler/icons-svelte-runes/icons/device-floppy';
  import ShieldCheck from '@tabler/icons-svelte-runes/icons/shield-check';
  import Archive from '@tabler/icons-svelte-runes/icons/archive';
  import MoreHorizontal from '@tabler/icons-svelte-runes/icons/dots';
  import { actionCapabilities, llmProviders } from '../../api/ai.js';
  import { agentBindings } from '../../api/agentBindings.js';
  import { agentRuns } from '../../api/agentRuns.js';
  import { agentSkills } from '../../api/agentSkills.js';
  import { updateQueryParams } from '../../router.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import Avatar from '../../components/Avatar.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import Badge from '../../components/Badge.svelte';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import FormField from '../../components/FormField.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import Tabs from '../../components/Tabs.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import { formatAuthenticatedDateTime } from '../../utils/authenticatedDateFormatter.js';
  import AgentRunnerSetup from './AgentRunnerSetup.svelte';

  let { workspaceId, agentId, tab = 'overview' } = $props();

  const tabIds = new Set(['overview', 'instructions', 'knowledge', 'tools', 'test', 'runs']);

  function normalizedTab(value) {
    return tabIds.has(value) ? value : 'overview';
  }

  let agent = $state(null);
  let adminProfile = $state(null);
  let validation = $state(null);
  let loading = $state(true);
  let checking = $state(false);
  let activating = $state(false);
  let error = $state('');
  let actionError = $state('');
  let activeTab = $state('overview');
  let profileRuns = $state([]);
  let archiveDialogOpen = $state(false);
  let lifecycleChanging = $state(false);
  let instructionsDraft = $state('');
  let savingInstructions = $state(false);
  let instructionsSaved = $state(false);
  let workspaceSkills = $state([]);
  let selectedSkillIds = $state([]);
  let savingKnowledge = $state(false);
  let knowledgeSaved = $state(false);
  let capabilityCatalog = $state([]);
  let selectedCapabilityGroups = $state([]);
  let savingTools = $state(false);
  let toolsSaved = $state(false);
  let overviewName = $state('');
  let overviewHandle = $state('');
  let overviewAvatarURL = $state('');
  let overviewPurpose = $state('');
  let savingOverview = $state(false);
  let overviewSaved = $state(false);
  let runnerPools = $state([]);
  let llmConnections = $state([]);
  let modelConnectionId = $state(null);
  let savingModel = $state(false);
  let modelSaved = $state(false);
  let selectedRunnerPoolId = $state(null);
  let runnerSetupMode = $state('existing');
  let pendingRunnerTokenId = $state(null);
  let pendingRunnerTokenPoolId = $state(null);
  let knownRunnerInstanceIds = $state([]);
  let connectingRunner = $state(false);
  let testPrompt = $state('');
  let testing = $state(false);
  let testResult = $state(null);
  let testRun = $state(null);
  let activeTestRunId = $state(null);
  let cancellingTest = $state(false);
  let testPollTimer = null;
  let testPollAttempt = 0;

  const tabs = [
    { id: 'overview', label: 'Overview', testid: 'agent-tab-overview' },
    { id: 'instructions', label: 'Instructions', testid: 'agent-tab-instructions' },
    { id: 'knowledge', label: 'Knowledge', testid: 'agent-tab-knowledge' },
    { id: 'tools', label: 'Tools and access', testid: 'agent-tab-tools' },
    { id: 'test', label: 'Test', testid: 'agent-tab-test' },
    { id: 'runs', label: 'Runs', testid: 'agent-tab-runs' },
  ];

  const canAdmin = $derived(workspacePermissions.canAdminWorkspace(workspaceId));
  const instructionsChanged = $derived(
    instructionsDraft !== (adminProfile?.instructions || '')
  );
  const knowledgeChanged = $derived.by(() => {
    const selected = [...selectedSkillIds].map(Number).sort((a, b) => a - b);
    const saved = [...(adminProfile?.skill_ids || [])].map(Number).sort((a, b) => a - b);
    return selected.join(',') !== saved.join(',');
  });
  const toolsChanged = $derived.by(() => {
    const selected = [...selectedCapabilityGroups].sort();
    const saved = [...(adminProfile?.capability_groups || [])].sort();
    return selected.join(',') !== saved.join(',');
  });
  const overviewChanged = $derived(
    overviewName !== (agent?.name || '')
      || overviewHandle !== (agent?.handle || '')
      || overviewAvatarURL !== (agent?.avatar_url || '')
      || overviewPurpose !== (agent?.purpose || '')
  );
  const modelChanged = $derived(
    Number(modelConnectionId || 0) !== Number(adminProfile?.llm_connection_id || 0)
  );
  const modelOptions = $derived([
    { value: null, label: 'Select an LLM connection', disabled: true },
    ...llmConnections.map((connection) => ({
      value: connection.id,
      label: [connection.name, connection.model].filter(Boolean).join(' · '),
    })),
  ]);
  const runnerStorageKey = $derived(`agent-studio-runner:${workspaceId}:${agentId}`);
  const runnerAssignmentLocked = $derived(Number(adminProfile?.target_pool_id) > 0);

  $effect(() => {
    activeTab = normalizedTab(tab);
  });

  $effect(() => {
    stopTestPolling();
    if (
      activeTab === 'test'
      && activeTestRunId
    ) {
      testPollAttempt = 0;
      scheduleTestPoll(0);
    }
    return stopTestPolling;
  });

  $effect(() => {
    if (!adminProfile || !canAdmin) return;
    try {
      if (pendingRunnerTokenId && pendingRunnerTokenPoolId) {
        window.localStorage.setItem(
          runnerStorageKey,
          JSON.stringify({
            setupMode: runnerSetupMode,
            pendingTokenId: pendingRunnerTokenId,
            pendingTokenPoolId: pendingRunnerTokenPoolId,
            knownInstanceIds: knownRunnerInstanceIds,
          })
        );
      } else {
        window.localStorage.removeItem(runnerStorageKey);
      }
    } catch {
      // Onboarding survives navigation when browser storage is available, but
      // storage policy must never block profile administration.
    }
  });

  onMount(load);
  onDestroy(stopTestPolling);

  async function load() {
    loading = true;
    error = '';
    try {
      const [catalog, recentRuns] = await Promise.all([
        agentBindings.listCatalog(workspaceId),
        agentRuns.listForWorkspace(workspaceId, { limit: 100 }).catch(() => []),
      ]);
      agent = (catalog || []).find((entry) => String(entry.id) === String(agentId)) || null;
      profileRuns = (recentRuns || []).filter(
        (run) => String(run.binding_id) === String(agentId)
      );
      if (!agent) {
        error = 'This agent is not available in the workspace.';
        return;
      }
      overviewName = agent.name || '';
      overviewHandle = agent.handle || '';
      overviewAvatarURL = agent.avatar_url || '';
      overviewPurpose = agent.purpose || '';
      if (canAdmin) {
        const [profiles, skills, capabilities, pools, connections] = await Promise.all([
          agentBindings.listForWorkspace(workspaceId),
          agentSkills.listForWorkspace(workspaceId).catch(() => []),
          agentBindings.listToolCapabilities(workspaceId).catch(() => []),
          actionCapabilities
            .getForWorkspace(workspaceId, 'runner_pool')
            .catch(() => []),
          llmProviders.getEnabled().catch(() => []),
        ]);
        workspaceSkills = skills || [];
        capabilityCatalog = capabilities || [];
        runnerPools = pools || [];
        llmConnections = connections || [];
        adminProfile =
          (profiles || []).find((entry) => String(entry.id) === String(agentId)) || null;
        instructionsDraft = adminProfile?.instructions || '';
        selectedSkillIds = [...(adminProfile?.skill_ids || [])];
        selectedCapabilityGroups = [...(adminProfile?.capability_groups || [])];
        modelConnectionId = adminProfile?.llm_connection_id ?? null;
        selectedRunnerPoolId = adminProfile?.target_pool_id || null;
        restoreRunnerSetup();
        await checkReadiness();
      }
    } catch (err) {
      error = err.message || 'The agent profile could not be loaded.';
    } finally {
      loading = false;
    }
  }

  function restoreRunnerSetup() {
    try {
      const saved = JSON.parse(window.localStorage.getItem(runnerStorageKey) || 'null');
      if (!saved || typeof saved !== 'object') return;
      if (saved.setupMode === 'existing') {
        runnerSetupMode = 'existing';
      } else if (['new', 'this_machine', 'another_machine'].includes(saved.setupMode)) {
        runnerSetupMode = 'new';
      }
      if (Number(saved.pendingTokenId) > 0) {
        pendingRunnerTokenId = Number(saved.pendingTokenId);
      }
      if (Number(saved.pendingTokenPoolId) > 0) {
        pendingRunnerTokenPoolId = Number(saved.pendingTokenPoolId);
      }
      knownRunnerInstanceIds = Array.isArray(saved.knownInstanceIds)
        ? saved.knownInstanceIds.map(Number).filter((id) => id > 0)
        : [];
      if (!selectedRunnerPoolId && pendingRunnerTokenPoolId) {
        selectedRunnerPoolId = pendingRunnerTokenPoolId;
      }
    } catch {
      try {
        window.localStorage.removeItem(runnerStorageKey);
      } catch {
        // Ignore unavailable browser storage.
      }
    }
  }

  async function checkReadiness() {
    if (!canAdmin) return;
    checking = true;
    actionError = '';
    try {
      validation = await agentBindings.validateProfile(workspaceId, agentId);
    } catch (err) {
      actionError = err.message || 'Readiness could not be checked.';
    } finally {
      checking = false;
    }
  }

  async function activate() {
    activating = true;
    actionError = '';
    try {
      adminProfile = await agentBindings.activateProfile(workspaceId, agentId);
      validation = { ready: true, errors: [] };
      agent = {
        ...agent,
        lifecycle: 'ready',
        availability: 'ready',
        available: true,
        profile_version: adminProfile.profile_version,
      };
    } catch (err) {
      if (err?.data?.errors) validation = err.data;
      actionError = err.message || 'The agent could not be made Ready.';
    } finally {
      activating = false;
    }
  }

  async function archiveProfile() {
    lifecycleChanging = true;
    actionError = '';
    try {
      await agentBindings.remove(workspaceId, agentId);
      agent = { ...agent, lifecycle: 'archived', availability: 'archived', available: false };
      adminProfile = { ...adminProfile, lifecycle: 'archived' };
    } catch (err) {
      actionError = err.message || 'The agent could not be archived.';
    } finally {
      lifecycleChanging = false;
    }
  }

  async function restoreProfile() {
    lifecycleChanging = true;
    actionError = '';
    try {
      adminProfile = await agentBindings.restore(workspaceId, agentId);
      agent = { ...agent, lifecycle: 'draft', availability: 'draft', available: false };
      await checkReadiness();
    } catch (err) {
      actionError = err.message || 'The agent could not be restored.';
    } finally {
      lifecycleChanging = false;
    }
  }

  async function saveInstructions() {
    if (!canAdmin || !adminProfile || !instructionsChanged || savingInstructions) return;
    // Capture the exact text being submitted; edits made while the request is
    // pending must stay unsaved and keep the Save button enabled.
    const submitted = instructionsDraft;
    savingInstructions = true;
    instructionsSaved = false;
    actionError = '';
    try {
      await agentBindings.updateAgentConfig(workspaceId, agentId, {
        instructions: submitted,
        skill_ids: adminProfile.skill_ids || [],
      });
      const nextVersion = markNewDraftVersion();
      adminProfile = {
        ...adminProfile,
        instructions: submitted,
        profile_version: nextVersion,
      };
      instructionsSaved = true;
      await checkReadiness();
    } catch (err) {
      actionError = err.message || 'Instructions could not be saved.';
    } finally {
      savingInstructions = false;
    }
  }

  function markNewDraftVersion() {
    const nextVersion = Number(adminProfile?.profile_version || agent?.profile_version || 1) + 1;
    adminProfile = {
      ...adminProfile,
      lifecycle: 'draft',
      profile_version: nextVersion,
    };
    agent = {
      ...agent,
      lifecycle: 'draft',
      availability: 'draft',
      available: false,
      profile_version: nextVersion,
    };
    return nextVersion;
  }

  function toggleSkill(skillId, checked) {
    selectedSkillIds = checked
      ? [...new Set([...selectedSkillIds, skillId])]
      : selectedSkillIds.filter((id) => Number(id) !== Number(skillId));
  }

  async function saveKnowledge() {
    if (!canAdmin || !adminProfile || !knowledgeChanged || savingKnowledge) return;
    savingKnowledge = true;
    knowledgeSaved = false;
    actionError = '';
    try {
      await agentBindings.updateAgentConfig(workspaceId, agentId, {
        instructions: adminProfile.instructions || '',
        skill_ids: selectedSkillIds,
      });
      const nextVersion = markNewDraftVersion();
      adminProfile = {
        ...adminProfile,
        skill_ids: [...selectedSkillIds],
        profile_version: nextVersion,
      };
      knowledgeSaved = true;
      await checkReadiness();
    } catch (err) {
      actionError = err.message || 'Knowledge sources could not be saved.';
    } finally {
      savingKnowledge = false;
    }
  }

  function toggleCapability(group, checked) {
    selectedCapabilityGroups = checked
      ? [...new Set([...selectedCapabilityGroups, group])]
      : selectedCapabilityGroups.filter((value) => value !== group);
  }

  async function saveTools() {
    if (
      !canAdmin
      || !adminProfile
      || agent.profile_type !== 'standard'
      || !toolsChanged
      || savingTools
    ) return;
    savingTools = true;
    toolsSaved = false;
    actionError = '';
    try {
      const updated = await agentBindings.update(workspaceId, agentId, {
        repos: [],
        llm_connection_id: adminProfile.llm_connection_id,
        token_ttl_minutes: adminProfile.token_ttl_minutes || 60,
        max_runs_per_day: adminProfile.max_runs_per_day || 0,
        instructions: adminProfile.instructions || '',
        capability_groups: selectedCapabilityGroups,
        skill_ids: adminProfile.skill_ids || [],
      });
      const nextVersion = Number(updated?.profile_version || markNewDraftVersion());
      adminProfile = {
        ...adminProfile,
        ...updated,
        capability_groups: [...selectedCapabilityGroups],
        lifecycle: 'draft',
        profile_version: nextVersion,
      };
      agent = {
        ...agent,
        lifecycle: 'draft',
        availability: 'draft',
        available: false,
        profile_version: nextVersion,
      };
      toolsSaved = true;
      await checkReadiness();
    } catch (err) {
      actionError = err.message || 'Tools and access could not be saved.';
    } finally {
      savingTools = false;
    }
  }

  async function saveModel() {
    if (!canAdmin || !adminProfile || !modelChanged || savingModel) return;
    savingModel = true;
    modelSaved = false;
    actionError = '';
    try {
      await agentBindings.updateAgentConfig(workspaceId, agentId, {
        instructions: adminProfile.instructions || '',
        skill_ids: adminProfile.skill_ids || [],
        llm_connection_id: Number(modelConnectionId),
      });
      const nextVersion = markNewDraftVersion();
      const connection = llmConnections.find(
        (candidate) => Number(candidate.id) === Number(modelConnectionId)
      );
      adminProfile = {
        ...adminProfile,
        llm_connection_id: Number(modelConnectionId),
        profile_version: nextVersion,
      };
      agent = {
        ...agent,
        model_summary: connection?.model || agent.model_summary,
      };
      modelSaved = true;
      await checkReadiness();
    } catch (err) {
      actionError = err.message || 'The model could not be changed.';
    } finally {
      savingModel = false;
    }
  }

  async function saveOverview() {
    if (!canAdmin || !adminProfile || !overviewChanged || savingOverview) return;
    savingOverview = true;
    overviewSaved = false;
    actionError = '';
    try {
      const updated = await agentBindings.updateProfile(workspaceId, agentId, {
        expected_version: adminProfile.profile_version,
        name: overviewName,
        handle: overviewHandle,
        avatar_url: overviewAvatarURL,
        purpose: overviewPurpose,
      });
      adminProfile = { ...adminProfile, ...updated };
      agent = {
        ...agent,
        name: updated.name || overviewName,
        handle: updated.handle || overviewHandle,
        avatar_url: updated.avatar_url || overviewAvatarURL,
        purpose: updated.purpose || overviewPurpose,
        lifecycle: 'draft',
        availability: 'draft',
        available: false,
        profile_version: updated.profile_version,
      };
      overviewName = agent.name || '';
      overviewHandle = agent.handle || '';
      overviewAvatarURL = agent.avatar_url || '';
      overviewPurpose = agent.purpose || '';
      overviewSaved = true;
      await checkReadiness();
    } catch (err) {
      actionError = err.message || 'Profile overview could not be saved.';
    } finally {
      savingOverview = false;
    }
  }

  async function connectRunner() {
    if (!selectedRunnerPoolId || connectingRunner || runnerAssignmentLocked) return;
    connectingRunner = true;
    actionError = '';
    try {
      const updated =
        agent.profile_type === 'legacy'
          ? await agentBindings.migrateLegacyProfile(
              workspaceId,
              agentId,
              selectedRunnerPoolId
            )
          : await agentBindings.connectCodingRunner(
              workspaceId,
              agentId,
              selectedRunnerPoolId
            );
      adminProfile = { ...adminProfile, ...updated };
      agent = {
        ...agent,
        profile_type: 'coding',
        runtime: 'authorized_runner',
        lifecycle: 'draft',
        availability: 'draft',
        available: false,
        profile_version: updated.profile_version,
      };
      await checkReadiness();
    } catch (err) {
      actionError = err.message || 'The runner pool could not be authorized.';
    } finally {
      connectingRunner = false;
    }
  }

  function runnerReady() {
    checkReadiness();
  }

  function stopTestPolling() {
    if (testPollTimer) {
      window.clearTimeout(testPollTimer);
      testPollTimer = null;
    }
  }

  function scheduleTestPoll(delay) {
    stopTestPolling();
    testPollTimer = window.setTimeout(pollTestRun, delay);
  }

  async function pollTestRun() {
    if (!activeTestRunId || activeTab !== 'test') return;
    try {
      const current = await agentRuns.get(activeTestRunId);
      testRun = current;
      if (['succeeded', 'failed', 'canceled'].includes(current.status)) {
        activeTestRunId = null;
        stopTestPolling();
        profileRuns = [
          current,
          ...profileRuns.filter((run) => Number(run.id) !== Number(current.id)),
        ];
        return;
      }
      testPollAttempt += 1;
      scheduleTestPoll(Math.min(10_000, 1_500 * 2 ** Math.min(testPollAttempt, 3)));
    } catch (err) {
      actionError = err.message || 'The verification run could not be refreshed.';
      testPollAttempt += 1;
      scheduleTestPoll(Math.min(10_000, 1_500 * 2 ** Math.min(testPollAttempt, 3)));
    }
  }

  async function runPrivateTest() {
    if (testing) return;
    testing = true;
    testResult = null;
    testRun = null;
    activeTestRunId = null;
    actionError = '';
    try {
      const result = await agentBindings.testProfile(workspaceId, agentId, testPrompt);
      if (result.mode === 'standard') {
        testResult = result;
      } else {
        testRun = { id: result.run_id, status: result.status || 'queued' };
        activeTestRunId = result.run_id;
      }
    } catch (err) {
      actionError = err.message || 'The private test could not be started.';
    } finally {
      testing = false;
    }
  }

  async function cancelTestRun() {
    if (!testRun?.id || cancellingTest) return;
    cancellingTest = true;
    actionError = '';
    try {
      await agentRuns.cancel(testRun.id);
      await pollTestRun();
    } catch (err) {
      actionError = err.message || 'The verification run could not be cancelled.';
    } finally {
      cancellingTest = false;
    }
  }

  function availabilityVariant(value) {
    if (value === 'ready') return 'success';
    if (value === 'offline') return 'warning';
    if (value === 'invalid') return 'danger';
    if (value === 'draft' || value === 'needs_setup') return 'warning';
    return 'neutral';
  }

  function label(value) {
    return String(value || '')
      .replaceAll('_', ' ')
      .replace(/\b\w/g, (letter) => letter.toUpperCase());
  }

  function profileActionItems() {
    return [
      {
        id: 'archive',
        type: 'regular',
        icon: Archive,
        title: 'Archive',
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        testid: 'agent-archive',
        onClick: () => archiveDialogOpen = true,
      },
    ];
  }

  function changeTab({ tab }) {
    activeTab = tab;
    updateQueryParams({ tab: tab === 'overview' ? null : tab }, { push: true });
  }
</script>

<section
  class="min-h-full px-4 py-6 sm:px-6 lg:px-8"
  style="background-color: var(--ds-surface);"
  data-testid="agent-profile"
>
  <div class="mx-auto max-w-5xl space-y-6">
    <BackButton
      href={`/workspaces/${workspaceId}/agents`}
      label="Back to agents"
      testid="agent-profile-back"
    />

    {#if loading}
      <StateDisplay type="loading" message="Loading agent profile…" class="py-20" />
    {:else if error}
      <StateDisplay
        type="error"
        title="Agent unavailable"
        message={error}
        onRetry={load}
        class="py-20"
      />
    {:else}
      <PageHeader
        icon={AgentIcon}
        title={agent.name || agent.handle}
        subtitle={agent.purpose || 'Workspace specialist'}
      >
        {#snippet actions()}
          <div class="flex min-h-8 items-center gap-2">
            <Badge variant={availabilityVariant(agent.availability)}>
              {label(agent.availability)}
            </Badge>
            {#if canAdmin}
              {#if agent.lifecycle === 'archived'}
                <Button
                  variant="default"
                  size="small"
                  loading={lifecycleChanging}
                  onclick={restoreProfile}
                  dataTestid="agent-restore"
                >
                  Restore as Draft
                </Button>
              {:else}
                <DropdownMenu
                  triggerIcon={MoreHorizontal}
                  triggerClass="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
                  triggerStyle="background-color: var(--ds-surface); color: var(--ds-text-subtle);"
                  triggerTestid="agent-profile-actions"
                  triggerLabel="Agent actions"
                  items={profileActionItems()}
                  maxWidth="max-w-48"
                  showChevron={false}
                  iconOnly={true}
                  disabled={lifecycleChanging}
                />
              {/if}
            {/if}
          </div>
        {/snippet}
      </PageHeader>

      <div class="overflow-x-auto">
        <Tabs {tabs} bind:activeTab onTabChange={changeTab}>
      {#if activeTab === 'overview'}
      <div class="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <Card variant="raised" padding="spacious" class="lg:col-span-2">
          <div class="flex items-start gap-4">
            <Avatar
              src={agent.avatar_url}
              name={agent.name || agent.handle}
              size="xl"
              variant={agent.profile_type === 'coding' ? 'purple' : 'blue'}
              ring
            />
            <div class="min-w-0">
              <h2 class="text-lg font-semibold" style="color: var(--ds-text);">
                Identity
              </h2>
              <p class="mt-1" style="color: var(--ds-text-subtle);">
                @{agent.handle}
              </p>
              <p class="mt-3 text-sm" style="color: var(--ds-text-subtle);">
                {label(agent.identity_class)} identity · Version {agent.profile_version}
              </p>
              {#if agent.owner_name}
                <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
                  Owned by {agent.owner_name}
                </p>
              {/if}
            </div>
          </div>

          <div class="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div class="rounded-lg border p-4" style="border-color: var(--ds-border);">
              <div class="flex items-center gap-2 font-medium" style="color: var(--ds-text);">
                {#if agent.profile_type === 'coding'}
                  <Code class="h-4 w-4" />
                {:else}
                  <Bolt class="h-4 w-4" />
                {/if}
                {label(agent.profile_type)} agent
              </div>
              <p class="mt-2 text-sm" style="color: var(--ds-text-subtle);">
                {label(agent.runtime)} runtime
              </p>
              <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
                {agent.model_summary || 'Model unavailable'}
              </p>
            </div>
            <div class="rounded-lg border p-4" style="border-color: var(--ds-border);">
              <div class="flex items-center gap-2 font-medium" style="color: var(--ds-text);">
                <Message class="h-4 w-4" />
                Ways to work
              </div>
              <p class="mt-2 text-sm" style="color: var(--ds-text-subtle);">
                {agent.profile_type === 'standard'
                  ? 'Assignment, mentions, and workspace chat'
                  : 'Direct assignment and mentions'}
              </p>
            </div>
          </div>
        </Card>

        <Card variant="outlined" padding="spacious">
          <div class="flex items-center gap-2">
            <ShieldCheck class="h-5 w-5" style="color: var(--ds-icon);" />
            <h2 class="font-semibold" style="color: var(--ds-text);">Availability</h2>
          </div>
          <p class="mt-3 text-sm" style="color: var(--ds-text-subtle);">
            {agent.availability === 'offline'
              ? 'Runner offline. New work can still be assigned and will queue until a runner returns.'
              : agent.available
                ? 'Ready to participate in this workspace.'
                : 'Not currently available for new work.'}
          </p>
          {#if agent.updated_at}
            <p class="mt-4 text-xs" style="color: var(--ds-text-subtlest);">
              Updated {formatAuthenticatedDateTime(agent.updated_at)}
            </p>
          {/if}
        </Card>
      </div>

      {#if canAdmin && agent.lifecycle !== 'archived'}
        <Card variant="outlined" padding="spacious" dataTestid="agent-overview-editor">
          <h2 class="text-lg font-semibold" style="color: var(--ds-text);">
            Profile details
          </h2>
          <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
            Identity fields are editable only for workspace-managed agents. Type and identity class never change.
          </p>
          <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
            <FormField
              label="Name"
              helper={agent.identity_class === 'workspace_managed'
                ? 'Shown anywhere this agent acts.'
                : 'Managed by the central identity.'}
            >
              <Input
                bind:value={overviewName}
                disabled={agent.identity_class !== 'workspace_managed'}
                dataTestid="agent-overview-name"
              />
            </FormField>
            <FormField
              label="Handle"
              helper="Three to 32 lowercase letters, numbers, dots, underscores, or hyphens."
            >
              <Input
                bind:value={overviewHandle}
                disabled={agent.identity_class !== 'workspace_managed'}
                dataTestid="agent-overview-handle"
              />
            </FormField>
            <FormField label="Avatar URL">
              <Input
                bind:value={overviewAvatarURL}
                disabled={agent.identity_class !== 'workspace_managed'}
                dataTestid="agent-overview-avatar"
              />
            </FormField>
            <FormField label="Purpose" helper="A concise teammate-facing description.">
              <Input bind:value={overviewPurpose} dataTestid="agent-overview-purpose" />
            </FormField>
          </div>
          <div class="mt-4 flex justify-end">
            <Button
              variant="primary"
              icon={Save}
              loading={savingOverview}
              disabled={!overviewChanged}
              onclick={saveOverview}
              dataTestid="agent-overview-save"
            >
              Save profile details
            </Button>
          </div>
          {#if overviewSaved}
            <AlertBox
              variant="success"
              message="Profile details saved as a new Draft version."
              class="mt-4"
            />
          {/if}
          {#if actionError}
            <AlertBox variant="error" message={actionError} class="mt-4" />
          {/if}
        </Card>
      {/if}

      {#if canAdmin && agent.lifecycle !== 'archived'}
        <Card variant="outlined" padding="spacious" dataTestid="agent-readiness">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <h2 class="text-lg font-semibold" style="color: var(--ds-text);">
                Readiness
              </h2>
              <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
                Validate live model, identity, permission, and runtime dependencies.
              </p>
            </div>
            <Button
              variant="default"
              size="small"
              icon={Refresh}
              loading={checking}
              onclick={checkReadiness}
              dataTestid="agent-readiness-check"
            >
              Check again
            </Button>
          </div>

          {#if actionError}
            <AlertBox variant="error" message={actionError} class="mt-4" />
          {/if}

          {#if validation?.ready}
            <AlertBox variant="success" class="mt-4">
              <div class="flex items-center justify-between gap-4">
                <span>All required dependencies are ready.</span>
                {#if adminProfile?.lifecycle !== 'ready'}
                  <Button
                    variant="primary"
                    size="small"
                    icon={Check}
                    loading={activating}
                    onclick={activate}
                    dataTestid="agent-make-ready"
                  >
                    Make Ready
                  </Button>
                {/if}
              </div>
            </AlertBox>
          {:else if validation?.errors?.length}
            <div class="mt-4 space-y-2">
              {#each validation.errors as issue (`${issue.code}:${issue.dependency}`)}
                <AlertBox variant="warning">
                  <div>
                    <p class="font-medium">{issue.message}</p>
                    {#if issue.dependency}
                      <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
                        Dependency: {issue.dependency}
                      </p>
                    {/if}
                    {#if issue.hint}
                      <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
                        {issue.hint}
                      </p>
                    {/if}
                    {#if issue.code === 'permission_missing'}
                      <Button
                        href={`/workspaces/${workspaceId}/settings/members`}
                        variant="default"
                        size="small"
                        class="mt-2"
                        dataTestid="agent-readiness-grant-permission"
                      >
                        Open workspace members
                      </Button>
                    {/if}
                  </div>
                </AlertBox>
              {/each}
            </div>
          {:else if checking}
            <StateDisplay type="loading" message="Checking readiness…" class="py-8" />
          {/if}
        </Card>
      {/if}
      {:else if activeTab === 'instructions'}
        {#if canAdmin}
          <Card variant="outlined" padding="spacious" dataTestid="agent-instructions">
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Instructions</h2>
            <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
              The effective prompt copied into profile version {agent.profile_version}.
            </p>
            <Textarea
              bind:value={instructionsDraft}
              rows={18}
              class="mt-4 font-mono text-sm"
              data-testid="agent-instructions-value"
            />
            <div class="mt-4 flex flex-wrap items-center justify-between gap-3">
              <p class="text-sm" style="color: var(--ds-text-subtle);">
                Saving instructions creates a new Draft profile version.
              </p>
              <Button
                variant="primary"
                icon={Save}
                loading={savingInstructions}
                disabled={!instructionsChanged}
                onclick={saveInstructions}
                dataTestid="agent-instructions-save"
              >
                Save instructions
              </Button>
            </div>
            {#if instructionsSaved}
              <AlertBox
                variant="success"
                message="Instructions saved as a new Draft version."
                class="mt-4"
              />
            {/if}
            {#if actionError}
              <AlertBox variant="error" message={actionError} class="mt-4" />
            {/if}
          </Card>
        {:else}
          <Card variant="outlined" padding="spacious">
            <StateDisplay
              type="empty"
              icon={Lock}
              title="Administrator access required"
              description="Full agent instructions are restricted to workspace administrators."
            />
          </Card>
        {/if}
      {:else if activeTab === 'knowledge'}
        {#if canAdmin}
          <Card variant="outlined" padding="spacious" dataTestid="agent-knowledge">
            <div class="flex items-center gap-2">
              <Notebook class="h-5 w-5" style="color: var(--ds-icon);" />
              <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Knowledge</h2>
            </div>
            <p class="mt-3 text-sm" style="color: var(--ds-text-subtle);">
              Attach reusable workspace knowledge packs to this profile.
            </p>
            {#if workspaceSkills.length}
              <div class="mt-4 space-y-3">
                {#each workspaceSkills as skill (skill.id)}
                  <div
                    class="rounded-lg border p-3"
                    style="border-color: var(--ds-border);"
                  >
                    <Checkbox
                      checked={selectedSkillIds.includes(skill.id)}
                      disabled={!skill.enabled && !selectedSkillIds.includes(skill.id)}
                      label={skill.enabled ? skill.name : `${skill.name} (disabled)`}
                      hint={skill.description}
                      onchange={(checked) => toggleSkill(skill.id, checked)}
                      dataTestid="agent-knowledge-skill"
                    />
                  </div>
                {/each}
              </div>
            {:else}
              <StateDisplay
                type="empty"
                title="No knowledge sources"
                description="Create reusable agent skills in workspace settings first."
                class="py-8"
              />
            {/if}
            <div class="mt-4 flex justify-end">
              <Button
                variant="primary"
                icon={Save}
                loading={savingKnowledge}
                disabled={!knowledgeChanged}
                onclick={saveKnowledge}
                dataTestid="agent-knowledge-save"
              >
                Save knowledge
              </Button>
            </div>
            {#if knowledgeSaved}
              <AlertBox
                variant="success"
                message="Knowledge sources saved as a new Draft version."
                class="mt-4"
              />
            {/if}
            {#if actionError}
              <AlertBox variant="error" message={actionError} class="mt-4" />
            {/if}
          </Card>
        {:else}
          <Card variant="outlined" padding="spacious">
            <StateDisplay
              type="empty"
              icon={Lock}
              title="Administrator access required"
              description="Knowledge scope configuration is restricted to workspace administrators."
            />
          </Card>
        {/if}
      {:else if activeTab === 'tools'}
        {#if canAdmin}
          <Card variant="outlined" padding="spacious" dataTestid="agent-tools-access">
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">
              Tools and access
            </h2>
            <!-- Model connection: editable for every profile type. The runtime
                 keeps the selected model until an admin retargets it here. -->
            <div
              class="mt-4 rounded-lg border p-4"
              style="border-color: var(--ds-border);"
              data-testid="agent-model"
            >
              <h3 class="text-base font-medium" style="color: var(--ds-text);">Model</h3>
              <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
                The LLM connection and model this agent runs on. Changing it returns the profile to
                Draft so the new model is validated before it accepts work.
              </p>
              {#if llmConnections.length === 0}
                <AlertBox
                  variant="warning"
                  message="No enabled LLM connection is available. Ask a system administrator to configure one under Admin → AI Connections."
                  class="mt-3"
                />
              {:else}
                <div class="mt-3 max-w-md">
                  <Select
                    id="agent-model-connection"
                    ariaLabel="Model"
                    bind:value={modelConnectionId}
                    options={modelOptions}
                  />
                </div>
              {/if}
              <div class="mt-3 flex justify-end">
                <Button
                  variant="primary"
                  icon={Save}
                  loading={savingModel}
                  disabled={!modelChanged || llmConnections.length === 0}
                  onclick={saveModel}
                  dataTestid="agent-model-save"
                >
                  Save model
                </Button>
              </div>
              {#if modelSaved}
                <AlertBox
                  variant="success"
                  message="Model changed. The profile is now a Draft and must be made Ready again."
                  class="mt-3"
                />
              {/if}
            </div>
            {#if agent.profile_type === 'standard'}
              <AlertBox
                variant="info"
                message="Read and comment is mandatory for every Standard profile."
                class="mt-4"
              />
              <div class="mt-4 space-y-3">
                {#each capabilityCatalog as group (group.key)}
                  <div
                    class="rounded-lg border p-3"
                    style="border-color: var(--ds-border);"
                  >
                    <Checkbox
                      checked={group.required || selectedCapabilityGroups.includes(group.key)}
                      disabled={group.required}
                      label={group.label}
                      hint={`${group.tools?.length || 0} approved tool${group.tools?.length === 1 ? '' : 's'}${group.required ? ' · required' : ''}`}
                      onchange={(checked) => toggleCapability(group.key, checked)}
                      dataTestid={group.required
                        ? 'agent-capability-required'
                        : 'agent-capability-optional'}
                    />
                  </div>
                {/each}
              </div>
              <div class="mt-4 flex justify-end">
                <Button
                  variant="primary"
                  icon={Save}
                  loading={savingTools}
                  disabled={!toolsChanged}
                  onclick={saveTools}
                  dataTestid="agent-tools-save"
                >
                  Save tools and access
                </Button>
              </div>
              {#if toolsSaved}
                <AlertBox
                  variant="success"
                  message="Tools and access saved as a new Draft version."
                  class="mt-4"
                />
              {/if}
              {#if actionError}
                <AlertBox variant="error" message={actionError} class="mt-4" />
              {/if}
            {:else}
              <div class="mt-4 space-y-4">
                {#if agent.profile_type === 'legacy'}
                  <AlertBox variant="warning">
                    This grandfathered profile still uses the Legacy local runtime. Its identity,
                    attribution, repositories, sessions, and history are preserved when migrated.
                  </AlertBox>
                {/if}
                <AgentRunnerSetup
                  {workspaceId}
                  pools={runnerPools}
                  bind:selectedPoolId={selectedRunnerPoolId}
                  bind:setupMode={runnerSetupMode}
                  bind:pendingTokenId={pendingRunnerTokenId}
                  bind:pendingTokenPoolId={pendingRunnerTokenPoolId}
                  bind:knownInstanceIds={knownRunnerInstanceIds}
                  poolLocked={runnerAssignmentLocked}
                  onready={runnerReady}
                />
                {#if !runnerAssignmentLocked}
                  <div class="flex justify-end">
                    <Button
                      variant="primary"
                      icon={ShieldCheck}
                      loading={connectingRunner}
                      disabled={!selectedRunnerPoolId}
                      onclick={connectRunner}
                      dataTestid="agent-runner-connect"
                    >
                      {agent.profile_type === 'legacy'
                        ? 'Migrate to authorized runner'
                        : 'Authorize runner pool'}
                    </Button>
                  </div>
                {:else}
                  <AlertBox
                    variant="info"
                    message={`Runner pool #${adminProfile.target_pool_id} is fixed for this profile. Reassignment requires a new profile.`}
                  />
                {/if}
                <p class="text-sm" style="color: var(--ds-text-subtle);">
                  {adminProfile?.repos?.length || 0} configured repository/repositories · Token
                  lifetime {adminProfile?.token_ttl_minutes || 60} minutes
                </p>
                {#if actionError}
                  <AlertBox variant="error" message={actionError} />
                {/if}
              </div>
            {/if}
          </Card>
        {:else}
          <Card variant="outlined" padding="spacious">
            <StateDisplay
              type="empty"
              icon={Lock}
              title="Administrator access required"
              description="Grants, limits, and runtime security are restricted to workspace administrators."
            />
          </Card>
        {/if}
      {:else if activeTab === 'test'}
        {#if canAdmin}
          <Card variant="outlined" padding="spacious" dataTestid="agent-test">
            <div class="flex items-center gap-2">
              <Flask class="h-5 w-5" style="color: var(--ds-icon);" />
              <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Private test</h2>
            </div>
            <p class="mt-3 text-sm" style="color: var(--ds-text-subtle);">
              Testing is optional. Readiness remains based on current structural dependencies and permissions.
            </p>
            {#if agent.profile_type === 'standard'}
              <FormField
                label="Test prompt"
                helper="The response is private, is not added to a conversation, and can use read-only tools only."
                class="mt-4"
              >
                <Textarea
                  bind:value={testPrompt}
                  rows={5}
                  placeholder="Confirm what you can access and summarize how you can help."
                  data-testid="agent-test-prompt"
                />
              </FormField>
            {:else}
              <AlertBox variant="info" class="mt-4">
                The bounded verification checks out the configured repository and calls the selected
                model. It cannot push, comment, or invoke a post-run mutation.
              </AlertBox>
            {/if}
            <div class="mt-4 flex flex-wrap gap-2">
              <Button
                variant="primary"
                icon={Flask}
                loading={testing}
                disabled={Boolean(activeTestRunId)}
                onclick={runPrivateTest}
                dataTestid="agent-test-run"
              >
                {agent.profile_type === 'standard'
                  ? 'Run private test'
                  : 'Run bounded verification'}
              </Button>
              <Button
                variant="default"
                icon={Refresh}
                loading={checking}
                onclick={checkReadiness}
                dataTestid="agent-test-readiness"
              >
                Run dependency check
              </Button>
              {#if activeTestRunId}
                <Button
                  variant="danger"
                  loading={cancellingTest}
                  onclick={cancelTestRun}
                  dataTestid="agent-test-cancel"
                >
                  Cancel verification
                </Button>
              {/if}
            </div>
            {#if testResult}
              <AlertBox variant="success" class="mt-4">
                <div data-testid="agent-test-answer">
                  <p class="font-medium">Private response</p>
                  <p class="mt-2 whitespace-pre-wrap">{testResult.answer}</p>
                  <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">
                    {testResult.iterations} iteration(s) · {testResult.tool_calls} read-only tool call(s)
                  </p>
                </div>
              </AlertBox>
            {/if}
            {#if testRun}
              <AlertBox
                variant={testRun.status === 'succeeded'
                  ? 'success'
                  : testRun.status === 'failed' || testRun.status === 'canceled'
                    ? 'warning'
                    : 'info'}
                class="mt-4"
              >
                <div data-testid="agent-test-run-status">
                  Verification run #{testRun.id} · {label(testRun.status)}
                  {#if testRun.error}
                    <p class="mt-1 text-xs">{testRun.error}</p>
                  {/if}
                </div>
              </AlertBox>
            {/if}
            {#if actionError}
              <AlertBox variant="error" message={actionError} class="mt-4" />
            {/if}
          </Card>
        {:else}
          <Card variant="outlined" padding="spacious">
            <StateDisplay
              type="empty"
              icon={Lock}
              title="Administrator access required"
              description="Private testing is restricted to workspace administrators."
            />
          </Card>
        {/if}
      {:else if activeTab === 'runs'}
        <Card variant="outlined" padding="spacious" dataTestid="agent-runs">
          <div class="flex items-center gap-2">
            <History class="h-5 w-5" style="color: var(--ds-icon);" />
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Recent runs</h2>
          </div>
          {#if profileRuns.length === 0}
            <StateDisplay
              type="empty"
              title="No runs yet"
              description="Activity will appear here after the agent is invoked."
              class="py-10"
            />
          {:else}
            <div class="mt-4 divide-y" style="border-color: var(--ds-border);">
              {#each profileRuns as run (run.id)}
                <div class="flex flex-wrap items-center justify-between gap-3 py-3">
                  <div>
                    <p class="text-sm font-medium" style="color: var(--ds-text);">
                      Run #{run.id}
                    </p>
                    <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
                      {run.item_id ? `Work item #${run.item_id}` : label(run.job_kind)}
                      · Profile version {run.profile_version || agent.profile_version}
                    </p>
                    {#if run.error}
                      <p
                        class="mt-1 text-xs"
                        style="color: var(--ds-text-danger);"
                        data-testid={`agent-run-error-${run.id}`}
                      >
                        {run.error}
                      </p>
                    {/if}
                  </div>
                  <Badge variant={run.status === 'succeeded' ? 'success' : run.status === 'failed' ? 'danger' : 'neutral'}>
                    {label(run.status)}
                  </Badge>
                </div>
              {/each}
            </div>
          {/if}
        </Card>
      {/if}
        </Tabs>
      </div>
    {/if}
  </div>
</section>

<ConfirmDialog
  bind:show={archiveDialogOpen}
  variant="danger"
  title="Archive agent?"
  message="The profile keeps its identity, attribution, sessions, and run history. New work stops and active work is canceled through the normal runtime path."
  confirmText="Archive agent"
  testIdPrefix="agent-archive-dialog"
  onconfirm={archiveProfile}
/>
