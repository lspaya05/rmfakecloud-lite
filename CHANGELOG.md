# 0.0.26-lite (headless edition)

First release of the headless "lite" fork. The web UI and several peripheral features are
removed; administration moves to the CLI and a new JSON admin API. See
[`CHANGES.md`](CHANGES.md) for the full migration notes.

## Features

- Headless admin JSON API under `/admin`, guarded by the `RM_ADMIN_API_TOKEN` bearer token:
  device pairing codes, passcode-reset approval, ICS calendar CRUD, and screen-share viewer
  signaling (curl-driven, no frontend).

## Internal change

- Removed the React web UI (`ui/`) and `internal/ui`; relocated `internal/ui/viewmodel` to
  `internal/viewmodel`.
- Removed email/SMTP (`internal/email`), handwriting recognition (`internal/hwr`), the PDF
  exporter (`internal/storage/exporter`), messaging webhooks, and the browser-extension
  endpoints.
- Trimmed integrations to ICS-only (removed Dropbox/WebDAV/FTP/localfs providers); trimmed
  `IntegrationConfig` accordingly (old `.userprofile` files still parse — unknown keys are
  ignored).
- Dropped the corresponding dependencies (`dropbox-sdk-go-unofficial`, `goftp`, `gowebdav`,
  `unipdf`, `go-remarkable2pdf`, `juruen/rmapi`) and de-UI'd the build system, Docker images,
  CI workflows, Helm chart, and packaging.

# 0.0.25

## Features

- Software compatibility with 3.20 (cdb45df0b8314e637b5cdb722b10f0b262d74f56)
- Handle messaging integrations (a88aee6ea5ad846cd8aaab2bcbe2f82d2898e5f4)
- [Webhook messaging integration](https://ddvk.github.io/rmfakecloud/usage/integrations/#messaging-webhook) (479887ee4b335cd99f8a4cb4afeb7577681a217b)
- New option: `RMAPI_HWR_LANG_OVERRIDE` to override the language specified in myScript requests (#352)

## Internal change

- Refactor hash function (#365)
