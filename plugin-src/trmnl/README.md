# TRMNL plugins for Windshift

Two [TRMNL](https://usetrmnl.com) private plugins that render Windshift work
items on an e-ink display.

These are TRMNL plugins, not Windshift plugins — unlike the siblings in
`plugin-src/`, there is no `manifest.json` and nothing is built into
`plugins/`. They run entirely on TRMNL's side and reach Windshift only
through the public REST API.

| Directory | Screen | Shows |
|---|---|---|
| `windshift-tasks/` | Task list | Open work items matching a query, newest first |
| `windshift-summary/` | Daily summary | Open / due today / overdue / closed today, next up, and what moved today |

They are separate plugins rather than two views of one, because a TRMNL
plugin renders a single screen per refresh — the four `.liquid` files in each
`src/` are size variants of that one screen, not a sequence. Install both and
put them on the same playlist to get two screens in rotation.

Nothing is installed into Windshift and no server-side change is required.
Each plugin polls the public REST API with a personal token.

## Configuration

Both plugins ask for the same two things, entered in TRMNL's plugin settings
form (or in `.trmnlp.yml` for local dev):

- **Windshift Server URL** — the origin, e.g. `https://windshift.example.com`.
  No `/rest/api/v2` suffix; a trailing slash is tolerated.
- **API Token** — a personal token from **Profile > API Tokens**, starting
  with `crw_`. It needs the `items:read` and `users:read` scopes and nothing
  else.

Plus one query each (`Query` / `Scope`), written in Windshift QL. Both default
to:

```
assignee = currentUser() AND status != Done
```

`currentUser()` resolves server-side to the token's owner, so the same query
works for everyone without hardcoding a user id.

### Query notes

QL fields that are useful here: `assignee`, `reporter`, `status`, `priority`,
`type`, `due_date`, `created`, `updated`, `label`, `project`, `workspace`.

Two things that bite:

- **String values need quotes.** `status = "In Progress"` works;
  `workspace = WI` is rejected as `unknown field: WI`.
- **There is no `ORDER BY`.** Sorting is a separate query parameter, already
  set in `polling_url`.

`currentUser()` is the only context function; there are no date functions like
`now()`. The summary plugin works around that by interpolating today's date
into two of its polling URLs with Liquid's `date` filter.

## Local preview

`trmnlp` needs Ruby >= 3.4, which macOS does not ship, so use the container:

```sh
cp .env.example .env      # then fill in URL + token
docker compose up tasks   # http://localhost:4567
docker compose up summary # http://localhost:4568
```

Both services read `WINDSHIFT_URL` and `WINDSHIFT_TOKEN` from `.env`; the
`.trmnlp.yml` files interpolate them into the plugin's custom fields, so no
credential is ever committed.

Useful endpoints on each server:

- `/full`, `/half_horizontal`, `/half_vertical`, `/quadrant` — live preview
- `/render/full.png` — the actual 800x480 1-bit render, which is what the
  device shows and the only reliable way to check layout
- `/poll` — force a refetch
- `/data` — the polled JSON as the templates see it

Edits to `src/` reload automatically. Changes to `.trmnlp.yml` need a
`/poll` to take effect.

Lint before pushing:

```sh
docker run --rm -v "$PWD/windshift-tasks:/plugin" trmnl/trmnlp lint
```

## Installing on trmnl.com

Create a Private Plugin with strategy **Polling**, then either paste the
contents of `src/*.liquid` into the markup editor and recreate the custom
fields from `src/settings.yml`, or push the directory:

```sh
docker run --rm -it -v "$HOME/.config/trmnlp:/root/.config/trmnlp" \
  -v "$PWD/windshift-tasks:/plugin" trmnl/trmnlp login
docker run --rm -v "$HOME/.config/trmnlp:/root/.config/trmnlp" \
  -v "$PWD/windshift-tasks:/plugin" trmnl/trmnlp push
```

`push` writes the new plugin's id back into `src/settings.yml`; keep it, or
every push creates another plugin.

**The Windshift server must be reachable from TRMNL's cloud.** Polling happens
on their servers, not on the device, so a Windshift instance on localhost or
behind a VPN will not work with the hosted service. Local preview via
`trmnlp serve` has no such constraint.

## How the data gets in

TRMNL's poller fetches each line of `polling_url` and exposes the responses as
`IDX_0`, `IDX_1`, … Both plugins use `IDX_0` for `/users/me` — it supplies the
display name and doubles as an auth check.

`windshift-tasks` adds one item query. `windshift-summary` adds three: the
backlog it computes from, a closed-today count, and today's activity across
every workspace the token can see. The two count-only queries use `page_size=1`
and read just `pagination.total_items`, so they stay cheap regardless of backlog
size.

Counts that come from `pagination.total_items` (open, closed today, moved today) are
exact at any size. Due-today and overdue are counted in Liquid from a 100-item
window, so they are exact as long as the scope query returns 100 items or
fewer — which the default per-assignee query will.

## Failure behaviour

A failed poll returns `{}` rather than raising, which would otherwise render a
silently blank screen. Both plugins detect this and name the problem on the
screen instead:

- no `IDX_0.id` → *Cannot reach Windshift. Check the server URL and API token.*
- no `IDX_1.pagination` → *Windshift rejected the query. Check the QL syntax.*

A poll that fails after a successful one keeps the last good data rather than
clearing the screen, so a brief outage shows stale items rather than an error.

## Layout gotchas

Three things cost real debugging time and are easy to reintroduce:

- **Do not nest `.layout` inside `.layout`.** The markup survives in the HTML
  preview but the e-ink render comes back blank. Branches that render a
  centred message must sit outside the list's `.layout` wrapper, not inside it.
- **`data-overflow-max-cols` is required, not optional.** Without it the
  overflow engine clips the trailing row instead of hiding it and emitting the
  "and N more" counter.
- **Item keys do not fit `.meta` / `.index`.** That slot is sized for a numeric
  rank; `WI-943` overflows it and lands on top of the title. Keys go in the
  label row.

Also avoid `remove_last: '/'` for trimming a trailing slash off the server
URL — with no trailing slash present it removes the last `/` anywhere in the
string, which is the one in `https://`. Both plugins append the path first and
then collapse a doubled slash immediately before `/rest/`.
