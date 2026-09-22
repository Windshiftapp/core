import { describe, expect, it } from 'vitest';
import { findHardcodedCopy } from './hardcoded-i18n-ast.js';

describe('mobile hardcoded copy guard', () => {
  it.each([
    '<p>not translated</p>',
    '<Sheet message="Unsaved changes" />',
    '<Sheet emptyText="No options" ariaLabel="Options" />',
    '<p>{item.title || "Untitled"}</p>',
    '<p>{busy ? "Saving" : "Save"}</p>',
    // biome-ignore lint/suspicious/noTemplateCurlyInString: The fixture must contain literal Svelte interpolation syntax.
    '<p>{`${count} linked`}</p>',
    '<Select options={[{ value: "none", label: "No template" }]} />',
    '<script>const tabs = [{ id: "home", label: "Home" }];</script>',
    '<script>let { saveLabel = "Save" } = $props();</script>',
    '<script>let error = $state(""); function save() { error = err.message || "Failed"; }</script>',
    '<script>function save() { successToast("Saved"); }</script>',
    '<script>function labelForField() { return "Priority"; }</script>',
  ])('rejects user-facing English in %s', (source) => {
    expect(findHardcodedCopy(source).length).toBeGreaterThan(0);
  });

  it('ignores technical strings, translated values and dynamic user content', () => {
    expect(
      findHardcodedCopy(`
      <script>
        const tabs = [{ id: 'home', labelKey: 'mobile.myWork.title', url: '/m' }];
        const options = [{ value: 'none', label: t('common.none') }];
        console.error('Technical diagnostic');
      </script>
      <p class="empty" data-testid="empty" style="color: red">{t('mobile.common.empty')}</p>
      <a href="/m">{item.title}</a>
      <p>{mode === 'edit' ? t('common.save') : user.name}</p>
      <span>{count > 99 ? '99+' : count}</span><kbd>Esc</kbd>
      <div style:transform={position ? 'translateY(0px)' : 'none'} class:active={mode === 'edit'}></div>
      <style>.empty::before { content: ''; }</style>
    `)
    ).toEqual([]);
  });

  it('checks command labels and reports source lines for JavaScript', () => {
    expect(
      findHardcodedCopy("// provider\nconst command = { label: 'Home', url: '/m' };", {
        scriptOnly: true,
      })
    ).toEqual([{ line: 2, text: 'Home' }]);
  });
});
