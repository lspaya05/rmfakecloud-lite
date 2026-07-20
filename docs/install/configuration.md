The configuration is made through environment variables.

## General configuration

| Variable name     | Description |
|-------------------|-------------|
| `JWT_SECRET_KEY`  | The secret key used to sign the authentication token.<br>If you don't provide it, a random secret is generated, invalidating all connections established previously to be closed.<br>A good secret is for example: `openssl rand -base64 48` |
| `STORAGE_URL`     | It controls whether file upload/download goes through the local proxy or to an external server. It's the full address (protocol, host, port, path) of rmfakecloud **as visible from the tablet**, especially if the host is behind a reverse proxy or in a container. Example: `http://192.168.2.3:3000` (default: `https://local.appspot.com`), on SW 3.15 only https without port will work |
| `PORT`            | listening port number (default: 3000) |
| `DATADIR`         | Set data/files directory (default: `data/` in current dir) |
| `LOGLEVEL`        | Set the log verbosity. Default is **info**, set to **debug** for more logging or **warn**, **error** for less |
| `RM_HTTPS_COOKIE` | Force auth cookies to be available only via https |
| `RM_TRUST_PROXY`  | Trust the proxy for client ip addresses (X-Forwarded-For/X-Real-IP) default false |
| `HASH_SCHEMA_VERSION` | Hash tree schema version: "3" or "4" (default: 3) |

## Admin API

The lite edition has no web UI; administrative tasks (generating pairing codes, approving
passcode resets, managing ICS calendar integrations, and screen-share viewer signaling) are
exposed through a headless JSON API under `/admin`, protected by a static bearer token.

| Variable name         | Description |
|-----------------------|-------------|
| `RM_ADMIN_API_TOKEN`  | Bearer token that enables the `/admin` endpoints. If unset, the admin API is **not** registered and a warning is logged at startup. |

See [Admin API](../usage/admin-api.md) for the route table and `curl` examples.

## Screen sharing

Screen sharing streams your tablet display to a browser via WebRTC. There are two signaling modes depending on your tablet's software version.

### REST-based (reMarkable OS 3.27+)

Starting with OS 3.27, the tablet can use REST-based signaling instead of MQTT. No additional setup is required, screen sharing works out of the box.

Start a screen share session from the sharing menu on your tablet. The server only relays
the WebRTC signaling; the viewer is an external client that drives it through the
[admin API screenshare endpoints](../usage/admin-api.md#screen-share-viewer-signaling)
(video is peer-to-peer). The admin endpoints find the active session and exchange the
offer/answer on the viewer's behalf.

| Variable name     | Description |
|-------------------|-------------|
| `ICE_SERVERS`     | JSON array of WebRTC ICE servers. Default: Google STUN server. Format: `[{"urls":["stun:stun.l.google.com:19302"]}]` or with TURN: `[{"urls":["turn:turn.example.com:3478"],"username":"user","credential":"pass"}]` |

Without `ICE_SERVERS` set, a public Google STUN server is used, which works when the tablet and browser / app are on the same network. If you want to screenshare across the internet, you may need a TURN server.

### MQTT-based (reMarkable OS < 3.27)

Older tablet software uses MQTT for screen share signaling. This requires additional configuration:

| Variable name     | Description |
|-------------------|-------------|
| `MQTT_PORT`       | Port for MQTT broker (default: 8883) |
| `ICE_SERVERS`     | JSON array of WebRTC ICE servers. Default: none. Format: `[{"urls":["stun:stun.l.google.com:19302"]}]` or with TURN: `[{"urls":["turn:turn.example.com:3478"],"username":"user","credential":"pass"}]` |
| `TLS_CERT`          | `path/to/cert`, required for MQTT screen sharing |
| `TLS_KEY`           | `/path/to/key`, required for MQTT screen sharing |

TLS certificates are required for MQTT screen sharing. Desktop apps may not use the system certificate store for MQTT.  
Requires overriding DNS for `vernemq-prod.cloud.remarkable.engineering` to point to your rmfakecloud instance and using a TCP (not HTTP) reverse proxy.  
Without `ICE_SERVERS` set, screen sharing will work over USB and if the tablet and desktop app are on the same network.

### Reverse proxy for MQTT (Screen sharing)

MQTT uses TCP with TLS. Typical reverse proxies require TCP stream forwarding rather than HTTP proxying.

#### nginx (stream module)

```nginx
stream {
    upstream mqtt {
        server rmfakecloud:8883;
    }

    server {
        listen 443;
        proxy_pass mqtt;
        proxy_connect_timeout 5s;
    }
}
```

#### Traefik (TCP router)

```yaml
tcp:
  routers:
    mqtt:
      rule: "HostSNI(`*`)"
      service: mqtt
      entryPoints:
        - mqtt
  services:
    mqtt:
      loadBalancer:
        servers:
          - address: "rmfakecloud:8883"

entryPoints:
  mqtt:
    address: ":443"
```
