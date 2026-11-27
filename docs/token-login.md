# Token login for browser clients

The broker gets a second, WebSocket listener with its own mountpoint (`web`).
Browser clients log in there with username = user id and password = Keycloak
access token. The listener is for reading events in the browser; it can neither
publish nor subscribe to anything but `event`.

## Applies when

You change the login, publish or subscribe webhook, the client id of a token
session, or the broker's listeners. The broker configuration (listener,
mountpoint) lives in the deployment, not in this repository.

## Configuration

| Variable | Meaning |
|---|---|
| `TOKEN_LOGIN_MOUNTPOINT` | mountpoint whose logins are token logins; empty disables the feature (default) |
| `KEYCLOAK_CERT_URL` | JWKS url of the realm (`.../protocol/openid-connect/certs`) that verifies the tokens; token logins are refused while it is empty |
| `VMQ_ADMIN_API_URL` | needed by the sweeper, see below |

## Login

The login webhook receives the mountpoint of the listener. With
`TOKEN_LOGIN_MOUNTPOINT` set and equal to it, the login is a token login:

1. The password is the access token; a `Bearer ` prefix is stripped.
2. Signature and key are verified against `KEYCLOAK_CERT_URL`. The keys are
   cached by `kid`; a token with an unknown `kid` triggers a download, at most
   one at a time and one per 10 seconds, with a 10 second timeout (service-commons
   from 2026-11-12 on). The listener is public, so without that bound every login
   attempt with a made-up `kid` would cost a request to Keycloak and could hold
   the webhook pool the device logins share.
3. The token must not be expired and must carry an `exp`; the connector reads
   `exp` itself, because the verification of service-commons does not check it.
4. The `sub` of the token must equal the username.
5. The session must be clean (`clean_session` or `clean_start`), regardless of
   `FORCE_CLEAN_SESSION` and `CLEAN_SESSION_ALLOW_USER_LIST`: a persistent
   session would keep its subscriptions and queue events past the expiry of
   its token. The connection limit applies as for every client. `CHECK_HUB`
   does not, a browser is no hub.

Any other non-empty mountpoint is refused, and every non-empty mountpoint when
`TOKEN_LOGIN_MOUNTPOINT` is empty. On the empty mountpoint the login works as
before, except that client ids starting with `web:` are refused. Both are
changes for existing clients: before this feature a listener with its own
mountpoint and a client id starting with `web:` logged in like any other.

Answer to a successful token login:

```json
{"result": "ok", "modifiers": {"mountpoint": "", "client_id": "web:<exp unix seconds>:<original client id>"}}
```

VerneMQ applies both modifiers (verified on v2.2.1). The session therefore
lives in the default mountpoint and receives the events that devices and hubs
publish there. The client id prefix `web:` is exclusive to token sessions; the
other webhooks and the sweeper rely on it.

## Restrictions of token sessions

| Hook | Rule |
|---|---|
| publish | every publish of a client id starting with `web:` is refused |
| subscribe | only topics whose first level is `event`; every other topic, `#` and `$share` subscriptions included, is rejected per topic. The `event` handler then applies unchanged: owner of the device, else read access through the device-repository |

MQTT 3.1.1 closes the connection after a refused publish.

## Sweeper

VerneMQ checks the password only at login, so a session would outlive its
token. Every 60 seconds the connector lists the broker sessions through
`VMQ_ADMIN_API_URL`, and disconnects those with a `web:<exp>:` client id whose
`<exp>` is not in the future. The client has to reconnect with a fresh token:
an MQTT client that reconnects on its own sends the password it connected with,
so it has to be given the new token before every reconnect, or it is refused
over and over.

The sweeper starts with the webhooks, only if `TOKEN_LOGIN_MOUNTPOINT` is set.
Without `VMQ_ADMIN_API_URL` (empty or `-`) it does not start and logs that once;
expired sessions then stay connected. The listing asks for all sessions
(`vmq-admin session show --client_id --limit=1000000`).

## Deployment order

Roll out the connector with this feature before the broker gets the `web`
listener. An older connector does not know the mountpoint and treats a login on
the `web` listener like a certificate login: it trusts the username and does not
look at the password.
