# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`rmfakecloud` is a self-hosted replacement for the reMarkable tablet's cloud: a single Go binary (gin HTTP server) that
implements the tablet-facing sync/auth/notification APIs, plus an embedded React web UI for managing documents and users.
Module path is `github.com/lspaya05/rmfakecloud-lite`; storage is plain files on disk, no database.

## Build / run / test

Everything goes through the `Makefile` (targets shell out to `pnpm` in `ui/` and to `go`).

```sh
make build      # UI (pnpm build -> ui/dist) then go build -> dist/rmfakecloud-x64
make all        # cross-compile x64, armv6, armv7, arm64, win64, docker
make run        # go run ./cmd/rmfakecloud (builds ui/dist first if stale); pass args via ARG=
make runui      # vite dev server on :3001, proxies /ui/api -> :3000
make testgo     # go test ./...
make test       # testui (currently a no-op stub) + testgo
make container  # docker build using Dockerfile.make on the prebuilt dist/rmfakecloud-docker
./dev.sh        # backend + UI dev loop; needs `entr`; sets JWT_SECRET_KEY=dev and local SMTP env
```

Single Go test: `go test ./internal/storage/fs/ -run TestName -v`.

**Gotcha:** `ui/assets.go` does `//go:embed dist/*` (guarded by `//go:build !ci`, but no `ci` alternative file exists), so
any `go build`/`go test` fails unless `ui/dist` exists. Run `make build` once, or create `ui/dist` with a placeholder file,
before invoking `go` directly.

UI lint: `cd ui && pnpm lint` (eslint 9 flat config, `ui/eslint.config.js`).

Docs are mkdocs-material (`mkdocs.yml`, `docs/`), published to GitHub Pages by `.github/workflows/mkdocs.yml`.

## Architecture

### Process wiring

`cmd/rmfakecloud/main.go` → `config.FromEnv()` (all configuration is environment variables, see the `env*` consts in
`internal/config/config.go`; `flag.Usage` prints the full list) → `cli.New(cfg).Handle(os.Args)` intercepts the
`setuser` / `listusers` / `rmuser` subcommands and exits → otherwise `app.NewApp(cfg).Start()`.

`internal/app/app.go` is the composition root. It builds `fs.NewStorage(cfg)` (one struct implementing
`DocumentStorer`, `BlobStorage`, `MetadataStorer`, `UserStorer` — interfaces declared in `internal/storage/storage.go`),
the notification `hub`, optional MQTT broker, screenshare `RoomManager`, HWR client, and mounts three route groups on one
gin engine:

- `internal/app/routes.go` — the **tablet-facing API** (device/user tokens, `/document-storage/json/2/*` for sync 1.0,
  `/sync/v2|v3|v4/*` and `/api/v1/signed-urls/*` for sync 1.5, integrations, HWR, email, screenshare, notifications WS).
- `internal/storage/fs/app.go` — the **blob/document transfer routes** (`/storage/:token`, `/blobstorage`) that the
  signed URLs handed out above point back to; they authenticate with an HMAC-signed token, not a JWT.
- `internal/ui/routes.go` — `/ui/api/*` for the web UI plus static serving of the embedded SPA (`NoRoute` falls back to
  `index.html` for anything that isn't `/api`/`/ui/api`).

### Two sync protocols coexist — this is the main structural split

Per-user flag `Sync15` on `model.User`. Everything downstream branches on it:

- **sync 1.0**: whole-document zip per doc, metadata in `.metadata` files. `internal/ui/backend10.go`.
- **sync 1.5 (a.k.a. sync15)**: content-addressed blobs + a hash tree, much less bandwidth. `internal/ui/backend15.go`,
  `internal/storage/fs/blobstore.go`, `internal/storage/models/hashtree.go`. Root generation numbers are tracked in
  `.root.history`, the tree is cached in `.tree`, and schema version 3 vs 4 is selectable via `HASH_SCHEMA_VERSION`.

The tablet's JWT scopes (`sync:tortoise` / `sync:fox`) decide the version for API requests — `app.authMiddleware()`
sets `SyncVersion` in the gin context (`internal/app/middleware.go`). For the web UI the user record decides, and
`internal/ui/ui.go` picks the matching `backend` implementation from a map keyed on `common.Sync15`/`common.Sync10`.
When adding a feature that touches documents, check whether both backends need it.

### On-disk layout

`$DATADIR` (default `./data`) → `users/<uid>/` holds the user profile and sync-1.0 documents; `users/<uid>/sync/` holds
sync-1.5 blobs, `root`, and `.root.history`. UIDs and file names are passed through `common.Sanitize*` before hitting the
filesystem — keep doing that for any new path construction.

### Other packages

- `internal/messages` — wire types for the tablet API (`auth0.go` holds the JWT claim shapes).
- `internal/integrations` — pluggable remote storage / messaging (localfs, ftp, webdav, dropbox, ics calendar, webhook)
  behind the `integrations.go` interface; per-user config lives in the user profile.
- `internal/storage/exporter` — PDF rendering/annotation export via unipdf + rmapi; `license.go` uses `go:linkname` to
  inject a community license key into unipdf, don't "clean it up".
- `internal/app/hub` — websocket fan-out of sync notifications to connected devices/browsers.
- `internal/mqtt`, `internal/screenshare` — newer-firmware notification transport and WebRTC signalling (ICE servers via
  `ICE_SERVERS`).
- `internal/hwr` — MyScript handwriting-recognition proxy.

### UI

`ui/` is Vite + React 18 (JSX, some TS), react-router v5, react-bootstrap. `ui/src/services/api.service.js` wraps the
`/ui/api` calls; auth state via `ui/src/common/useAuthContext.jsx`. Pages under `ui/src/pages/` map to the feature areas
(Documents, Admin, Integrations, Connect, ScreenShare, Profile).

## Conventions worth knowing

- Logging is `logrus` aliased as `log`; request/body tracing only at trace level, and paths in `dontLogBody`
  (`internal/app/middleware.go`) are never body-logged — add upload-style endpoints there.
- Adding config means adding an `env*` const, parsing in `FromEnv`, and a line in `EnvVars()` so `--help` stays accurate;
  document it in `docs/install/configuration.md`.
- Shell scripts in `test/` are manual smoke tests against a running server (`test/common.env` + `ui_*.sh`, `poc.hurl`),
  not part of `make test`.
- CI (`.github/workflows/go.yml`) runs `make build`, `make testui`, `make testgo` on Go ^1.23 / Node 21 / pnpm 9;
  tagged `v*.*.*` pushes run `make all` (release binaries) and `.github/workflows/ghcr.yml` (multi-arch image to
  `ghcr.io/lspaya05/rmfakecloud-lite`).
- This is a fork of `ddvk/rmfakecloud`. Self-references (module path, docs site, clone URLs, image) point at
  `lspaya05/rmfakecloud-lite`; the remaining `ddvk` links are deliberate — upstream issues/PRs that don't exist in this
  fork's numbering, and the separate `ddvk/rmfakecloud-proxy` / `ddvk/rmapi` projects. Don't "fix" those.
