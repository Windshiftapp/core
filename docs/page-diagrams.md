# Page diagram storage and lifecycle

Page diagrams are immutable attachment-backed content embedded in Page Markdown.
The canonical block is an `excalidraw` fence whose JSON contains an attachment
ID and display name:

````markdown
```excalidraw
{"attachmentId":123,"name":"Architecture"}
```
````

The PageDiagram service owns the lifecycle across agent tools, REST, CLI, and
the Page editor:

1. Create validates either an Excalidraw scene or a Mermaid seed, uploads the
   payload as a Page-scoped attachment, and inserts one fence at the requested
   start/end placement.
2. Reads resolve only fences on the requested Page and verify attachment
   ownership in the same workspace/Page before returning the payload.
3. Update validates and uploads a new attachment, replaces exactly one matching
   fence, and leaves the old attachment available for historical revisions.
4. If the Page mutation fails (including an optimistic-concurrency conflict),
   the newly uploaded attachment is deleted as compensation. Successful old
   attachments are never deleted by update.
5. Page mutations accept `expected_content_hash`; a stale value fails before
   Page content changes. Callers should use the hash returned by their latest
   Page or diagram read.

Mermaid is stored as a small `{type:"mermaid",source:"..."}` seed. The Page
editor converts it once when opening the Excalidraw editor; saving stores the
resulting editable Excalidraw scene. Excalidraw scenes are structurally
validated and bounded before storage.

All service operations require the caller's `pages:read`/`pages:write` scope
at the transport boundary and the Page's `page.view`/`page.edit` permission.
There is intentionally no direct diagram-delete operation: attachments that
are referenced by live or historical Page revisions must remain readable.

The frontend editor fingerprints complete element content, position, style,
and files to detect unsaved changes. Page revision restore restores the old
Markdown fence and therefore makes its original attachment render again.
