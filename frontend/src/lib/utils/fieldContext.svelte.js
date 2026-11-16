import { getContext, setContext } from 'svelte';

export const FIELD_CONTEXT = Symbol('form-field');

export function descriptionIds(...values) {
  return (
    [...new Set(values.filter(Boolean).join(' ').split(/\s+/).filter(Boolean))].join(' ') ||
    undefined
  );
}

// Controls read reactive field state; explicit control attributes remain additive.
export function fieldControl(props) {
  const field = getContext(FIELD_CONTEXT);
  setContext(FIELD_CONTEXT, undefined);
  $effect(() => {
    const id = props().id;
    if (id && field && id !== field.id) return field.registerId(id);
  });
  return {
    get id() {
      return props().id ?? field?.id;
    },
    get describedby() {
      return descriptionIds(props().ariaDescribedby, field?.describedby);
    },
    get invalid() {
      return props().ariaInvalid || field?.invalid || undefined;
    },
    get required() {
      return Boolean(props().required || field?.required);
    },
    get ariaRequired() {
      return Boolean(props().required || field?.ariaRequired);
    },
    get labelledby() {
      return props().ariaLabel ? undefined : field?.labelId;
    },
  };
}
