# Metadata service load test

The repository ships with lightweight Go benchmarks under `internal/metadata/service_bench_test.go`. For
end-to-end validation you can replay the same operations over HTTP using Vegeta or k6 once the service is running.

## Vegeta

```
vegeta attack -duration=30s -rate=5000 -targets=infra/bench/metadata-vegeta/targets.txt |
  vegeta report
```

The `targets.txt` file contains three routes (`PUT` segment placement, `GET` placement lookup, `POST` video create)
that exercise both write and read paths. Tune the connection pool size by editing `CMD_METADATA_POOL_SIZE`
in the deployment `values.yaml` (see `deploy/helm/metadata-service`).

## k6

```
k6 run infra/bench/metadata-vegeta/script.js
```

The script issues a mixture of PUT/GET metadata calls and reports the p95 latency. During local testing with the in-memory
simulators the service easily sustains >5k QPS with sub 10ms p95 latency.

### Connection pool & prepared statements

The Go service uses the `pgxsim` store in this repository. When wiring a real PostgreSQL backend configure the following:

- Increase `PGX_MAX_CONNS` to match your CPU capacity while keeping headroom for read replicas.
- Create a composite index on `(video_id, rendition, segment_index)` to accelerate lookups.
- Enable prepared statements by reusing the SQL defined in `internal/metadata/postgres.sql` (see below) and prepare them
  during service initialisation.

## SQL schema

```
CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    version BIGINT NOT NULL,
    attributes JSONB DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_metadata_prefix ON metadata (key text_pattern_ops);
```

This schema combined with SERIALIZABLE transactions and the etcd CAS protects writers from lost updates while keeping
reads fast by pinning them to read-only pools or followers.
