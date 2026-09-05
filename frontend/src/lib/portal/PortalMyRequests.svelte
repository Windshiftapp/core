<script>
  import {
    ArrowLeft,
    Calendar,
    ChevronRight,
    FileText,
    Info,
    List,
    MessageSquare,
    Tag,
  } from '@lucide/svelte';
  import Spinner from '../components/Spinner.svelte';
  import Badge from '../components/Badge.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import SafeMarkdown from '../components/SafeMarkdown.svelte';
  import { portalRequestsStore } from '../stores/portalActivity.svelte.js';
  import { iconMap } from '../stores/portalPresentation.js';
  import { formatDateSimple, formatDateTimeLocale } from '../utils/dateFormatter.js';
</script>

{#if portalRequestsStore.selected}
  {@const request = portalRequestsStore.selected}
  {@const RequestIcon = iconMap[request.request_type_icon] || FileText}

  <div class="request-detail-view">
    <button
      type="button"
      onclick={() => portalRequestsStore.closeDetail()}
      class="inline-flex items-center gap-2 text-sm font-medium mb-7 hover:underline"
      style="color: var(--ds-text-link);"
      data-testid="portal-request-detail-back"
    >
      <ArrowLeft class="w-4 h-4" />
      Back to requests
    </button>

    <div class="request-detail-grid">
      <div class="request-detail-main">
        <header class="request-detail-heading">
          <div class="flex flex-wrap items-center gap-2 mb-4">
            <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">
              {request.workspace_key}-{request.workspace_item_number}
            </span>
            <StatusBadge
              status={{ label: request.status, categoryColor: request.status_category_color }}
              size="md"
            />
          </div>

          <div class="flex items-start gap-4">
            <div
              class="request-detail-icon"
              style="--request-accent: {request.request_type_color || 'var(--ds-interactive)'};"
              aria-hidden="true"
            >
              <RequestIcon class="w-5 h-5" />
            </div>
            <div class="min-w-0">
              <h1
                data-testid="portal-request-detail-title"
                class="text-3xl sm:text-4xl font-semibold tracking-tight leading-tight"
                style="color: var(--ds-text);"
              >
                {request.title}
              </h1>
              {#if request.description}
                <div class="text-base mt-4 leading-relaxed max-w-3xl" style="color: var(--ds-text-subtle);">
                  <SafeMarkdown html={request.description_html} source={request.description} />
                </div>
              {/if}
            </div>
          </div>

          <div class="flex flex-wrap gap-x-5 gap-y-2 mt-6 text-sm" style="color: var(--ds-text-subtle);">
            <div class="flex items-center gap-1.5">
              <Calendar class="w-4 h-4" />
              Created {formatDateSimple(request.created_at)}
            </div>
            {#if request.comment_count > 0}
              <div class="flex items-center gap-1.5">
                <MessageSquare class="w-4 h-4" />
                {request.comment_count} {request.comment_count === 1 ? 'comment' : 'comments'}
              </div>
            {/if}
          </div>
        </header>

        <section class="request-activity">
          <div class="flex items-center justify-between gap-3 mb-5">
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Activity</h2>
            {#if portalRequestsStore.comments.length > 0}
              <Badge variant="neutral" size="sm">{portalRequestsStore.comments.length}</Badge>
            {/if}
          </div>

          {#if portalRequestsStore.loadingComments}
            <div class="flex justify-center py-8">
              <Spinner />
            </div>
          {:else}
            <div class="space-y-3 mb-7">
              {#each portalRequestsStore.comments as comment}
                <article class="request-comment">
                  <div class="request-comment-avatar" aria-hidden="true">
                    {(comment.author_name || '?').slice(0, 1).toUpperCase()}
                  </div>
                  <div class="min-w-0 flex-1">
                    <div class="flex flex-wrap items-center justify-between gap-2 mb-1.5">
                      <div class="font-medium text-sm" style="color: var(--ds-text);">
                        {comment.author_name}
                      </div>
                      <time class="text-xs" style="color: var(--ds-text-subtle);">
                        {formatDateTimeLocale(comment.created_at)}
                      </time>
                    </div>
                    <div class="text-sm leading-relaxed" style="color: var(--ds-text);">
                      <SafeMarkdown html={comment.content_html} source={comment.content} compact={true} />
                    </div>
                  </div>
                </article>
              {:else}
                <div class="request-empty-activity">
                  <MessageSquare class="w-5 h-5" style="color: var(--ds-icon-subtle);" />
                  <p class="text-sm" style="color: var(--ds-text-subtle);">No comments yet.</p>
                </div>
              {/each}
            </div>

            <div class="request-comment-form">
              <label class="block text-sm font-medium mb-2" style="color: var(--ds-text);" for="portal-request-comment">
                Add a comment
              </label>
              <Textarea
                id="portal-request-comment"
                value={portalRequestsStore.newComment}
                oninput={(event) => (portalRequestsStore.newComment = event.target.value)}
                placeholder="Write an update or question…"
                rows={3}
              />
              <div class="flex justify-end mt-3">
                <!-- shortcut-guard-exempt: portal comments are an explicit, form-scoped submit action. -->
                <Button
                  variant="primary"
                  onclick={() => portalRequestsStore.addComment()}
                  disabled={!portalRequestsStore.newComment.trim() || portalRequestsStore.addingComment}
                  loading={portalRequestsStore.addingComment}
                >
                  Add comment
                </Button>
              </div>
            </div>
          {/if}
        </section>
      </div>

      <aside class="request-detail-sidebar" data-testid="portal-request-details">
        <div class="request-details-card">
          <div class="flex items-center gap-2 pb-4 border-b" style="border-color: var(--ds-border);">
            <Info class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
            <h2 class="text-base font-semibold" style="color: var(--ds-text);">Request details</h2>
          </div>

          <dl class="space-y-5 pt-5">
            <div data-testid="portal-request-detail-status">
              <dt class="text-sm mb-1.5" style="color: var(--ds-text-subtle);">Status</dt>
              <dd>
                <StatusBadge
                  status={{ label: request.status, categoryColor: request.status_category_color }}
                  size="md"
                />
              </dd>
            </div>

            <div data-testid="portal-request-detail-type">
              <dt class="text-sm mb-1.5" style="color: var(--ds-text-subtle);">Request type</dt>
              <dd class="text-sm font-semibold" style="color: var(--ds-text);">
                {request.request_type_name || 'Not specified'}
              </dd>
            </div>

            <div data-testid="portal-request-detail-service">
              <dt class="text-sm mb-1.5" style="color: var(--ds-text-subtle);">Service</dt>
              <dd class="text-sm font-semibold" style="color: var(--ds-text);">
                {request.workspace_name || request.workspace_key || 'Not specified'}
              </dd>
            </div>

            <div data-testid="portal-request-detail-priority">
              <dt class="text-sm mb-1.5" style="color: var(--ds-text-subtle);">Priority</dt>
              <dd class="text-sm font-semibold" style="color: {request.priority ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};">
                {request.priority || 'Not specified'}
              </dd>
            </div>

            <div data-testid="portal-request-detail-created">
              <dt class="text-sm mb-1.5" style="color: var(--ds-text-subtle);">Created</dt>
              <dd class="text-sm font-semibold" style="color: var(--ds-text);">
                {formatDateSimple(request.created_at)}
              </dd>
            </div>
          </dl>
        </div>
      </aside>
    </div>
  </div>
{:else}
  <div>
    <PageHeader
      title="My requests"
      subtitle="Track requests and continue conversations with the team."
      count={!portalRequestsStore.loading && portalRequestsStore.requests.length > 0
        ? portalRequestsStore.requests.length
        : null}
    />

    {#if portalRequestsStore.loading}
      <div class="flex justify-center py-12"><Spinner size="lg" /></div>
    {:else if portalRequestsStore.requests.length === 0}
      <div class="max-w-xl py-8 border-t" style="border-color: var(--ds-border);">
        <div class="flex items-start gap-3">
          <List class="w-5 h-5 mt-0.5" style="color: var(--ds-text-subtle);" />
          <div>
            <h2 class="text-base font-medium" style="color: var(--ds-text);">No requests yet</h2>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              Requests submitted through this portal will appear here.
            </p>
          </div>
        </div>
      </div>
    {:else}
      <div class="request-list" data-testid="portal-my-requests-list">
        {#each portalRequestsStore.requests as request}
          {@const RequestIcon = iconMap[request.request_type_icon] || MessageSquare}
          <button
            data-testid="portal-my-requests-row"
            data-request-id={request.id}
            onclick={() => portalRequestsStore.view(request)}
            class="request-list-row group w-full px-4 sm:px-5 py-5 border-b last:border-b-0 text-left transition-colors"
            style="--request-accent: {request.request_type_color || 'var(--ds-interactive)'};{request.status_is_completed ? ' opacity: 0.7;' : ''}"
          >
            <div class="flex items-start gap-4">
              <div class="request-row-icon" aria-hidden="true">
                <RequestIcon class="w-5 h-5" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2 mb-1.5">
                  <span class="text-xs font-mono" style="color: var(--ds-text-subtle);">
                    {request.workspace_key}-{request.workspace_item_number}
                  </span>
                  <StatusBadge status={{ label: request.status, categoryColor: request.status_category_color }} />
                </div>
                <h2 class="font-semibold" style="color: var(--ds-text);">{request.title}</h2>
                {#if request.description}
                  <p class="text-sm mt-1 line-clamp-2" style="color: var(--ds-text-subtle);">
                    {request.description}
                  </p>
                {/if}
                <div class="flex flex-wrap items-center gap-x-4 gap-y-1 mt-3 text-xs" style="color: var(--ds-text-subtle);">
                  <span class="flex items-center gap-1"><Calendar class="w-3.5 h-3.5" />{formatDateSimple(request.created_at)}</span>
                  {#if request.comment_count > 0}
                    <span class="flex items-center gap-1"><MessageSquare class="w-3.5 h-3.5" />{request.comment_count}</span>
                  {/if}
                  {#if request.request_type_name}
                    <span class="flex items-center gap-1"><Tag class="w-3.5 h-3.5" />{request.request_type_name}</span>
                  {/if}
                </div>
              </div>
              <span class="request-row-chevron mt-2 flex-none">
                <ChevronRight class="w-4 h-4" />
              </span>
            </div>
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  .request-list {
    overflow: hidden;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg);
    background-color: var(--ds-surface-card);
    background-image: radial-gradient(
      circle,
      color-mix(in srgb, var(--ds-interactive) 14%, transparent) 1px,
      transparent 1.2px
    );
    background-position: top right;
    background-size: 18px 18px;
    box-shadow: var(--ds-shadow-raised);
  }

  .request-list-row {
    background-color: color-mix(in srgb, var(--ds-surface-card) 96%, transparent);
    border-color: var(--ds-border);
  }

  .request-list-row:hover {
    background-color: color-mix(in srgb, var(--request-accent) 5%, var(--ds-surface-card));
  }

  .request-list-row:focus-visible,
  .request-detail-view button:focus-visible {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: -2px;
  }

  .request-row-icon,
  .request-detail-icon {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    border: 1px solid color-mix(in srgb, var(--request-accent) 20%, var(--ds-border));
    background-color: color-mix(in srgb, var(--request-accent) 12%, var(--ds-surface-card));
    color: var(--request-accent);
  }

  .request-row-icon {
    width: 2.5rem;
    height: 2.5rem;
    border-radius: var(--radius-md);
  }

  .request-detail-icon {
    width: 2.75rem;
    height: 2.75rem;
    margin-top: 0.25rem;
    border-radius: var(--radius-md);
  }

  .request-row-chevron {
    color: var(--ds-text-subtlest);
    transition: color 150ms ease, transform 150ms ease;
  }

  .request-list-row:hover .request-row-chevron {
    color: var(--request-accent);
    transform: translateX(2px);
  }

  .request-detail-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(16rem, 18rem);
    gap: 3rem;
    align-items: start;
  }

  .request-detail-heading {
    padding-bottom: 2rem;
    border-bottom: 1px solid var(--ds-border);
  }

  .request-activity {
    max-width: 48rem;
    padding-top: 2rem;
  }

  .request-detail-sidebar {
    position: sticky;
    top: 1.5rem;
  }

  .request-details-card {
    padding: 1.25rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg);
    background-color: var(--ds-surface-card);
    box-shadow: var(--ds-shadow-raised);
  }

  .request-comment {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    padding: 1rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg);
    background-color: color-mix(in srgb, var(--ds-background-neutral) 52%, var(--ds-surface-card));
  }

  .request-comment-avatar {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border-radius: var(--radius-full);
    background-color: var(--ds-interactive-subtle);
    color: var(--ds-interactive);
    font-size: 0.75rem;
    font-weight: 600;
  }

  .request-empty-activity {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    padding: 1rem;
    border: 1px dashed var(--ds-border-bold);
    border-radius: var(--radius-lg);
    background-color: var(--ds-background-neutral);
  }

  .request-comment-form {
    padding-top: 1.5rem;
    border-top: 1px solid var(--ds-border);
  }

  @media (max-width: 1023px) {
    .request-detail-grid {
      grid-template-columns: minmax(0, 1fr);
      gap: 2rem;
    }

    .request-detail-sidebar {
      position: static;
      order: -1;
    }

    .request-details-card {
      max-width: 48rem;
    }
  }

  @media (max-width: 639px) {
    .request-detail-heading {
      padding-bottom: 1.5rem;
    }

    .request-detail-icon {
      display: none;
    }

    .request-activity {
      padding-top: 1.5rem;
    }

    .request-list-row {
      padding-top: 1rem;
      padding-bottom: 1rem;
    }

    .request-row-icon {
      width: 2.25rem;
      height: 2.25rem;
    }
  }

  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>
