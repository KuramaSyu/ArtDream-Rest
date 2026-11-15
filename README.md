# ArtDream

A playful REST service and website for photography / digital art. People
submit artwork, vote in image duels (classic winner/loser or a fine-grained
AHP scale), and the site paints itself with colours pulled from the artwork on
screen.

- **Backend:** Go + Gin + SQLite (pure-Go driver), versioned API at `/api/v1`.
- **Frontend:** Vite + TypeScript + React + MUI + node-vibrant.
- **Theme generation:** adapted from
  [KuramaSyu/WerSu-Frontend](https://github.com/KuramaSyu/WerSu-Frontend)'s
  `imageTheme` / `CustomThemeImpl`, with the spacing scale adjusted to MUI's
  standard linear tokens.

```
/
├── backend/        Go API, image processing, SQLite
├── frontend/       React SPA
├── docker-compose.yaml
└── Request.md
```

## Quick start (Docker)

```bash
docker compose up --build
```

- Website: http://localhost:8082
- API: http://localhost:8080/api/v1
- SQLite database and uploaded originals live in the `artdream-data` volume.
- Override ports if they are taken: `BACKEND_PORT=9000 FRONTEND_PORT=9001 docker compose up --build`.
- The compose services carry Traefik labels for `artdream.inu-the-bot.com`
  (frontend, port 80) and `artdream-api.inu-the-bot.com` (backend, port 8080);
  attach the services to your Traefik network if it is not the default one.
  Watchtower is disabled for both.

On first boot there is no account. The landing page asks you to create the
**first account**, which automatically becomes the administrator. After that,
regular accounts can self-register; only admins can grant admin rights.

## Local development

Backend:

```bash
cd backend
go run .
# listens on :8080, data in ./data
```

Frontend:

```bash
cd frontend
pnpm install
pnpm dev
# http://localhost:5173, proxies /api to http://localhost:8080
```

Production frontend build:

```bash
cd frontend
pnpm build          # dist/
cd ../backend
FRONTEND_DIR=../frontend/dist go run .
# the Go server then serves the SPA too
```

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | Backend listen address |
| `DATA_DIR` | `./data` | Base data directory |
| `DB_PATH` | `$DATA_DIR/artdream.db` | SQLite file |
| `UPLOAD_DIR` | `$DATA_DIR/images` | Pristine uploads |
| `CACHE_DIR` | `$DATA_DIR/cache` | Rendered derivative cache |
| `FRONTEND_DIR` | auto (`../frontend/dist`) | Optional SPA directory served by Go |
| `ALLOWED_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | CORS allow-list (`*` allowed) |
| `GIN_MODE` | | Set to `release` for production |
| `BACKEND_PORT` | `8080` | Compose host port for the API |
| `FRONTEND_PORT` | `8082` | Compose host port for the website |
| `VITE_API_TARGET` | `http://localhost:8080` | Vite dev proxy target |
| `VITE_API_BASE` | `/api/v1` | Frontend API base at build time |

SQLite uses WAL mode. Uploads are validated, limited to 30 MiB and 120
megapixels, and currently **must be landscape** (width > height).

## Users & auth

- `POST /users/bootstrap` — only works while zero users exist; creates the first
  **admin** and returns a session.
- `POST /auth/register` — public self-registration, always non-admin, disabled
  until the first account exists.
- `POST /auth/login` — username + password, returns a bearer token.
- `POST /auth/logout`, `GET /auth/me`, `PATCH /auth/me` — session profile and
  own password change (current password required).
- Admin-only: `GET/POST /users`, `PATCH/DELETE /users/:id`. The last admin
  cannot be demoted/deleted; you cannot delete your own account.

Session tokens are stored in SQLite and sent as
`Authorization: Bearer <token>` (or `X-Session-Token`).

## Submissions & anonymous claims

The submit page (`/submit`) requires an **artist name**; the title is optional.
The artist name is independent from the account username.

- Logged in: the upload is linked to the account's `user_id` (`uploaded_by`).
- Logged out: the upload is still stored, but anonymously. The API returns a
  secret `claimToken`, and the client stores `{id, claimToken, author, title}`
  in `localStorage`. Later, after signing in or registering, the UI offers to
  attach those submissions to the account (`POST /images/claim`). Claim tokens
  are cleared once used, so knowing a public image id is not enough to claim it.
- The artist name and title chosen at upload time never change when claiming.

## Ranking model

Two tables hold the extensibility:

- `vote_modes` — named scales (`duel`, `ahp5`, `ahp100`, plus admin-created
  modes) with `min_value`, `max_value`, `is_binary`.
- `votes` — mode id/name, `left_image_id`, `right_image_id`, derived
  `winner_image_id` / `loser_image_id`, raw `value`, the effective
  `min_value`/`max_value`, a normalized `weight`, `user_id`, `username`,
  metadata JSON, duration and timestamp.

Modes can be edited or extended at runtime; each vote stores the scale actually
used, so changing a mode's default later does not rewrite history.

Outcome derivation:

- Binary (`duel`): body `winner` may be `left`, `right`, `tie`, or an image id.
- AHP (`ahp5`, `ahp100`, custom): low values favour the left image, high values
  favour the right image. `mid = (min + max) / 2`; values within ±0.5 of the
  midpoint are recorded as a draw. Otherwise the winner gets
  `weight = |value - mid| / ((max - min) / 2)` (0…1), so a hesitant vote counts
  less than a decisive one.
- Rank aggregation sums the winner's weight; draws add 0.5 to both sides.
  Entries are ordered by score, wins, win rate, then votes.

Time ranges use rolling windows: `day` = 24h, `week` = 7d, `month` = 30d,
`year` = 365d. `custom` accepts `seconds=N`, or `from`/`to` as RFC3339 or unix
seconds. `all` covers everything.

## API overview

All routes are under `/api/v1`. Responses are JSON except image bytes.

### Public

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/health` | Liveness |
| GET | `/config` | `{needsBootstrap, userCount}` |
| GET | `/frontend-config` | Server-backed frontend display config, e.g. `{rankingAnonymized}` |
| GET | `/images/random?amount=2&author=&exclude=id,id` | Bare array of images |
| GET | `/images/author?author=Ada&amount=&offset=` | `{items, total, author}` |
| GET | `/images/rank?time=day&amount=10&offset=0` | Ranked winners; also `seconds`, `from`, `to`, `author`, `titleRegex`, `mode` |
| GET | `/images/:id?format=webp|png|jpg&w=&h=&quality=&download=true&original=true` | Image bytes; default WebP capped to 1920px long edge / 1080p. `original=true` opts into the pristine upload |
| GET | `/images/:id/meta` | Image metadata |
| GET | `/rank/modes` | Available vote modes |
| POST | `/images` | Multipart upload (`image`, `author`, optional `title`). Auth optional. Landscape only. |

Image render example:

```bash
# Default: WebP, capped to a display-friendly size (no huge originals)
curl -o dream.webp "http://localhost:8080/api/v1/images/<id>"

# Display sizes used by the site (pre-warmed on upload)
curl -o card.webp  "http://localhost:8080/api/v1/images/<id>?w=640&quality=78"
curl -o rank.webp  "http://localhost:8080/api/v1/images/<id>?h=720&quality=82"
curl -o home.webp  "http://localhost:8080/api/v1/images/<id>?h=1080&quality=85"

# Explicit conversions and an explicit original download
curl -o dream.png  "http://localhost:8080/api/v1/images/<id>?format=png&w=1200"
curl -o crop.jpg   "http://localhost:8080/api/v1/images/<id>?format=jpg&w=800&h=600&quality=85"
curl -o original   "http://localhost:8080/api/v1/images/<id>?original=true&download=true"
```

Single-dimension requests never upscale; they clamp to the source size. The
default endpoint also avoids returning multi-megapixel originals: normal page
requests stay at WebP display sizes. `original=true` is the deliberate escape
hatch. On upload, the backend warms WebP variants (360, 640, 720p, 1080p) in the
background so the first gallery/ranking/home view is a cache hit.

Upload response:

```json
{
  "image": {
    "id": "...",
    "url": "/api/v1/images/...",
    "author": "Ada",
    "title": "Dreaming in Green",
    "width": 2400,
    "height": 1600,
    "...": "..."
  },
  "claimToken": "only present for anonymous uploads"
}
```

### Authenticated

| Method | Path | Notes |
| --- | --- | --- |
| POST | `/images/claim` | `{claims:[{id, token}]}` — link anonymous uploads to the account |
| POST | `/rank/vote` | See below |
| GET | `/rank/me?time=&amount=&offset=` | The signed-in user's votes |

Vote request (flexible key spellings are accepted — `leftImageId`, `left-img`,
`left_img`, `left`, …):

```json
{
  "mode": "ahp5",
  "leftImageId": "abc",
  "rightImageId": "def",
  "value": 2,
  "minValue": 1,
  "maxValue": 5,
  "metadata": { "ui": "image-click", "side": "left" }
}
```

```json
{ "mode": "duel", "left-img": "abc", "right-img": "def", "winner": "right" }
```

`minValue`/`maxValue` are optional overrides stored with the vote.

### Admin

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/admin/votes` | Filters: `time`, `seconds`, `user`, `author`, `titleRegex`, `mode`, `imageId`, `amount`, `offset` |
| PATCH | `/admin/votes/:id` | Change mode/winner/value/min/max/metadata |
| DELETE | `/admin/votes/:id` | Delete one vote |
| DELETE | `/admin/votes` | Delete every vote matching the same filters (`ids=1,2` also supported) |
| GET | `/admin/images` | Filters: `author`, `titleRegex`, `amount`, `offset` |
| PATCH | `/admin/images/:id` | Change author/title |
| DELETE | `/admin/images/:id` | Delete image; `keepVotes=true` preserves votes |
| DELETE | `/admin/images` | Bulk delete by `ids` or filters; `keepVotes=true` optional |
| POST | `/admin/rank/modes` | Create mode |
| PATCH | `/admin/rank/modes/:id` | Update mode |
| DELETE | `/admin/rank/modes/:id` | Delete mode (blocked while votes use it) |
| PATCH | `/admin/frontend-config` | `{rankingAnonymized: true|false}` — global ranking author masking |

Admin filter example — delete all votes from the last week for author “Ada”
with titles matching `/green/i`:

```bash
curl -X DELETE \
  "http://localhost:8080/api/v1/admin/votes?time=week&author=Ada&titleRegex=green" \
  -H "Authorization: Bearer $TOKEN"
```

## Frontend notes

- **Landing:** winners for the chosen period (day by default) drift behind a
  minimal hero at 1080p. A dim period switcher lives in the bottom-right corner
  next to a small backdrop-resolution menu (720p / 1080p / 1440p / original).
  With no votes yet it falls back to random images.
- **Rank:** two images shown at 720p, with a fullscreen button for a larger
  view. Click one for a gentle 2/5 or 4/5 vote, hover to nudge the long slider,
  or drag the 1–100 slider yourself. Arrow keys vote when nothing interactive
  is focused; Enter submits the slider. `Skip pair` avoids the current pair.
- **Admin:** rankings (filters, edit, delete, bulk delete), images (edit,
  delete, bulk delete), people (create, edit, grant/revoke admin, delete),
  and vote-mode management. The header has a global
  **Anonymize ranking / Reveal ranking authors** button. It writes the
  server-backed frontend config (`PATCH /admin/frontend-config`), so every
  visitor sees the same choice. Ranking surfaces (rank page, fullscreen,
  landing attribution, alt text) hide all authors while the API keeps
  returning them unchanged.
- **Theme:** whenever the set of displayed image URLs changes, the host picks
  one at random and rebuilds a full MUI theme from a 640px sample (Vibrant is
  deferred until the page has painted). Swatches are cached in `localStorage`;
  the sparkle button in the header repaints from another on-screen artwork.
  Pages rely on the generated `background.default` / `paper` tokens plus a
  static colour wash — there is no full-screen background image or large
  backdrop blur behind the app.
- **Submit:** bulk landscape upload (up to 12 files, 30 MiB each), one shared
  artist name, optional per-image titles, anonymous local claim list, and a
  prompt to attach pending submissions after sign-up. Uploads run with three
  concurrent requests and show per-batch progress. Empty titles are simply
  omitted — no "Untitled" placeholders anywhere.
  MUI spacing tokens use the standard 0.25rem scale.

## Development checks

```bash
cd backend && gofmt -l . && go vet ./... && go build ./...
cd frontend && pnpm typecheck && pnpm build
```

## Known limitations / next steps

- Landscape enforcement is intentional for now.
- Public registration has no email verification or rate limiting; add a
  reverse-proxy rate limit for a public deployment.
- Ranking is aggregated in Go over the filtered vote row set. For very large
  vote histories this should become a materialized stats table or SQL
  aggregation.
- Image derivatives are cached on disk; the cache is invalidated per image on
  delete, and keyed by source identity + render options.