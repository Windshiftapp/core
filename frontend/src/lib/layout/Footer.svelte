<script>
  // Footer component for the entire app
  import { navigate } from '../router.js';
  import { versionLabel } from '../version.js';
  import { t } from '../stores/i18n.svelte.js';
</script>

<footer class="border-t mt-auto" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
  <div class="w-full px-6 py-6">
    <div class="flex flex-col md:flex-row items-center justify-between space-y-4 md:space-y-0">
      <!-- Logo and branding -->
      <div class="flex items-center space-x-4">
        <span class="text-sm" style="color: var(--ds-text-subtle);">
          {t('footer.platformName')}
          <span style="color: var(--ds-text-subtlest);">({versionLabel})</span>
        </span>
      </div>
      
      <!-- Links -->
      <div class="flex items-center space-x-6 text-sm">
        <button
          onclick={() => navigate('/about')}
          class="text-[var(--ds-text-link)] hover:text-[var(--ds-text-link-hovered)] transition-colors cursor-pointer bg-transparent border-none"
        >
          {t('footer.aboutWindshift')}
        </button>
        <a href="https://github.com/Windshiftapp/core/issues" target="_blank" rel="noopener noreferrer" class="text-[var(--ds-text-link)] hover:text-[var(--ds-text-link-hovered)] transition-colors">
          {t('footer.reportProblem')}
        </a>
      </div>
    </div>
  </div>
</footer>
