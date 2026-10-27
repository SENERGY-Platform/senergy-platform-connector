# MGW health reports

Gateways (MGW) publish a health summary on the MQTT topic `health`; the handler
in `lib/handler/health` forwards it to Kafka. The payload is defined by
mgw-device-cloud-connector (`docs/health-reporting.md` there), the channels are
in `docs/asyncapi.json`.

## Applies when

You change the health handler, its configuration, or the Kafka message it
produces. connection-log-worker consumes `gateway_health` and parses the
message below; a change to it reaches the worker.

## The client id is checked against the user

Login does not bind the MQTT client id to a hub of the user (`CHECK_HUB` is
false in prod), so any client with valid credentials can connect under any
client id. Before forwarding, the handler checks that the client id is a hub
the publishing user can access (`Iot().ExistsHub(clientId, userToken)`).
Positive results are cached per (user, client id) for
`HEALTH_HUB_CHECK_CACHE_DURATION`; negative results and errors are not cached.
Admins go through the same check.

`ExistsHub` in platform-connector-lib returns `exists=true` together with an
error when the request fails (status 0 is below 300). The error has to be
checked first, otherwise every report passes while device-repository is
unreachable.

## Rejections

| Case | Result |
|---|---|
| payload above `HEALTH_MAX_PAYLOAD_SIZE` (64 KiB) | rejected |
| not a UTF-8 JSON object | rejected |
| client id not a hub of the user | rejected |
| token, hub check or Kafka failure | error |
| subscribe on `health` | rejected |

Every rejected or failed publish carries an error, because the VerneMQ webhook
reads it for both results.

## Kafka message

Topic `GATEWAY_HEALTH_TOPIC` (`gateway_health`), key = hub id:

```json
{"hub_id": "urn:infai:ses:hub:...", "user_id": "...", "time": "2026-10-27T14:00:00.123456789Z", "report": {"...": "MQTT payload, byte for byte"}}
```

`time` is the connector's receive time with a fixed nine-digit fraction, so
string order matches time order; consumers order by it, not by the gateway
clock in `report.time`.

## Configuration

`HEALTH_TOPIC` defaults to `-`, which disables the handler. It is enabled in
prod only together with the Kafka topic.
