<script>
  import { navigate } from '../router.js';
  import { Github, Heart, Users, Zap } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import { versionLabel } from '../version.js';
  import { t } from '../stores/i18n.svelte.js';

  let { version = versionLabel } = $props();
</script>

<div class="min-h-screen" style="background-color: var(--ds-surface);">
  <div class="max-w-4xl mx-auto px-6 py-12">

    <!-- Logo and Title -->
    <div class="text-center mb-12">
      <div class="flex justify-center mb-6">
        <img src="/cmicon-2.svg" alt="Windshift" class="h-24 w-24" />
      </div>
      <h1 class="text-4xl font-bold mb-4" style="color: var(--ds-text);">
        {t('about.title')}
      </h1>
      <p class="text-xl" style="color: var(--ds-text-subtle);">
        {t('about.subtitle')}
      </p>
    </div>

    <!-- Main Content -->
    <div class="space-y-12">
      <!-- About Section -->
      <section>
        <h2 class="text-2xl font-semibold mb-6" style="color: var(--ds-text);">{t('about.whatIs')}</h2>
        <div class="prose max-w-none" style="color: var(--ds-text);">
          <p class="text-lg leading-relaxed mb-4">
            {t('about.description')}
          </p>
        </div>
      </section>

      <!-- Features Section -->
      <section>
        <h2 class="text-2xl font-semibold mb-6" style="color: var(--ds-text);">{t('about.keyFeatures')}</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          <div class="p-6 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="flex items-center gap-3 mb-4">
              <div class="p-2 rounded" style="background-color: var(--ds-interactive-subtle);">
                <Users class="w-5 h-5" style="color: var(--ds-interactive);" />
              </div>
              <h3 class="font-semibold" style="color: var(--ds-text);">{t('about.projectManagement')}</h3>
            </div>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('about.projectManagementDesc')}
            </p>
          </div>

          <div class="p-6 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="flex items-center gap-3 mb-4">
              <div class="p-2 rounded" style="background-color: var(--ds-interactive-subtle);">
                <Zap class="w-5 h-5" style="color: var(--ds-interactive);" />
              </div>
              <h3 class="font-semibold" style="color: var(--ds-text);">{t('about.testManagement')}</h3>
            </div>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('about.testManagementDesc')}
            </p>
          </div>

          <div class="p-6 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="flex items-center gap-3 mb-4">
              <div class="p-2 rounded" style="background-color: var(--ds-interactive-subtle);">
                <Heart class="w-5 h-5" style="color: var(--ds-interactive);" />
              </div>
              <h3 class="font-semibold" style="color: var(--ds-text);">{t('about.timeTracking')}</h3>
            </div>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('about.timeTrackingDesc')}
            </p>
          </div>
        </div>
      </section>

      <!-- Technology Stack -->
      <section>
        <h2 class="text-2xl font-semibold mb-6" style="color: var(--ds-text);">{t('about.builtWith')}</h2>
        <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
          <div class="text-center p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="font-semibold mb-1" style="color: var(--ds-text);">{t('about.frontend')}</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">Svelte + Vite</div>
          </div>
          <div class="text-center p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="font-semibold mb-1" style="color: var(--ds-text);">{t('about.backend')}</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">Go + Gorilla Mux</div>
          </div>
          <div class="text-center p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="font-semibold mb-1" style="color: var(--ds-text);">{t('about.database')}</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">SQLite</div>
          </div>
          <div class="text-center p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="font-semibold mb-1" style="color: var(--ds-text);">{t('about.styling')}</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">Tailwind CSS</div>
          </div>
        </div>
      </section>

      <!-- Version Info -->
      <section class="text-center py-8">
        <div class="p-8 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
          <h2 class="text-xl font-semibold mb-4" style="color: var(--ds-text);">{t('about.versionInfo')}</h2>
          <div class="space-y-2">
            <div>
              <span class="font-medium" style="color: var(--ds-text);">{t('about.version')}:</span>
              <span class="ml-2" style="color: var(--ds-text-subtle);">{version}</span>
            </div>
            <div>
              <span class="font-medium" style="color: var(--ds-text);">{t('about.builtBy')}:</span>
              <span class="ml-2" style="color: var(--ds-text-subtle);">♥ {t('about.builtByTeam')}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- Footer Actions -->
      <section class="text-center">
        <div class="flex justify-center gap-4">
          <Button
            onclick={() => window.open('https://github.com/Windshiftapp/core', '_blank')}
            variant="default"
            size="medium"
            icon={Github}
          >
            {t('about.reportIssues')}
          </Button>
          <Button
            onclick={() => navigate('https://windshift.sh/docs')}
            variant="primary"
            size="medium"
          >
            {t('about.getStarted')}
          </Button>
        </div>
      </section>
    </div>
  </div>
</div>
