<script>
  import Toast from '../notifications/Toast.svelte';
  import { XCircle } from 'lucide-svelte';

  // Props - controlled component pattern (no mutation)
  let {
    show = false,
    title = 'Error',
    message = '',
    position = 'bottom-right',
    onClose = () => {}
  } = $props();
</script>

<Toast
  {show}
  {position}
  variant="error"
  showCloseButton={true}
  width="min-w-[300px] max-w-[500px]"
  {onClose}
>
  {#snippet icon()}
    <XCircle class="w-5 h-5 text-red-500" />
  {/snippet}

  {#snippet children()}
    <div class="py-3 pr-2">
      <div class="font-semibold text-sm mb-1" style="color: var(--ds-text);">
        {title}
      </div>
      <div class="text-sm" style="color: var(--ds-text-subtle);">
        {message}
      </div>
    </div>
  {/snippet}
</Toast>
