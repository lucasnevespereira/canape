# Freebox

canape reads two things from a Freebox: who's on the network (`canape devices`) and the Wi-Fi name (to prefill `canape wifi set`). It never changes the box's settings.

## Pairing

`canape setup` offers it, or run `canape freebox pair`. The box shows the request on its small screen: use its arrows to pick **Oui** and confirm.

On a Pop, the screen can stay on "Authentification en cours" after you confirm. That's a known display bug ([FS#37677](https://dev.freebox.fr/bugs/task/37677)); canape checks the result itself and pairing works.

If the box refuses new apps, check that "Permettre les nouvelles demandes d'association" is ticked in Freebox OS (http://mafreebox.freebox.fr) → Paramètres de la Freebox → Gestion des accès → Paramètres.

## Permissions

Leave **"Modification des réglages de la Freebox"** unticked for canape (Gestion des accès → Applications). canape only reads, and that permission would let anyone holding canape's token change any setting on the box, from outside too, since the box allows remote access for apps. You can revoke canape there at any time.

## How canape talks to the box

Official docs: https://dev.freebox.fr/sdk/os/

- **Discovery:** `GET http://mafreebox.freebox.fr/api_version` returns `api_base_url` and `api_version`; canape builds URLs from it rather than hardcoding a version. Plain HTTP on the LAN: HTTPS uses Free's own certificate authority, which system trust stores don't have.
- **Authorization, once:** `POST <base>/login/authorize/` with `app_id`, `app_name`, `app_version` and `device_name` returns an `app_token` and a `track_id`. canape polls `GET <base>/login/authorize/<track_id>` until `granted`.
- **Session, each run:** `GET <base>/login/` returns a `challenge`; the password is `hex(hmac_sha1(key=app_token, msg=challenge))`, so the token itself never goes on the wire. `POST <base>/login/session/` returns a `session_token`, sent as `X-Fbx-App-Auth`. Sessions expire: on `auth_required`, canape opens a new one and retries once.
- **Endpoints canape uses:** `lan/browser/pub/` (devices), `wifi/bss/` (network name; the API masks the password as `********`).

## Guest keys (not used)

The box can create temporary guest Wi-Fi passwords (`POST wifi/custom_key/` with `description`, `key`, `max_use_count`, `duration` in seconds and `access_type`, where `net_only` keeps guests off the LAN). canape doesn't use them: they need the settings permission above. The field names come from [hacf-fr/freebox-api](https://github.com/hacf-fr/freebox-api) and [uaifai](https://github.com/Procsiab/uaifai), not the official docs, and haven't been tried on a Pop.
