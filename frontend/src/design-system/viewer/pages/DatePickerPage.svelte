<script>
  import DatePicker from '../../../lib/pickers/DatePicker.svelte'

  let basic = $state('')
  let prefilled = $state('2026-11-07')
  let persian = $state('2026-11-07')
  let gregorian = $state('2026-11-07')
  let bounded = $state('')
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">DatePicker</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    A calendar-aware date picker built on Melt UI's headless calendar and
    <code>@internationalized/date</code>. The bound value stays a Gregorian
    <code>YYYY-MM-DD</code> string; the calendar follows the UI locale
    (Persian for <code>fa</code>) unless overridden.
  </p>

  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic</h2>
    <div class="max-w-xs">
      <DatePicker bind:value={basic} label="Due date" dataTestid="ds-datepicker-basic" />
    </div>
    <p class="mt-4 text-xs" style="color: var(--ds-text-subtle);">Value: {basic || '—'}</p>
  </section>

  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Prefilled and clearable</h2>
    <div class="max-w-xs">
      <DatePicker bind:value={prefilled} label="Start date" dataTestid="ds-datepicker-prefilled" />
    </div>
    <p class="mt-4 text-xs" style="color: var(--ds-text-subtle);">Value: {prefilled || '—'}</p>
  </section>

  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Calendar override</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Force the Persian or Gregorian calendar regardless of the UI locale.
    </p>
    <div class="grid grid-cols-2 gap-8 max-w-xl">
      <DatePicker
        bind:value={persian}
        calendar="persian"
        locale="en"
        label="Persian (en)"
        dataTestid="ds-datepicker-persian"
      />
      <DatePicker
        bind:value={gregorian}
        calendar="gregory"
        locale="en"
        label="Gregorian (en)"
        dataTestid="ds-datepicker-gregorian"
      />
    </div>
  </section>

  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Bounded</h2>
    <div class="max-w-xs">
      <DatePicker
        bind:value={bounded}
        min="2026-11-01"
        max="2026-11-30"
        label="November only"
        dataTestid="ds-datepicker-bounded"
      />
    </div>
  </section>
</div>
