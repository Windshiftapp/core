<script>
  import InlineEditorActions from './InlineEditorActions.svelte';

  // Single inline-edit primitive. It owns the edit state machine and the
  // commit/cancel outcome rule; callers own parsing, validation and the
  // request. See docs/editable-field-wrapper-design.md.
  let {
    value = null,
    editable = true,
    oncommit = null,
    // Maps the stored value to the editor's representation (e.g. minutes →
    // "5h", ISO date → input date). Identity by default.
    toDraft = (stored) => stored,
    // Enter commits for single-line editors; multiline editors insert instead.
    multiline = false,
    enableSingleClick = false,
    enableDoubleClick = false,
    showActions = false,
    // Editors embedded in a clickable row/card must not let clicks or keys
    // reach the surrounding navigation handler.
    stopPropagation = false,
    // Headless: render the display snippet without a button. The caller owns
    // the activation element and calls `startEditing()` via a bound ref.
    headless = false,
    displayTestid = undefined,
    onclick: onclickProp = null,
    onstartedit = null,
    display,
    editor,
  } = $props();

  let editing = $state(false);
  let draft = $state(null);
  let saving = $state(false);
  let error = $state('');
  let committed = false;
  let suppressCommit = false;
  let container = $state(null);
  let clickTimeout = null;

  export function isEditing() {
    return editing;
  }

  export function startEditing() {
    if (!editable || editing) return;
    draft = toDraft(value);
    error = '';
    committed = false;
    editing = true;
    onstartedit?.();
  }

  export function cancelEditing() {
    committed = true;
    editing = false;
    saving = false;
    error = '';
  }

  export function setError(message) {
    error = message || '';
  }

  async function commit() {
    if (committed || saving || !editing) return;
    if (draft === toDraft(value)) {
      cancelEditing();
      return;
    }
    committed = true;
    saving = true;
    try {
      await oncommit?.(draft);
      error = '';
      cancelEditing();
    } catch (err) {
      // Validation (synchronous throw) and save failures (rejection) are the
      // same outcome: stay in the editor with the message.
      error = err?.message || 'Failed to save';
      committed = false;
    } finally {
      saving = false;
    }
  }

  function handleFocusOut(event) {
    if (suppressCommit) {
      suppressCommit = false;
      return;
    }
    // Focus moving to a control inside the wrapper (save/cancel button, picker)
    // is not a commit trigger.
    if (event.relatedTarget && container?.contains(event.relatedTarget)) return;
    void commit();
  }

  function handleKeydown(event) {
    if (event.key === 'Enter' && !multiline && !event.shiftKey) {
      event.preventDefault();
      void commit();
    } else if (event.key === 'Escape') {
      // Cancel, but do not preventDefault: the surrounding modal decides
      // whether Escape also closes it.
      cancelEditing();
    } else {
      return;
    }
    if (stopPropagation) event.stopPropagation();
  }

  function handleAction(fn) {
    suppressCommit = true;
    void fn();
    suppressCommit = false;
  }

  function handleClick(event) {
    if (stopPropagation) event.stopPropagation();
    if (enableDoubleClick && enableSingleClick) {
      if (clickTimeout) {
        clearTimeout(clickTimeout);
        clickTimeout = null;
        startEditing();
      } else {
        clickTimeout = setTimeout(() => {
          clickTimeout = null;
          onclickProp?.();
        }, 200);
      }
      return;
    }
    if (enableSingleClick) {
      onclickProp?.();
      return;
    }
    if (enableDoubleClick) return;
    startEditing();
  }

  function handleDoubleClick(event) {
    if (stopPropagation) event.stopPropagation();
    if (enableDoubleClick && !enableSingleClick) startEditing();
  }
</script>

{#if editing}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    bind:this={container}
    class="relative"
    role="group"
    onblurcapture={handleFocusOut}
    onkeydown={handleKeydown}
  >
    {@render editor({ draft, setDraft: (next) => (draft = next), commit, cancel: cancelEditing, saving, error, markAction: handleAction })}
    {#if showActions}
      <div class="absolute -right-1 -top-1">
        <InlineEditorActions
          {saving}
          onsave={() => handleAction(commit)}
          oncancel={() => handleAction(cancelEditing)}
        />
      </div>
    {/if}
  </div>
{:else if headless}
  {@render display()}
{:else}
  <button
    type="button"
    onclick={handleClick}
    ondblclick={handleDoubleClick}
    class="block w-full text-left"
    disabled={!editable}
    data-testid={displayTestid}
    style="all: unset; display: block; width: 100%;"
  >
    {@render display()}
  </button>
{/if}
