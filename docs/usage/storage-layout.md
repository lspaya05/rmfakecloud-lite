# Storage Layout

rmfakecloud stores everything on disk under the data directory
([`DATADIR`](../install/configuration.md), default `data/`). Nothing is stored in a database.
This page documents the on-disk layout so you can back it up, inspect it, or migrate it.

## Top level

```
DATADIR/
└── users/
    ├── <uid>/            # one directory per user account
    ├── <uid2>/
    └── ...
```

The `<uid>` directory name is the username set when the account is created with the
[CLI](userprofile.md#edit-settings-through-cli).

## Per-user directory

```
users/<uid>/
├── .userprofile         # YAML account config (password hash, sync15 flag, ICS integrations)
├── <docUUID>.metadata   # sync 1.0: per-document metadata (JSON)
├── <docUUID>.zip        # sync 1.0: raw document content
├── .trash/              # sync 1.0: documents deleted (trashed) on the tablet
└── sync/                # sync 1.5+ (diff sync): content-addressed blob store
    ├── <hash>           # blobs, named by their content hash
    ├── .root.history    # root modification log / generation-number source
    └── .tree            # cached hash tree (rebuilt if missing)
```

### `.userprofile`

Account settings in YAML. See [User Profile](userprofile.md) for the fields. Only `ics`
integrations are honored; fields from removed integrations are ignored on load.

### Sync 1.0 (legacy) — `<docUUID>.zip` + `<docUUID>.metadata`

Used by older tablet software and by accounts that have **not** enabled sync 1.5. Each
document is a `.zip` of the raw files plus a `.metadata` JSON sidecar. Trashed documents live
under `.trash/`.

### Sync 1.5+ (diff sync) — `sync/`

Enabled per user with `setuser -u <user> -s` (or `sync15: true` in `.userprofile`). Content is
stored in a content-addressed blob store under `sync/`: blobs are named by their hash,
`.root.history` records root generations, and `.tree` caches the reconstructed hash tree. See
[Diff Sync](diff-sync.md) for how the sync protocol uses these.

!!! warning
    Deleting files from a user's directory on the host causes them to be deleted from the
    device on the next sync. If you delete a whole user directory by mistake, disconnect and
    reconnect the cloud from the tablet.
