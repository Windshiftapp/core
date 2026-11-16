import { getContext, setContext } from 'svelte';

const ACTION_EDITOR_CONTEXT = Symbol('action-editor');

export function provideActionEditorContext(context) {
  setContext(ACTION_EDITOR_CONTEXT, context);
}

export function useActionEditorContext() {
  return getContext(ACTION_EDITOR_CONTEXT);
}
