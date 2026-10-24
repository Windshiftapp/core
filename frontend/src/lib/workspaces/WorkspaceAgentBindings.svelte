<script>
  // Workspace agent bindings (WI-88). The page workspace admins use to
  // wire up coding-agent runners: pick an acting identity (an
  // allowlisted centralized service user), point it at a repo via
  // one of the workspace's SCM connections, set token budget knobs.
  // Backend chokepoint re-validates the identity at create time; the
  // candidates endpoint just keeps the picker honest.

  import { onDestroy, onMount, untrack } from 'svelte';
  import Spinner from '../components/Spinner.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import FlaskConical from '@lucide/svelte/icons/flask-conical';
  import Orbit from '@lucide/svelte/icons/orbit';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { workspaceSCM } from '../api/scm.js';
  import { actionCapabilities, llmProviders } from '../api/ai.js';
  import { agentBindings } from '../api/agentBindings.js';
  import { agentRuns } from '../api/agentRuns.js';
  import { agentSkills } from '../api/agentSkills.js';
  import Panel from '../components/Panel.svelte';
  import Button from '../components/Button.svelte';
  import { BasePicker } from '../pickers';
  import UserPicker from '../pickers/UserPicker.svelte';
  import Select from '../components/Select.svelte';
  import Input from '../components/Input.svelte';
  import Radio from '../components/Radio.svelte';
  import Label from '../components/Label.svelte';
  import Textarea from '../components/Textarea.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import SectionHeader from '../layout/SectionHeader.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import ConfirmDialog from '../dialogs/ConfirmDialog.svelte';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
	import TextField from '../components/TextField.svelte';
	import SelectField from '../components/SelectField.svelte';

  // skillsVersion: bumped by the parent when the skills panel below this one
  // creates/edits/deletes a skill, so the attach-pickers here don't go stale.
  let { workspaceId, skillsVersion = 0, oncreatingchange = () => {} } = $props();

  let loading = $state(true);
  let bindings = $state([]);
  let candidates = $state([]);
  let scmConnections = $state([]);
  // Enabled LLM connections (global — not workspace-scoped). Any enabled
  // connection can back any workspace's binding; the binding stores the
  // connection id directly. A binding requires one: a run with no LLM can't
  // reach a model (the llm-proxy 403s a run with no LLM grant).
  let llmConnections = $state([]);
  // Runner pools (action_capabilities of type runner_pool) this workspace may
  // dispatch to. A binding runs on a pool (remote) or the local in-process
  // runtime (null target).
  let runnerPools = $state([]);

  // Create uses an inline, page-sized flow; edit remains a compact modal.
  // editingBinding is null for create, or the binding being edited.
  let showModal = $state(false);
  let editingBinding = $state(null);
  let formActingUserId = $state(null);
  let formTargetPoolId = $state(null); // null = local in-process runtime
  // Custom coding-agent image for this binding's remote (pool) runs; '' = the
  // runner's default windshift-agent image. Only meaningful when a pool is
  // selected (WI-450).
  let formRunnerImage = $state('');
  // A binding may bind multiple repos (WI-449), each under its own SCM
  // connection. Exactly one row is primary (its PR links to the work item).
  // Repo slugs are never typed by hand: each is derived from the repository
  // the admin picks under that row's SCM connection (WI-90), so the backend
  // keeps deriving remote URLs from a trusted SCM connection.
  // Row shape: { connId, repositoryId, repoSlug, repoBaseRef, isPrimary }
  let formRepos = $state([]);
  let formLLMConnectionId = $state(null);
  let formTokenTTLMinutes = $state(60);
  let formMaxRunsPerDay = $state(0);
  let formInstructions = $state('');
  let formSkillIds = $state([]);
  let saving = $state(false);

  // The effective server prompt is intentionally lazy-loaded: it is long and
  // may be runtime-overridden, so it is fetched and shown only on request.
  let standardPromptOpen = $state(false);
  let standardPromptLoading = $state(false);
  let standardPrompt = $state('');
  let standardPromptError = $state('');

  // Workspace skills library (WI-258) for the attach pickers.
  let workspaceSkills = $state([]);

  function skillLabel(skill) {
    if (!skill) return '';
    return skill.enabled ? skill.name : `${skill.name} (disabled)`;
  }

  // Linked repositories cached per SCM connection id, so multiple repo rows on
  // different connections don't refetch or clobber each other (WI-449).
  let linkedReposByConn = $state({}); // connId -> repo[]
  let loadingReposByConn = $state({}); // connId -> bool

  // Delete confirmation dialog.
  let deleteDialogOpen = $state(false);
  let pendingDelete = $state(null); // { id, label }

  // Per-binding "Test run" state, keyed by binding id.
  let testing = $state({}); // id -> bool
  let testResults = $state({}); // id -> { runId?, status?, lines?: string[], error?: string }
  // Cancellation tokens so a new test (or unmount) abandons a stale poll loop.
  const watchTokens = {}; // id -> Symbol
  onDestroy(() => {
    for (const id of Object.keys(watchTokens)) delete watchTokens[id];
    if (showModal && !editingBinding) oncreatingchange(false);
  });

  const TEST_RUN_POLL_MS = 1500;
  const TEST_RUN_TIMEOUT_MS = 5 * 60 * 1000;
  const TEST_RUN_TERMINAL = ['succeeded', 'failed', 'canceled', 'killed'];

  function isTerminalTestStatus(status) {
    return TEST_RUN_TERMINAL.includes(status);
  }

  function testRunStatusLabel(status) {
    switch (status) {
      case 'starting':
        return 'starting…';
      case 'queued':
        return 'queued…';
      case 'running':
        return 'running…';
      case 'succeeded':
        return '✓ succeeded';
      case 'failed':
        return '✗ failed';
      case 'canceled':
        return 'canceled';
      case 'killed':
        return 'killed';
      case 'timeout':
        return 'still running (stopped watching)';
      default:
        return status || '…';
    }
  }

  function testRunStatusColor(status) {
    if (status === 'succeeded') return 'var(--ds-text-success, var(--ds-text))';
    if (status === 'failed' || status === 'killed') return 'var(--ds-text-danger)';
    return 'var(--ds-text-subtle)';
  }

  // Render canonical agent messages and tool calls. Drop streaming content
  // duplicates and lifecycle events, which the status badge already conveys.
  function eventText(ev) {
    if (ev.type === 'lifecycle') return null;
    let payload;
    try {
      payload = JSON.parse(ev.payload_json);
    } catch {
      return ev.payload_json; // non-JSON line, show as-is
    }
    switch (payload.type) {
      case 'message':
        // Canonical final assistant message.
        return typeof payload.text === 'string' ? payload.text : null;
      case 'content':
        // Duplicate streaming delta.
        return null;
      case 'tool_start': {
        if (payload.args?.cmd) return `$ ${payload.args.cmd}`;
        const path = payload.tool === 'read_file' && typeof payload.args?.path === 'string'
          ? payload.args.path.trim()
          : '';
        return `→ ${payload.tool || 'tool'}${path ? ` ${path}` : ''}`;
      }
      case 'tool_done':
        return null; // Large output is implied by the next message.
      case 'error':
        return payload.message ? `⚠ ${payload.message}` : null;
      case 'lifecycle':
      case 'starting':
      case 'session_idle':
        return null; // Status-level, not transcript content.
      default:
        // Surface known text from unknown shapes when available.
        return typeof (payload.text ?? payload.message ?? payload.line) === 'string'
          ? (payload.text ?? payload.message ?? payload.line)
          : null;
    }
  }

  function appendLines(id, events) {
    const fresh = (events || [])
      .map(eventText)
      .filter((t) => typeof t === 'string' && t.trim() !== '');
    if (!fresh.length) return;
    const cur = testResults[id] || {};
    testResults = {
      ...testResults,
      [id]: { ...cur, lines: [...(cur.lines || []), ...fresh].slice(-40) },
    };
  }

  onMount(load);

  // Refresh just the skills list when the sibling skills panel reports a
  // change (initial value is covered by load()). The untrack'd snapshot is
  // intentional: it pins the mount-time version so the effect only fires
  // for changes after mount.
  let lastSkillsVersion = untrack(() => skillsVersion);
  $effect(() => {
    if (skillsVersion === lastSkillsVersion) return;
    lastSkillsVersion = skillsVersion;
    agentSkills
      .listForWorkspace(workspaceId)
      .then((skills) => (workspaceSkills = skills ?? []))
      .catch(() => {});
  });

  async function load() {
    loading = true;
    try {
      const [list, cands, conns, llmConns, pools, skills] = await Promise.all([
        agentBindings.listForWorkspace(workspaceId),
        agentBindings.getCandidates(workspaceId),
        workspaceSCM.getConnections(workspaceId).catch(() => []),
        // Global enabled LLM connections — any authenticated user may list the
        // slim public view. Fall back to an empty list rather than breaking
        // the whole form if it fails.
        llmProviders.getEnabled().catch(() => []),
        // Runner pools this workspace can target (empty if none / not allowed).
        actionCapabilities.getForWorkspace(workspaceId, 'runner_pool').catch(() => []),
        // Skills library for the attach pickers (WI-258).
        agentSkills.listForWorkspace(workspaceId).catch(() => []),
      ]);
      bindings = list ?? [];
      candidates = cands ?? [];
      scmConnections = conns ?? [];
      llmConnections = llmConns ?? [];
      runnerPools = pools ?? [];
      workspaceSkills = skills ?? [];
    } catch (err) {
      console.error('Failed to load agent bindings:', err);
      errorToast(err?.message || 'Failed to load agent bindings');
    } finally {
      loading = false;
    }
  }

  // Shape the candidate service users for UserPicker (searchable combobox):
  // there can be hundreds, so a plain <select> doesn't scale. The endpoint
  // only returns the combined git display name; map it into first_name so
  // the trigger label and the search both work, with email shown beneath.
  let candidateUsers = $derived(
    (candidates || []).map((c) => ({
      id: c.user_id,
      first_name: c.name || c.username || `User #${c.user_id}`,
      last_name: '',
      email: c.email,
      username: c.username,
    }))
  );

  let scmConnectionOptions = $derived([
    { value: null, label: '(none)', disabled: false },
    ...(scmConnections || []).map((c) => ({
      value: c.id,
      label: `${c.provider_name || c.provider_slug || `Connection #${c.id}`}`,
      disabled: false,
    })),
  ]);

  // LLM picker: every enabled connection (global — not workspace-scoped). The
  // binding stores the connection id directly. The connection is required, so
  // there is no "use defaults" option. The default connection is labelled as
  // such (the endpoint already returns it first).
  let llmOptions = $derived.by(() => {
    const opts = [{ value: null, label: 'Select an LLM connection', disabled: true }];
    for (const c of llmConnections || []) {
      const tags = [c.is_default ? 'default' : null, c.model].filter(Boolean).join(' · ');
      opts.push({
        value: c.id,
        label: tags ? `${c.name} (${tags})` : c.name || `Connection #${c.id}`,
        disabled: false,
      });
    }
    return opts;
  });

  // Legacy direct Anthropic rows can still exist on upgraded instances, but
  // the coding-agent broker accepts OpenAI-compatible connections only.
  let selectedLLMIsDirectAnthropic = $derived(
    (llmConnections || []).find((c) => c.id === formLLMConnectionId)?.provider_type === 'anthropic'
  );

  // A bound model without vision can't analyse images attached to work items.
  // The binding is still valid (text-only agents are fine), but the operator
  // should choose it knowingly rather than discover the gap mid-run.
  let selectedLLMNoVision = $derived.by(() => {
    const c = (llmConnections || []).find((x) => x.id === formLLMConnectionId);
    return c != null && c.provider_type !== 'anthropic' && c.supports_vision === false;
  });

  // Repository <select> options for a given repo row's chosen SCM connection.
  // Disabled (with an explanatory placeholder) until a connection is chosen.
  function repoOptionsFor(connId) {
    if (!connId) return [{ value: null, label: 'Select an SCM connection first', disabled: true }];
    if (loadingReposByConn[connId]) return [{ value: null, label: 'Loading repositories…', disabled: true }];
    const repos = linkedReposByConn[connId] || [];
    if (repos.length === 0) return [{ value: null, label: 'No repositories linked to this connection', disabled: true }];
    return [
      { value: null, label: 'Pick a repository', disabled: true },
      ...repos.map((r) => ({
        value: r.id,
        label: r.repository_name || r.repository_url || `Repo #${r.id}`,
        disabled: false,
      })),
    ];
  }

  // Lazily load + cache a connection's linked repositories.
  async function ensureLinkedRepos(connId) {
    if (!connId || linkedReposByConn[connId] || loadingReposByConn[connId]) return;
    loadingReposByConn = { ...loadingReposByConn, [connId]: true };
    try {
      const repos = await workspaceSCM.getLinkedRepos(workspaceId, connId);
      linkedReposByConn = { ...linkedReposByConn, [connId]: repos ?? [] };
    } catch (err) {
      console.error('Failed to load repositories for connection:', err);
      errorToast(err?.message || 'Failed to load repositories');
      linkedReposByConn = { ...linkedReposByConn, [connId]: [] };
    } finally {
      loadingReposByConn = { ...loadingReposByConn, [connId]: false };
    }
  }

  function addRepoRow() {
    const row = { connId: null, repositoryId: null, repoSlug: '', repoBaseRef: '', isPrimary: formRepos.length === 0 };
    formRepos = [...formRepos, row];
  }

  function removeRepoRow(idx) {
    const removed = formRepos[idx];
    formRepos = formRepos.filter((_, i) => i !== idx);
    // If we removed the primary, promote the first remaining row.
    if (removed?.isPrimary && formRepos.length > 0 && !formRepos.some((r) => r.isPrimary)) {
      formRepos[0].isPrimary = true;
      formRepos = [...formRepos];
    }
  }

  function setPrimaryRepo(idx) {
    formRepos = formRepos.map((r, i) => ({ ...r, isPrimary: i === idx }));
  }

  function onRepoRowConnectionChange(idx, connId) {
    // Reset this row's repo selection — the previous repo belonged to a
    // different connection.
    formRepos[idx] = { ...formRepos[idx], connId, repositoryId: null, repoSlug: '', repoBaseRef: '' };
    formRepos = [...formRepos];
    void ensureLinkedRepos(connId);
  }

  function onRepoRowRepositoryChange(idx, repoId) {
    const row = formRepos[idx];
    const repo = (linkedReposByConn[row.connId] || []).find((r) => r.id === repoId);
    // Mirror the linked repo's coordinates into the row the create request
    // posts. The base ref defaults to the repo's default branch but stays
    // editable below.
    formRepos[idx] = {
      ...row,
      repositoryId: repoId,
      repoSlug: repo?.repository_name || '',
      repoBaseRef: repo?.default_branch || '',
    };
    formRepos = [...formRepos];
  }

  // One-line summary of a binding's bound repos for the table / read-only edit
  // view. Marks the primary when more than one repo is bound (WI-449). Falls
  // back to the legacy scalar fields for rows from before the migration.
  function bindingReposLabel(b) {
    const repos = b?.repos || [];
    if (repos.length === 0) {
      return b?.repo_slug ? `${b.repo_slug}${b.repo_base_ref ? ` @ ${b.repo_base_ref}` : ''}` : '—';
    }
    return repos
      .map((r) => {
        const ref = r.repo_base_ref ? ` @ ${r.repo_base_ref}` : '';
        const primary = repos.length > 1 && r.is_primary ? ' (primary)' : '';
        return `${r.repo_slug}${ref}${primary}`;
      })
      .join(', ');
  }

  // Resolve display names for the existing bindings table without an
  // extra fetch — candidates already covers everyone the admin can see.
  let displayActingUser = $derived((userId) => {
    const c = (candidates || []).find((c) => c.user_id === userId);
    return c?.name || c?.username || `User #${userId}`;
  });

  let displaySCMConnection = $derived((connId) => {
    if (!connId) return '—';
    const c = (scmConnections || []).find((c) => c.id === connId);
    return c?.provider_name || c?.provider_slug || `Connection #${connId}`;
  });

  let displayLLMConnection = $derived((connId) => {
    if (!connId) return '—';
    const c = (llmConnections || []).find((c) => c.id === connId);
    if (!c) return `Connection #${connId}`;
    return c.model ? `${c.name} · ${c.model}` : c.name;
  });

  // Where a binding runs: a named pool, or the local in-process runtime.
  let displayTarget = $derived((poolId) => {
    if (!poolId) return 'Local';
    const p = (runnerPools || []).find((p) => p.id === poolId);
    return p?.name || `Pool #${poolId}`;
  });

  // "Run on" options: local runtime (null) + each runner pool this workspace
  // may target.
  let targetPoolOptions = $derived([
    { value: null, label: 'Local (in-process)' },
    ...(runnerPools || []).map((p) => ({ value: p.id, label: `Pool: ${p.name}` })),
  ]);

  // An LLM connection is mandatory in both modes; create additionally needs an
  // acting identity (immutable, so not required on edit).
  let canSubmit = $derived(
    saving ? false : !!formLLMConnectionId && (!!editingBinding || !!formActingUserId)
  );

  function resetForm() {
    formActingUserId = null;
    formTargetPoolId = null;
    formRepos = [];
    formLLMConnectionId = null;
    formTokenTTLMinutes = 60;
    formMaxRunsPerDay = 0;
    formInstructions = '';
    formRunnerImage = '';
    formSkillIds = [];
  }

  function openCreate() {
    editingBinding = null;
    resetForm();
    standardPromptOpen = false;
    showModal = true;
    oncreatingchange(true);
  }

  function openEdit(b) {
    editingBinding = b;
    resetForm();
    // Prime every editable field from the binding. Acting identity + target
    // pool are shown read-only (WI-450), but primed so gating/submit work.
    formActingUserId = b.acting_user_id ?? null;
    formTargetPoolId = b.target_pool_id ?? null;
    formLLMConnectionId = b.llm_connection_id ?? null;
    formTokenTTLMinutes = b.token_ttl_minutes || 60;
    formMaxRunsPerDay = b.max_runs_per_day || 0;
    formInstructions = b.instructions || '';
    formRunnerImage = b.runner_image || '';
    formSkillIds = [...(b.skill_ids || [])];
    formRepos = (b.repos || []).map((r) => ({
      connId: r.scm_connection_id ?? null,
      repositoryId: null, // reconciled from the slug once the connection's repos load
      repoSlug: r.repo_slug || '',
      repoBaseRef: r.repo_base_ref || '',
      isPrimary: !!r.is_primary,
    }));
    showModal = true;
    void reconcileEditRepos();
  }

  // Resolve each primed repo row's repositoryId (for the Repository picker) by
  // matching its slug within the connection's linked repos. The submit payload
  // uses repoSlug regardless, so this is display-only.
  async function reconcileEditRepos() {
    const connIds = [...new Set(formRepos.map((r) => r.connId).filter(Boolean))];
    await Promise.all(connIds.map(ensureLinkedRepos));
    formRepos = formRepos.map((r) => {
      if (r.repositoryId || !r.connId) return r;
      const match = (linkedReposByConn[r.connId] || []).find((x) => x.repository_name === r.repoSlug);
      return match ? { ...r, repositoryId: match.id } : r;
    });
  }

  // The repo rows that resolved to a slug, in the create/update payload shape.
  function buildReposPayload() {
    return formRepos
      .filter((r) => r.repoSlug && r.repoSlug.trim() && r.connId)
      .map((r) => ({
        repo_slug: r.repoSlug.trim(),
        repo_base_ref: r.repoBaseRef.trim(),
        scm_connection_id: r.connId,
        is_primary: !!r.isPrimary,
      }));
  }

  function closeModal() {
    const wasCreating = !editingBinding;
    showModal = false;
    editingBinding = null;
    standardPromptOpen = false;
    resetForm();
    if (wasCreating) oncreatingchange(false);
  }

  async function toggleStandardPrompt() {
    standardPromptOpen = !standardPromptOpen;
    if (!standardPromptOpen || standardPrompt || standardPromptLoading) return;
    standardPromptLoading = true;
    standardPromptError = '';
    try {
      const response = await agentBindings.getStandardPrompt(workspaceId);
      standardPrompt = response?.prompt || '';
      if (!standardPrompt) standardPromptError = 'The standard prompt is not configured on this server.';
    } catch (err) {
      standardPromptError = err?.message || 'Failed to load the standard prompt';
    } finally {
      standardPromptLoading = false;
    }
  }

  async function submitModal() {
    if (!canSubmit) return;
    saving = true;
    try {
      if (editingBinding) {
        // Full edit (WI-450): everything except the acting identity + target
        // pool. runner_image is sent for pool bindings only ('' clears it).
        await agentBindings.update(workspaceId, editingBinding.id, {
          repos: buildReposPayload(),
          llm_connection_id: formLLMConnectionId,
          token_ttl_minutes: formTokenTTLMinutes || 60,
          max_runs_per_day: formMaxRunsPerDay || 0,
          instructions: formInstructions,
          runner_image: formTargetPoolId ? formRunnerImage.trim() : '',
          skill_ids: formSkillIds,
        });
        successToast('Agent binding saved');
      } else {
        const body = {
          acting_user_id: formActingUserId,
          token_ttl_minutes: formTokenTTLMinutes || 60,
          max_runs_per_day: formMaxRunsPerDay || 0,
        };
        if (formTargetPoolId) body.target_pool_id = formTargetPoolId;
        // A custom runner image is only honored on a pool (remote) binding.
        if (formTargetPoolId && formRunnerImage.trim()) body.runner_image = formRunnerImage.trim();
        if (formLLMConnectionId) body.llm_connection_id = formLLMConnectionId;
        const repos = buildReposPayload();
        if (repos.length) body.repos = repos;
        if (formInstructions.trim()) body.instructions = formInstructions.trim();
        if (formSkillIds.length) body.skill_ids = formSkillIds;
        await agentBindings.create(workspaceId, body);
        successToast('Agent binding created');
      }
      closeModal();
      await load();
    } catch (err) {
      errorToast(err?.message || 'Failed to save binding');
      console.error('Failed to save binding:', err);
    } finally {
      saving = false;
    }
  }

  // Provision a real (but ephemeral, item-less) coding-agent container run for
  // the binding and watch it to completion — proving the full chain: the model
  // is reachable, the repo checks out into a worktree, and the agent can read
  // its files. The run can never push a branch or open a PR (server marks it
  // ephemeral); the agent's prompt is read-only.
  async function testBinding(b) {
    const token = Symbol('test-run');
    watchTokens[b.id] = token;
    testing = { ...testing, [b.id]: true };
    testResults = { ...testResults, [b.id]: { status: 'starting', lines: [] } };
    try {
      const { run_id: runId } = await agentBindings.testRun(workspaceId, b.id);
      if (watchTokens[b.id] !== token) return;
      testResults = { ...testResults, [b.id]: { runId, status: 'queued', lines: [] } };

      let afterId = 0;
      const deadline = Date.now() + TEST_RUN_TIMEOUT_MS;
      while (watchTokens[b.id] === token) {
        const events = await agentRuns.listEventsAfter(runId, afterId, 200);
        if (watchTokens[b.id] !== token) return;
        if (events?.length) {
          afterId = events[events.length - 1].id;
          appendLines(b.id, events);
        }

        const run = await agentRuns.get(runId);
        if (watchTokens[b.id] !== token) return;
        const cur = testResults[b.id] || {};
        testResults = { ...testResults, [b.id]: { ...cur, status: run.status, error: run.error || '' } };

        if (isTerminalTestStatus(run.status)) {
          // Final drain so the agent's last output isn't lost to timing.
          const tail = await agentRuns.listEventsAfter(runId, afterId, 200);
          if (watchTokens[b.id] === token && tail?.length) appendLines(b.id, tail);
          break;
        }
        if (Date.now() > deadline) {
          const c = testResults[b.id] || {};
          testResults = {
            ...testResults,
            [b.id]: {
              ...c,
              status: 'timeout',
              error: 'Stopped watching after 5 min — see the Agent runs panel for the rest.',
            },
          };
          break;
        }
        await new Promise((r) => setTimeout(r, TEST_RUN_POLL_MS));
      }
    } catch (err) {
      if (watchTokens[b.id] === token) {
        const cur = testResults[b.id] || {};
        testResults = {
          ...testResults,
          [b.id]: { ...cur, status: cur.status || 'error', error: err?.message || 'Test run failed to start' },
        };
      }
    } finally {
      if (watchTokens[b.id] === token) testing = { ...testing, [b.id]: false };
    }
  }

  function openDeleteDialog(binding) {
    pendingDelete = {
      id: binding.id,
      label: `${displayActingUser(binding.acting_user_id)}${binding.repo_slug ? ` (${binding.repo_slug})` : ''}`,
    };
    deleteDialogOpen = true;
  }

  async function confirmDelete() {
    if (!pendingDelete) return;
    try {
      await agentBindings.remove(workspaceId, pendingDelete.id);
      successToast('Agent binding removed');
      pendingDelete = null;
      await load();
    } catch (err) {
      errorToast(err?.message || 'Failed to remove binding');
      console.error('Failed to remove binding:', err);
    }
  }

  function cancelDelete() {
    pendingDelete = null;
  }
</script>

<!-- Dropdown row for the skill attach-picker in the modal. -->
{#snippet skillOption({ item: skill })}
  <div class="flex flex-col min-w-0">
    <span class="font-medium truncate">{skillLabel(skill)}</span>
    {#if skill.description}
      <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{skill.description}</span>
    {/if}
  </div>
{/snippet}

{#if !showModal || editingBinding}
<Panel padding="spacious">
  <SectionHeader
    title="Agent bindings"
    subtitle="Wire a centralized service user to a repo — assigning a work item to it spawns a coding-agent run."
  >
    {#snippet actions()}
      <Button
        size="sm"
        icon={Plus}
        onclick={openCreate}
        dataTestid="binding-add"
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('agentBindings', 'add'), guard: () => !showModal }}
      >
        Add binding
      </Button>
    {/snippet}
  </SectionHeader>

  {#if loading}
    <StateDisplay type="loading" />
  {:else if bindings.length === 0}
    <div data-testid="binding-empty-state">
      <EmptyState
        icon={Orbit}
        title="No bindings yet"
        description="Add one to enable assignee-driven coding-agent runs in this workspace."
      >
        {#snippet action()}
          <!-- shortcut-guard-exempt: duplicate of the section-header "Add binding" action, which carries the A hotkey -->
          <Button size="sm" icon={Plus} onclick={openCreate}>Add binding</Button>
        {/snippet}
      </EmptyState>
    </div>
  {:else}
    <div class="border rounded-md overflow-hidden" style="border-color: var(--ds-border);">
      <table class="w-full text-sm">
        <thead>
          <tr style="background-color: var(--ds-background-neutral);">
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">Acting identity</th>
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">Kind</th>
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">Runs on</th>
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">Repo</th>
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">SCM</th>
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">LLM</th>
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">Budget</th>
            <th class="px-3 py-2"></th>
          </tr>
        </thead>
        <tbody>
          {#each bindings as b (b.id)}
            <tr class="border-t" style="border-color: var(--ds-border);">
              <td class="px-3 py-2" style="color: var(--ds-text);">{displayActingUser(b.acting_user_id)}</td>
              <td class="px-3 py-2" style="color: var(--ds-text-subtle);">
                {b.acting_user_kind === 'agent' ? 'Owned agent' : 'Centralized service user'}
              </td>
              <td class="px-3 py-2" style="color: var(--ds-text-subtle);">{displayTarget(b.target_pool_id)}</td>
              <td class="px-3 py-2" style="color: var(--ds-text-subtle);">
                {bindingReposLabel(b)}
              </td>
              <td class="px-3 py-2" style="color: var(--ds-text-subtle);">{displaySCMConnection(b.scm_connection_id)}</td>
              <td class="px-3 py-2" style="color: var(--ds-text-subtle);">{displayLLMConnection(b.llm_connection_id)}</td>
              <td class="px-3 py-2" style="color: var(--ds-text-subtle);">
                {b.max_runs_per_day > 0 ? `${b.max_runs_per_day}/day` : 'unlimited'} · token {b.token_ttl_minutes}m
              </td>
              <td class="px-3 py-2 text-right whitespace-nowrap">
                <Button
                  size="sm"
                  variant="ghost"
                  onclick={() => testBinding(b)}
                  disabled={testing[b.id] || !b.llm_connection_id || !b.repo_slug || !!b.target_pool_id}
                  loading={testing[b.id]}
                  icon={FlaskConical}
                  title={!b.llm_connection_id
                    ? 'No LLM connection on this binding to test'
                    : !b.repo_slug
                      ? 'This binding has no repo — a test run needs one to check out'
                      : b.target_pool_id
                        ? 'Test runs execute on the local runtime and are not supported for runner-pool bindings — assign a real work item to verify the pool'
                        : 'Test run: provision a real container, check out the repo, have the agent list its files'}
                />
                <Button
                  size="sm"
                  variant="ghost"
                  onclick={() => openEdit(b)}
                  title="Edit binding"
                  dataTestid="binding-edit-{b.id}"
                >
                  <Pencil class="w-4 h-4" />
                </Button>
                <Button size="sm" variant="danger-ghost" icon={Trash2} onclick={() => openDeleteDialog(b)} title="Remove binding"></Button>
              </td>
            </tr>
            {#if testResults[b.id]}
              <tr style="border-color: var(--ds-border);">
                <td colspan="8" class="px-3 pb-3">
                  {#if testResults[b.id].error && !testResults[b.id].lines?.length}
                    <div
                      class="text-xs rounded p-2"
                      style="background-color: var(--ds-background-danger-subtle, var(--ds-background-neutral)); color: var(--ds-text-danger);"
                    >
                      ✗ {testResults[b.id].error}
                    </div>
                  {:else}
                    <div
                      class="text-xs rounded p-2 space-y-1"
                      style="background-color: var(--ds-background-neutral); color: var(--ds-text);"
                    >
                      <div class="flex items-center gap-2" style="color: var(--ds-text-subtle);">
                        <span>Test run{testResults[b.id].runId ? ` #${testResults[b.id].runId}` : ''}:</span>
                        <span style="color: {testRunStatusColor(testResults[b.id].status)};">
                          {testRunStatusLabel(testResults[b.id].status)}
                        </span>
                        {#if !isTerminalTestStatus(testResults[b.id].status)}
                          <Spinner size="sm" />
                        {/if}
                      </div>
                      {#if testResults[b.id].lines?.length}
                        <!-- XSS: this is agent + repo-derived output (e.g. a
                             file named "<img onerror=...>"). It MUST stay a
                             plain {…} interpolation so Svelte HTML-escapes it.
                             Never switch this to {@html}/markdown without
                             routing through sanitizeHtml (utils/sanitize). -->
                        <pre
                          class="whitespace-pre-wrap break-words rounded p-2 m-0"
                          style="background-color: var(--ds-surface-sunken, var(--ds-background)); color: var(--ds-text); max-height: 12rem; overflow: auto;"
                        >{testResults[b.id].lines.join('\n')}</pre>
                      {/if}
                      {#if testResults[b.id].error}
                        <div style="color: var(--ds-text-danger);">✗ {testResults[b.id].error}</div>
                      {/if}
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</Panel>
{/if}

<!-- Creation is an inline, page-sized flow. Editing retains dialog treatment
     because it is a shorter update to an existing binding. -->
<Modal
  isOpen={showModal}
  inline={!editingBinding}
  onclose={closeModal}
  onSubmit={submitModal}
  submitDisabled={!canSubmit}
  maxWidth="max-w-4xl"
>
  {#snippet children(submitHint)}
    <ModalHeader
      title={editingBinding ? 'Edit binding' : 'Add agent binding'}
      subtitle={editingBinding ? '' : 'Configure how this agent runs and what context it receives.'}
      icon={Orbit}
      showCloseButton={!!editingBinding}
      onclose={closeModal}
    />
    <div class="px-6 py-5" data-testid={editingBinding ? 'binding-modal' : 'binding-create-page'}>
      {#if !editingBinding && candidates.length === 0}
        <AlertBox variant="warning" message="No acting identities are available. Ask a global admin to create a service user (User management → Create user → Service user), enable centralized service users in Security settings, and allowlist it for this workspace." />
      {:else}
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <Label for="binding-acting-user" required={!editingBinding} class="mb-1">Acting identity</Label>
            {#if editingBinding}
              <!-- Identity is fixed at create (WI-450). -->
              <p class="text-sm py-2" style="color: var(--ds-text);" data-testid="binding-acting-user-readonly">{displayActingUser(editingBinding.acting_user_id)}</p>
            {:else}
              <UserPicker
                bind:value={formActingUserId}
                users={candidateUsers}
                placeholder="Pick a service user"
                class="w-full"
              />
            {/if}
          </div>
          <div>
            <Label for="binding-target-pool" class="mb-1">Runs on</Label>
            {#if editingBinding}
              <!-- Routing (local vs pool) is fixed at create; recreate to re-route. -->
              <p class="text-sm py-2" style="color: var(--ds-text);" data-testid="binding-target-pool-readonly">{displayTarget(editingBinding.target_pool_id)}</p>
            {:else}
              <Select id="binding-target-pool" bind:value={formTargetPoolId} options={targetPoolOptions} />
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                {runnerPools.length === 0
                  ? 'No runner pools available — runs use the local in-process runtime.'
                  : 'Local runs the agent on this server; a pool dispatches to a registered remote runner.'}
              </p>
            {/if}
          </div>
          {#if formTargetPoolId}
            <div>
              <TextField
                label={"Custom runner image"}
                id="binding-runner-image"
                dataTestid="binding-runner-image"
                placeholder="ghcr.io/windshiftapp/windshift-agent:latest"
                bind:value={formRunnerImage}
              />
              <p class="text-xs mt-1 text-[var(--ds-text-subtle)]">
                Optional. Container image for this pool's coding-agent runs — e.g. a Node+Chrome image for Playwright e2e. Leave blank for the default agent image.
              </p>
            </div>
          {/if}
          <div>
            <SelectField
              label={"LLM connection"}
              id="binding-llm"
              required
              options={llmOptions}
              bind:value={formLLMConnectionId}
            />
            {#if llmConnections.length === 0}
              <p class="text-xs mt-1" style="color: var(--ds-text-danger);">No enabled LLM connections. Ask a global admin to add one under Admin → AI Connections.</p>
            {:else if selectedLLMIsDirectAnthropic}
              <p class="text-xs mt-1" style="color: var(--ds-text-warning, var(--ds-text-subtle));">Direct Anthropic connections are legacy and not usable by the coding agent. Pick an OpenAI-compatible provider such as OpenRouter.</p>
            {:else if selectedLLMNoVision}
              <p data-testid="binding-no-vision-warning" class="text-xs mt-1" style="color: var(--ds-text-warning, var(--ds-text-subtle));">This model has no vision support, so the agent can't analyse images attached to work items. Fine for text-only work; pick a vision-capable model (or set the connection's vision override) if image analysis matters.</p>
            {/if}
          </div>
          <div>
            <TextField
              label={"Per-run token TTL (minutes)"}
              id="binding-ttl"
              type="number"
              min="5"
              max="1440"
              bind:value={formTokenTTLMinutes}
            />
          </div>
          <!-- Max runs / day is hidden for now but the capability is retained:
               formMaxRunsPerDay is still primed on edit and sent in the payload,
               so existing budgets are preserved and the field can be restored.
          <div>
            <Label for="binding-budget" class="mb-1">Max runs / day (0 = unlimited)</Label>
            <Input id="binding-budget" type="number" min="0" bind:value={formMaxRunsPerDay} />
          </div>
          -->
        </div>
        <!-- Repositories (WI-449): a binding may bind multiple repos so the
             agent checks them all out (e.g. core + core-tests) and opens one
             PR per changed repo. Exactly one is primary. -->
        <div class="mt-5 pt-4 border-t" style="border-color: var(--ds-border);" data-testid="binding-repos-section">
          <Label class="mb-1">Repositories</Label>
          <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
            Repos the agent checks out. The primary repo's PR is the one linked to the work item.
          </p>
          {#if formRepos.length === 0}
            <div class="rounded-lg border border-dashed px-4 py-6 text-center" style="border-color: var(--ds-border);">
              <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">No repositories yet — the agent uses whatever the orchestrator picks.</p>
              <Button variant="secondary" size="small" onclick={addRepoRow} dataTestid="binding-repo-add">+ Add repository</Button>
            </div>
          {:else}
            <div class="rounded-lg border divide-y divide-[var(--ds-border)]" style="border-color: var(--ds-border);">
              {#each formRepos as repo, idx (idx)}
                <div class="group flex flex-wrap items-end gap-2 px-3 py-3" data-testid="binding-repo-row">
              <div class="flex-1 min-w-[180px]">
                {#if idx === 0}<Label for={`binding-repo-conn-${idx}`} class="mb-1">SCM connection</Label>{/if}
                <Select
                  id={`binding-repo-conn-${idx}`}
                  value={repo.connId}
                  onchange={(v) => onRepoRowConnectionChange(idx, v)}
                  options={scmConnectionOptions}
                />
              </div>
              <div class="flex-1 min-w-[180px]">
                {#if idx === 0}<Label for={`binding-repo-sel-${idx}`} class="mb-1">Repository</Label>{/if}
                <Select
                  id={`binding-repo-sel-${idx}`}
                  value={repo.repositoryId}
                  onchange={(v) => onRepoRowRepositoryChange(idx, v)}
                  options={repoOptionsFor(repo.connId)}
                  disabled={!repo.connId || loadingReposByConn[repo.connId]}
                />
              </div>
              <div class="w-28">
                {#if idx === 0}<Label for={`binding-repo-base-${idx}`} class="mb-1">Base ref</Label>{/if}
                <Input id={`binding-repo-base-${idx}`} size="small" value={repo.repoBaseRef} oninput={(e) => { formRepos[idx].repoBaseRef = e.currentTarget.value; }} placeholder="main" />
              </div>
              <label class="flex items-center gap-1 text-xs pb-2" style="color: var(--ds-text-subtle);">
                <Radio
                  name="binding-primary-repo"
                  checked={repo.isPrimary}
                  onchange={() => setPrimaryRepo(idx)}
                  dataTestid="binding-repo-primary"
                />
                Primary
              </label>
              <!-- Trash collapses to zero width when idle so the fields use the
                   row's full width, then slides in from the right on hover/focus. -->
              <div
                class="flex-none overflow-hidden transition-all duration-200 ease-out max-w-0 translate-x-2 opacity-0 group-hover:max-w-12 group-hover:translate-x-0 group-hover:opacity-100 group-focus-within:max-w-12 group-focus-within:translate-x-0 group-focus-within:opacity-100"
              >
                <Button
                  variant="danger-ghost"
                  size="small"
                  icon={Trash2}
                  onclick={() => removeRepoRow(idx)}
                  dataTestid="binding-repo-remove"
                  title="Remove repository"
                  class="pb-2"
                ></Button>
              </div>
            </div>
              {/each}
            </div>
            <div class="mt-2">
              <Button variant="secondary" size="small" onclick={addRepoRow} dataTestid="binding-repo-add">+ Add repository</Button>
            </div>
          {/if}
        </div>
        <!-- Persona + skills (WI-258): appended to the run's standard prompt. -->
        <div class="mt-5 pt-4 border-t" style="border-color: var(--ds-border);">
          <div class="flex flex-wrap items-center justify-between gap-2 mb-1">
            <Label for="binding-instructions">Custom instructions (optional persona — "You are our release manager…")</Label>
            <button
              type="button"
              class="inline-flex items-center gap-1 text-xs font-medium hover:underline focus:outline-none focus:ring-2 rounded"
              style="color: var(--ds-text-link);"
              aria-expanded={standardPromptOpen}
              onclick={toggleStandardPrompt}
              data-testid="binding-standard-prompt-toggle"
            >
              {standardPromptOpen ? 'Hide standard prompt' : 'View standard prompt'}
              <ChevronDown class={`w-3.5 h-3.5 transition-transform ${standardPromptOpen ? 'rotate-180' : ''}`} />
            </button>
          </div>
          <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">
            These instructions are appended to Windshift's standard operational prompt.
          </p>
          {#if standardPromptOpen}
            <div
              class="mb-3 rounded-md border p-3"
              style="border-color: var(--ds-border); background-color: var(--ds-surface-sunken, var(--ds-background-neutral));"
              data-testid="binding-standard-prompt"
            >
              {#if standardPromptLoading}
                <div class="flex items-center gap-2 text-xs" style="color: var(--ds-text-subtle);">
                  <Spinner size="sm" /> Loading standard prompt…
                </div>
              {:else if standardPromptError}
                <p class="text-xs" style="color: var(--ds-text-danger);">{standardPromptError}</p>
              {:else}
                <pre
                  class="m-0 max-h-80 overflow-auto whitespace-pre-wrap break-words text-xs leading-5"
                  style="color: var(--ds-text);"
                  data-testid="binding-standard-prompt-content"
                >{standardPrompt}</pre>
              {/if}
            </div>
          {/if}
          <Textarea
            id="binding-instructions"
            bind:value={formInstructions}
            rows={3}
            size="small"
            placeholder="Appended to the standard agent prompt as the agent's role. The operational rules (commit, comment, no push) stay in place."
          />
        </div>
        {#if workspaceSkills.length > 0}
          <div class="mt-3" data-testid="binding-skills">
            <Label class="mb-1">Skills</Label>
            <BasePicker
              bind:value={formSkillIds}
              items={workspaceSkills}
              multiple={true}
              placeholder="Attach skills…"
              searchFields={['name', 'description']}
              getValue={(s) => s?.id}
              getLabel={skillLabel}
              itemSnippet={skillOption}
            />
          </div>
        {/if}
      {/if}
    </div>
    <DialogFooter
      onCancel={closeModal}
      onConfirm={submitModal}
      confirmLabel={editingBinding ? 'Save changes' : 'Add binding'}
      disabled={!canSubmit}
      loading={saving}
      confirmTestid="binding-save"
      showKeyboardHint={!!editingBinding}
      confirmKeyboardHint={submitHint}
    />
  {/snippet}
</Modal>

<ConfirmDialog
  bind:show={deleteDialogOpen}
  variant="danger"
  title="Remove agent binding?"
  message={`Removing the binding for ${pendingDelete?.label ?? ''} stops the assignee-driven run trigger. In-flight runs continue to completion; future assignments produce no run until you re-create the binding.`}
  confirmText="Remove binding"
  onconfirm={confirmDelete}
  oncancel={cancelDelete}
/>
