# VectorScan

VectorScan is a learning-first vector database project in Go.

Current milestone: **V3**.

- V0: in-memory insert/get/delete
- V1: cosine similarity + exact brute-force top-K search
- V2: concurrency safety with `sync.RWMutex`
- V3: HTTP API using only the Go standard library

Persistence, WAL, mmap, HNSW, replication, and sharding are intentionally not part of this milestone.

## Run

```bash
go run ./cmd/vectorscan
```

The server listens on `:8080`.

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

## V3 architecture

```text
process startup
    |
    v
NewDB() -------- one shared DB instance
    |
    v
net/http Server
    |
    +-- handler goroutine ---- Insert ---- Lock
    |
    +-- handler goroutine ---- Get ------- RLock
    |
    +-- handler goroutine ---- Search ---- RLock + brute-force scan
    |
    +-- handler goroutine ---- Delete ---- Lock
```

`net/http` creates goroutines for concurrent requests. VectorScan does not create a request worker pool in V3.

## Memory ownership

The DB copies both the input `[]float32` and metadata map on insertion. `Get` also returns copies. This prevents callers from mutating DB-owned memory without going through the DB API.

The in-memory shape is roughly:

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

## Search complexity

V3 performs exact brute-force search. For `N` vectors of dimension `d`, cosine scoring is `O(N*d)`. The current implementation then sorts all `N` results, adding `O(N log N)` time and `O(N)` result memory.

That is deliberately simple. A bounded top-K heap is a later optimization, and an ANN index such as HNSW comes only after the baseline is measured.

## Next milestone

V4 will add persistence so vectors survive process restart. After that we can introduce WAL/recovery and benchmark the storage/search paths before discussing HNSW.
