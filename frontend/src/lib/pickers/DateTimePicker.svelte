<script>
  import DatePicker from './DatePicker.svelte';
  import Input from '../components/Input.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(''), inputRef = $bindable(null), id = undefined,
    name = undefined, min = undefined, max = undefined, step = undefined,
    disabled = false, readonly = false, required = false, autofocus = false,
    size = 'medium', class: className = '', style = '', title = undefined,
    placeholder = '', dataTestid = undefined, ariaLabel = undefined,
    ariaDescribedby = undefined, ariaInvalid = undefined, ariaRequired = undefined, ariaLabelledby = undefined, onchange = undefined, onfocus = undefined,
    onblur = undefined, onkeydown = undefined, onclick = undefined, onkeyup = undefined,
  } = $props();

  let date = $state('');
  let time = $state('');
  let lastValue;
  $effect(() => {
    if (value === lastValue) return;
    lastValue = value;
    date = value?.split('T')[0] || '';
    time = value?.split('T')[1] || '';
  });

  function commit() {
    const next = date && time ? `${date}T${time}` : '';
    if (next === value) return;
    lastValue = next;
    value = next;
    onchange?.(next);
  }

  const descriptionId = $props.id();
  let validationMessage = $state('');
  $effect(() => {
    void [value, min, max, step, disabled, readonly, required];
    validationMessage = '';
  });
</script>

<div class="inline-flex flex-col gap-1">
  <input type="datetime-local" {name} {min} {max} {step} {disabled} {readonly} {required} {value}
    tabindex="-1" aria-hidden="true" style="position:absolute; width:1px; height:1px; opacity:0; pointer-events:none;"
    oninvalid={(event) => { event.preventDefault(); validationMessage = event.currentTarget.validationMessage; inputRef?.focus(); }} />
  <div class="flex items-center gap-2">
    <DatePicker {id} value={date} bind:inputRef {disabled} {readonly} {autofocus} {size}
      min={min?.split('T')[0]} max={max?.split('T')[0]} {placeholder} {title} {style}
      class={className} {dataTestid} {ariaLabel} {ariaInvalid} {ariaRequired} {ariaLabelledby}
      ariaDescribedby={[ariaDescribedby, validationMessage ? descriptionId : ''].filter(Boolean).join(' ') || undefined}
      {onfocus} {onblur} {onkeydown} {onclick} {onkeyup}
      onchange={(next) => { date = next; commit(); }} />
    <Input type="time" value={time} {disabled} {readonly} {size} {step}
      min={date === min?.split('T')[0] ? min?.split('T')[1] : undefined}
      max={date === max?.split('T')[0] ? max?.split('T')[1] : undefined}
      ariaLabel={t('common.time')} dataTestid={dataTestid ? `${dataTestid}-time` : undefined}
      oninput={(event) => { time = event.target.value; commit(); }} />
  </div>
  {#if validationMessage}
    <span id={descriptionId} role="alert" class="text-xs text-ds-text-danger">{validationMessage}</span>
  {/if}
</div>
