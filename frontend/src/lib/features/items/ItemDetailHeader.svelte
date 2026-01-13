<script>
  import { Check, X } from "lucide-svelte";
  import Button from "../../components/Button.svelte";
  import { errorToast } from '../../stores/toasts.svelte.js';

  export let item;
  export let workspace;
  export let editingTitle = false;
  export let editTitle = "";
  export let saving = false;

  // Event dispatchers
  import { createEventDispatcher } from "svelte";
  const dispatch = createEventDispatcher();


  function startEditingTitle() {
    editTitle = item.title;
    editingTitle = true;
  }

  function validateAndSaveTitle() {
    const trimmedTitle = editTitle.trim();

    if (!trimmedTitle) {
      // Show error toast
      errorToast('Previous value remains unchanged', 'Title cannot be empty');

      // Revert to original title
      editTitle = item.title;
      editingTitle = false;
      return;
    }

    dispatch("save-field", { field: "title", value: trimmedTitle });
  }

  function handleKeydown(event) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      validateAndSaveTitle();
    } else if (event.key === "Escape") {
      event.preventDefault();
      dispatch("cancel-edit", { field: "title" });
    }
  }

</script>

<div class="mb-8 w-full max-w-full">
  <div class="w-full min-w-0 overflow-hidden max-w-full">
    <div class="mb-2">
      {#if editingTitle}
        <div class="flex items-center gap-3 w-full pr-4 ">
          <!-- Issue Key (in edit mode) -->

          <div class="min-w-[80%]">
            <input
              type="text"
              bind:value={editTitle}
              onkeydown={handleKeydown}
              class="w-full text-2xl font-medium bg-transparent border-0 py-1 focus:outline-none break-words"
              style="color: var(--ds-text); word-wrap: break-word; overflow-wrap: break-word;"
              placeholder="Enter title..."
              autofocus
            />
          </div>
          <div class="flex gap-2 mt-2 hidden">
            <Button
              variant="primary"
              size="small"
              icon={Check}
              onclick={validateAndSaveTitle}
              disabled={saving}
            />
            <Button
              variant="default"
              size="small"
              icon={X}
              onclick={() => dispatch("cancel-edit", { field: "title" })}
            />
          </div>
        </div>
      {:else}
        <!-- Issue Key -->

        <button
          onclick={startEditingTitle}
          class="text-2xl font-medium pr-4 py-1 rounded transition-colors text-left cursor-pointer w-full title-button break-words"
          style="color: var(--ds-text); word-wrap: break-word; overflow-wrap: break-word;"
          title="Click to edit title"
        >
          {item.title}
        </button>
      {/if}
    </div>
  </div>
</div>


<style>
  .title-button:hover {
    background-color: var(--ds-surface);
  }
</style>
