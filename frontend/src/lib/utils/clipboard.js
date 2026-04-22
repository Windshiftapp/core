// Copy `text` to the system clipboard, falling back to a hidden textarea +
// document.execCommand for older browsers or non-secure contexts where
// navigator.clipboard is unavailable. Resolves true on success, false on
// failure. Never throws.
export async function copyToClipboard(text) {
  if (text == null) return false;
  const value = String(text);

  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return true;
    } catch {
      // fall through to legacy path
    }
  }

  try {
    const textArea = document.createElement('textarea');
    textArea.value = value;
    textArea.style.position = 'fixed';
    textArea.style.opacity = '0';
    textArea.style.pointerEvents = 'none';
    document.body.appendChild(textArea);
    textArea.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(textArea);
    return ok;
  } catch {
    return false;
  }
}
