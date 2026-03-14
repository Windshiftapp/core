<script>
  import { Lock, Home, ArrowLeft, Info } from 'lucide-svelte';
  import { navigate } from '../router.js';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { requiredPermission = null, message = null, showBackButton = true } = $props();

  let displayMessage = $derived(message || t('errors.INSUFFICIENT_PERMISSION'));
</script>

<div class="min-h-screen flex flex-col justify-center py-12 sm:px-6 lg:px-8" style="background-color: var(--ds-surface);">
  <div class="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
    <Card shadow padding="none" rounded="none" class="py-8 px-4 sm:rounded sm:px-10">
      <div class="text-center">
        <div class="mx-auto h-16 w-16 bg-blue-100 rounded-full flex items-center justify-center mb-4">
          <Lock class="h-8 w-8 text-blue-600" />
        </div>
        <h2 class="text-xl font-medium mb-2" style="color: var(--ds-text);">Access Restricted</h2>
        <p class="mb-6 text-sm" style="color: var(--ds-text-subtle);">{displayMessage}</p>
        
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
              {t('common.back')}
            </Button>
          {/if}

          <Button
            variant="primary"
            fullWidth={true}
            icon={Home}
            onclick={() => navigate('/dashboard')}
          >
            {t('dashboard.title')}
          </Button>
        </div>
        
        <p class="text-xs mt-6" style="color: var(--ds-text-subtlest);">
          If you believe you should have access to this page, please contact your administrator.
        </p>
      </div>
    </Card>
  </div>
</div>