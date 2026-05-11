<script>
  import { onMount } from 'svelte';
  import Button from '../components/Button.svelte';
  import ItemCard from '../features/items/ItemCard.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { X, Grip } from '@lucide/svelte';
  import { workspaceIconMap } from '../utils/icons.js';
  import { useEventListener } from 'runed';

  let {
    workspaceCount = 0,
    itemCount = 0,
    userName = '',
    canCreateWorkspaces = false,
    accessibleWorkspaces = [],
    ondismiss = () => {}
  } = $props();

  let isDismissed = $state(false);

  const STORAGE_KEY = 'windshift-dashboard-onboarding-dismissed';
  const TOTAL_STEPS = 2;

  // Derived: show onboarding until both workspace and item are created, or user dismisses
  let showOnboarding = $derived(!isDismissed && (
    canCreateWorkspaces
      ? !(workspaceCount > 0 && itemCount > 0)
      : true
  ));

  // Derived: completed steps count (admin path only)
  let completedCount = $derived((workspaceCount > 0 ? 1 : 0) + (itemCount > 0 ? 1 : 0));

  // Derived: progress percentage (admin path only)
  let progressPercent = $derived((completedCount / TOTAL_STEPS) * 100);

  // Derived: determine which step is active (first incomplete step, admin path only)
  let activeStep = $derived(workspaceCount === 0 ? 1 : (itemCount === 0 ? 2 : 0));

  onMount(() => {
    // Check if user has dismissed the onboarding
    const dismissed = localStorage.getItem(STORAGE_KEY);
    isDismissed = dismissed === 'true';
  });

  useEventListener(() => document, 'keydown', handleKeydown);

  // Auto-dismiss when both workspace and item exist (admin path only)
  $effect(() => {
    if (canCreateWorkspaces && workspaceCount > 0 && itemCount > 0 && showOnboarding) {
      dismissOnboarding();
    }
  });

  function handleKeydown(e) {
    // Disable keyboard shortcuts for non-admin path
    if (!canCreateWorkspaces) return;

    // Check if we're in an input field
    const isInInputField = e.target.tagName === 'INPUT' ||
                          e.target.tagName === 'TEXTAREA' ||
                          e.target.contentEditable === 'true' ||
                          e.target.closest('[contenteditable="true"]');

    // Only handle shortcuts if panel is visible and not in input field
    if (!showOnboarding || isInInputField || e.ctrlKey || e.metaKey || e.altKey) {
      return;
    }

    // Handle W key for workspace creation
    if (e.key === 'w' && workspaceCount === 0) {
      e.preventDefault();
      createWorkspace();
    }
    // Handle I key for work item creation
    else if (e.key === 'i' && workspaceCount > 0 && itemCount === 0) {
      e.preventDefault();
      createWorkItem();
    }
  }

  function dismissOnboarding() {
    isDismissed = true;
    localStorage.setItem(STORAGE_KEY, 'true');
    ondismiss();
  }

  function createWorkspace() {
    // Dispatch create modal event with workspace type, skip navigation to stay on onboarding
    window.dispatchEvent(new CustomEvent('show-create-modal', {
      detail: { type: 'workspace', skipNavigate: true }
    }));
  }

  function createWorkItem() {
    // Dispatch create modal event with work-item type
    window.dispatchEvent(new CustomEvent('show-create-modal', {
      detail: { type: 'work-item' }
    }));
  }
</script>

{#if showOnboarding}
  <div class="mb-6 rounded-lg border relative overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <!-- Close button -->
    <button
      onclick={dismissOnboarding}
      class="absolute top-4 right-4 p-2 rounded transition-all hover-bg z-10"
      style="color: var(--ds-text-subtle);"
      title={t('onboarding.dismissOnboarding')}
      aria-label={t('onboarding.dismissOnboarding')}
    >
      <X class="w-4 h-4" />
    </button>

    <div class="px-6 py-6">
      <!-- Header -->
      <div class="flex items-center gap-4 mb-6">
        <div class="flex-shrink-0">
          <img src="/windshift-3.svg" alt="Windshift" class="w-14 h-14" />
        </div>
        <div class="flex-1">
          <h1 class="text-2xl font-semibold mb-1" style="color: var(--ds-text);">
            {t('onboarding.welcomeTo')}, {userName}!
          </h1>
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {#if canCreateWorkspaces}
              {t('onboarding.getStartedMessage')}
            {:else}
              {t('onboarding.getStartedMember')}
            {/if}
          </p>
        </div>
      </div>

      {#if canCreateWorkspaces}
        <!-- Admin path: existing 2-step wizard -->

        <!-- Progress Section -->
        <div class="mb-6 pb-6 border-b" style="border-color: var(--ds-border);">
          <div class="flex items-center justify-between mb-2">
            <span class="text-sm font-medium" style="color: var(--ds-text);">{t('onboarding.progress')}</span>
            <span class="text-sm" style="color: var(--ds-text-subtle);">{completedCount} {t('onboarding.of')} {TOTAL_STEPS} {t('onboarding.completed')}</span>
          </div>
          <div class="w-full rounded-full h-2" style="background-color: var(--ds-surface);">
            <div
              class="h-2 rounded-full transition-all duration-300"
              style="width: {progressPercent}%; background: linear-gradient(90deg, #1388E7 0%, #1AB1BC 100%);"
            ></div>
          </div>
        </div>

        <!-- Step List (vertical) -->
        <div class="space-y-2">
          <!-- Step 1: Create Workspace -->
          <div
            class="rounded py-3 px-4 transition-all"
            class:border-l-6={activeStep === 1}
            class:border-l-sky-600={activeStep === 1}
            style={activeStep === 1 ? 'background-color: rgba(19, 136, 231, 0.05);' : ''}
          >
            {#if workspaceCount > 0}
              <!-- Completed state: strikethrough -->
              <h3 class="text-sm line-through" style="color: var(--ds-text-subtle);">{t('onboarding.createWorkspace')}</h3>
            {:else}
              <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('onboarding.createWorkspace')}</h3>
              {#if activeStep === 1}
                <p class="text-sm mt-1 mb-3" style="color: var(--ds-text-subtle);">
                  {t('onboarding.workspacesHelp')}
                </p>
                <Button
                  variant="primary"
                  size="small"
                  keyboardHint="W"
                  onclick={createWorkspace}
                >
                  {t('onboarding.createWorkspaceBtn')}
                </Button>
              {/if}
            {/if}
          </div>

          <!-- Step 2: Create Work Item -->
          <div
            class="rounded py-3 px-4 transition-all"
            class:border-l-6={activeStep === 2}
            class:border-l-sky-600={activeStep === 2}
            style={activeStep === 2 ? 'background-color: rgba(19, 136, 231, 0.05);' : ''}
          >
            {#if itemCount > 0}
              <!-- Completed state: strikethrough -->
              <h3 class="text-sm line-through" style="color: var(--ds-text-subtle);">{t('onboarding.createFirstWorkItem')}</h3>
            {:else if activeStep === 2}
              <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('onboarding.createFirstWorkItem')}</h3>
              <p class="text-sm mt-1 mb-3" style="color: var(--ds-text-subtle);">
                {t('onboarding.trackTasks')}
              </p>
              <Button
                variant="primary"
                size="small"
                keyboardHint="I"
                onclick={createWorkItem}
              >
                {t('onboarding.createWorkItemBtn')}
              </Button>
            {:else}
              <!-- Pending state -->
              <h3 class="text-sm" style="color: var(--ds-text-subtle);">{t('onboarding.createFirstWorkItem')}</h3>
            {/if}
          </div>
        </div>
      {:else if accessibleWorkspaces.length > 0}
        <!-- Member path with workspaces: show workspace selection -->
        <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
          {t('onboarding.selectWorkspace')}
        </p>
        <div class="space-y-2">
          {#each accessibleWorkspaces as workspace}
            <ItemCard href="/workspaces/{workspace.id}" compact={true}>
              {#snippet children()}
                {@const WsIcon = workspaceIconMap[workspace.icon] || Grip}
                <div class="flex items-center gap-2">
                  <div class="w-6 h-6 rounded flex items-center justify-center flex-shrink-0" style="background-color: {workspace.color || '#3b82f6'};">
                    <WsIcon size={14} color="white" />
                  </div>
                  <div>
                    <h3 class="text-sm" style="color: var(--ds-text);">
                      {workspace.name}
                    </h3>
                    {#if workspace.description}
                      <p class="text-xs" style="color: var(--ds-text-subtle);">
                        {workspace.description}
                      </p>
                    {/if}
                  </div>
                </div>
              {/snippet}
            </ItemCard>
          {/each}
        </div>
      {:else}
        <!-- Member path without workspaces: contact admin message -->
        <div class="text-center py-6">
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {t('onboarding.noWorkspacesAvailable')}
          </p>
        </div>
      {/if}

      <!-- Dismiss Link -->
      <div class="mt-6 pt-4 border-t text-center" style="border-color: var(--ds-border);">
        <button
          onclick={dismissOnboarding}
          class="text-sm transition-colors hover:underline"
          style="color: var(--ds-text-subtle);"
        >
          {t('onboarding.dismissAssistant')}
        </button>
      </div>
    </div>
  </div>
{/if}
