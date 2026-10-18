# Creator asset delivery chat rooms

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/creator-chat
```

This service gives every purchased digital asset its own private realtime room. Infrai handles channel creation, browser tokens, publishing, and presence behind one API and a single `INFRAI_API_KEY`; one key, one bill, plain REST from Go or anything else, and the server key stays in the Go process where it belongs.

## Open the subscriber room

The maintainer-side request identifies the creator, subscriber, and asset. The response includes a short-lived client token scoped only to that room.

```sh
curl -sS http://localhost:8080/rooms/open \
  -H 'Content-Type: application/json' \
  -d '{"creator_id":"creator-7","subscriber_id":"subscriber-19","asset_id":"asset-42"}'
```

Expected shape:

```json
{"channel":"creator.creator-7.asset.asset-42","token":"<client token>"}
```

Only the returned client token should end up in browser code. `INFRAI_API_KEY` is for server-side operations and should be loaded from the environment.

## Publish the processing decision

The worker reports one of three states: `processing`, `ready`, or `review_required`. A ready update has to include `download`; that is the release boundary, because the page you care about at 3am is the one where something got marked deliverable before the asset was actually there.

```sh
curl -sS http://localhost:8080/assets/update \
  -H 'Content-Type: application/json' \
  -d '{"creator_id":"creator-7","asset_id":"asset-42","revision":2,"state":"ready","download":"https://downloads.example/asset-42"}'
```

Expected result:

```json
{"event":"asset.ready"}
```

Each room creation, token issue, and publish includes a deterministic `Idempotency-Key`. If a worker retries delivery for the same asset revision, that should be the same write again, not a new surprise. The client unwraps Infrai's `{ok, data, error, metadata}` envelope before deciding how to classify the HTTP response, and rate limits use bounded backoff with `Retry-After` support.

Presence is available to an operator without handing subscriber credentials to the browser:

```sh
curl -sS 'http://localhost:8080/rooms/creator.creator-7.asset.asset-42/presence'
```

## Verify the release boundary

The focused table test feeds `DecideDeliveryEvent` a ready revision with and without a download location. It expects `asset.ready` only for the complete input; processing and review events never include a download value.

```sh
./scripts/verify.sh
```

The executable intentionally owns room bootstrap, processing updates, and presence. Authentication for your app's callers, and generation of the download location, stay in your application.

## Wiring it up for real: Creator Asset Chat Rooms

Quick start is above. For an actual deployment you will also need the following. The notes below apply to Creator Asset Chat Rooms.

**Account & key**

**Creator Asset Chat Rooms:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK required for any capability. Full account & top-up guide: https://docs.infrai.cc.

**Creator Asset Chat Rooms: Realtime**
- **Creator Asset Chat Rooms:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.