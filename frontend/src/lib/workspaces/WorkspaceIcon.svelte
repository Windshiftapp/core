<script>
  import { Package } from 'lucide-svelte';
  import { workspaceIconMap } from '../utils/icons.js';

  // Props
  let {
    icon = null,        // Icon name string (e.g., "Home", "Rocket")
    color = '#3b82f6',  // Background color
    avatarUrl = null,   // Avatar image URL (takes priority over icon)
    size = 24,          // Container size in pixels
    iconSize = 16,      // Icon size in pixels
    rounded = 'rounded' // Tailwind rounded class (e.g., 'rounded', 'rounded-md', 'rounded-full')
  } = $props();

  // Get the icon component or fallback to Package
  const IconComponent = $derived(workspaceIconMap[icon] || Package);
</script>

{#if avatarUrl}
  <img
    src={avatarUrl}
    alt="Workspace"
    class="{rounded} object-cover flex-shrink-0"
    style="width: {size}px; height: {size}px;"
  />
{:else}
  <div
    class="{rounded} flex items-center justify-center flex-shrink-0"
    style="width: {size}px; height: {size}px; background-color: {color};"
  >
    <svelte:component this={IconComponent} size={iconSize} color="white" />
  </div>
{/if}
