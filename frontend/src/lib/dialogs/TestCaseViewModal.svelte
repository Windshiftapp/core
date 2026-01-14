<script>
  import { createEventDispatcher } from 'svelte';
  import {
    ArrowLeft,
    Edit,
    Play,
    CheckCircle,
    XCircle,
    AlertCircle,
    Clock,
    ListOrdered,
    ClipboardList,
    History
  } from 'lucide-svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import { api } from '../api.js';
  import { formatDateTimeLocale } from '../utils/dateFormatter.js';

  let {
    isOpen = $bindable(false),
    testCaseId = null,
    workspaceId = null,
    embedded = false
  } = $props();

  const dispatch = createEventDispatcher();

  let loading = $state(false);
  let error = $state(null);
  let testCase = $state(null);
  let testSteps = $state([]);
  let executions = $state([]);
  let lastLoadedId = $state(null);
  const workspaceTestsBasePath = $derived.by(() => workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces');

  $effect(() => {
    if (isOpen && testCaseId && testCaseId !== lastLoadedId) {
      loadTestCaseData(testCaseId);
    }
  });

  async function loadTestCaseData(id) {
    if (!id) return;

    const numericId = Number(id);
    if (!Number.isFinite(numericId)) {
      console.warn('Invalid test case ID provided to TestCaseViewModal:', id);
      error = 'Unable to load linked test case.';
      return;
    }

    loading = true;
    error = null;

    try {
      const [caseData, stepsData] = await Promise.all([
        api.tests.testCases.get(workspaceId, numericId),
        api.tests.testCases.steps.getAll(workspaceId, numericId)
      ]);

      let connections = null;
      try {
        connections = await api.tests.testCases.connections(workspaceId, numericId);
      } catch (connErr) {
        if (!(connErr?.message && connErr.message.includes('404'))) {
          throw connErr;
        }
        console.warn('Test case connections unavailable:', connErr);
      }

      testCase = caseData;
      testSteps = Array.isArray(stepsData) ? stepsData : [];
      executions = connections?.executions || [];
      lastLoadedId = numericId;
    } catch (err) {
      console.error('Failed to load test case detail:', err);
      error = err?.message || 'Failed to load test case';
    } finally {
      loading = false;
    }
  }

  function handleClose() {
    isOpen = false;
    dispatch('close');
  }

  function getStatusPillStyle(status) {
    if (!status) {
      return 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);';
    }
    const normalized = status.toLowerCase();
    if (normalized === 'passed') {
      return 'background-color: var(--ds-background-success); color: var(--ds-text-success);';
    }
    if (normalized === 'failed') {
      return 'background-color: var(--ds-background-danger); color: var(--ds-text-danger);';
    }
    if (normalized === 'blocked') {
      return 'background-color: var(--ds-background-warning); color: var(--ds-text-warning);';
    }
    if (normalized === 'in_progress') {
      return 'background-color: var(--ds-background-information); color: var(--ds-text-information);';
    }
    return 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);';
  }

  function getStatusIcon(status) {
    if (!status) return Clock;
    const normalized = status.toLowerCase();
    if (normalized === 'passed') return CheckCircle;
    if (normalized === 'failed') return XCircle;
    if (normalized === 'blocked') return AlertCircle;
    if (normalized === 'in_progress') return Clock;
    return Clock;
  }

  function getStatusIconColor(status) {
    if (!status) return 'var(--ds-icon)';
    const normalized = status.toLowerCase();
    if (normalized === 'passed') return 'var(--ds-text-success)';
    if (normalized === 'failed') return 'var(--ds-text-danger)';
    if (normalized === 'blocked') return 'var(--ds-text-warning)';
    if (normalized === 'in_progress') return 'var(--ds-text-information)';
    return 'var(--ds-icon)';
  }

  // Priority color helper
  function getPriorityColor(priority) {
    const colors = {
      low: '#6B7280',
      medium: '#3B82F6',
      high: '#F59E0B',
      critical: '#EF4444'
    };
    return colors[priority] || colors.medium;
  }

  // Format duration helper
  function formatDuration(seconds) {
    if (!seconds || seconds === 0) return null;
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (hours > 0 && minutes > 0) return `${hours}h ${minutes}m`;
    if (hours > 0) return `${hours}h`;
    return `${minutes}m`;
  }

  // Test case status style helper
  function getTestCaseStatusStyle(status) {
    if (status === 'inactive') {
      return 'background-color: var(--ds-background-neutral); color: var(--ds-text-disabled);';
    }
    if (status === 'draft') {
      return 'background-color: var(--ds-background-warning-bold); color: white;';
    }
    return 'background-color: var(--ds-background-success); color: var(--ds-text-success);';
  }
</script>

{#if embedded}
  <div class="w-full bg-white rounded-2xl shadow-2xl border border-gray-100 max-h-full flex flex-col overflow-hidden">
    <div class="flex-1 overflow-y-auto">
      {@render previewContent()}
    </div>
  </div>
{:else}
  <Modal
    bind:isOpen
    maxWidth="max-w-4xl"
    zIndexClass="z-60"
    on:close={handleClose}
  >
    {@render previewContent()}
  </Modal>
{/if}

{#snippet previewContent()}
  <div class="p-6 space-y-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <p class="text-xs uppercase tracking-wide text-gray-400 mb-1">Test Case Preview</p>
        <h2 class="text-2xl font-semibold" style="color: var(--ds-text);">
          {testCase ? testCase.title : 'Loading...'}
        </h2>
        {#if testCase?.folder_name}
          <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
            Folder: {testCase.folder_name}
          </p>
        {/if}
        <!-- Metadata badges -->
        {#if testCase}
          <div class="flex items-center gap-2 mt-3">
            <!-- Priority badge -->
            <span
              class="inline-flex items-center px-2 py-1 text-xs font-medium rounded text-white capitalize"
              style="background-color: {getPriorityColor(testCase.priority || 'medium')};"
            >
              {testCase.priority || 'medium'} priority
            </span>
            <!-- Status badge -->
            <span
              class="inline-flex items-center px-2 py-1 text-xs font-medium rounded capitalize"
              style={getTestCaseStatusStyle(testCase.status || 'active')}
            >
              {testCase.status || 'active'}
            </span>
            <!-- Duration badge -->
            {#if formatDuration(testCase.estimated_duration)}
              <span
                class="inline-flex items-center gap-1 px-2 py-1 text-xs font-medium rounded"
                style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
              >
                <Clock class="w-3 h-3" />
                {formatDuration(testCase.estimated_duration)}
              </span>
            {/if}
          </div>
        {/if}
      </div>
      {#if embedded}
        <button
          class="inline-flex items-center gap-2 rounded border border-gray-200 px-3 py-2 text-sm font-medium text-gray-600 hover-bg transition"
          onclick={handleClose}
        >
          <ArrowLeft class="w-4 h-4" />
          Back to Item
        </button>
      {/if}
    </div>

    {#if loading}
      <div class="flex items-center justify-center py-10">
        <div
          class="h-12 w-12 animate-spin rounded-full border-2 border-t-transparent"
          style="border-color: var(--ds-interactive); border-top-color: transparent;"
        ></div>
      </div>
    {:else if error}
      <div
        class="rounded border p-4"
        style="border-color: var(--ds-border-danger); background-color: var(--ds-background-danger); color: var(--ds-text-danger);"
      >
        {error}
      </div>
    {:else if testCase}
      <div class="space-y-6">
        <!-- Action Buttons -->
        <div class="flex flex-wrap gap-3">
          <Button
            variant="primary"
            icon={Edit}
            size="medium"
            onclick={() => window.location.href = `${workspaceTestsBasePath}/cases/${testCase.id}/steps`}
          >
            Edit Test Steps
          </Button>
          <Button
            variant="default"
            icon={Play}
            size="medium"
            onclick={() => window.location.href = `${workspaceTestsBasePath}/runs`}
          >
            View Test Runs
          </Button>
        </div>

        {#if testCase.preconditions}
          <div
            class="rounded-2xl border shadow-sm p-5"
            style="border-color: var(--ds-border); background-color: var(--ds-background-neutral); color: var(--ds-text);"
          >
            <p
              class="text-xs font-semibold uppercase tracking-widest mb-2"
              style="color: var(--ds-interactive);"
            >
              Preconditions
            </p>
            <p class="text-sm" style="color: var(--ds-text);">
              {testCase.preconditions}
            </p>
          </div>
        {/if}

        <!-- Test Steps Section -->
        <div
          class="rounded-2xl border shadow-sm overflow-hidden"
          style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
        >
          <div class="flex items-center justify-between px-6 py-4 border-b" style="border-color: var(--ds-border);">
            <h2 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
              <ListOrdered class="w-5 h-5" style="color: var(--ds-interactive);" />
              Test Steps ({testSteps.length})
            </h2>
          </div>
          <div class="p-6">
            {#if testSteps.length === 0}
              <div
                class="rounded-xl border border-dashed text-center space-y-4 p-8"
                style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);"
              >
                <div
                  class="w-16 h-16 mx-auto flex items-center justify-center rounded-full"
                  style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-interactive);"
                >
                  <ClipboardList class="w-7 h-7" />
                </div>
                <div>
                  <div class="text-lg font-medium mb-1" style="color: var(--ds-text);">
                    No Steps Defined
                  </div>
                  <div class="text-sm mb-4" style="color: var(--ds-text-subtle);">
                    This test case doesn't have any steps yet.
                  </div>
                  <Button
                    variant="primary"
                    icon={Edit}
                    size="medium"
                    onclick={() => window.location.href = `${workspaceTestsBasePath}/cases/${testCase.id}/steps`}
                  >
                    Add Steps
                  </Button>
                </div>
              </div>
            {:else}
              <div class="space-y-4">
                {#each testSteps as step}
                  <div
                    class="rounded-xl border shadow-sm p-4"
                    style="border-color: var(--ds-border); background-color: var(--ds-surface);"
                  >
                    <div class="flex items-start gap-4">
                      <div
                        class="flex-shrink-0 w-10 h-10 rounded-full text-white font-semibold text-base flex items-center justify-center"
                        style="background-color: var(--ds-interactive);"
                      >
                        {step.step_number}
                      </div>
                      <div class="flex-1 space-y-4">
                        <div>
                          <p class="text-xs font-semibold uppercase tracking-wider mb-1" style="color: var(--ds-interactive);">
                            Action
                          </p>
                          <p class="text-sm" style="color: var(--ds-text);">
                            {step.action || '—'}
                          </p>
                        </div>

                        {#if step.data}
                          <div>
                            <p
                              class="text-xs font-semibold uppercase tracking-wider mb-1"
                              style="color: var(--ds-icon-accent-purple);"
                            >
                              Data
                            </p>
                            <p class="text-sm" style="color: var(--ds-text);">
                              {step.data}
                            </p>
                          </div>
                        {/if}

                        <div>
                          <p
                            class="text-xs font-semibold uppercase tracking-wider mb-1"
                            style="color: var(--ds-icon-accent-green);"
                          >
                            Expected Result
                          </p>
                          <p class="text-sm" style="color: var(--ds-text);">
                            {step.expected || '—'}
                          </p>
                        </div>
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>

        <!-- Recent Executions Section -->
        <div
          class="rounded-2xl border shadow-sm overflow-hidden"
          style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
        >
          <div class="flex items-center justify-between px-6 py-4 border-b" style="border-color: var(--ds-border);">
            <h2 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
              <Play class="w-5 h-5" style="color: var(--ds-icon-accent-green);" />
              Recent Executions ({executions.length})
            </h2>
          </div>
          <div class="p-6">
            {#if executions.length === 0}
              <div
                class="rounded-xl border border-dashed text-center space-y-3 p-6"
                style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);"
              >
                <div
                  class="w-14 h-14 mx-auto flex items-center justify-center rounded-full"
                  style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-text-information);"
                >
                  <History class="w-6 h-6" />
                </div>
                <div class="text-sm" style="color: var(--ds-text-subtle);">
                  This test case hasn't been executed in any recent test runs.
                </div>
              </div>
            {:else}
              <div class="space-y-3">
                {#each executions as execution}
                  <a
                    href={`${workspaceTestsBasePath}/runs/${execution.run_id}`}
                    class="flex items-start gap-3 rounded-xl border p-4 transition hover:shadow-sm"
                    style="border-color: var(--ds-border); background-color: var(--ds-surface);"
                  >
                    <div class="flex-shrink-0 mt-0.5">
                      <svelte:component
                        this={getStatusIcon(execution.status)}
                        class="w-5 h-5"
                        style={`color: ${getStatusIconColor(execution.status)};`}
                      />
                    </div>
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center justify-between gap-3 mb-1">
                        <div class="font-semibold text-sm truncate" style="color: var(--ds-text);">
                          {execution.run_name}
                        </div>
                        <span
                          class="text-xs px-2 py-1 rounded-full font-semibold whitespace-nowrap"
                          style={getStatusPillStyle(execution.status)}
                        >
                          {execution.status || 'not_run'}
                        </span>
                      </div>
                      <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs" style="color: var(--ds-text-subtle);">
                        <span class="flex items-center gap-1">
                          <Clock class="w-3.5 h-3.5" />
                          {formatDateTimeLocale(execution.started_at) || '—'}
                        </span>
                        {#if execution.set_name}
                          <span>• Set: {execution.set_name}</span>
                        {/if}
                        {#if execution.template_name}
                          <span>• Template: {execution.template_name}</span>
                        {/if}
                      </div>
                    </div>
                  </a>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </div>
{/snippet}
