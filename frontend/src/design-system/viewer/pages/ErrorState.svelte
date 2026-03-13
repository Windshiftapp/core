<script>
  import ErrorState from '../../../lib/components/ErrorState.svelte';
  import { FileX, WifiOff, Lock, ServerCrash } from 'lucide-svelte';

  function handleRetry() {
    alert('Retry clicked!');
  }
</script>

<div class="space-y-8">
  <div>
    <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">ErrorState</h1>
    <p style="color: var(--ds-text-subtle);">
      Error display with optional retry button.
      Use for failed data loads, connection errors, and error recovery flows.
    </p>
  </div>

  <!-- Basic -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <ErrorState title="Something went wrong" />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ErrorState title="Something went wrong" />`}
    </pre>
  </section>

  <!-- With Message -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Message</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <ErrorState
        title="Failed to load data"
        message="Please check your connection and try again."
      />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ErrorState
  title="Failed to load data"
  message="Please check your connection and try again."
/>`}
    </pre>
  </section>

  <!-- With Retry -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Retry Button</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <ErrorState
        title="Connection error"
        message="Could not reach the server."
        onRetry={handleRetry}
      />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ErrorState
  title="Connection error"
  message="Could not reach the server."
  onRetry={() => loadData()}
/>`}
    </pre>
  </section>

  <!-- Custom Icons -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Icons</h2>
    <div class="grid grid-cols-2 gap-4">
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <ErrorState
          title="Not found"
          message="The requested item could not be found."
          icon={FileX}
        />
      </div>
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <ErrorState
          title="Offline"
          message="No internet connection."
          icon={WifiOff}
          onRetry={handleRetry}
        />
      </div>
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <ErrorState
          title="Access denied"
          message="You don't have permission to view this."
          icon={Lock}
        />
      </div>
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <ErrorState
          title="Server error"
          message="An unexpected error occurred."
          icon={ServerCrash}
          onRetry={handleRetry}
        />
      </div>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ErrorState title="Not found" icon={FileX} />
<ErrorState title="Offline" icon={WifiOff} onRetry={handleRetry} />
<ErrorState title="Access denied" icon={Lock} />
<ErrorState title="Server error" icon={ServerCrash} onRetry={handleRetry} />`}
    </pre>
  </section>

  <!-- Custom Retry Label -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Retry Label</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <ErrorState
        title="Session expired"
        message="Your session has timed out."
        onRetry={handleRetry}
        retryLabel="Refresh Session"
      />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<ErrorState
  title="Session expired"
  message="Your session has timed out."
  onRetry={handleRetry}
  retryLabel="Refresh Session"
/>`}
    </pre>
  </section>

  <!-- Props -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Props</h2>
    <div class="overflow-x-auto">
      <table class="w-full text-sm" style="color: var(--ds-text);">
        <thead>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <th class="text-left py-2 pr-4">Prop</th>
            <th class="text-left py-2 pr-4">Type</th>
            <th class="text-left py-2 pr-4">Default</th>
            <th class="text-left py-2">Description</th>
          </tr>
        </thead>
        <tbody>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">title</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">'Something went wrong'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Error title</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">message</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">''</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Detailed error message</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">onRetry</td>
            <td class="py-2 pr-4 font-mono text-xs">() => void</td>
            <td class="py-2 pr-4 font-mono text-xs">null</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Retry callback (shows button if provided)</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">retryLabel</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">'Try Again'</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Retry button text</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">icon</td>
            <td class="py-2 pr-4 font-mono text-xs">Component</td>
            <td class="py-2 pr-4 font-mono text-xs">AlertCircle</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Lucide icon component</td>
          </tr>
          <tr>
            <td class="py-2 pr-4 font-mono text-xs">class</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">''</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Additional CSS classes</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
