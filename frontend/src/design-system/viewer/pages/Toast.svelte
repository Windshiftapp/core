<script>
  import Toast from '../../../lib/Toast.svelte';
  import ErrorToast from '../../../lib/ErrorToast.svelte';
  import ToastContainer from '../../../lib/ToastContainer.svelte';
  import { addToast, successToast, errorToast, warningToast, infoToast, clearToasts } from '../../../lib/stores/toasts.svelte.js';
  import { CircleCheck, XCircle, AlertCircle, Info } from 'lucide-svelte';

  // Demo state
  let showDefault = $state(false);
  let showSuccess = $state(false);
  let showWarning = $state(false);
  let showError = $state(false);

  let showBottomRight = $state(false);
  let showBottomLeft = $state(false);
  let showTopRight = $state(false);
  let showTopLeft = $state(false);
  let showBottomCenter = $state(false);
  let showTopCenter = $state(false);

  let showWithIcon = $state(false);
  let showClickable = $state(false);
  let showNoClose = $state(false);

  let showErrorToast = $state(false);

  // Stacking demo counter
  let stackingCounter = $state(1);

  function addStackingToast() {
    addToast({
      title: `Toast #${stackingCounter}`,
      message: `This is stacked toast number ${stackingCounter}. Dismiss to see the next one.`,
      variant: ['success', 'error', 'warning', 'info'][stackingCounter % 4],
      duration: 0 // No auto-hide for demo
    });
    stackingCounter++;
  }
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Toast</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Toast notifications for displaying brief, non-blocking messages to users.
    Uses a card-style design with a colored left border to indicate message type.
  </p>

  <!-- Stacking Toasts (Recommended) -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Stacking Toasts (Recommended)</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Use ToastContainer with the toast store for production apps. Toasts stack at the top-center
      of the screen. Newest toast appears on top - dismiss it to reveal the next one underneath.
    </p>
    <div class="flex flex-wrap gap-3">
      <button
        class="px-4 py-2 rounded text-sm font-medium text-white transition-colors"
        style="background-color: var(--ds-interactive);"
        onclick={addStackingToast}
      >
        Add Stacking Toast
      </button>
      <button
        class="px-4 py-2 rounded text-sm font-medium transition-colors"
        style="background-color: var(--ds-background-success-bold); color: white;"
        onclick={() => successToast('Operation completed successfully!')}
      >
        Success
      </button>
      <button
        class="px-4 py-2 rounded text-sm font-medium transition-colors"
        style="background-color: var(--ds-background-danger-bold); color: white;"
        onclick={() => errorToast('Something went wrong. Please try again.')}
      >
        Error
      </button>
      <button
        class="px-4 py-2 rounded text-sm font-medium transition-colors"
        style="background-color: var(--ds-background-warning-bold); color: var(--ds-text);"
        onclick={() => warningToast('Please review your changes before saving.')}
      >
        Warning
      </button>
      <button
        class="px-4 py-2 rounded text-sm font-medium transition-colors"
        style="background-color: var(--ds-background-accent-bold); color: white;"
        onclick={() => infoToast('New updates are available.')}
      >
        Info
      </button>
      <button
        class="px-4 py-2 rounded text-sm font-medium transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
        onclick={clearToasts}
      >
        Clear All
      </button>
    </div>
    <div class="mt-4 p-4 rounded" style="background-color: var(--ds-surface-sunken); border: 1px solid var(--ds-border);">
      <p class="text-sm font-medium mb-2" style="color: var(--ds-text);">Usage:</p>
      <pre class="text-xs overflow-x-auto" style="color: var(--ds-text-subtle);"><code>{`// In your app root (e.g., App.svelte):
import ToastContainer from './lib/ToastContainer.svelte';
// Add <ToastContainer /> somewhere in your app

// Anywhere in your app:
import { successToast, errorToast, addToast } from './lib/stores/toasts.svelte.js';

successToast('Changes saved!');
errorToast('Failed to save', 'Error Title');
addToast({ title: 'Custom', message: 'Toast', variant: 'info', duration: 3000 });`}</code></pre>
    </div>
  </section>

  <!-- Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Variants</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Toasts come in four variants with different border colors to indicate message type.
    </p>
    <div class="flex flex-wrap gap-3">
      <button
        class="px-4 py-2 rounded text-sm font-medium transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
        onclick={() => showDefault = true}
      >
        Default
      </button>
      <button
        class="px-4 py-2 rounded text-sm font-medium text-white transition-colors"
        style="background-color: var(--ds-background-success-bold);"
        onclick={() => showSuccess = true}
      >
        Success
      </button>
      <button
        class="px-4 py-2 rounded text-sm font-medium transition-colors"
        style="background-color: var(--ds-background-warning-bold); color: var(--ds-text);"
        onclick={() => showWarning = true}
      >
        Warning
      </button>
      <button
        class="px-4 py-2 rounded text-sm font-medium text-white transition-colors"
        style="background-color: var(--ds-background-danger-bold);"
        onclick={() => showError = true}
      >
        Error
      </button>
    </div>
  </section>

  <!-- Positions -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Positions</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Toasts can be positioned in 6 different locations on the screen.
    </p>
    <div class="grid grid-cols-3 gap-3 max-w-md">
      <button
        class="px-3 py-2 rounded text-xs font-medium transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
        onclick={() => showTopLeft = true}
      >
        Top Left
      </button>
      <button
        class="px-3 py-2 rounded text-xs font-medium transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
        onclick={() => showTopCenter = true}
      >
        Top Center
      </button>
      <button
        class="px-3 py-2 rounded text-xs font-medium transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
        onclick={() => showTopRight = true}
      >
        Top Right
      </button>
      <button
        class="px-3 py-2 rounded text-xs font-medium transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
        onclick={() => showBottomLeft = true}
      >
        Bottom Left
      </button>
      <button
        class="px-3 py-2 rounded text-xs font-medium transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
        onclick={() => showBottomCenter = true}
      >
        Bottom Center
      </button>
      <button
        class="px-3 py-2 rounded text-xs font-medium transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
        onclick={() => showBottomRight = true}
      >
        Bottom Right
      </button>
    </div>
  </section>

  <!-- Features -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Features</h2>
    <div class="space-y-4">
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">With Custom Icon</h3>
        <button
          class="px-4 py-2 rounded text-sm font-medium transition-colors"
          style="background-color: var(--ds-interactive); color: white;"
          onclick={() => showWithIcon = true}
        >
          Show Toast with Icon
        </button>
      </div>

      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Clickable Toast</h3>
        <button
          class="px-4 py-2 rounded text-sm font-medium transition-colors"
          style="background-color: var(--ds-interactive); color: white;"
          onclick={() => showClickable = true}
        >
          Show Clickable Toast
        </button>
      </div>

      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Without Close Button</h3>
        <button
          class="px-4 py-2 rounded text-sm font-medium transition-colors"
          style="background-color: var(--ds-interactive); color: white;"
          onclick={() => showNoClose = true}
        >
          Show Toast (no close button)
        </button>
      </div>
    </div>
  </section>

  <!-- ErrorToast -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">ErrorToast Component</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      A pre-built error toast with title and message fields.
    </p>
    <button
      class="px-4 py-2 rounded text-sm font-medium text-white transition-colors"
      style="background-color: var(--ds-background-danger-bold);"
      onclick={() => showErrorToast = true}
    >
      Show Error Toast
    </button>
  </section>

  <!-- Props Reference - Toast -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Toast Props Reference</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>show</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>duration</code></td>
            <td class="p-2">number (ms)</td>
            <td class="p-2">5000 (0 = no auto-hide)</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>position</code></td>
            <td class="p-2">'bottom-right' | 'bottom-left' | 'top-right' | 'top-left' | 'bottom-center' | 'top-center'</td>
            <td class="p-2">'bottom-right'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>variant</code></td>
            <td class="p-2">'default' | 'success' | 'warning' | 'error'</td>
            <td class="p-2">'default'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>clickable</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showCloseButton</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>width</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'auto'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onClose</code></td>
            <td class="p-2">() => void</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onHide</code></td>
            <td class="p-2">() => void</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onClick</code></td>
            <td class="p-2">() => void</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>children</code></td>
            <td class="p-2">Snippet</td>
            <td class="p-2">-</td>
          </tr>
          <tr>
            <td class="p-2"><code>icon</code></td>
            <td class="p-2">Snippet</td>
            <td class="p-2">-</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Props Reference - ErrorToast -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">ErrorToast Props Reference</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>show</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>title</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Error'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>message</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>position</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'bottom-right'</td>
          </tr>
          <tr>
            <td class="p-2"><code>onClose</code></td>
            <td class="p-2">() => void</td>
            <td class="p-2">-</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>

<!-- Variant Toasts -->
<Toast
  show={showDefault}
  variant="default"
  onHide={() => showDefault = false}
  onClose={() => showDefault = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm" style="color: var(--ds-text);">This is a default toast message.</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showSuccess}
  variant="success"
  onHide={() => showSuccess = false}
  onClose={() => showSuccess = false}
>
  {#snippet icon()}
    <CircleCheck class="w-5 h-5" style="color: var(--ds-icon-success);" />
  {/snippet}
  {#snippet children()}
    <div class="py-3 pr-2">
      <p class="text-sm" style="color: var(--ds-text);">Your changes have been saved.</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showWarning}
  variant="warning"
  onHide={() => showWarning = false}
  onClose={() => showWarning = false}
>
  {#snippet icon()}
    <AlertCircle class="w-5 h-5" style="color: var(--ds-icon-warning);" />
  {/snippet}
  {#snippet children()}
    <div class="py-3 pr-2">
      <p class="text-sm" style="color: var(--ds-text);">Please review before continuing.</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showError}
  variant="error"
  onHide={() => showError = false}
  onClose={() => showError = false}
>
  {#snippet icon()}
    <XCircle class="w-5 h-5" style="color: var(--ds-icon-danger);" />
  {/snippet}
  {#snippet children()}
    <div class="py-3 pr-2">
      <p class="text-sm" style="color: var(--ds-text);">Something went wrong.</p>
    </div>
  {/snippet}
</Toast>

<!-- Position Toasts -->
<Toast
  show={showBottomRight}
  position="bottom-right"
  onHide={() => showBottomRight = false}
  onClose={() => showBottomRight = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm" style="color: var(--ds-text);">Bottom Right</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showBottomLeft}
  position="bottom-left"
  onHide={() => showBottomLeft = false}
  onClose={() => showBottomLeft = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm" style="color: var(--ds-text);">Bottom Left</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showTopRight}
  position="top-right"
  onHide={() => showTopRight = false}
  onClose={() => showTopRight = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm" style="color: var(--ds-text);">Top Right</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showTopLeft}
  position="top-left"
  onHide={() => showTopLeft = false}
  onClose={() => showTopLeft = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm" style="color: var(--ds-text);">Top Left</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showBottomCenter}
  position="bottom-center"
  onHide={() => showBottomCenter = false}
  onClose={() => showBottomCenter = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm" style="color: var(--ds-text);">Bottom Center</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showTopCenter}
  position="top-center"
  onHide={() => showTopCenter = false}
  onClose={() => showTopCenter = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm" style="color: var(--ds-text);">Top Center</p>
    </div>
  {/snippet}
</Toast>

<!-- Feature Toasts -->
<Toast
  show={showWithIcon}
  variant="info"
  onHide={() => showWithIcon = false}
  onClose={() => showWithIcon = false}
>
  {#snippet icon()}
    <Info class="w-5 h-5" style="color: var(--ds-icon-information);" />
  {/snippet}
  {#snippet children()}
    <div class="py-3 pr-2">
      <p class="text-sm" style="color: var(--ds-text);">This toast has a custom icon.</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showClickable}
  clickable={true}
  onClick={() => { showClickable = false; alert('Toast clicked!'); }}
  onHide={() => showClickable = false}
  onClose={() => showClickable = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm font-medium" style="color: var(--ds-text);">Clickable Toast</p>
      <p class="text-sm" style="color: var(--ds-text-subtle);">Click anywhere on this toast.</p>
    </div>
  {/snippet}
</Toast>

<Toast
  show={showNoClose}
  showCloseButton={false}
  duration={3000}
  onHide={() => showNoClose = false}
>
  {#snippet children()}
    <div class="px-4 py-3">
      <p class="text-sm" style="color: var(--ds-text);">This toast has no close button and auto-hides in 3 seconds.</p>
    </div>
  {/snippet}
</Toast>

<!-- ErrorToast -->
<ErrorToast
  show={showErrorToast}
  title="Operation Failed"
  message="Unable to save changes. Please try again later."
  onClose={() => showErrorToast = false}
/>

<!-- Toast Container for stacking demo -->
<ToastContainer />
