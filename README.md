# in_mem_Go

A small Redis-style in-memory key-value datastore written in Go and exposed over
HTTP. It supports TTL-based expiration, conditional set operations (NX / XX), and
FIFO queues (QPUSH / QPOP). The store is safe for concurrent use.

## Running

```sh
go run .
```

The server listens on `:8080`. Import `go_in_mem.postman_collection.json` into
Postman to try the endpoints, or use `curl` (examples below).

## Endpoints

### `POST /set`

Stores a key. The body carries a single space-delimited `command` string:

```json
{ "command": "<key> <value> [EX <n><unit>] [NX|XX]" }
```

- `EX <n><unit>` — optional TTL. Unit is `S`, `M`, `H`, or `D` (e.g. `EX 10M` = 10 minutes).
- `NX` — only set if the key does **not** already exist.
- `XX` — only set if the key **does** already exist.

Status codes: `201` set, `400` invalid command / expiry, `404` `XX` on a missing
key, `409` `NX` on an existing key, `500` internal error.

```sh
curl -X POST localhost:8080/set -d '{"command": "user1 alice EX 10M NX"}'
```

### `GET /get?key=<key>`

Returns `{"value": "<value>"}`. Status codes: `200` ok, `400` missing `key`
param, `404` key not found or expired.

```sh
curl 'localhost:8080/get?key=user1'
```

### `POST /qpush`

Appends one or more values to the queue at `<key>`:

```json
{ "command": "QPUSH", "args": ["<key>", "v1", "v2", "v3"] }
```

Status codes: `200` ok, `400` invalid command, `500` internal error.

```sh
curl -X POST localhost:8080/qpush -d '{"command":"QPUSH","args":["jobs","a","b"]}'
```

### `POST /qpop`

Removes and returns the **front** of the queue (FIFO):

```json
{ "command": "QPOP", "key": "<key>" }
```

Status codes: `200` `{"value": "<value>"}`, `404` queue missing or empty.

```sh
curl -X POST localhost:8080/qpop -d '{"command":"QPOP","key":"jobs"}'
```

### `GET /getall`

Returns every **live (non-expired)** key as a JSON object. Queue keys are joined
with `, `. Status codes: `200` ok, `500` internal error.

```sh
curl localhost:8080/getall
```

## Testing

```sh
go test ./...          # unit tests
go test -race ./...    # unit tests under the race detector
go test -bench=. ./... # QPUSH benchmarks
```

## Design notes

- `datastore` holds a `map[string]*dataValue` guarded by a `sync.RWMutex`.
- Each `dataValue` has its own `sync.Mutex` (`queueMu`) guarding its queue slice,
  so pops on different queues do not contend.
- `value` / `expTime` are immutable once a `*dataValue` is created, so reads only
  need a read lock.
- Expiration is **lazy**: an expired key is removed when it is next accessed
  (`/get`) or filtered out by `/getall`. There is no background sweeper.
- Lock ordering is always `mu -> queueMu`, which keeps the store deadlock-free.
