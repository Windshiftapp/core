/**
 * Windshift Forms Widget
 * Lightweight embeddable form widget (~1KB)
 *
 * Usage:
 *   <div id="ws-form-my-form"></div>
 *   <script src="/forms/widget.js" data-slug="my-form" data-target="ws-form-my-form"></script>
 */
(() => {
  const script = document.currentScript;
  if (!script) return;

  const slug = script.getAttribute('data-slug');
  const targetId = script.getAttribute('data-target');
  if (!slug || !targetId) {
    console.error('[Windshift Forms] Missing data-slug or data-target attribute');
    return;
  }

  const origin = script.src.replace(/\/forms\/widget\.js.*$/, '');

  const init = () => {
    const target = document.getElementById(targetId);
    if (!target) {
      console.error(`[Windshift Forms] Target element not found: #${targetId}`);
      return;
    }

    const iframe = document.createElement('iframe');
    iframe.src = `${origin}/forms/${encodeURIComponent(slug)}?embed=true`;
    iframe.style.width = '100%';
    iframe.style.border = 'none';
    iframe.style.minHeight = '400px';
    iframe.style.height = '600px';
    iframe.setAttribute('loading', 'lazy');
    iframe.setAttribute('allow', 'clipboard-write');
    iframe.title = 'Windshift Form';

    // Listen for resize messages from the embedded form
    window.addEventListener('message', (event) => {
      if (event.source !== iframe.contentWindow) return;
      try {
        const data = typeof event.data === 'string' ? JSON.parse(event.data) : event.data;
        if (data.type === 'ws-form-resize' && data.height) {
          iframe.style.height = `${data.height}px`;
        }
      } catch {
        // Ignore non-JSON messages
      }
    });

    target.appendChild(iframe);
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
