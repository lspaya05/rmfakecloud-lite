Have a look inside `data` directory ([`DATADIR`](../install/configuration.md)):
you'll find under `data/users/` a directory by user (since v0.0.3). The
directory name is the username, which is set when the account is created with the
[CLI](#edit-settings-through-cli). See [Storage Layout](storage-layout.md) for the full
directory structure.


## User Settings

rmfakecloud stores user configuration (password, options, ...) in a file
inside its directory, named `.userprofile`. This is a hidden file.

This file, written in YAML, have the following relevant entries:

| Entry      | Description |
|------------|-------------|
| `password` | Password to access the account (in Argon2 format) |
| `name` | Display name for the account |
| `isadmin` | Boolean indicating if the user can perform administration tasks (currently managing user accounts) |
| `sync15` | Boolean value that indicates if the user is using the [diff synchronization](diff-sync.md) (aka. sync 1.5) |
| `integrations` | Array with the user's ICS calendar integrations. See [Calendar (ICS)](calendar.md) |

### ICS calendar integration example

The only supported integration is an ICS calendar subscription. Integrations are normally
managed through the [admin API](admin-api.md#calendar-ics-integrations), but they can also
be added by editing `.userprofile` directly:

```yaml
integrations:
  - id: 8a1f...            # any unique id (uuid); the admin API generates one for you
    provider: ics
    name: My Calendar
    address: https://example.com/calendar.ics
    insecure: false        # set true to skip TLS certificate verification
```

!!! note "Legacy fields"
    Older `.userprofile` files may contain fields from removed storage integrations
    (`username`, `password`, `path`, `accesstoken`, ...). These are ignored on load, so old
    profiles keep working; only `ics` integrations are honored.


### Edit settings through CLI

Use the same binary as for launching the server: it takes some specials commands described bellow.

When using the Docker image, you can run :

```sh
docker exec rmfakecloud /rmfakecloud-docker special-command
```

#### `rmfakecloud listusers`

This commands lists existing users.

#### `rmfakecloud setuser`

This commands edit or create account.

To create/update an admin account `myuser`:

```sh
rmfakecloud setuser -u myuser -a
```

To reset a password:

```sh
read -s -p "New password: " NEWPASSWD && rmfakecloud setuser -u myuser -p "${NEWPASSWD}"
```


## Directory Structure

In a user directory, there are files like `[UUID].metadata` and `[UUID].zip`
(if you are not using [sync 1.5](diff-sync.md)): this corresponds to your raw
documents on your tablet.

There is also a `trash` directory, containing deleted files on the tablet, in
its trash.

If you are using [sync 1.5](diff-sync.md), the magic happen in the `sync`
directory.
