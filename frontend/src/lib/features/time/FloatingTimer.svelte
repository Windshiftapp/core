<script>
  import { onMount, onDestroy } from 'svelte';
  import { useTimer } from '../../composables/useTimer.svelte.js';
  import { Clock, Square, Maximize2, Minimize2, ExternalLink } from 'lucide-svelte';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';

  // Initialize timer composable with reactive stores
  const timer = useTimer();
  const { activeTimer, formattedDuration, timerSyncing } = timer;

  let dragging = $state(false);
  let dragOffset = $state({ x: 0, y: 0 });
  let timerElement = $state();
  let position = $state({ x: 20, y: 20 }); // Default position, will be updated on mount
  let collapsed = $state(false);
  let animationFrameId = $state(null);
  let pendingPosition = $state(null);

  // Initialize timer on mount
  onMount(async () => {
    await timer.initialize();
    
    // Load saved position and collapsed state from localStorage
    const savedPosition = localStorage.getItem('windshift-timer-position');
    const savedCollapsed = localStorage.getItem('windshift-timer-collapsed');
    
    if (savedPosition) {
      try {
        const savedPos = JSON.parse(savedPosition);
        // Ensure saved position is within current viewport
        position = {
          x: Math.max(0, Math.min(savedPos.x, window.innerWidth - 300)), // Leave room for timer width
          y: Math.max(0, Math.min(savedPos.y, window.innerHeight - 150))  // Leave room for timer height
        };
      } catch (e) {
        console.warn('Failed to parse saved timer position:', e);
        // Fallback to default position
        position = {
          x: Math.max(20, window.innerWidth - 320),
          y: Math.max(20, window.innerHeight - 200)
        };
      }
    } else {
      // Default to bottom-right corner
      position = {
        x: Math.max(20, window.innerWidth - 320),
        y: Math.max(20, window.innerHeight - 200)
      };
    }
    
    if (savedCollapsed !== null) {
      collapsed = savedCollapsed === 'true';
    }
  });

  function handleMouseDown(e) {
    if (e.target.closest('button')) return; // Don't drag when clicking buttons
    
    dragging = true;
    const rect = timerElement.getBoundingClientRect();
    dragOffset = {
      x: e.clientX - rect.left,
      y: e.clientY - rect.top
    };
    
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    e.preventDefault();
  }

  function handleMouseMove(e) {
    if (!dragging) return;
    
    const newX = e.clientX - dragOffset.x;
    const newY = e.clientY - dragOffset.y;
    
    // Store pending position to avoid multiple calculations per frame
    pendingPosition = { x: newX, y: newY };
    
    // Use requestAnimationFrame to throttle updates
    if (!animationFrameId) {
      animationFrameId = requestAnimationFrame(updatePosition);
    }
  }

  function updatePosition() {
    if (!pendingPosition || !timerElement) {
      animationFrameId = null;
      return;
    }
    
    // Keep within viewport bounds
    const maxX = window.innerWidth - timerElement.offsetWidth;
    const maxY = window.innerHeight - timerElement.offsetHeight;
    
    position = {
      x: Math.max(0, Math.min(pendingPosition.x, maxX)),
      y: Math.max(0, Math.min(pendingPosition.y, maxY))
    };
    
    pendingPosition = null;
    animationFrameId = null;
  }

  function handleMouseUp() {
    dragging = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
    
    // Cancel any pending animation frame
    if (animationFrameId) {
      cancelAnimationFrame(animationFrameId);
      animationFrameId = null;
    }
    
    // Apply any final pending position update
    if (pendingPosition) {
      updatePosition();
    }
    
    // Save position to localStorage
    localStorage.setItem('windshift-timer-position', JSON.stringify(position));
  }

  async function handleStopTimer() {
    try {
      await timer.stop();
    } catch (error) {
      console.error('Failed to stop timer:', error);
      // Error is already handled in the composable
    }
  }

  function toggleCollapsed() {
    collapsed = !collapsed;
    localStorage.setItem('windshift-timer-collapsed', collapsed.toString());
  }

  function navigateToItem() {
    const currentTimer = $activeTimer;
    if (currentTimer && currentTimer.item_id && currentTimer.workspace_id) {
      navigate(`/workspaces/${currentTimer.workspace_id}/items/${currentTimer.item_id}`);
    }
  }

  function getWorkItemKey() {
    const currentTimer = $activeTimer;
    if (currentTimer && currentTimer.workspace_key && currentTimer.item_id) {
      return `${currentTimer.workspace_key}-${currentTimer.item_id}`;
    }
    return null;
  }

  // Cleanup event listeners on destroy
  onDestroy(() => {
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);

    // Cancel any pending animation frame
    if (animationFrameId) {
      cancelAnimationFrame(animationFrameId);
      animationFrameId = null;
    }

    // Cleanup timer intervals
    timer.cleanup();
  });

  // Reactive position style
  const positionStyle = $derived(`left: ${position.x}px; top: ${position.y}px;`);
</script>

{#if $activeTimer}
  <div
    bind:this={timerElement}
    class="fixed z-50 select-none {dragging ? 'cursor-grabbing' : 'cursor-grab transition-all duration-200 ease-in-out'}"
    style={positionStyle}
    onmousedown={handleMouseDown}
    role="button"
    tabindex="0"
    onkeydown={(e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        toggleCollapsed();
      }
    }}
  >
    <div 
      class="bg-gradient-to-r from-blue-500 to-blue-600 rounded-xl border-2 border-blue-400 shadow-xl overflow-hidden transition-all duration-200 backdrop-blur-sm"
      class:collapsed
    >
      <!-- Timer header (always visible) -->
      <div class="flex items-center gap-2 px-3 py-2 min-w-0">
        <Clock class="w-4 h-4 flex-shrink-0 text-white" />
        
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <span class="font-mono text-sm font-bold text-white">
              {$formattedDuration}
            </span>
            {#if $timerSyncing}
              <div class="w-2 h-2 bg-white rounded-full animate-pulse opacity-75"></div>
            {/if}
          </div>
          
          {#if !collapsed && getWorkItemKey()}
            <button
              onclick={(e) => { e.stopPropagation(); navigateToItem(); }}
              class="text-xs font-mono bg-blue-400 bg-opacity-30 text-blue-100 px-1.5 py-0.5 rounded hover:bg-opacity-50 transition-colors flex items-center gap-1"
              title={t('time.timer.goToWorkItem', { title: $activeTimer.item_title || t('items.workItem') })}
              type="button"
            >
              {getWorkItemKey()}
              <ExternalLink class="w-2.5 h-2.5" />
            </button>
          {/if}
        </div>

        <div class="flex items-center gap-1 flex-shrink-0">
          <button
            onclick={(e) => { e.stopPropagation(); toggleCollapsed(); }}
            class="p-1 rounded hover:bg-blue-400 hover:bg-opacity-50 transition-colors text-white"
            title={collapsed ? t('time.timer.expandTimer') : t('time.timer.collapseTimer')}
            type="button"
          >
            {#if collapsed}
              <Maximize2 class="w-3 h-3" />
            {:else}
              <Minimize2 class="w-3 h-3" />
            {/if}
          </button>

          <button
            onclick={(e) => { e.stopPropagation(); handleStopTimer(); }}
            class="p-1 rounded hover:bg-red-500 hover:bg-opacity-80 text-white transition-colors"
            title={t('time.stopTimer')}
            disabled={$timerSyncing}
            type="button"
          >
            <Square class="w-3 h-3" />
          </button>
        </div>
      </div>

      <!-- Expanded content -->
      {#if !collapsed}
        <div class="px-3 py-2 border-t border-blue-400 border-opacity-30 bg-blue-600 bg-opacity-20">
          <div class="space-y-1.5 text-xs">
            {#if $activeTimer.project_name}
              <div class="text-blue-100">
                <span class="font-medium">{t('time.timer.project')}:</span> {$activeTimer.project_name}
                {#if $activeTimer.customer_name}
                  ({$activeTimer.customer_name})
                {/if}
              </div>
            {/if}

            {#if $activeTimer.workspace_name}
              <div class="text-blue-100">
                <span class="font-medium">{t('time.timer.workspace')}:</span> {$activeTimer.workspace_name}
              </div>
            {/if}

            <!-- Remove duplicate task name since it's already shown in header -->
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .collapsed {
    /* Collapsed state styling */
  }
  
  /* Ensure the timer stays on top */
  .z-50 {
    z-index: 50;
  }
  
  /* Prevent text selection when dragging */
  .select-none {
    -webkit-user-select: none;
    -moz-user-select: none;
    -ms-user-select: none;
    user-select: none;
  }
</style>