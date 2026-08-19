# VectorScan

VectorScan is a learning-first vector database project in Go.

Current milestone: **V4**.

- V0: in-memory insert/get/delete
- V1: cosine similarity + exact brute-force top-K search
- V2: concurrency safety with `sync.RWMutex`
- V3: HTTP API using only the Go standard library
- V4: crash-resistant snapshot persistence and bootstrap recovery

WAL, mmap, HNSW, replication, and sharding are intentionally not part of this milestone.

## Run

```bash
go run ./cmd/vectorscan
```

The server listens on `:8080`.

By default VectorScan loads from and saves to:

```text
vectorscan.snapshot.json
```

Override the path with:

```bash
VECTORSCAN_SNAPSHOT=/path/to/snapshot.json go run ./cmd/vectorscan
```

On first boot, a missing snapshot starts an empty DB. A corrupt snapshot fails startup instead of silently discarding data. On graceful shutdown, VectorScan saves a new snapshot.

## API

Insert or replace a vector:

```bash
curl -i -X POST localhost:8080/vectors \
  -H 'Content-Type: application/json' \
  -d '{"id":"vec-1","values":[1,0,0],"metadata":{"source":"demo"}}'
```

Get it:

```bash
curl -i localhost:8080/vectors/vec-1
```

Search:

```bash
curl -i -X POST localhost:8080/search \
  -H 'Content-Type: application/json' \
  -d '{"vector":[1,0,0],"k":5}'
```

Delete it:

```bash
curl -i -X DELETE localhost:8080/vectors/vec-1
```

Health/state summary:

```bash
curl -i localhost:8080/healthz
```

## Test

```bash
go test ./...
go test -race ./...
```

## V4 architecture

```text
startup
  |
  v
snapshot file -- Load() --> validate --> rebuild DB in RAM
                                      |
                                      v
                                  HTTP server
                                      |
                     +----------------+----------------+
                     |                |                |
                   Insert           Get/Search       Delete
                    Lock              RLock            Lock

snapshot save
  |
  v
RLock
  |
  +-- deep-copy dimension + vectors + metadata
  |
RUnlock
  |
  v
JSON encode -> temp file -> fsync -> close -> atomic rename -> fsync directory
```

The important locking boundary is that disk I/O happens **after** `RUnlock`. Writers pause only while a point-in-time RAM copy is created; they do not wait for JSON encoding or disk `fsync`.

## Snapshot format vs RAM format

The durable `snapshot` / `snapshotVector` structs are separate from the in-memory `DB` / `Vector` structs. That lets the RAM layout change later without automatically changing the on-disk contract.

The current in-memory shape remains:

```text
DB
 |
 +-- map[string]*Vector
         |
         +-- id ---> Vector
                     |
                     +-- Values ---> float32 backing array
                     +-- Metadata -> map storage
```

A future version may move vectors into a contiguous RAM arena and keep ID-to-offset references. V4 intentionally does not make that redesign yet.

## V4 durability boundary

V4 is snapshot persistence, not per-write durability.

If a snapshot is saved at 10:00, a vector is inserted at 10:01, and the process crashes before another snapshot is saved, that vector can be lost.

V5 will solve that with a write-ahead log (WAL) and recovery replay.

## Search complexity

VectorScan still performs exact brute-force search. For `N` vectors of dimension `d`, cosine scoring is `O(N*d)`. The current implementation then sorts all `N` results, adding `O(N log N)` time and `O(N)` result memory.
