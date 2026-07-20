# Welcome

rmfakecloud-lite is a **headless** clone of the cloud sync the reMarkable tablet uses, for
people who want to sync/backup their files with full control of the hosting/storage
environment. It has no web UI: accounts are managed with the CLI and administrative actions
are driven through a JSON [Admin API](usage/admin-api.md).

## Features

* File synchronization (compatible with revisions 1.0 and 1.5+)
* Device registration / pairing (all reMarkable devices)
* Screen sharing (viewer signaling relayed through the admin API)
* Calendar integration via an [ICS](usage/calendar.md) subscription
* Passcode (PIN) reset approval (reMarkable 1 / reMarkable 2 only)

Administration is done through:

* the CLI (`setuser` / `listusers`) for accounts — see [User Profile](usage/userprofile.md)
* the [Admin API](usage/admin-api.md) for pairing codes, passcode resets, calendar
  integrations, and screen-share signaling

This lite edition removes the web UI, email/SMTP, handwriting recognition, storage
integrations (Dropbox/WebDAV/FTP/local), messaging webhooks, and PDF export from upstream
rmfakecloud. See [`CHANGES.md`](https://github.com/lspaya05/rmfakecloud-lite/blob/master/CHANGES.md)
for details.
