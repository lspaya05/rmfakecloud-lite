# Admin API

The lite edition has no web UI. Administrative actions that used to live in the React
frontend are exposed as a headless JSON API under `/admin`, meant to be driven with `curl`
or a small local microservice.

## Authentication

All `/admin` routes require a static bearer token, configured with the
[`RM_ADMIN_API_TOKEN`](../install/configuration.md#admin-api) environment variable:

```
Authorization: Bearer <RM_ADMIN_API_TOKEN>
```

If `RM_ADMIN_API_TOKEN` is unset, the `/admin` routes are **not registered** and a warning is
logged at startup. A missing or wrong token returns `401`. The token is compared in constant
time.

User accounts themselves are still managed with the CLI (`setuser` / `listusers`), not this
API — see [User Profile](userprofile.md#edit-settings-through-cli).

In the examples below:

```sh
TOKEN=your-admin-token
UID=your-username
BASE=http://localhost:3000
```

## Route table

All routes are under `/admin/users/:userid`.

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/newcode` | generate a device pairing code |
| `GET` | `/passcode/resets` | list pending passcode-reset requests |
| `POST` | `/passcode/resets/:uuid/approve` | approve a passcode-reset request |
| `DELETE` | `/passcode/resets/:uuid` | dismiss a passcode-reset request |
| `GET` | `/integrations` | list the user's integrations |
| `POST` | `/integrations` | create an ICS integration |
| `GET` | `/integrations/:intid` | get one integration |
| `PUT` | `/integrations/:intid` | update an ICS integration |
| `DELETE` | `/integrations/:intid` | delete an integration |
| `GET` | `/screenshare/room` | join the active screenshare room |
| `GET` | `/screenshare/room/:roomId` | room info |
| `GET` | `/screenshare/offer` | request an offer (viewer signaling) |
| `POST` | `/screenshare/room/:roomId/answer` | send an answer (viewer signaling) |
| `DELETE` | `/screenshare/room/:roomId` | tear down the user's screenshare rooms |

## Device pairing

Generate a one-time code, then enter it on the tablet (or feed it to `test/register.sh`) to
pair the device. Pairing must work before any device token exists, which is why it lives
behind the admin token rather than a device login.

```sh
curl -H "Authorization: Bearer $TOKEN" $BASE/admin/users/$UID/newcode
# -> "abcd1234"
```

## Passcode resets

See [Passcode Reset](passcode-reset.md) for the end-to-end flow.

```sh
# list
curl -H "Authorization: Bearer $TOKEN" $BASE/admin/users/$UID/passcode/resets

# approve
curl -X POST -H "Authorization: Bearer $TOKEN" \
  $BASE/admin/users/$UID/passcode/resets/<uuid>/approve

# dismiss
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  $BASE/admin/users/$UID/passcode/resets/<uuid>
```

## Calendar (ICS) integrations

Only the `ics` provider is accepted; any other provider value returns `400`.

```sh
# create
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  $BASE/admin/users/$UID/integrations \
  -d '{"provider":"ics","name":"My Calendar","address":"https://example.com/calendar.ics"}'

# list
curl -H "Authorization: Bearer $TOKEN" $BASE/admin/users/$UID/integrations

# delete
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  $BASE/admin/users/$UID/integrations/<intid>
```

See [Calendar (ICS)](calendar.md) for the field reference.

## Screen-share viewer signaling

Screen sharing is peer-to-peer WebRTC; rmfakecloud only relays the signaling. The viewer is
an external client (e.g. a local microservice) that uses these endpoints to be the WebRTC
peer. Because there is no browser cookie, a **`clientId`** query parameter identifies the
viewer; if you omit it, the server generates one and returns it so you can reuse it on
subsequent calls.

Typical flow:

```sh
# 1. find the active room and its ICE servers
curl -H "Authorization: Bearer $TOKEN" $BASE/admin/users/$UID/screenshare/room

# 2. request an offer from the tablet (clientId optional; returned if omitted).
#    This long-polls (up to 30s) until the tablet produces an offer.
curl -H "Authorization: Bearer $TOKEN" \
  "$BASE/admin/users/$UID/screenshare/offer?clientId=viewer-1"

# 3. send your WebRTC answer back to the tablet
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/admin/users/$UID/screenshare/room/<roomId>/answer?clientId=viewer-1" \
  -d '{"targetClientId":"<owner-client-id>","payload":{ /* SDP answer */ }}'

# 4. tear down when done
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  $BASE/admin/users/$UID/screenshare/room/<roomId>
```

The `offer` response includes `roomId`, the `clientId` in use, the queued `messages`
(containing the tablet's offer), and the configured `iceServers`.
