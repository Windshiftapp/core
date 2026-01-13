<script>
  import { navigate } from '../router.js';
  import Button from '../components/Button.svelte';
  import { Telescope } from 'lucide-svelte';
  
  function goHome() {
    navigate('/');
  }
</script>

<div class="p-6 text-center">
  <div class="max-w-md mx-auto">
    <div class="mb-8">
      <div class="mb-4 text-gray-400 flex justify-center">
        <Telescope class="w-12 h-12" />
      </div>
      <h1 class="text-6xl font-bold text-gray-400 mb-2">404</h1>
      <h2 class="text-2xl font-semibold text-gray-700 mb-2">Page Not Found</h2>
      <p class="text-gray-600">
        Sorry, the page you're looking for doesn't exist or has been moved.
      </p>
    </div>
    
    <div class="space-y-4">
      <div class="flex justify-center">
        <Button
          onclick={goHome}
          variant="primary"
          size="large"
        >
          Take me home
        </Button>
      </div>
      
      <div class="text-sm text-gray-500">
        <p>Or use the navigation on the left to find what you're looking for.</p>
      </div>
    </div>
  </div>
</div>
