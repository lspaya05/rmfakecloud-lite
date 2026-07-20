# Passcode (PIN) Reset

If you forget the passcode on your tablet, the reMarkable lockscreen offers a
**Forgot PIN** flow that asks the cloud to approve the reset. rmfakecloud
implements the same flow so you can recover a locked device without needing
the official cloud.

!!! note "Device support"
    This flow only exists on **reMarkable 1** and **reMarkable 2**. Later devices use a recovery code or a factory reset.

## How it works

1. After several incorrect passcode attempts, an option that says **swipe
   to unlock** will appear at the bottom of your screen. Swipe this and
   then press **Reset passcode**.
2. Follow the instructions on your device. The tablet sends a reset request to
   rmfakecloud, which is now pending administrator approval.
3. Approve the request through the [admin API](admin-api.md) (the lite edition has no web
   UI). List the pending requests, then approve the one for your device:

    ```sh
    TOKEN=your-admin-token
    UID=your-username

    # list pending reset requests
    curl -H "Authorization: Bearer $TOKEN" \
      http://localhost:3000/admin/users/$UID/passcode/resets

    # approve one (uuid taken from the list above)
    curl -X POST -H "Authorization: Bearer $TOKEN" \
      http://localhost:3000/admin/users/$UID/passcode/resets/<uuid>/approve
    ```

4. Your old passcode has been reset. Enter a new passcode on your reMarkable.

If the request wasn't initiated by you, dismiss it instead — this removes it from
rmfakecloud so the tablet can no longer complete the reset for that attempt:

```sh
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://localhost:3000/admin/users/$UID/passcode/resets/<uuid>
```

Pending requests are kept in memory only and expire after 24 hours. If the
rmfakecloud container restarts before you approve, just tap **Forgot PIN**
again on the tablet to create a fresh request.
