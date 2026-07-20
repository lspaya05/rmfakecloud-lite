# Calendar (ICS)

rmfakecloud-lite can serve calendar events to the tablet's calendar view by subscribing to
an external **ICS** (iCalendar) URL on the user's behalf. This is the only integration kept
in the lite edition — the storage and messaging integrations from upstream rmfakecloud have
been removed.

## How it works

1. You register an ICS integration for a user (an `.ics` URL to subscribe to).
2. The tablet asks rmfakecloud for the list of available integrations and then for calendar
   events in a time window.
3. rmfakecloud fetches the `.ics` (results are cached for 5 minutes), parses it, and returns
   the events that fall inside the requested window.

## Registering an ICS integration

Integrations are managed through the [admin API](admin-api.md#calendar-ics-integrations).
The only supported provider is `ics`; any other provider value is rejected with `400`.

```sh
TOKEN=your-admin-token
UID=your-username

curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  http://localhost:3000/admin/users/$UID/integrations \
  -d '{"provider":"ics","name":"My Calendar","address":"https://example.com/calendar.ics"}'
```

Fields:

| Field      | Description |
|------------|-------------|
| `provider` | must be `ics` |
| `name`     | display name |
| `address`  | URL of the `.ics` file to subscribe to |
| `insecure` | optional; set `true` to skip TLS certificate verification when fetching the ICS |

You can also add the same entry by editing `.userprofile` directly — see
[User Profile](userprofile.md#ics-calendar-integration-example).

## Tablet-facing endpoints

These are consumed by the tablet automatically (using the device token); you normally don't
call them by hand:

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/integrations/v1/` | list integrations |
| `GET` | `/integrations/v2/instances` | list integration instances |
| `GET` | `/integrations/v2/calendars/:id/events` | calendar events for integration `:id` in a time window |

## Timezones

The parser understands both IANA timezone names and Windows timezone names (mapped to IANA),
falling back to UTC for anything unknown. Recurring events are expanded within the requested
window.
