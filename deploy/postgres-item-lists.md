# PostgreSQL maintenance after a bulk item import

After a large import or logical restore commits, run this in the Windshift
database as the table owner, outside a transaction:

```sql
VACUUM (ANALYZE) items;
```

Numbered pages skip rows in workspace rank order. The active-item index
contains the IDs needed to select the page, but PostgreSQL still checks the
heap for entries on pages that aren't marked all-visible. `VACUUM` updates
the visibility map; `ANALYZE` alone only updates planner statistics.
Autovacuum can process insert-heavy tables, but it may not have caught up
when an import finishes. Long-running transactions can also delay visibility.

To verify, use a populated workspace ID and an offset within its item count:

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT id FROM items
WHERE workspace_id = 1 AND merged_into_item_id IS NULL
ORDER BY frac_index LIMIT 50 OFFSET 50000;
```

Look for `Index Only Scan using idx_items_workspace_frac_index_active` and
few or zero `Heap Fetches`. Writes clear visibility bits, so heap fetches can
rise again between vacuums. Even an index-only scan visits the skipped index
entries; use cursor pagination for sequential traversal.
