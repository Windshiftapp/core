# markdown-raw-editor

A reference `editor.markdown` plugin. It renders a plain `<textarea>` so a
surface can be edited as raw Markdown source instead of through the WYSIWYG
Milkdown editor. It is frontend-only; the backend WASM file is an empty stub
that the plugin loader requires.

## Why a plugin instead of a native source mode

Milkdown is WYSIWYG-only: content round-trips through
`remark-parse → ProseMirror → remark-stringify`, which normalizes bullets,
emphasis, and code fences and drops constructs outside its schema. A raw mode
inside Milkdown would silently rewrite what the user typed. A plugin edits a
copy of the exact source instead, and the host stays the only writer.

## Layout

```
manifest.json          editor.markdown extension declaration
backend/main.go        empty WASM entry point
assets/editor.html     iframe document
assets/editor.js       editor.markdown protocol + textarea behaviour
assets/editor.css      iframe styles
build.sh               builds plugin.wasm and packages the zip
```

## Build and install

```bash
./build.sh
rm -rf ../../plugins/markdown-raw-editor
mkdir -p ../../plugins/markdown-raw-editor
unzip -o markdown-raw-editor.zip -d ../../plugins/markdown-raw-editor/
```

The server loads plugins from `plugins/` (or `PLUGIN_DIR`) at startup. Restart
the server after installing. An admin can also upload `markdown-raw-editor.zip`
through the plugin management API/UI.

## Protocol

The host side lives in `frontend/src/lib/services/PluginBridge.js` and
`frontend/src/lib/editors/PluginEditorHost.svelte`.

| Direction | Message | Payload |
| --- | --- | --- |
| plugin → host | `plugin:editorReady` | none (handshake, no `sessionId` yet) |
| host → plugin | `host:editorInit` | `sessionId`, `entityType`, `entityId`, `workspaceId`, `markdown`, `readonly`, `locale`, `theme`, `capabilities` |
| plugin → host | `plugin:editorChange` | `sessionId`, `markdown`, `dirty` |
| plugin → host | `plugin:editorSave` | `sessionId`, `markdown` |
| plugin → host | `plugin:editorCancel` | `sessionId` |
| plugin → host | `plugin:editorError` | `sessionId`, `message` |
| plugin → host | `plugin:resize` | `height` |
| host → plugin | `host:editorUpdate` | `sessionId`, `markdown` |
| host → plugin | `host:editorSaveRequest` / `host:editorCancelRequest` | `sessionId` |
| host → plugin | `host:editorDispose` | `sessionId` |

`plugin:editorReady` is the only message sent before a session exists, so it
carries no `sessionId`. Every message after `host:editorInit` echoes the
`sessionId`; the bridge drops mismatches so two editors on one page cannot
cross-talk. The bridge also rejects messages whose `event.source` is not its own
iframe.

Saving writes through the host: the plugin streams `plugin:editorChange` as the
user types, `PluginEditorHost` mirrors that into its bound `content`, and the
surface's own Save/Cancel buttons persist or discard it through the normal
item/page API — exactly like the native editor. The plugin has no save button of
its own. The plugin never reads or writes the database.

## Notes for editor plugin authors

- Do not declare a `styles` asset. `PluginEditorHost` injects that asset into
  the **host** document, where `html, body { … }` rules leak into the app. Load
  CSS from the iframe document instead.
- No inline `<script>`: the host CSP is `script-src 'self' 'nonce-…'`, so load a
  same-origin `.js` file.
- Raw HTML typed here still will not render. The Go renderer escapes it
  (`internal/markdown/render.go`) and Milkdown only models inline HTML, so do
  not promise more than the platform renders.
- The host refuses saves over 256 KB (`validation.MarkdownMaxBytes`). This
  plugin enforces the same limit locally so the user gets feedback before the
  round trip.
