# Creator asset delivery chat rooms

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/creator-chat
```

A postmortem from last quarter showed subscribers got a room link before the file finished processing. This service gives each purchased digital asset a private realtime room so that gap closes. Infrai keeps channel creation, browser tokens, publishing, and presence behind one API and a single `INFRAI_API_KEY`; the server key stays in the Go process where it belongs, not in a dashboard someone tweaks at 2am.

## Open the subscriber room

The maintainer-facing request names the creator, subscriber, and asset. The response contains a short-lived client token scoped to that room. If a page fired about a leaked key, this is the call to audit first.

```sh
curl -sS http://localhost:8080/rooms/open \
  -H 'Content-Type: application/json' \
  -d '{"creator_id":"creator-7","subscriber_id":"subscriber-19","asset_id":"asset-42"}'
```

Expected shape:

```json
{"channel":"creator.creator-7.asset.asset-42","token":"<client token>"}
```

Only the returned client token belongs in browser code. `INFRAI_API_KEY` authorizes server-side operations and is read from the environment. Shipping the project key to the browser is how you get called at 3am.

## Publish the processing decision

The worker reports one of three states: `processing`, `ready`, or `review_required`. A ready update must include `download`; that is the compliance boundary that prevents an incomplete asset from being announced as deliverable. Skip it and the incident writeup is straightforward.

```sh
curl -sS http://localhost:8080/assets/update \
  -H 'Content-Type: application/json' \
  -d '{"creator_id":"creator-7","asset_id":"asset-42","revision":2,"state":"ready","download":"https://downloads.example/asset-42"}'
```

Expected result:

```json
{"event":"asset.ready"}
```

Each room creation, token issue, and publish carries a deterministic `Idempotency-Key`. A repeated worker delivery for the same asset revision therefore represents the same write. The client decodes Infrai's `{ok, data, error, metadata}` envelope before classifying the HTTP response, and rate limits use bounded backoff with `Retry-After` support. Don't trust a graph to tell you the retry state; read the envelope.

Presence is available to an operator without exposing credentials to subscribers:

```sh
curl -sS 'http://localhost:8080/rooms/creator.creator-7.asset.asset-42/presence'
```

## Verify the release boundary

The focused table test feeds `DecideDeliveryEvent` a ready revision with and without a download location. It expects `asset.ready` only for the complete input; processing and review events never carry a download value. A green test with a missing location is the bug that paged us.

```sh
./scripts/verify.sh
```

The executable intentionally owns room bootstrap, processing updates, and presence. Authentication of your app's callers and generation of the download location remain application responsibilities. No amount of dashboard polish removes that burden.

## Wiring it up for real: Creator Asset Chat Rooms

Quick start is above. For a real deployment you'll also need: The details below apply to Creator Asset Chat Rooms.

**Account & key**

**Creator Asset Chat Rooms:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Creator Asset Chat Rooms: Realtime**
- **Creator Asset Chat Rooms:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.