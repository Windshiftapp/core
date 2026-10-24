<script>
  import { onDestroy } from 'svelte';
  import Refresh from '@tabler/icons-svelte-runes/icons/refresh';
  import Server from '@tabler/icons-svelte-runes/icons/server';
  import X from '@tabler/icons-svelte-runes/icons/x';
  import { runnerPools } from '../../api/ai.js';
  import AlertBox from '../../components/AlertBox.svelte';
  import CopyButton from '../../components/CopyButton.svelte';
  import Badge from '../../components/Badge.svelte';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import FormField from '../../components/FormField.svelte';
  import Select from '../../components/Select.svelte';

  let {
    workspaceId,
    pools = [],
    selectedPoolId = $bindable(null),
    setupMode = $bindable('existing'),
    pendingTokenId = $bindable(null),
    pendingTokenPoolId = $bindable(null),
    knownInstanceIds = $bindable([]),
    onready = undefined,
    disabled = false,
    poolLocked = false,
  } = $props();

  let minting = $state(false);
  let cancelling = $state(false);
  let checking = $state(false);
  let error = $state('');
  let installCommand = $state('');
  let plaintextToken = $state('');
  let setupStatus = $state('idle');
  let runnerName = $state('');
  let pollTimer = null;
  let pollAttempt = 0;
  let consumedPolls = 0;

  const modeOptions = [
    { value: 'existing', label: 'Use an existing runner pool' },
    { value: 'new', label: 'Set up a new runner' },
  ];
  const poolOptions = $derived([
    { value: null, label: 'Select an authorized runner pool', disabled: true },
    ...pools.map((pool) => ({ value: pool.id, label: pool.name })),
  ]);
  const selectedPool = $derived(
    pools.find((pool) => String(pool.id) === String(selectedPoolId))
  );
  const isOnboarding = $derived(setupMode !== 'existing');
  const canMint = $derived(
    !disabled && !minting && Number(selectedPoolId) > 0 && isOnboarding
  );

  $effect(() => {
    const tokenId = Number(pendingTokenId);
    const poolId = Number(pendingTokenPoolId);
    const shouldPoll = isOnboarding && tokenId > 0 && poolId > 0;
    stopPolling();
    if (shouldPoll) {
      pollAttempt = 0;
      consumedPolls = 0;
      setupStatus = 'waiting';
      schedulePoll(0);
    }
    return stopPolling;
  });

  onDestroy(stopPolling);

  function stopPolling() {
    if (pollTimer) {
      window.clearTimeout(pollTimer);
      pollTimer = null;
    }
  }

  function schedulePoll(delay) {
    stopPolling();
    pollTimer = window.setTimeout(checkSetup, delay);
  }

  function heartbeatFresh(instance) {
    if (instance?.status !== 'active' || instance?.revoked_at) return false;
    const heartbeat = Date.parse(instance?.last_heartbeat_at || instance?.registered_at || '');
    return Number.isFinite(heartbeat) && Date.now() - heartbeat <= 90_000;
  }

  async function checkSetup() {
    if (!pendingTokenId || !pendingTokenPoolId) return;
    checking = true;
    error = '';
    try {
      const [tokens, instances] = await Promise.all([
        runnerPools.listWorkspaceTokens(workspaceId, pendingTokenPoolId),
        runnerPools.listWorkspaceInstances(workspaceId, pendingTokenPoolId),
      ]);
      const baseline = new Set((knownInstanceIds || []).map(Number));
      const connected = (instances || []).find(
        (instance) => !baseline.has(Number(instance.id)) && heartbeatFresh(instance)
      );
      if (connected) {
        runnerName = connected.name || `Runner #${connected.id}`;
        setupStatus = 'ready';
        pendingTokenId = null;
        pendingTokenPoolId = null;
        knownInstanceIds = [];
        installCommand = '';
        plaintextToken = '';
        stopPolling();
        onready?.(connected);
        return;
      }
      const token = (tokens || []).find(
        (entry) => Number(entry.id) === Number(pendingTokenId)
      );
      if (!token) {
        setupStatus = 'expired';
        stopPolling();
        return;
      }
      if (token.revoked_at) {
        setupStatus = 'consumed';
        // Registration consumes the token before inserting the instance. Give
        // that boundary a few checks before declaring setup incomplete.
        consumedPolls += 1;
        if (consumedPolls <= 3) {
          schedulePoll(2_000 * consumedPolls);
        } else {
          stopPolling();
        }
        return;
      }
      if (token.expires_at && Date.parse(token.expires_at) <= Date.now()) {
        setupStatus = 'expired';
        stopPolling();
        return;
      }
      setupStatus = 'waiting';
      pollAttempt += 1;
      schedulePoll(Math.min(15_000, 2_000 * 2 ** Math.min(pollAttempt, 3)));
    } catch (err) {
      error = err.message || 'Runner status could not be checked.';
      pollAttempt += 1;
      schedulePoll(Math.min(15_000, 2_000 * 2 ** Math.min(pollAttempt, 3)));
    } finally {
      checking = false;
    }
  }

  async function revokePending() {
    const tokenId = Number(pendingTokenId);
    const poolId = Number(pendingTokenPoolId);
    if (tokenId > 0 && poolId > 0) {
      await runnerPools
        .revokeWorkspaceToken(workspaceId, poolId, tokenId)
        .catch(() => {});
    }
    pendingTokenId = null;
    pendingTokenPoolId = null;
    knownInstanceIds = [];
    installCommand = '';
    plaintextToken = '';
    setupStatus = 'idle';
    stopPolling();
  }

  async function changeMode(nextMode) {
    if (nextMode === 'existing' && pendingTokenId) await revokePending();
    setupMode = nextMode;
  }

  async function changePool(nextPoolId) {
    if (pendingTokenId && String(nextPoolId) !== String(pendingTokenPoolId)) {
      await revokePending();
    }
    selectedPoolId = nextPoolId;
  }

  async function mintSetupToken() {
    if (!canMint) return;
    minting = true;
    error = '';
    try {
      if (pendingTokenId) await revokePending();
      const instances = await runnerPools.listWorkspaceInstances(
        workspaceId,
        selectedPoolId
      );
      knownInstanceIds = (instances || []).map((instance) => Number(instance.id));
      const response = await runnerPools.mintWorkspaceToken(
        workspaceId,
        selectedPoolId,
        {
          description: 'Agent Studio · new runner',
          ttl_hours: 720,
        }
      );
      pendingTokenId = response.id;
      pendingTokenPoolId = Number(selectedPoolId);
      plaintextToken = response.token || '';
      installCommand = response.install_command || '';
      setupStatus = 'waiting';
    } catch (err) {
      error = err.message || 'A runner setup token could not be created.';
    } finally {
      minting = false;
    }
  }

  async function cancelSetup() {
    cancelling = true;
    error = '';
    try {
      await revokePending();
    } catch (err) {
      error = err.message || 'Runner setup could not be cancelled.';
    } finally {
      cancelling = false;
    }
  }
</script>

<Card variant="outlined" padding="default" dataTestid="agent-runner-setup">
  <div class="space-y-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold" style="color: var(--ds-text);">Authorized runner</h3>
        <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
          Coding agents execute only in a runner pool already authorized for this workspace.
        </p>
      </div>
      {#if setupStatus === 'ready'}
        <Badge variant="success">Connected</Badge>
      {:else if pendingTokenId}
        <Badge variant="info">Waiting for runner</Badge>
      {/if}
    </div>

    <FormField
      id="agent-runner-mode"
      label="Runner setup"
      helper="Use a connected pool, or generate a one-time command for a new runner host."
    >
      <Select
        id="agent-runner-mode"
        options={modeOptions}
        bind:value={setupMode}
        onchange={changeMode}
        {disabled}
      />
    </FormField>

    <FormField
      id="agent-runner-pool"
      label="Runner pool"
      helper={selectedPool
        ? `Jobs will be restricted to ${selectedPool.name}.`
        : 'A system administrator must authorize at least one runner pool for this workspace.'}
    >
      <Select
        id="agent-runner-pool"
        options={poolOptions}
        bind:value={selectedPoolId}
        onchange={changePool}
        disabled={disabled || poolLocked || pools.length === 0}
      />
    </FormField>

    {#if isOnboarding}
      <AlertBox variant="warning">
        The runner container can access its host Docker socket. Run this only on a machine
        whose container authority you trust. The registration token is single-use, expires
        within 30 days, and is shown only once.
      </AlertBox>

      {#if pendingTokenId && !installCommand}
        <AlertBox variant="info">
          This setup was restored without its plaintext token. Agent Studio never stores that
          secret. Check the target machine, or replace the token to generate a new command.
        </AlertBox>
      {/if}

      {#if installCommand}
        <div class="space-y-2" data-testid="agent-runner-command">
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm font-medium" style="color: var(--ds-text);">
              Run on the runner machine
            </span>
            <CopyButton
              getText={() => installCommand}
              label="Copy command"
              copiedLabel="Copied"
              dataTestid="agent-runner-copy-command"
              onError={() => (error = 'Clipboard access is unavailable. Select and copy the value manually.')}
            />
          </div>
          <code
            class="block max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-md border p-3 text-xs"
            style="background: var(--ds-surface-sunken); color: var(--ds-text); border-color: var(--ds-border);"
          >{installCommand}</code>
        </div>
      {:else if plaintextToken}
        <div class="space-y-2" data-testid="agent-runner-token">
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm font-medium" style="color: var(--ds-text);">
              One-time registration token
            </span>
            <CopyButton
              getText={() => plaintextToken}
              label="Copy token"
              copiedLabel="Copied"
              dataTestid="agent-runner-copy-token"
              onError={() => (error = 'Clipboard access is unavailable. Select and copy the value manually.')}
            />
          </div>
          <code
            class="block overflow-auto break-all rounded-md border p-3 text-xs"
            style="background: var(--ds-surface-sunken); color: var(--ds-text); border-color: var(--ds-border);"
          >{plaintextToken}</code>
        </div>
      {/if}

      {#if setupStatus === 'ready'}
        <AlertBox variant="success">
          <span data-testid="agent-runner-ready">{runnerName || 'The runner'} is online and ready.</span>
        </AlertBox>
      {:else if setupStatus === 'consumed'}
        <AlertBox variant="warning">
          The setup token was consumed, but no new live runner is visible yet. Check the runner
          logs, then generate a replacement if registration did not finish.
        </AlertBox>
      {:else if setupStatus === 'expired'}
        <AlertBox variant="warning">
          The setup token is no longer active. Generate a replacement to continue.
        </AlertBox>
      {:else if pendingTokenId}
        <div
          class="flex items-center gap-2 text-sm"
          style="color: var(--ds-text-subtle);"
          data-testid="agent-runner-waiting"
        >
          <Refresh class={checking ? 'animate-spin' : ''} size={16} />
          Waiting for the runner to register and send a heartbeat…
        </div>
      {/if}

      <AlertBox variant="info">
        If setup reports an unsupported host or version mismatch, keep the token private and
        inspect the runner logs. A system administrator can use Runner pools to upgrade the
        runner image, revoke tokens or instances, and remove the pool.
      </AlertBox>

      {#if error}<AlertBox variant="error" message={error} />{/if}

      <div class="flex flex-wrap gap-2">
        <Button
          variant="primary"
          icon={Server}
          loading={minting}
          disabled={!canMint}
          dataTestid="agent-runner-generate"
          onclick={mintSetupToken}
        >
          {pendingTokenId ? 'Generate replacement' : 'Generate setup command'}
        </Button>
        {#if pendingTokenId}
          <Button
            variant="secondary"
            icon={X}
            loading={cancelling}
            dataTestid="agent-runner-cancel"
            onclick={cancelSetup}
          >
            Cancel setup
          </Button>
        {/if}
      </div>
    {/if}
  </div>
</Card>
