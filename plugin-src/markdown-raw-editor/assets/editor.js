/**
 * Raw Markdown editor — plugin side of the `editor.markdown` protocol.
 *
 * The host owns the document: it holds the Markdown, validates it, and persists
 * it through the normal item/page/comment APIs. This plugin only edits a copy
 * and reports every change back, so the surrounding surface's own Save/Cancel
 * buttons remain the only save path — there is no second save button here.
 *
 * Handshake (see frontend/src/lib/services/PluginBridge.js):
 *   plugin -> host  plugin:editorReady                         (no sessionId)
 *   host   -> plugin host:editorInit  { sessionId, markdown, … }
 *   plugin -> host  plugin:editorChange { markdown, dirty }   (live)
 *   plugin -> host  plugin:editorSave / editorCancel / editorError
 *
 * `plugin:editorReady` is the one message sent before a session exists, so it
 * carries no sessionId. Every later message echoes the sessionId from
 * host:editorInit and the host drops mismatches.
 */
(function () {
  'use strict';

  var HOST_ORIGIN = window.location.origin;
  // Keep in step with validation.MarkdownMaxBytes in core.
  var MAX_BYTES = 256 * 1024;
  var READY_RETRY_MS = 250;
  var READY_MAX_ATTEMPTS = 40;

  var TYPES = {
    // host -> plugin
    INIT: 'host:editorInit',
    UPDATE: 'host:editorUpdate',
    SAVE_REQUEST: 'host:editorSaveRequest',
    CANCEL_REQUEST: 'host:editorCancelRequest',
    DISPOSE: 'host:editorDispose',
    THEME_UPDATE: 'host:themeUpdate',
    // plugin -> host
    READY: 'plugin:editorReady',
    CHANGE: 'plugin:editorChange',
    SAVE: 'plugin:editorSave',
    CANCEL: 'plugin:editorCancel',
    ERROR: 'plugin:editorError',
    RESIZE: 'plugin:resize',
  };

  var root = document.getElementById('editor-root');
  var textarea = document.getElementById('markdown-input');
  var countEl = document.getElementById('byte-count');
  var statusEl = document.getElementById('status');

  var encoder = new TextEncoder();

  var sessionId = null;
  var baseline = '';
  var readonly = false;
  var disposed = false;
  var dirty = false;
  var readyTimer = null;
  var resizeFrame = null;

  // --- messaging -----------------------------------------------------------

  function postToHost(type, payload) {
    window.parent.postMessage(Object.assign({ type: type, sessionId: sessionId }, payload), HOST_ORIGIN);
  }

  function postReady() {
    window.parent.postMessage({ type: TYPES.READY }, HOST_ORIGIN);
  }

  // The host attaches its bridge listener on the iframe's load event, which can
  // fire after this script runs. Retry until host:editorInit arrives so a ready
  // sent into the void is not fatal.
  function startHandshake() {
    var attempts = 0;
    var tick = function () {
      if (disposed || sessionId) return;
      postReady();
      attempts += 1;
      if (attempts >= READY_MAX_ATTEMPTS) {
        setStatus('The host did not start an editor session.', true);
        return;
      }
      readyTimer = window.setTimeout(tick, READY_RETRY_MS);
    };
    tick();
  }

  function reportError(message) {
    if (!sessionId) return;
    postToHost(TYPES.ERROR, { message: message });
  }

  // --- rendering -----------------------------------------------------------

  function byteLength(value) {
    return encoder.encode(value).length;
  }

  function formatBytes(bytes) {
    if (bytes < 1024) return bytes + ' B';
    return (bytes / 1024).toFixed(1) + ' KB';
  }

  function setStatus(message, isError) {
    statusEl.textContent = message || '';
    statusEl.dataset.error = isError ? 'true' : 'false';
  }

  function updateCount(value) {
    var bytes = byteLength(value);
    countEl.textContent = formatBytes(bytes) + ' / ' + formatBytes(MAX_BYTES);
    countEl.dataset.over = bytes > MAX_BYTES ? 'true' : 'false';
    return bytes;
  }

  function autosize() {
    textarea.style.height = 'auto';
    textarea.style.height = textarea.scrollHeight + 'px';
    scheduleResize();
  }

  function scheduleResize() {
    if (resizeFrame != null || disposed) return;
    resizeFrame = window.requestAnimationFrame(function () {
      resizeFrame = null;
      if (disposed) return;
      var height = Math.ceil(root.getBoundingClientRect().height);
      window.parent.postMessage({ type: TYPES.RESIZE, height: height }, HOST_ORIGIN);
    });
  }

  function applyTheme(theme) {
    document.documentElement.dataset.theme = theme === 'dark' ? 'dark' : 'light';
  }

  // --- protocol ------------------------------------------------------------

  // The host persists through its own surface, so a "save" is only sent when a
  // host explicitly asks for one; the normal path is the live editorChange.
  function flush() {
    if (!sessionId || readonly) return;
    var markdown = textarea.value;
    var bytes = byteLength(markdown);
    if (bytes > MAX_BYTES) {
      var message = 'Markdown is ' + formatBytes(bytes) + '; the limit is ' + formatBytes(MAX_BYTES) + '.';
      setStatus(message, true);
      reportError(message);
      return;
    }
    postToHost(TYPES.SAVE, { markdown: markdown });
  }

  function handleInput() {
    var value = textarea.value;
    dirty = value !== baseline;
    updateCount(value);
    autosize();
    if (sessionId) {
      postToHost(TYPES.CHANGE, { markdown: value, dirty: dirty });
    }
  }

  function handleInit(message) {
    sessionId = message.sessionId;
    window.clearTimeout(readyTimer);
    readyTimer = null;

    readonly = !!message.readonly;
    baseline = typeof message.markdown === 'string' ? message.markdown : '';
    textarea.value = baseline;
    textarea.readOnly = readonly;
    dirty = false;

    applyTheme(message.theme);
    updateCount(baseline);
    autosize();

    if (Array.isArray(message.capabilities) && message.capabilities.indexOf('markdown.raw-editor') === -1) {
      setStatus('The host did not advertise markdown.raw-editor; saves may be rejected.', true);
    } else {
      setStatus(readonly ? 'Read-only' : '');
    }

    if (!readonly) textarea.focus();
  }

  function handleUpdate(message) {
    if (typeof message.markdown !== 'string') return;
    baseline = message.markdown;
    textarea.value = baseline;
    dirty = false;
    updateCount(baseline);
    autosize();
  }

  function handleMessage(event) {
    if (event.origin !== HOST_ORIGIN) return;
    if (event.source !== window.parent) return;

    var message = event.data;
    if (!message || typeof message.type !== 'string') return;

    switch (message.type) {
      case TYPES.INIT:
        handleInit(message);
        break;
      case TYPES.UPDATE:
        if (message.sessionId !== sessionId) return;
        handleUpdate(message);
        break;
      case TYPES.SAVE_REQUEST:
        if (message.sessionId !== sessionId) return;
        flush();
        break;
      case TYPES.CANCEL_REQUEST:
        if (message.sessionId !== sessionId) return;
        postToHost(TYPES.CANCEL);
        break;
      case TYPES.THEME_UPDATE:
        // The host currently sends only the theme name in editorInit; accept a
        // theme update too so a future host change does not require a new plugin.
        if (typeof message.theme === 'string') applyTheme(message.theme);
        break;
      case TYPES.DISPOSE:
        dispose();
        break;
      default:
        break;
    }
  }

  function dispose() {
    disposed = true;
    window.clearTimeout(readyTimer);
    if (resizeFrame != null) window.cancelAnimationFrame(resizeFrame);
    window.removeEventListener('message', handleMessage);
  }

  // --- wiring --------------------------------------------------------------

  textarea.addEventListener('input', handleInput);
  window.addEventListener('message', handleMessage);

  startHandshake();
})();
