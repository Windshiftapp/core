<script>
  import { navigate } from '../router.js';
  import { Github, Heart, Users, Zap } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import { versionLabel } from '../version.js';

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
        About Windshift
      </h1>
      <p class="text-xl" style="color: var(--ds-text-subtle);">
        A comprehensive work management platform designed for modern teams
      </p>
    </div>

    <!-- Main Content -->
    <div class="space-y-12">
      <!-- About Section -->
      <section>
        <h2 class="text-2xl font-semibold mb-6" style="color: var(--ds-text);">What is Windshift?</h2>
        <div class="prose max-w-none" style="color: var(--ds-text);">
          <p class="text-lg leading-relaxed mb-4">
            Windshift is a powerful work management application that brings together project tracking,
            test management, time tracking, and team collaboration in one unified platform. Built with
            modern technologies and designed for flexibility, it adapts to your team's unique workflow.
          </p>
        </div>
      </section>

      <!-- Features Section -->
      <section>
        <h2 class="text-2xl font-semibold mb-6" style="color: var(--ds-text);">Key Features</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          <div class="p-6 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="flex items-center gap-3 mb-4">
              <div class="p-2 rounded" style="background-color: var(--ds-interactive-subtle);">
                <Users class="w-5 h-5" style="color: var(--ds-interactive);" />
              </div>
              <h3 class="font-semibold" style="color: var(--ds-text);">Project Management</h3>
            </div>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              Organize work items hierarchically with custom fields, workflows, and status tracking.
            </p>
          </div>

          <div class="p-6 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="flex items-center gap-3 mb-4">
              <div class="p-2 rounded" style="background-color: var(--ds-interactive-subtle);">
                <Zap class="w-5 h-5" style="color: var(--ds-interactive);" />
              </div>
              <h3 class="font-semibold" style="color: var(--ds-text);">Test Management</h3>
            </div>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              Create test cases, organize test plans, execute test runs, and track defects efficiently.
            </p>
          </div>

          <div class="p-6 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="flex items-center gap-3 mb-4">
              <div class="p-2 rounded" style="background-color: var(--ds-interactive-subtle);">
                <Heart class="w-5 h-5" style="color: var(--ds-interactive);" />
              </div>
              <h3 class="font-semibold" style="color: var(--ds-text);">Time Tracking</h3>
            </div>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              Track time spent on projects and tasks with detailed reporting and analytics.
            </p>
          </div>
        </div>
      </section>

      <!-- Technology Stack -->
      <section>
        <h2 class="text-2xl font-semibold mb-6" style="color: var(--ds-text);">Built With</h2>
        <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
          <div class="text-center p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="font-semibold mb-1" style="color: var(--ds-text);">Frontend</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">Svelte + Vite</div>
          </div>
          <div class="text-center p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="font-semibold mb-1" style="color: var(--ds-text);">Backend</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">Go + Gorilla Mux</div>
          </div>
          <div class="text-center p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="font-semibold mb-1" style="color: var(--ds-text);">Database</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">SQLite</div>
          </div>
          <div class="text-center p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="font-semibold mb-1" style="color: var(--ds-text);">Styling</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">Tailwind CSS</div>
          </div>
        </div>
      </section>

      <!-- Version Info -->
      <section class="text-center py-8">
        <div class="p-8 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
          <h2 class="text-xl font-semibold mb-4" style="color: var(--ds-text);">Version Information</h2>
          <div class="space-y-2">
            <div>
              <span class="font-medium" style="color: var(--ds-text);">Version:</span>
              <span class="ml-2" style="color: var(--ds-text-subtle);">{version}</span>
            </div>
            <div>
              <span class="font-medium" style="color: var(--ds-text);">Built with:</span>
              <span class="ml-2" style="color: var(--ds-text-subtle);">♥ by the development team</span>
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
            Report Issues
          </Button>
          <Button
            onclick={() => navigate('https://windshift.sh/docs')}
            variant="primary"
            size="medium"
          >
            Get Started
          </Button>
        </div>
      </section>
    </div>
  </div>
</div>
