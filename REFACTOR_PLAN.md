# rmfakecloud-lite refactor plan

## Context

Fork of ddvk/rmfakecloud. Goal: headless "lite" service. KEEP: device registration/pairing (all reMarkables), file sync 1.0 + 1.5/2/3/4 (sync15/blobstorage), screen sharing, calendar ICS, passcode (PIN) reset. REMOVE: web UI (React `ui/` + `internal/ui`), email/SMTP, handwriting recognition (hwr), storage integrations (dropbox/ftp/webdav/localfs), messaging webhooks, PDF exporter, browser-extension endpoints (`/doc/v1|v2/files`). Synced files already persist under DATADIR (sync 1.0: `users/<uid>/<id>.zip`; sync15: `users/<uid>/sync/<hash>` content-addressed) — keep as-is, document layout. Principles: change/remove as little as possible, reuse existing tests, update docs, end w/ CHANGES.md.

Decisions locked (user-approved):
- Admin surface = headless JSON API in `internal/app` (curl-driven, no frontend). User mgmt stays on existing CLI (`setuser`/`listusers`, `internal/cli/managerusers.go`).
- Screenshare viewer-side signaling exposed as admin JSON endpoints so a future local microservice can be the WebRTC peer (server only relays signaling; video is p2p, latency impact negligible).
- Raw DATADIR storage suffices; no PDF export on sync → exporter package deleted.

## Verified baseline facts (trust these)

- **Repo does NOT build today**: `ui/dist` missing, `ui/assets.go` has `//go:embed dist/*` (build tag `!ci`, no ci fallback). Phase 1 creates placeholder `ui/dist/index.html`; deleted with `ui/` in Phase 3.
- **`go` not on PATH** (bash nor PowerShell) on this machine. First agent must install/locate Go toolchain before any gate.
- Viewer-side screenshare handlers relay via `mqttBridge.PublishSignaling` server-side (`internal/ui/handlers.go:834,885`) — viewer never connects to MQTT directly → `ui.WebUserClaims` branch in `validateMQTTToken` (`internal/app/app.go:212-216`) can just be deleted.
- `internal/config/config.go:15` imports `internal/email` → config strip must ship with email deletion (Phase 1).
- `cmd/history2git15/main.go` imports `internal/ui/viewmodel` → move package, don't delete.
- Exporter used only by UI-download paths: `fs/blobstore.go` `Export`/`ExportRmDoc`, `fs/documents.go` `ExportDocument`, `models/archive.go` `ArchiveFromHashDoc`, `storage.go` `DocumentStorer.ExportDocument`. No tablet sync path uses it.
- Sync handlers do NOT call email/hwr/integrations — leaf packages, safe delete.

## Admin API design

Auth: static bearer token. New config `AdminAPIToken`, env `RM_ADMIN_API_TOKEN`. Unset → admin routes not registered + startup warning. Middleware: `common.GetToken(c)` + `crypto/subtle.ConstantTimeCompare`, 401 on mismatch. Param middleware sets `userIDKey` from sanitized `:userid` so relocated handlers keep reading uid from context (minimal diff vs ui originals). Rationale: pairing-code gen must work before any device token exists; JWT login flow would resurrect deleted UI auth code.

Routes (all under adminAuthMiddleware, new file `internal/app/admin.go`):
```
GET    /admin/users/:userid/newcode                          ← ui/handlers.go:219 newCode
GET    /admin/users/:userid/passcode/resets                  ← ui/passcode.go
POST   /admin/users/:userid/passcode/resets/:uuid/approve    ← ui/passcode.go (calls hub.NotifyPasscodeReset)
DELETE /admin/users/:userid/passcode/resets/:uuid            ← ui/passcode.go
GET    /admin/users/:userid/integrations                     ← ui/handlers.go:540-698 (ICS CRUD)
POST   /admin/users/:userid/integrations
GET    /admin/users/:userid/integrations/:intid
PUT    /admin/users/:userid/integrations/:intid
DELETE /admin/users/:userid/integrations/:intid
GET    /admin/users/:userid/screenshare/room                 ← ui/handlers.go:777+ joinActive
GET    /admin/users/:userid/screenshare/room/:roomId         ← getRoom
GET    /admin/users/:userid/screenshare/offer                ← getOffer (?clientId=; empty → server generates UUID, returns in response)
POST   /admin/users/:userid/screenshare/room/:roomId/answer  ← sendAnswer (?clientId=)
DELETE /admin/users/:userid/screenshare/room/:roomId         ← deleteRoom
```
clientId replaces UI cookie `browserIDContextKey`. Relocate `mqttBridge` interface (`ui/ui.go:63-66`) into admin.go; `app.mqttBroker` already satisfies it. Replace `viewmodel.NewErrorResponse` w/ existing `badReq`/`gin.H`.

---

## Phase 1 — Prune tablet API: delete email + hwr, strip config, unbreak build

- Create placeholder `ui/dist/index.html` (any content; makes embed compile; removed Phase 3).
- Delete dirs: `internal/email/` (incl smtp_test.go), `internal/hwr/`. Delete `test/sendmail.sh`.
- `internal/app/routes.go`: remove registrations lines 118-131 + 134-137 + 141-145: `/api/v2/document`, `/share/v1/email`, `/api/v1/page`, `/convert/v1/handwriting`, `/doc/v1/files`, `/doc/v2/files` (+OPTIONS), integrations v1 folder/file endpoints, v2 storage endpoints, v2 messaging. KEEP lines 138/140/146: `/integrations/v1/`, `/integrations/v2/instances`, `/integrations/v2/calendars/:id/events`. `folderKey`/`fileKey` consts: `fileKey` still used by sync v3 routes — keep; drop `folderKey` if unused after edit.
- `internal/app/handlers.go`: delete `sendEmail`, `handleHwr`, `uploadDoc`, `uploadDocV2`, `integrationsList`, `integrationsGetMetadata`, `integrationsGetFile`, `integrationsUpload`, `integrationsSendMessage` (KEEP `integrations` + `integrationsCalendarEvents`); drop now-unused imports (email, hwr, integrations-storage helpers); in `newUserToken` (~:150-165) drop `"hwcmail:-1"`, `"hwc"`, `"mail:-1"` scope branches, keep `"intgr"`, `"screenshare"`, `"docedit"`, sync scopes.
- `internal/app/app.go`: remove `hwrClient` field (:46), construction (:172-174), hwr import. Leave ui wiring untouched this phase.
- `internal/config/config.go`: remove SMTP consts/fields/parse/Verify-warning (:52-67, :98, :123, :210-236), HWR (:69-75, :100-103, :127-132, :278-281), `internal/email` import, corresponding `EnvVars()` help lines. Keep MQTTPort, ICEServers, DataDir, StorageURL, JWTSecretKey, TLS, RegistrationOpen, hash schema, cookie/proxy settings (ui still alive).
- `internal/app/middleware.go`: drop `dontLogBody` entries for removed routes (`/api/v2/document`, `/doc/v1/files`).
- **Gate**: `go build ./... && go test ./...` green. Grep: no refs to deleted symbols. Manual optional: server starts.

## Phase 2 — Headless admin JSON API + tests

- Create `internal/app/admin.go` per design above: `adminAuthMiddleware`, userid param middleware, relocated handlers (near-verbatim copies from `internal/ui/handlers.go` + `internal/ui/passcode.go` — copy from live source, UI still present this phase). Keep integration CRUD's provider handling as-is for now (trimmed Phase 4).
- `internal/config/config.go`: add `AdminAPIToken` + `RM_ADMIN_API_TOKEN` + EnvVars line.
- Register `/admin` group in `registerRoutes` only when `cfg.AdminAPIToken != ""`; else `log.Warn("admin API disabled...")`.
- Create `internal/app/admin_test.go` (httptest + gin, patterns from `codeconnector_test.go`/`passcodestore/store_test.go`): 401 no/wrong token; newcode returns 8-char code consumable by `codeConnector.ConsumeCode`; passcode list→approve→dismiss; integration create/list/delete against `fs` storage on `t.TempDir()`.
- **Gate**: build + tests green. Manual: `curl -H "Authorization: Bearer $TOK" .../admin/users/<uid>/newcode` → code works with `test/register.sh`.

## Phase 3 — Delete web UI, move viewmodel, remove exporter

- Move `internal/ui/viewmodel/` → `internal/viewmodel/`; fix import in `cmd/history2git15/main.go`.
- Delete: `internal/ui/` (whole pkg), `ui/` (whole dir incl `assets.go` + placeholder), `internal/storage/exporter/`, `test/ui_*.sh`, `test/upload_web.sh` (+ `test/output.pdf`/`test.pdf` if only referenced by deleted scripts — grep first).
- `internal/app/app.go`: remove ui import, `ui.New(...)` + `uiApp.RegisterRoutes(router)` (:183-184), `ui.WebUserClaims` branch in `validateMQTTToken` (:212-216).
- `internal/storage/fs/blobstore.go`: remove `Export`, `ExportRmDoc` + exporter import. `internal/storage/fs/documents.go`: remove `ExportDocument` + import. `internal/storage/models/archive.go`: remove `ArchiveFromHashDoc`. `internal/storage/storage.go`: remove `ExportDocument` from `DocumentStorer` iface; remove `ExportOption` type/consts if grep shows no users.
- `internal/app/middleware.go`: drop `/ui/api/documents/upload` dontLogBody entry.
- `go mod tidy` (expect unipdf, go-remarkable2pdf + transitives drop; `juruen/rmapi` stays — used by sync models/hub).
- **Gate**: `go build ./... ./cmd/...` (history2git15 = canary) + tests green (`documentcreator_test.go` must pass). `grep -r "internal/ui\|rmfakecloud/ui"` → 0 hits. Manual: server starts, `GET /` 404, admin API works.

## Phase 4 — Trim integrations to ICS-only, trim model

- Delete: `internal/integrations/{dropbox.go, ftp.go, webdav.go, webdav_test.go, localfs.go, localfs_test.go, webhook.go}` + `testfs/` fixtures + stray `test.md`/`test.txt` if only used by deleted tests.
- `internal/integrations/integrations.go`: keep ICS case in `getIntegrationProvider`, `GetCalendarIntegrationProvider`, `List`, `ProviderType`, `fixProviderName`, ICS const. Delete other provider consts, `StorageIntegrationProvider`/`MessagingIntegrationProvider` ifaces, `GetStorageIntegrationProvider`, `GetMessagingIntegrationProvider`, `visitDir`. KEEP `ics.go` + `ics_test.go` untouched.
- `internal/app/admin.go`: integration create/update validates `Provider == ics` (badReq otherwise); drop inherited localfs guard.
- `internal/model/user.go`: trim `IntegrationConfig` to fields ICS uses (grep `ics.go`: expect ID, Provider, Name, Address, Insecure). yaml.v3 ignores unknown keys → old `.userprofile` files still load; verify w/ quick test.
- `go mod tidy` (drops dropbox-sdk, goftp, gowebdav; check `stretchr/testify` still needed by remaining tests).
- **Gate**: build + tests green (`ics_test.go` green). Manual: create ICS integration via admin API → `/integrations/v2/instances` lists it → `/integrations/v2/calendars/:id/events` serves events.

## Phase 5 — Build system, CI, packaging

- `Makefile`: remove `ASSETS=ui/dist`, `GOFILES += $(ASSETS)` (line ~9), `$(ASSETS)` rule, `runui`, `testui`, pnpm refs, stale `ui/build` cleanup; `test` target = `testgo` only; fix `run`/`clean`.
- `Dockerfile`: drop `uibuilder` stage + `COPY --from=uibuilder` line; verify `go generate ./...` still valid (embed gone). `Dockerfile.make`: drop commented HWR/SMTP env hints.
- `dev.sh`: drop SMTP mock env + `make runui`.
- `.github/workflows/go.yml` + `release.yml`: drop pnpm/node setup steps + testui. `codeql-analysis.yml`: languages `['go']` only. `dockerhub.yml`: check image name for fork, no UI-stage refs.
- `helm/values.yaml` + `helm/templates/deployment.yaml`: drop `RM_SMTP_*`, `RMAPI_HWR_*` env; add optional `RM_ADMIN_API_TOKEN`. `other/rmfakecloud.env`: drop SMTP lines; check `rmfakecloud.service`/`playbook.yml` for removed env.
- **Gate**: `make build` succeeds w/o node/pnpm; `docker build .` if docker available; workflow YAML sanity-read.

## Phase 6 — Docs + CHANGES.md

- `README.md`: rewrite feature table (kept vs removed per Context above), drop UI dev instructions.
- `mkdocs.yml` nav: remove Integrations + Browser Extension entries; add Admin API + Calendar + Storage-layout entries.
- Delete `docs/browser-extension.md`. Replace `docs/usage/integrations.md` w/ calendar/ICS-only page (or new `docs/usage/calendar.md`).
- `docs/install/configuration.md`: drop HWR + Email sections; keep screenshare/ICE/MQTT; add `RM_ADMIN_API_TOKEN`.
- `docs/usage/userprofile.md`: ICS-only integration yaml example, drop webUI-login mentions, keep CLI/sync15 parts.
- `docs/usage/passcode-reset.md`: repoint approval flow from UI to admin API curl.
- NEW `docs/usage/admin-api.md`: route table + curl examples (pairing code, passcode approve, ICS CRUD, screenshare viewer signaling flow incl clientId semantics).
- NEW `docs/usage/storage-layout.md`: DATADIR layout — `users/<uid>/.userprofile`, sync 1.0 `<id>.zip`/`.metadata`/`.trash/`, sync15 `sync/<hash>` blobs + `.root.history` + `.tree` cache.
- `CHANGELOG.md`: new `# <next-version>` entry, `## Features` / `## Internal change` bullets (existing convention).
- NEW `CHANGES.md` (repo root, final deliverable): what removed/kept/relocated, admin API summary, migration notes (env vars removed, `.userprofile` compat, UI gone, pairing via curl now).
- **Gate**: full `go build ./... && go test ./...` one last time; every mkdocs nav entry resolves; `test/` contains only `connect.sh`, `getusertoken.sh`, `register.sh`, `poc.hurl`, `common.env`, `.gitignore`.

## Cross-phase risks

- Baseline build broken (missing `ui/dist`) → Phase 1 placeholder mandatory; don't reorder UI deletion before admin relocation (copy handlers from live source).
- Go toolchain missing on this machine → resolve before Phase 1 gate.
- `history2git15` = viewmodel-move canary; always build `./cmd/...`.
- Calendar shares `/integrations` route prefix w/ removed storage endpoints — surgical removal only (keep lines 138/140/146 of routes.go).
- `ExportOption`/`fileKey` consts: grep before deleting.
- `go mod tidy` only in Phases 3+4 (after importing code gone), never earlier.
- Old `.userprofile` yaml w/ removed fields must still parse (yaml.v3 default-ignores unknown keys — verify once in Phase 4).

## Verification (end-to-end)

1. `go build ./... && go test ./...` green, `go vet ./...` clean.
2. Start server w/ temp DATADIR + `RM_ADMIN_API_TOKEN`; CLI `setuser -u test -s`; admin `newcode`; `test/register.sh` + `test/connect.sh` pair a (simulated) device; `cmd/testclient` for ws check.
3. Sync round-trip: hit sync15 endpoints (`/sync/v3/root` etc.) w/ device token; confirm blobs appear under `DATADIR/users/test/sync/`.
4. ICS: add integration via admin API pointing at a local .ics file server; `/integrations/v2/calendars/:id/events` returns events.
5. Passcode: `POST /passcode/v1/resets/:uuid` (device token) → admin list → approve → `GET` shows approved.
6. Screenshare signaling: create room via `/screenshare/v1/rooms` (user token) → admin `offer`/`answer` endpoints respond sanely.
7. `make build` + `docker build .` w/o node/pnpm.

---

## Progress log

### 2026-07-19 — Phases 1–3 implemented (Go 1.26.5)

Environment resolved: Go installed at `C:\Program Files\Go\bin` (invoke via full path; not on PATH). Each phase's gate (`go build ./...`, `go vet ./...`, `go test ./...`) passed before moving on.

**Phase 1 — prune tablet API (email + hwr), strip config, unbreak build ✅**
- Added placeholder `ui/dist/index.html` so `//go:embed dist/*` compiles (removed in Phase 3).
- Deleted `internal/email/`, `internal/hwr/`, `test/sendmail.sh`.
- `routes.go`: removed email/hwr/`doc` + v1/v2 storage + v2 messaging routes; kept `/integrations/v1/`, `/integrations/v2/instances`, `/integrations/v2/calendars/:id/events`; dropped unused `folderKey` const.
- `handlers.go`: deleted `sendEmail`, `handleHwr`, `uploadDoc`/`uploadDocV2` + orphaned helpers (`saveUpload`, `getSyncVersion`, `extFromContentType`, `metapayload`, `emailForm`, `stripAds`), and `integrationsList/GetMetadata/GetFile/Upload/SendMessage`; kept `integrations` + `integrationsCalendarEvents`; dropped hwr/smtp scope branches in `newUserToken`; removed now-unused imports.
- `app.go`: removed `hwrClient` field + construction + hwr import.
- `config.go`: removed SMTP + HWR consts/fields/parse/Verify-warnings/EnvVars help + `internal/email` + `net/mail` imports.
- `middleware.go`: dropped `/api/v2/document` and `/doc/v1/files` from `dontLogBody`.
- Gate: build + vet + full test suite green.

**Phase 2 — headless admin JSON API + tests ✅**
- New `internal/app/admin.go`: `adminAuthMiddleware` (static bearer via `common.GetToken` + `crypto/subtle.ConstantTimeCompare`), `adminUserParam` middleware (sets `userIDKey` from sanitized `:userid`), and relocated handlers (newcode, passcode list/approve/dismiss, integration CRUD, screenshare viewer signaling). `clientId` query param replaces the old browser cookie; `app.mqttBroker` used directly (satisfies the old `mqttBridge` interface); `viewmodel.NewErrorResponse` replaced with `badReq`/`gin.H`.
- `config.go`: added `AdminAPIToken` + `RM_ADMIN_API_TOKEN` + EnvVars line.
- `routes.go`: `registerAdminRoutes(router)` registers `/admin` only when `AdminAPIToken != ""`, else logs a warning.
- New `internal/app/admin_test.go`: 401 (no/wrong token), newcode 8-char code consumable by `codeConnector.ConsumeCode`, passcode list→approve→dismiss (+404 unknown), integration create/list/get/delete on `fs` storage in `t.TempDir()`, and routes-disabled-without-token (404). All pass.
- Gate: build + vet + full test suite green.

**Phase 3 — delete web UI, move viewmodel, remove exporter ✅**
- Moved `internal/ui/viewmodel/` → `internal/viewmodel/`; fixed import in `cmd/history2git15/main.go` (canary builds).
- Deleted `internal/ui/`, `ui/` (incl. `assets.go` + placeholder), `internal/storage/exporter/`, `internal/storage/models/archive.go` (`ArchiveFromHashDoc`), `test/ui_*.sh`, `test/upload_web.sh`, `test/output.pdf`, `test/test.pdf`.
- `app.go`: removed ui import, `ui.New(...)` + `RegisterRoutes`, and the `ui.WebUserClaims` branch in `validateMQTTToken`.
- Removed exporter users: `fs/blobstore.go` (`Export`, `ExportRmDoc` + import), `fs/documents.go` (`ExportDocument` + `exporter`/`errors` imports), `storage.go` (`ExportDocument` from `DocumentStorer`, `ExportOption` type/consts).
- `middleware.go`: dropped `/ui/api/documents/upload` from `dontLogBody`.
- `go mod tidy` (exporter deps dropped; dropbox/goftp/gowebdav remain until Phase 4).
- Gate: `go build ./... ./cmd/...` + vet + tests green; `grep -r internal/ui|rmfakecloud/ui` → 0 hits. Server smoke test: `GET /` → 404, admin auth 401 (no/wrong token), `[admin] admin API enabled under /admin` logged.

**Remaining:** Phases 4–6 (trim integrations to ICS-only + model; build system/CI/packaging; docs + CHANGES.md) not yet started.

### 2026-07-19 — Phases 4–6 implemented (Go 1.26.5)

**Phase 4 — trim integrations to ICS-only + trim model ✅**
- Deleted `internal/integrations/{dropbox,ftp,webdav,webdav_test,localfs,localfs_test,webhook}.go`, `testfs/`, and `test.md`. Kept `ics.go` + `ics_test.go` untouched.
- Rewrote `integrations.go` to the ICS-only path: kept `IcsProvider`, `IntegrationProvider`, `CalendarIntegrationProvider`, `GetCalendarIntegrationProvider`, `List`, `fixProviderName`/`ProviderType` (ics+default); deleted the other provider consts, `StorageIntegrationProvider`/`MessagingIntegrationProvider` ifaces + their `Get*` funcs, and `visitDir` (its helpers `encodeName`/`decodeName`/`contentTypeFromExt` lived in webdav.go, `loggerfs` in localfs.go — all gone).
- `admin.go`: removed `warnLocalfsEdition` + the localfs guards; create/update now reject any `Provider != ics` with `badReq`; dropped the `yaml.v3` import.
- `model/user.go`: trimmed `IntegrationConfig` to `ID, Provider, Name, Address, Insecure`. Verified via throwaway test that a legacy `.userprofile` containing removed keys (username/password/path/accesstoken/endpoint/activetransfers) still unmarshals (yaml.v3 ignores unknown keys); test removed after.
- `go mod tidy` dropped dropbox-sdk/goftp/gowebdav (+ oauth2/appengine transitives); testify auto-dropped (no remaining test imports it). `messages.go` Integration comment de-stale'd.
- Gate: build + vet + full tests green (`ics_test.go` green).

**Phase 5 — build system, CI, packaging ✅**
- `Makefile`: removed ASSETS/GOFILES-embed/`$(ASSETS)` rule/`runui`/`testui`/pnpm; `test` → `testgo`; `run`/`clean` no longer reference UI.
- `Dockerfile`: dropped the `uibuilder` node stage + `COPY --from=uibuilder` and the now-noop `go generate` (no `//go:generate` remain). `Dockerfile.make`: HWR/SMTP hints → `RM_ADMIN_API_TOKEN`.
- `dev.sh`: dropped SMTP mock env + `make runui`; now just rebuild/rerun backend on Go changes.
- Workflows: `go.yml`/`release.yml` dropped pnpm/node setup + Test-UI step; `codeql-analysis.yml` languages → `['go']`; `dockerhub.yml` unchanged (no UI refs).
- `helm/values.yaml` + `deployment.yaml`: removed RM_SMTP_*/RMAPI_HWR_*; added `RM_ADMIN_API_TOKEN`. `other/rmfakecloud.env`: SMTP → `RM_ADMIN_API_TOKEN`; service/playbook clean.
- Gate: `make` not installed on this box → ran the `build` target's exact command (`GOOS=linux go build … ./cmd/rmfakecloud`) successfully with no node/pnpm; `docker build .` succeeded end-to-end (UI stage gone). Test artifacts cleaned up.

**Phase 6 — docs + CHANGES.md ✅**
- `README.md`: new feature table (kept/removed), headless intro, de-UI'd dev section.
- `mkdocs.yml` nav: dropped Integrations + Browser Extension; added Admin API, Calendar (ICS), Storage Layout. Deleted `docs/browser-extension.md` + `docs/usage/integrations.md`.
- Rewrote `configuration.md` (dropped HWR + Email; added Admin API section; screenshare now points at admin endpoints), `userprofile.md` (ICS-only yaml example + legacy-field note, CLI-based), `passcode-reset.md` (curl-driven admin approval), `index.md` (headless features), `remarkable/setup.md` + `reverse-proxy/apache.md` + `install/source.md` (pairing via admin API, dropped SMTP/HWR env).
- NEW `docs/usage/admin-api.md` (route table + curl for pairing/passcode/ICS/screenshare incl. clientId), `docs/usage/calendar.md` (ICS), `docs/usage/storage-layout.md` (verified on-disk names: `.userprofile`, `.trash`, `sync/`, `.root.history`, `.tree`).
- `CHANGELOG.md`: added `# 0.0.26-lite` entry. NEW `CHANGES.md` (root): removed/kept/relocated + admin API summary + migration notes.
- Gate: `go build ./... ./cmd/...` + vet + full tests green; every mkdocs nav entry resolves; no links to deleted pages; `test/` = connect/getusertoken/register/poc.hurl/common.env/.gitignore only; `grep dropbox|goftp|gowebdav|LocalfsProvider|StorageIntegrationProvider` over code+go.mod → 0.

**All six phases complete and green. Nothing committed.**
