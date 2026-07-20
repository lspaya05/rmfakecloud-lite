# rmfakecloud-lite

A headless fork of [rmfakecloud](https://github.com/ddvk/rmfakecloud): a replacement of
the reMarkable cloud for people who want to sync/backup their files with full control of
the hosting environment. This "lite" edition drops the web UI and several peripheral
features to keep the service small and easy to operate; everything an administrator needs
is exposed through a CLI and a headless JSON API driven with `curl`.

See the [project documentation](https://lspaya05.github.io/rmfakecloud-lite/) for setup and configuration.

## Supported Devices

| Device               | Is Supported |
| -------------------- | ------------ |
| reMarkable 1         | ✅           |
| reMarkable 2         | ✅           |
| reMarkable Paper Pro | ✅           |
| reMarkable Paper Pro Move | ✅           |
| reMarkable Paper Pure| ✅           |

The current release of rmfakecloud supports file synchronization up to **reMarkable software 3.27.1**. Newer releases have not been tested yet.

See the [documentation](https://lspaya05.github.io/rmfakecloud-lite/remarkable/setup/) for how to setup your device to use rmfakecloud.


## Features

This lite edition keeps everything the tablet needs to sync and pair, plus a small set of
admin capabilities. Features that are not core to headless sync have been removed.

| Feature | Status | Notes |
| ------- | ------ | ----- |
| File synchronization (1.0) | ✅ |  |
| File synchronization (1.5, 2, 3, 4) | ✅ | [diff sync](https://lspaya05.github.io/rmfakecloud-lite/usage/diff-sync/) |
| Device registration / pairing | ✅ | all reMarkable devices; pairing code via the [admin API](https://lspaya05.github.io/rmfakecloud-lite/usage/admin-api/) |
| [Screen sharing](https://lspaya05.github.io/rmfakecloud-lite/install/configuration/#screen-sharing) | ✅ | viewer signaling relayed through the admin API |
| [Calendar integration (ICS)](https://lspaya05.github.io/rmfakecloud-lite/usage/calendar/) | ✅ | subscribe to an `.ics` URL |
| [Passcode (PIN) reset](https://lspaya05.github.io/rmfakecloud-lite/usage/passcode-reset/) | ✅ | reMarkable 1 / reMarkable 2 only; approved via the admin API |
| Web UI | ❌ | removed in the lite edition — use the CLI + [admin API](https://lspaya05.github.io/rmfakecloud-lite/usage/admin-api/) |
| Send document by email / SMTP | ❌ | removed |
| Handwriting recognition | ❌ | removed |
| Storage integrations (Dropbox / WebDAV / FTP / local) | ❌ | removed |
| Messaging integrations / webhooks | ❌ | removed |
| Browser extension endpoints | ❌ | removed |

See [`CHANGES.md`](CHANGES.md) for the full list of what was removed, kept, and relocated
relative to upstream rmfakecloud.


## Breaking Changes

- For SW >= 3.15 `STORAGE_URL` should not be set (or only https://some.ho.st without a port should be used)
- after v0.0.3 the files in `/data` will have to be manually moved to the user that will be created
- with v0.0.5 the new diff sync15 is added as an option, in order to use it modify the user with `setuser -u user -s`
  or modify the profile and add `sync15:true`
  a full resync will be needed (the tablet will do it), the old files are kept as they were and everything is put in a new directory

## Development

Run `./dev.sh` which rebuilds and reruns the headless backend on any Go file change
(requires [`entr`](https://github.com/eradman/entr)). Set `RM_ADMIN_API_TOKEN` to enable
the admin API while developing.

### Caveats/ WARNING

- (applies when you don't have security, version <= 0.0.3) connecting to the api will delete all your files, unless you mark them as not synced `synced:false` prior to syncing (advisable just to disconnect, reconnect the cloud)
- **if you delete files from the users directory** on the host, on the next sync those will be deleted from the device
- if you delete the whole user directory (by mistake) on the host, you should disconnect the cloud from the device and reconnect it
- after an official update, the proxy and hosts file changes will be removed, the tablet will automatically disconnect from the cloud (by sending an invalid token to the official cloud and getting 403)
  just reinstall the proxy and reconnect to your cloud

## Troubleshooting
- check the connectivity between the tablet and the host:
    ping my.remarkable.com (should be localhost)
    ping local.remarkable.com (should be localhost)
    ping thehostpc
    wget -qO- http://host:3000 (or relevant ports, should get Working...)
    wget -qO- https://local.appspot.com (should get Working...)

- check that the proxy is running and certs are installed:
    ```
    echo Q | openssl s_client -connect localhost:443  -verify_hostname local.appspot.com -CAfile /etc/ssl/certs/ca-certificates.crt 2>&1 | grep Verify
    ```
    You should see: *Verify return code: 0 (ok)*

- if both (host and tablet) are on a wifi make sure "Client Isolation" is not activated on the AP

- check if the proxy is configured correctly
    ```
    systemctl status proxy

    #or

    journalctl -u proxy
    ```
- check whether the CA cert was installed correctly
    when doing `update-ca-certificates` there should have been `1 added`
    check the logs

- check xochitls's logs, stop the service, start manually with more logging
    ```
    systemctl stop xochitl
    QT_LOGGING_RULES=rm.network.*=true xochitl | grep -A3 QUrl

    ```
    if you see *SSL Handshake failed* then something is wrong with the certs
- check sync logs
   ```
   journalctl -u rm-sync
   ```
