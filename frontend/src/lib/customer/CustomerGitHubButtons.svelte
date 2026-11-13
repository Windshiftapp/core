<script>
  import Github from '@tabler/icons-svelte-runes/icons/brand-github';
  import { toExternal } from '../runtime/contextPath.js';
  let { kind, slug, providers = [] } = $props();
</script>

{#each providers as provider (provider.slug)}
  <!-- inline-style-guard-exempt: GitHub brand colors match the backend GitHub login button. -->
  <a
    style="background: #24292f; color: white;"
    data-testid={`customer-github-${provider.slug}`}
    href={toExternal(`/api/sso/login/${encodeURIComponent(provider.slug)}?${new URLSearchParams({ resource_kind: kind, resource_slug: slug })}`)}
    onclick={event => { event.preventDefault(); window.location.assign(event.currentTarget.href); }}
    class="flex items-center justify-center gap-2 w-full rounded-md px-4 py-2.5 text-sm font-medium"
  ><Github size={18} />Continue with {provider.name}</a>
{/each}
