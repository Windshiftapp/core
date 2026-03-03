<script>
  import { X, Calendar, Tag, MessageSquare, List } from 'lucide-svelte';
  import Spinner from '../components/Spinner.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Button from '../components/Button.svelte';
  import { portalStore } from '../stores/portal.svelte.js';
  import { formatDateSimple } from '../utils/dateFormatter.js';
</script>

<!-- My Requests View -->
<div class="space-y-6">
  {#if portalStore.selectedRequest}
    <!-- Request Detail View -->
    <div class="space-y-4">
      <!-- Request Header -->
      <div class="p-6 rounded" style="background-color: var(--ds-surface-card); border: 1px solid var(--ds-border);">
        <div class="flex items-start justify-between mb-4">
          <div class="flex-1">
            <div class="flex items-center gap-2 mb-2">
              <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">
                {portalStore.selectedRequest.workspace_key}-{portalStore.selectedRequest.workspace_item_number}
              </span>
              <span class="px-2 py-0.5 rounded text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">
                {portalStore.selectedRequest.status}
              </span>
            </div>
            <h3 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">
              {portalStore.selectedRequest.title}
            </h3>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {portalStore.selectedRequest.description}
            </p>
          </div>
          <button
            onclick={() => portalStore.closeRequestDetail()}
            class="p-2 rounded hover:bg-black/5 transition-colors"
            style="color: var(--ds-text-subtle);"
          >
            <X class="w-5 h-5" />
          </button>
        </div>
        <div class="flex gap-4 text-sm" style="color: var(--ds-text-subtle);">
          <div class="flex items-center gap-1">
            <Calendar class="w-4 h-4" />
            Created: {formatDateSimple(portalStore.selectedRequest.created_at)}
          </div>
          {#if portalStore.selectedRequest.request_type_name}
            <div class="flex items-center gap-1">
              <Tag class="w-4 h-4" />
              {portalStore.selectedRequest.request_type_name}
            </div>
          {/if}
        </div>
      </div>

      <!-- Comments Section -->
      <div class="p-6 rounded" style="background-color: var(--ds-surface-card); border: 1px solid var(--ds-border);">
        <h4 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Comments</h4>

        {#if portalStore.loadingComments}
          <div class="flex justify-center py-8">
            <Spinner />
          </div>
        {:else}
          <div class="space-y-4 mb-6">
            {#each portalStore.requestComments as comment}
              <div class="p-4 rounded" style="background-color: var(--ds-surface-raised);">
                <div class="flex items-start justify-between mb-2">
                  <div class="font-medium text-sm" style="color: var(--ds-text);">
                    {comment.author_name}
                  </div>
                  <div class="text-xs" style="color: var(--ds-text-subtle);">
                    {new Date(comment.created_at).toLocaleString()}
                  </div>
                </div>
                <p class="text-sm" style="color: var(--ds-text);">{comment.content}</p>
              </div>
            {:else}
              <p class="text-center py-4" style="color: var(--ds-text-subtle);">No comments yet</p>
            {/each}
          </div>

          <!-- Add Comment Form -->
          <div class="border-t pt-4" style="border-color: var(--ds-border);">
            <Textarea
              value={portalStore.newCommentContent}
              oninput={(e) => portalStore.newCommentContent = e.target.value}
              placeholder="Add a comment..."
              rows={3}
            />
            <div class="flex justify-end mt-2">
              <Button
                variant="primary"
                onclick={() => portalStore.addComment()}
                disabled={!portalStore.newCommentContent.trim() || portalStore.addingComment}
                loading={portalStore.addingComment}
              >
                Add Comment
              </Button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  {:else}
    <!-- Requests List -->
    {#if portalStore.loadingRequests}
      <div class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    {:else if portalStore.myRequests.length === 0}
      <div class="text-center py-12">
        <div class="w-16 h-16 rounded-full mx-auto mb-4 flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
          <List class="w-8 h-8" style="color: var(--ds-text-subtle);" />
        </div>
        <h3 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">No Requests Yet</h3>
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          You haven't submitted any requests through this portal yet.
        </p>
      </div>
    {:else}
      <div class="space-y-4">
        {#each portalStore.myRequests as request}
          <button
            onclick={() => portalStore.viewRequest(request)}
            class="w-full p-4 rounded text-left transition-all hover:shadow-md"
            style="background-color: var(--ds-surface-card); border: 1px solid var(--ds-border);"
          >
            <div class="flex items-start justify-between mb-2">
              <div class="flex-1">
                <div class="flex items-center gap-2 mb-1">
                  <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">
                    {request.workspace_key}-{request.workspace_item_number}
                  </span>
                  <span class="px-2 py-0.5 rounded text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">
                    {request.status}
                  </span>
                </div>
                <h4 class="font-semibold mb-1" style="color: var(--ds-text);">
                  {request.title}
                </h4>
                <p class="text-sm line-clamp-2" style="color: var(--ds-text-subtle);">
                  {request.description}
                </p>
              </div>
            </div>
            <div class="flex items-center gap-4 mt-3 text-sm" style="color: var(--ds-text-subtle);">
              <div class="flex items-center gap-1">
                <Calendar class="w-4 h-4" />
                {formatDateSimple(request.created_at)}
              </div>
              {#if request.comment_count > 0}
                <div class="flex items-center gap-1">
                  <MessageSquare class="w-4 h-4" />
                  {request.comment_count} {request.comment_count === 1 ? 'comment' : 'comments'}
                </div>
              {/if}
              {#if request.request_type_name}
                <div class="flex items-center gap-1">
                  <Tag class="w-4 h-4" />
                  {request.request_type_name}
                </div>
              {/if}
            </div>
          </button>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>
