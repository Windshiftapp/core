<script>
  import { Lock, Home, ArrowLeft, Info } from 'lucide-svelte';
  import { navigate } from '../router.js';
  import Button from '../components/Button.svelte';

  export let requiredPermission = null;
  export let message = 'You do not have permission to access this page.';
  export let showBackButton = true;
</script>

<div class="min-h-screen bg-gray-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8">
  <div class="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
    <div class="bg-white py-8 px-4 shadow-sm sm:rounded sm:px-10 border border-gray-200">
      <div class="text-center">
        <div class="mx-auto h-16 w-16 bg-blue-100 rounded-full flex items-center justify-center mb-4">
          <Lock class="h-8 w-8 text-blue-600" />
        </div>
        <h2 class="text-xl font-medium text-gray-900 mb-2">Access Restricted</h2>
        <p class="text-gray-500 mb-6 text-sm">{message}</p>
        
        {#if requiredPermission}
          <div class="bg-blue-50 border border-blue-200 rounded-md p-3 mb-6">
            <div class="flex items-start">
              <div class="flex-shrink-0">
                <Info class="h-4 w-4 text-blue-500 mt-0.5" />
              </div>
              <div class="ml-3 text-left">
                <h3 class="text-sm font-medium text-blue-900 mb-1">
                  Required Permission
                </h3>
                <div class="text-sm text-blue-700">
                  <p class="mb-1">This page requires the following permission:</p>
                  <code class="bg-blue-100 px-2 py-1 rounded text-xs font-mono">
                    {requiredPermission}
                  </code>
                </div>
              </div>
            </div>
          </div>
        {/if}

        <div class="space-y-2">
          {#if showBackButton}
            <Button
              variant="default"
              fullWidth={true}
              icon={ArrowLeft}
              onclick={() => window.history.back()}
            >
              Go Back
            </Button>
          {/if}

          <Button
            variant="primary"
            fullWidth={true}
            icon={Home}
            onclick={() => navigate('/dashboard')}
          >
            Go to Dashboard
          </Button>
        </div>
        
        <p class="text-xs text-gray-400 mt-6">
          If you believe you should have access to this page, please contact your administrator.
        </p>
      </div>
    </div>
  </div>
</div>