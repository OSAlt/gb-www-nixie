# Social Metrics Migration — Plan & API Key Guide

> **Status:** implemented. The live source is **Metrika** (§1.0) — one free
> API key covering all seven platforms. The per-platform APIs in §1.1–1.4 are
> kept as automatic gap-fillers and are no longer required.

Goal: replace the hardcoded follower counts that build the "social llama" chart with
real, periodically-refreshed numbers pulled directly from each platform, implemented
in this repo (gb-www-nixie).

- Strategy: **one aggregator** — Metrika returns every platform in a single
  batched request. Official APIs (YouTube/Twitch/Discord) and the Pulse scraper
  remain wired as fallbacks for any platform Metrika cannot deliver.
- Refresh: **daily** background worker (Metrika only refreshes its own data
  daily). ~30 requests/month against a 500/month free quota.
- Results are cached in Postgres as last-known-good; a failed platform keeps
  its previous value instead of charting a drop.
- Client: `/api/v1/social/count/all` keeps the same shape `{ "<platform>": <int> }`
  that `static/js/stats.js` already expects (keys lowercase).

---

## 1. API keys / credentials needed

Config loader reminder: `internal/config/config.go` loads `.env` then lowercases env var
names, so `YOUTUBE_API_KEY` becomes config key `youtube_api_key`.
Add to `.env` (gitignored) and to `env.template`.

### 1.0 Metrika — primary source (one key, free)

| Setting | Value |
|---|---|
| Env vars | `METRIKA_API_KEY`, `METRIKA_BASE_URL`, `METRIKA_DISCORD_VANITY` |
| Endpoint | `POST https://metrika.run/api/v1/resources/search` |
| Auth | `Authorization: Bearer <token>` |
| Batch | up to **10 resources** per request (all 8 of ours fit in one) |
| Free tier | **500 requests/month**, rate limit 100 req/60s |

How to get the token: <https://metrika.run/users/sign_up> (free, no card) →
dashboard → **API Tokens**. Put it in `.env` as `METRIKA_API_KEY`.

Resource map (identifiers are **bare** — no `@`, no URLs):

| Chart key | Metrika platform | Type | Identifier |
|---|---|---|---|
| `youtube` (summed) | `youtube` | `account` | `NixiePixel` (legacy `user/nixiedoeslinux`, 202k) |
| `youtube` (summed) | `youtube` | `account` | `NixiePixelGB` (vlog, 102k) |
| `twitter` | `x` | `account` | `NixiePixel` |
| `tiktok` | `tiktok` | `account` | `nixiepixel` |
| `facebook` | `facebook` | `account` | `nixiepixel` |
| `instagram` | `instagram` | `account` | `nixie.pixel` |
| `twitch` | `twitch` | `account` | `nixiepixel` |
| `discord` | `discord` | `community` | `geekbeacon` (vanity code) |

**Quota math:** one request per refresh × 1 refresh/day = **~30/month (6% of
the free 500)**. Metrika refreshes its own datapoints daily, so a faster cadence
buys no freshness. The worker also skips its boot refresh when the stored data
is newer than the interval, so restarts and deploys do not spend requests.

**Observed live values (2026-10-25):** youtube 202,000 + 102,000 = **304,000**,
x 26,000, tiktok 3,616, facebook 41,655, instagram **18,000**, twitch 2,595,
discord **5,425** (matches `discord.gg/geekbeacon` exactly).

Gotchas that cost time if forgotten:

- **No `@` prefix, no URLs.** `@nixiedoeslinux` is rejected with `422`.
- **A single invalid identifier rejects the entire batch** (`422`). The fetcher
  retries one resource at a time, logs the offender, and quarantines it until
  restart so later cycles stay at one request per refresh.
- **`nixiedoeslinux` is not a Metrika identifier.** The legacy channel resolves
  as `NixiePixel`; using the old name leaves it `pending` forever.
- **Discord only accepts vanity codes.** Generated invites are rejected. If the
  server ever loses its vanity URL, set `METRIKA_DISCORD_VANITY=""` and the
  Discord bot/other fallback takes over.
- **Facebook `followers` is page _likes_** (the metric carries
  `semantic: "likes"`). Fine for a growth chart, just not literal followers.
- **First lookup is `pending`.** New resources are collected in the background
  and return no datapoint on the first request; the chart keeps the seed /
  last-known-good value until collection completes (observed within minutes).
- Error codes: `401` bad token, `403` monthly quota exhausted (message says when
  it resets), `429` rate limited, `422` validation.

**Source precedence.** With `METRIKA_API_KEY` set, Metrika is primary and the
per-platform adapters below only run for platforms Metrika did not answer that
cycle (gap-fill). With no key, behaviour is unchanged: official APIs where
configured, Pulse scraper for the rest.

### 1.1 YouTube — optional fallback (official, fully free)

| Setting | Value |
|---|---|
| Env vars | `YOUTUBE_API_KEY`, `YOUTUBE_CHANNELS` |
| Metric | Subscribers (`statistics.subscriberCount`), **summed across channels** |

Nixie's YouTube audience is split across **two distinct channels**, so the
llama chart's single "youtube" slice is the combined total:

- <https://www.youtube.com/@NixiePixelGB> ("NixiePixel")
- <https://www.youtube.com/user/nixiedoeslinux> ("NixieDoesLinux")

Configure them comma-separated (default already matches):

```
YOUTUBE_CHANNELS="NixiePixelGB,user/nixiedoeslinux"
```

Each entry can be `@handle`, `user/legacy-name`, a bare handle/username, or a
`UC...` channel ID. The fetcher resolves each channel and returns the sum. If
**any** configured channel fails to resolve, it returns an error instead of a
partial sum, so the last-known-good combined total is kept (a partial total
would otherwise look like an audience drop).

How to obtain the key:

1. Go to <https://console.cloud.google.com/> and create a new project (e.g. `nixie-social`).
2. In the project, open **APIs & Services → Library**, search for **"YouTube Data API v3"** and click **Enable**.
3. Open **APIs & Services → Credentials → Create Credentials → API key**.
4. (Recommended) Click the new key → **Restrict key** → "API restrictions" → restrict to *YouTube Data API v3*.
   No OAuth is needed — channel statistics are public and require only the plain API key. The key does
   **not** need to belong to Nixie's Google account; any Google account's key can read public stats.

Quota math: default project quota is **10,000 units/day**; `channels.list` costs **1 unit**.
Two channels × up to 2 selector attempts = ≤4 units/hour ≈ **96 units/day**. Effectively unlimited.

Caveat: YouTube rounds each channel's subscriber count to 3 significant figures,
so the combined total has slightly wider rounding error (±~1k). Irrelevant for the chart.

### 1.2 Twitch — optional fallback (official, fully free)

| Setting | Value |
|---|---|
| Env vars | `TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET` |
| Metric | Followers (`channels/followers.total`) |

How to obtain:

1. Go to <https://dev.twitch.tv/console/apps> (any free Twitch account) → **Register Your Application**.
2. Fill in: Name (e.g. `nixie-social`), OAuth redirect URLs (required field; `http://localhost:3000`
   is fine since we only use app tokens), Category = *Website Integration*, Create.
3. On the app's **Manage** page click **New Secret**. You now have Client ID + Client Secret.
4. The worker obtains an **app access token** itself (no manual step):
   `POST https://id.twitch.tv/oauth2/token?client_id=...&client_secret=...&grant_type=client_credentials`
   Tokens expire — refresh when expired (they typically last weeks; handle 401 → re-auth).
5. Resolve `broadcaster_id` once via `GET https://api.twitch.tv/helix/users?login=nixiepixel`
   (Client-Id header + app token), then cache it.

Rate limits: 800 points/min bucket; one follower-count call/hour is trivial.
"Total count only" is all we need — the full follower list requires a broadcaster
moderator token, which we deliberately skip.

### 1.3 Discord — optional fallback (official, free)

| Setting | Value |
|---|---|
| Env vars | `DISCORD_BOT_TOKEN`, `DISCORD_GUILD_ID` |
| Metric | Server member count (`approximate_member_count`; no true "followers") |

How to obtain:

1. Go to <https://discord.com/developers/applications> → **New Application** (e.g. `nixie-social`).
2. In the **Bot** tab click **Reset Token** and copy the token (shown only once).
3. No privileged intents are required for the REST endpoint we use
   (`GET /guilds/:id?with_counts=true` returns `approximate_member_count`).
4. Invite the bot into the GeekBeacon/Nixie Discord server:
   **OAuth2 → URL Generator** → scopes `bot` → permissions *View Channels* → open the URL → authorize.
5. Get the **guild (server) ID**: enable Developer Mode in Discord
   (User Settings → Advanced), right-click the server icon → **Copy Server ID**.

Note: this is member count, which is what the current placeholder `6336` represents,
not "followers" (Discord has no follower concept).

### 1.4 TikTok / Instagram / Facebook / X (Twitter) — Pulse fallback (no keys)

| Setting | Value |
|---|---|
| Env vars | `SOCIAL_SCRAPE_URL` (default: Pulse, see below) |
| Metrics | Followers per platform profile |

No keys are needed if we use the aggregator route. The chart's X / Instagram /
Facebook / TikTok numbers come from a small server-side fetch of each public profile.

Primary (free, no key, per our research): **Pulse** — `GET <base>/profile` with a
profile URL returns followers/following/post count/verified for
YouTube, TikTok, Instagram, X, Facebook, Bluesky.
- Free for personal/indie use; Pro $19/mo only covers commercial use — GeekBeacon is a
  nonprofit community project, so the free tier is the intended lane. Confirm the exact
  endpoint and response shape during implementation (docs: <https://pulse.walls.sh/>).

Fallback design (resilience): if the aggregator is down or the parser breaks
(these services inevitably drift when platforms change their front end), the DB keeps the
last-known-good value and `GetSocialCounts` keeps serving it. The existing hardcoded
seed values are loaded at migration time, so the chart never goes empty.

**Optional later upgrades (Phase 2, not needed for v1):**

- **X / Twitter official API** — console.x.com pay-per-use; "owned reads" of the
  @NixiePixel account cost $0.001/resource (~$0.03/month at hourly). No free tier.
- **Meta (Instagram/Facebook)** — free & reliable once approved, but requires a
  <https://developers.facebook.com> app + Meta app review (Page Public Content Access
  / `instagram_basic`) and an Instagram Business account. High friction, low urgency.

### 1.5 Summary table

| Platform | Source | Credentials | Where to get it | Free? |
|---|---|---|---|---|
| **All 7** | **Metrika (primary)** | `METRIKA_API_KEY` | metrika.run/users/sign_up | ✅ |
| YouTube | Official v3 API (2 channels summed) — fallback | `YOUTUBE_API_KEY`, `YOUTUBE_CHANNELS` | console.cloud.google.com | ✅ |
| Twitch | Official Helix — fallback | `TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET` | dev.twitch.tv/console | ✅ |
| Discord | Official REST (bot) — fallback | `DISCORD_BOT_TOKEN`, `DISCORD_GUILD_ID` | discord.com/developers | ✅ |
| TikTok | Pulse scrape — fallback | none (`SOCIAL_SCRAPE_URL` optional) | — | ✅ |
| Instagram | Pulse scrape — fallback | none (official route = Meta app review) | — | ✅ |
| Facebook | Pulse scrape — fallback | none (official route = Meta app review) | — | ✅ |
| X/Twitter | Pulse scrape — fallback | none (official route = pay-per-use) | — | ✅ |

Only **one** credential is needed for the full set: `METRIKA_API_KEY`. Everything
below it is an optional gap-filler.

### 1.6 Account ownership / linking requirements

| Credential | Needs Nixie's account? | Notes |
|---|---|---|
| `YOUTUBE_API_KEY` | ❌ No | Any Google account's project works; public stats only. The **channels** being read need no linking either. |
| Twitch app | ❌ No | Public follower totals only; the app owner need not be the broadcaster. (A broadcaster moderator token would be needed only for the full follower *list*.) |
| Discord bot | ⚠️ Invite only | App can be registered under any account, but the bot must be invited into the target server — requires *Manage Server* there (Nixie or a GeekBeacon admin). |
| Pulse scraper | ❌ No | No account at all; reads public profile pages. |
| X official (Phase 2) | ❌ No | "Owned reads" pricing ($0.001) applies to your own account; an app under Nixie's account makes it cheapest. Linking not required. |
| Meta IG/FB (Phase 2) | ✅ Yes | Graph API requires the app to be connected to her IG Business account / Page, plus app review. |

Practical tip: if the site runs under GeekBeacon's infrastructure, register the
Google Cloud project, Twitch app and Discord app under an org-controlled
account so credentials survive personal account changes.

---

## 2. Implementation plan

### Phase 0 — groundwork
1. `cp env.template .env` additions (keys above) + document in `env.template` as comments.
2. Verify: `mage dbup`, `.env` present; build baseline is green (`go build ./cmd/server`).

### Phase 1 — DB layer
1. `mage migratecreate social_metrics`:
   ```sql
   -- +migrate Up
   CREATE TABLE IF NOT EXISTS social_metrics (
       platform     TEXT PRIMARY KEY,
       followers    BIGINT NOT NULL,
       refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   INSERT INTO social_metrics (platform, followers) VALUES
       ('youtube', 330000), ('twitter', 31102), ('twitch', 2500),
       ('discord', 6336), ('facebook', 50466), ('instagram', 7498),
       ('tiktok', 0)
   ON CONFLICT DO NOTHING;

   -- +migrate Down
   DROP TABLE social_metrics;
   ```
2. `db/queries/queries.sql`: add
   - `GetSocialCounts` → `SELECT platform, followers FROM social_metrics;`
   - `UpsertSocialCount` → `INSERT ... ON CONFLICT (platform) DO UPDATE SET followers=$2, refreshed_at=now();`
3. `mage migrate && mage generate` (needs dev DB running). Wire results into
   `internal/domain` types, `internal/adapters/db/adapter.go` (`GetSocialCounts` now
   reads the table; add `UpsertSocialCount`), and sqlc regenerates the rest.
4. Keep `DBService.GetSocialCounts(ctx) (map[string]int, error)` signature unchanged —
   adapter converts rows → `map[string]int` (lowercase keys as today).

### Phase 2 — fetcher adapter
New package `internal/adapters/social/`:
- `social.go` — `Client` interface, `FetchCounts(ctx) map[string]int` running each
  configured platform concurrently; per-platform failures logged, never fatal.
- `youtube.go` — 1 unit/call as documented above.
- `twitch.go` — app-token flow with expiry, cached in memory; 401 → re-auth and retry once.
- `discord.go` — bot token + `with_counts=true`.
- `scrape.go` — `SOCIAL_SCRAPE_URL` provider, one profile URL per platform
  (TikTok, Instagram, Facebook, X); placeholder URLs currently on the social page:
  `@nixiepixel` (TikTok), `nixie.pixel` (IG), `www.facebook.com/nixiepixel`,
  `NixiePixel` (X).
- A client is a nil interface when its env vars are missing → platform silently skipped.

### Phase 3 — worker & wiring
1. `cmd/server/main.go`: start a goroutine when a fetch is possible (any non-nil client):
   fetch on boot, then `time.Ticker` hourly; upsert via `dbService`.
   - Guard: if/if `DATABASE_URL` is nil dbService behavior is untouched — endpoint keeps
     hardcoded fallback (documented regression-free path for no-DB local runs).
   - Per-platform `sync.Mutex` against overlapping ticked runs.
2. `internal/config/config.go`: add the new fields (they will be picked up automatically
   by the lowercased env var loader; add `.env` template comments).

### Phase 4 — frontend
1. `static/js/stats.js`: point the fetch at `/api/v1/social/count/all` (same-origin);
   keep remote `social.geekbeacon.org` + hardcoded data as the fallback chain.
2. Optional while here: add TikTok as a 7th llama slice (*+ color entry*); bump `?v=`
   cache busters on `stats.js` in `internal/templates/layout.templ`.

### Phase 5 — verification
- `go test ./...`, `gofmt`, `go vet ./...` (no CI tests — must run locally; CI only builds Docker image).
- Manual: `mage dev`, hit `/api/v1/social/count/all`, confirm real numbers + `refreshed_at`
  advancing; kill a credential and confirm chart keeps last-good values.

### Risks / notes
- **Aggregator drift** (biggest risk): mitigated by last-good caching + easy one-file
  swap-out of `scrape.go` provider via `SOCIAL_SCRAPE_URL`.
- **Google Cloud account** can flag the YT API project for verification — using key
  restriction and normal usage keeps this a non-issue.
- **Twitch app token expiry** must be handled programmatically (re-auth on 401).
- **Meta/X official routes** deferred to Phase 2 (dotenv-gated later swap).
- Note: `social.geekbeacon.org` should eventually be pointed at this same endpoint or
  retired; not in scope for v1.

---

## 3. v1 status (implemented 2026-10-24)

- Migration `002_social_metrics.sql` + sqlc queries (`GetSocialCounts`,
  `UpsertSocialCount`) live; run against the dev DB (goose version 2).
- Fix applied along the way: the `goose` CLI (mage) now uses
  `-table nixie.goose_db_version` to match the app's runtime migrations table,
  which was previously tracked in `public.goose_db_version`.
- Live worker results with NO credentials configured (scraper-only run):
  `twitter=26028`, `facebook=41655`, `tiktok=3615` landed as real values in
  `nixie.social_metrics`; instagram stays login-walled (keeps 7498);
  youtube/twitch/discord keep last-good until their env creds are added.
  Note: the Go-side scraper reaches X fine even where a browser/curl gets a
  Cloudflare interstitial — the earlier curl failure was client-side.
- `stats.js` now hits same-origin `/api/v1/social/count/all` → GeekBeacon
  remote → hardcoded `fallbackData`, merging sources over the fallback so
  TikTok charts even if a remote omits it. TikTok added as 7th llama slice.
- Unit tests: `internal/adapters/social/social_test.go` covers YouTube
  (handle/username/ID selectors), Twitch (token flow + headers), Discord
  (auth + URL shape), Pulse scraper (parse, in-band errors, wrapper,
  aggregator merge). DB-backed queries require a live DBTX (documented).
- YouTube is a **combined total**: `YOUTUBE_CHANNELS` default
  `NixiePixelGB,user/nixiedoeslinux` → 102,000 + 202,000 = **304,000** seed.
  Fetch is all-or-nothing: if either channel fails, the last-known-good sum
  is kept rather than charting a partial (misleading) drop. Multi-channel
  sum and failure cases have unit tests.
- `/social` now shows a second YouTube card ("YouTube Vlog") linking
  <https://www.youtube.com/@NixiePixelGB>. The llama's YouTube slice remains
  the combined total of both channels (304k seed); the card is link-only and
  does not affect counts. The FAQ links both channels as well.

### 3.1 Metrika consolidation (implemented 2026-10-25)

- New `internal/adapters/social/metrika.go`: one batched
  `POST /api/v1/resources/search` covers all 8 resources (7 platforms). YouTube
  is summed all-or-nothing; every other platform is a single resource.
- Per-resource `status` is respected: `pending`, `stale` and `unavailable`
  omit that platform for the cycle (the DB keeps last-known-good).
- `Aggregator` gained `BulkFetcher` support (`NewAggregatorWithBulk`): bulk
  sources run first, per-platform fetchers only fill the gaps.
- `422` handling: because Metrika rejects the whole batch on one bad
  identifier, the fetcher re-runs resources individually, quarantines the
  offender until restart, and keeps the remaining platforms live.
- Cadence default is now `SOCIAL_REFRESH_SECONDS=86400` (~30 requests/month).
  The worker skips its boot refresh when `max(refreshed_at)` is newer than the
  interval (`LastSocialRefresh` sqlc query), so restarts cost no quota.
- Migration `002` seeds were corrected to the verified values and stamped
  `refreshed_at = 'epoch'` so a fresh install fetches immediately.
- Tests: single-batch parse + YouTube sum, unavailable platform, YouTube
  partial failure, 422 isolation + quarantine, 403 quota, missing key, and
  bulk precedence / gap-fill.

### 3.2 Resilience & monitoring

**Panic containment.** In Go a panic in *any* goroutine takes down the whole
process, so every fetch path is wrapped:

- each per-platform fetch goroutine and the Metrika bulk call recover, log the
  panic with a stack, and report that platform as failed;
- each worker cycle has its own recover, so one bad cycle cannot stop the ticker;
- the worker loop is supervised — if it ever returns or panics while the context
  is still live, it restarts with exponential backoff (1m, 2m, 4m … capped at 1h).

Note there is no long-lived per-platform goroutine to "resurrect": each cycle
spawns short-lived fetches that are retried on the next cycle. The supervisor
protects the loop that drives them.

**Health endpoint.** `GET /api/v1/social/health` (always HTTP 200):

```json
{
  "status": "ok",
  "checked_at": "2026-10-25T02:45:00Z",
  "interval_seconds": 86400,
  "stale_after_seconds": 172800,
  "stale_platforms": [],
  "platforms": {
    "facebook": {"followers": 41655, "refreshed_at": "...", "age_seconds": 3600, "stale": false}
  }
}
```

- `status` is `degraded` when any platform's `refreshed_at` is older than
  **2 × interval** (48h at the daily default), or when the table is empty.
- HTTP stays 200 so a stale feed is never confused with a dead server; `503` is
  reserved for "database not configured or unreachable".
- `refreshed_at` advances on every successful write, so the endpoint also
  detects a dead worker: all platforms go stale together.

Quick checks:

```
curl -s localhost:8080/api/v1/social/health | jq '.status, .stale_platforms'
docker ps --format '{{.Names}}\t{{.Status}}'
```

**Infrastructure.** `docker-compose.yaml` sets `restart: unless-stopped` and a
healthcheck that greps the endpoint for `"status":"ok"`. A stale platform marks
the container `unhealthy` (visible in `docker ps` and to uptime monitors)
without forcing restarts, so a slow upstream cannot cause a restart loop.

**What this does not do:** a platform that fails every cycle is still retried
every cycle and its last-known-good value stays on the chart. The health endpoint
is what tells you it has been failing.

---

## 4. Post-v1 upgrades ("Phase 2") — official APIs for X and Meta

These are the deferred "official" routes for the platforms currently covered
by the free scraper (`scrape.go`). The scraper keeps working as a fallback in
all cases: official fetchers are additive, activated by config, and a
platform removed from `Profiles` stops being scraped.

### 4.1 X / Twitter official API

**Why:** the scraper is the most fragile link in the chain (Cloudflare/bot
challenges), and the chart's X slice is otherwise only as fresh as the last
successful scrape.

**Cost (pay-per-use, as of late 2026):**
- No free tier; new accounts get one-time starter credits ($20 + matched
  first recharge up to $50).
- $0.010 per user/follower read; **$0.001 for "owned reads"** (your own
  account's data) — reading @NixiePixel's own profile qualifies if the app is
  associated with that account.
- **Deduplication:** the same resource read again within a 24h UTC window is
  billed once. With an hourly worker that means ~1 billable read/day
  (~30/month) → roughly **$0.03/mo owned / $0.30/mo standard**.
- Set a spending cap in the console (defaults to unlimited).

**Setup:**
1. Register a developer account at <https://console.x.com>.
2. Create an app; note its **Bearer Token** (app-only auth is enough for
   public profile lookups).
3. Optionally associate the app with @NixiePixel's account to get the owned
   read rate.
4. Store as `TWITTER_BEARER_TOKEN` (+ optional `TWITTER_USERNAME`, default
   `NixiePixel`) in `.env`; add to `env.template`.

**Implementation:**
- New `internal/adapters/social/twitter.go` implementing `Fetcher` (`Key():
  "twitter"`):
  `GET https://api.x.com/2/users/by/username/{username}?user.fields=public_metrics`
  with `Authorization: Bearer …`; parse `data.public_metrics.followers_count`.
- Wire in `newSocialAggregator` only when the token is set.
- **Stop scraping X** when the official fetcher is active (skip the
  `"twitter"` entry when building `scrape.Profiles`) to avoid duplicate calls
  and noisy warnings.
- Tests: httptest with a canned profile payload; missing-token case.

**Effort:** small (½ day incl. tests/docs).

### 4.2 Meta (Instagram + Facebook Page)

**Why:** Instagram is currently login-walled through the scraper and simply
holds its seeded value (7,498); Facebook works via scraping but is likewise
fragile. The Graph API is free and authoritative.

**Hard requirement (the reason it's deferred):** the app must be linked to
Nixie's own Meta assets, and production access needs app review.

**Prerequisites (Nixie's side):**
- Instagram account converted to **Business/Creator** and linked to a
  Facebook Page (admin access required).
- Facebook **Page** for the follower count (`nixiepixel`).

**Setup:**
1. Create an app at <https://developers.facebook.com> (type: Business).
2. Add products **Facebook Login** + **Instagram Graph API**.
3. Permissions to request: `instagram_basic`, `pages_show_list`,
   `pages_read_engagement`; request **Page Public Content Access** if Page
   follower counts are gated.
4. While unreviewed, Graph API works **only for users with a role on the app**
   (i.e. you/Nixie) — enough for testing.
5. Submit **App Review** with: screencast of the use case, privacy-policy URL,
   data-use description. Expect days–weeks; rejection is common for
   "display follower counts" unless the screencast shows the live page.
6. Tokens: exchange the short-lived user token for a **long-lived (~60 day)**
   user token; derive the Page token from it. IG/FB counts then come from:
   - `GET https://graph.facebook.com/v21.0/{page-id}?fields=followers_count`
   - `GET https://graph.facebook.com/v21.0/{ig-user-id}?fields=followers_count`

**Token storage — use the existing schema, not `.env`:**
`config.social_app` (client_id/secret/callback) and `config.user_social`
(user_id/meta_data/access_token/secret_token) already exist for exactly this
and are unused by v1. Plan: add sqlc queries to upsert/read META tokens,
seed `social_app_type` with `FACEBOOK`/`INSTAGRAM` rows (already present),
and let a small refresh job replace the token when it is near expiry. Fallback
for a quick start: `META_ACCESS_TOKEN` in `.env`, manually refreshed every
~60 days.

**Implementation:**
- New `internal/adapters/social/meta.go` with two `Fetcher`s (`"instagram"`,
  `"facebook"`), sharing the graph client and token source.
- New config fields: `META_ACCESS_TOKEN`, `INSTAGRAM_BUSINESS_ID`,
  `FACEBOOK_PAGE_ID` (or DB-backed lookup).
- Remove `"instagram"`/`"facebook"` from `scrape.Profiles` when official
  fetchers are configured.
- Tests: httptest for both endpoints + token-expiry error surfacing.

**Effort:** small code, large external friction (app review/verification).
Do this only if Instagram freshness matters enough; the chart keeps a
last-known-good value in the meantime.

### 4.3 Other Phase 2 cleanups

- **Retire `social.geekbeacon.org`** from `stats.js` once the local worker has
  been stable for a few weeks (keep the hardcoded fallback).
- ~~**Staleness visibility**~~ — done in §3.2 (`/api/v1/social/health`, 2× the
  refresh interval marks a platform `degraded`).
- **Rotation runbook:** document how to rotate the Google API key, Twitch
  secret, Discord bot token and (if used) Meta/X tokens; all are `.env`-only
  except Meta if DB-backed.
- ~~**Optional debug endpoint**~~ — done: `refreshed_at` per platform is exposed
  by `/api/v1/social/health`.

### 4.4 Phase 2 acceptance criteria

- X slice sourced officially with a valid Bearer token; scraper no longer
  attempts X; unit tests cover the API payload.
- Instagram/Facebook sourced from Graph API with a token that survives
  restarts; Meta token refresh path documented/tested.
- No regression in fallback behavior: dropping any credential returns to the
  last-known-good DB value on the next worker tick.
- `go test ./...`, `go vet ./...`, `gofmt` clean; docs updated for new env vars.
