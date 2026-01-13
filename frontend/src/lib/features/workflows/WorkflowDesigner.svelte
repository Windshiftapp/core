<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { api } from '../../api.js';
  import { ArrowLeft, Save, X, Plus, Trash2 } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';

  let workflow = null;
  let statuses = [];
  let loading = true;
  let loadingFlow = false;
  let SvelteFlowDesigner = null;

  // Get workflow ID from route params
  $: workflowId = $currentRoute.params?.id;

  onMount(async () => {
    if (workflowId) {
      await Promise.all([loadWorkflowData(), loadSvelteFlowDesigner()]);
    }
  });

  async function loadWorkflowData() {
    try {
      loading = true;
      
      // Load workflow and statuses in parallel
      const [workflowData, statusesData] = await Promise.all([
        api.get(`/workflows/${workflowId}`),
        api.get('/statuses')
      ]);
      
      workflow = workflowData;
      statuses = statusesData || [];
      
    } catch (error) {
      console.error('Failed to load workflow data:', error);
    } finally {
      loading = false;
    }
  }

  async function loadSvelteFlowDesigner() {
    if (SvelteFlowDesigner) return;
    
    try {
      loadingFlow = true;
      const module = await import('./SvelteFlowDesigner.svelte');
      SvelteFlowDesigner = module.default;
    } catch (error) {
      console.error('Failed to load Svelte Flow designer:', error);
      alert('Failed to load the workflow designer. Please refresh the page and try again.');
    } finally {
      loadingFlow = false;
    }
  }

  async function handleSave(allTransitions) {
    try {
      await api.put(`/workflows/${workflow.id}/transitions`, allTransitions);
      navigate('/admin/workflows');
    } catch (error) {
      throw error;
    }
  }

  function handleCancel() {
    navigate('/admin/workflows');
  }

</script>

<div class="min-h-screen workflow-theme">
  <!-- Header -->
  <div class="workflow-header border-b shadow-sm">
    <div class="max-w-full px-6 py-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <Button
            variant="default"
            icon={ArrowLeft}
            onclick={() => navigate('/admin/workflows')}
          >
            Back to Workflows
          </Button>
          <div>
            <h1 class="text-xl font-medium" style="color: var(--ds-text);">
              {workflow?.name || 'Loading...'} - Workflow Designer
            </h1>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              Drag statuses from the left panel, connect them by dragging from connection points
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>

  {#if loading}
    <div class="flex items-center justify-center h-64">
      <div class="animate-pulse loading-message">Loading workflow data...</div>
    </div>
  {:else if loadingFlow}
    <div class="flex items-center justify-center h-64">
      <div class="animate-pulse loading-message">Loading workflow designer...</div>
    </div>
  {:else if SvelteFlowDesigner && workflow && statuses.length > 0}
    <div class="h-[calc(100vh-120px)]">
      <svelte:component 
        this={SvelteFlowDesigner} 
        {workflow} 
        {statuses} 
        onSave={handleSave}
        onCancel={handleCancel}
      />
    </div>
  {:else}
    <div class="flex items-center justify-center h-64">
      <div class="text-center loading-message">
        <p class="text-lg mb-2">Failed to load workflow designer</p>
        <p class="text-sm">Please refresh the page and try again</p>
      </div>
    </div>
  {/if}
</div>

<style>
  .workflow-header {
    background-color: var(--workflow-panel);
    border-color: var(--workflow-border);
  }

  .workflow-theme {
    background-color: var(--workflow-surface);
    color: var(--workflow-text);
  }

  .loading-message {
    color: var(--workflow-text-subtle);
  }
</style>
