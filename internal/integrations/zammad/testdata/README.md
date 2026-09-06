# Zammad API fixtures

`users_search_expand_zammad_7_1_1.json` is a reduced and anonymized projection of the response shape verified against the restricted `ticket.agent` service account in the Windshift Zammad 7.1.1 lab on 2026-09-01.
The `group_ids` access map was observed on the expanded user records returned with `expand=true`.
The fixture intentionally retains only the response fields consumed by owner discovery.
For those retained fields, the array envelope, field names, and field types are unchanged; identifying values and numeric IDs were replaced.
