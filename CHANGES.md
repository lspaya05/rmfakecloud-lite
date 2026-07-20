# rmfakecloud-lite — changes from upstream rmfakecloud

This fork reshapes [ddvk/rmfakecloud](https://github.com/ddvk/rmfakecloud) into a **headless**
"lite" service. The goal was to keep everything the tablet needs to pair and sync, plus a few
admin capabilities, while removing the web UI and peripheral features — changing as little of
the retained code as possible.

## What was kept

- **Device registration / pairing** for all reMarkable devices.
- **File sync 1.0** and **1.5/2/3/4** (diff sync / blob storage), stored under `DATADIR`
  exactly as before.
- **Screen sharing** (WebRTC; the server only relays signaling, video is peer-to-peer).
- **Calendar (ICS)** integration.
- **Passcode (PIN) reset** approval flow (reMarkable 1 / 2).
- **User management CLI** (`setuser` / `listusers`).

## What was removed

- **Web UI** — the React app in `ui/` and the `internal/ui` package.
- **Email / SMTP** (`internal/email`) and the "send document by email" endpoints.
- **Handwriting recognition** (`internal/hwr`) and its endpoints.
- **PDF exporter** (`internal/storage/exporter`) and the document-export code paths in the
  storage layer — raw `DATADIR` storage is sufficient without on-sync PDF rendering.
- **Storage integrations**: Dropbox, WebDAV, FTP, and local filesystem providers.
- **Messaging integrations / webhooks** (Slack, generic webhook).
- **Browser-extension endpoints** (`/doc/v1|v2/files`).

## What was relocated

- `internal/ui/viewmodel` → `internal/viewmodel` (still used by `cmd/history2git15`).
- The web-UI admin actions (pairing code, passcode resets, calendar CRUD, screen-share
  signaling) → a headless JSON **admin API** in `internal/app/admin.go`.

## Admin API

A new JSON API under `/admin`, protected by a static bearer token
(`RM_ADMIN_API_TOKEN`; when unset the routes are not registered and a startup warning is
logged). It is meant to be driven with `curl` or a small local microservice — there is no
frontend. It covers:

- **Pairing** — `GET /admin/users/:userid/newcode`
- **Passcode resets** — list / approve / dismiss
- **Calendar (ICS) integrations** — CRUD (only the `ics` provider is accepted)
- **Screen-share viewer signaling** — join room, request offer, send answer, delete room; a
  `clientId` query parameter replaces the former browser cookie (auto-generated and returned
  when omitted)

See [`docs/usage/admin-api.md`](docs/usage/admin-api.md) for the full route table and
examples.

## Migration notes

- **Environment variables removed**: all `RM_SMTP_*`, all `RMAPI_HWR_*`. **Added**:
  `RM_ADMIN_API_TOKEN` (optional; enables the admin API).
- **`.userprofile` compatibility**: `IntegrationConfig` was trimmed to the ICS fields
  (`id`, `provider`, `name`, `address`, `insecure`). Old profiles that still contain fields
  from removed integrations (`username`, `password`, `path`, `accesstoken`, `endpoint`,
  `activetransfers`) **continue to load** — YAML unknown keys are ignored — and only `ics`
  integrations are honored.
- **No web UI**: register accounts with the CLI (`setuser`) and pair devices by generating a
  code through the admin API (`GET /admin/users/:userid/newcode`) instead of the old web
  login + "Generate Code" page. Passcode resets are approved via the admin API instead of the
  UI banner.
- **Dependencies dropped**: `dropbox-sdk-go-unofficial`, `secsy/goftp`, `studio-b12/gowebdav`,
  `unidoc/unipdf`, `poundifdef/go-remarkable2pdf`, `juruen/rmapi`, and their transitives.
- **Build/packaging de-UI'd**: `make build` no longer needs node/pnpm; the Dockerfile dropped
  its Node UI-builder stage; CI workflows dropped node/pnpm setup and the UI test step; the
  Helm chart and env files dropped SMTP/HWR variables and added `RM_ADMIN_API_TOKEN`.

## Storage

Synced files persist under `DATADIR` unchanged. See
[`docs/usage/storage-layout.md`](docs/usage/storage-layout.md) for the full on-disk layout
(`users/<uid>/.userprofile`, sync 1.0 `<id>.zip` + `.metadata` + `.trash/`, and sync 1.5+
`sync/<hash>` blobs with `.root.history` and the `.tree` cache).
