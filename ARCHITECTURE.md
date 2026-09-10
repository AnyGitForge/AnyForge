# Architecture decisions

These decisions describe the intended architecture, not implemented behavior.

## 1. One binary with independent roles

The CLI HTTP skeleton uses Go and its standard library. The monorepo structure is language-agnostic. `go-git` remains a candidate for Git serving, storage, maintenance, and distribution.

The same binary supports serving, maintenance, and distribution roles. Roles can run together for a small installation or in separate processes.

Distribution does not require the serving process. It uses durable storage and can restart or scale independently, subject to consumer coordination.

- **Git storage and serving:** accept pushes and serve fetches and clones.
- **Distribution:** transfer Git changes to other forges and publish repository events through destination adapters.
- **Authentication and authorization:** identify callers and enforce repository permissions and ref policies before acceptance.

Social records and disposable read models remain outside AnyForge.

## 2. Disposable processing, durable storage

Durable storage is the source of truth. Process memory and local repository files are disposable caches, not the only copies of accepted work.

S3-compatible object storage holds Git objects, the write-ahead log (WAL), repository manifests, checkpoints, and delivery progress. Other backends must provide equivalent durability and consistency guarantees.

Replacement instances recover from durable storage. Reads validate repository state against durable storage before they use cached state.

If durable storage is unavailable, AnyForge cannot acknowledge new pushes. Durable acceptance does not depend on destination-forge availability.

## 3. WAL-first acceptance

AnyForge validates objects and proposed ref updates, then uploads immutable objects and a log entry.

A conditional update of the repository manifest commits the change. This compare-and-swap operation is the commit point, not the earlier uploads.

On a concurrent update, AnyForge reloads state and validates the ref updates again before retrying.

A successful push means that the required objects and ref changes are durable. It does not mean that another forge accepted them.

Reads include committed changes before background materialization finishes.

## 4. One log, independent consumers

The WAL supports recovery, storage materialization, forge delivery, and repository-event publication. No separate outbox duplicates change payloads.

Each committed repository change has a stable identifier, repository identifier, ref updates, durable object references, and required actor context.

Distribution reads committed changes from the log. The push path does not call destinations. Internal maintenance records do not produce ref events.

Consumers maintain independent durable checkpoints. Delivery failure adds pending work but does not undo or delay durable push acceptance.

## 5. Distribution reads accepted changes from storage

Distribution is a background reader of durable storage. The serving process does not send it jobs or wait for delivery.

- **The committed log** tells workers what changed.
- **Git objects** supply the actual repository content.
- **A delivery checkpoint** is a durable bookmark for one repository and destination. It records how far delivery succeeded.

```text
Git client -> serving role -> durable objects + log -> manifest CAS
                    |                                      |
                    +------ push success after CAS <-------+

                       Durable storage
                  committed log + Git objects
                       /             \
                      v               v
                GitHub worker    AT Protocol worker
                 pushes Git       publishes events
                      |               |
                 GitHub bookmark  AT Protocol bookmark
                    (both saved in durable storage)
```

### Example: main moves from A to B

AnyForge stores the required objects and records `main: A -> B` as log entry 42. The manifest update commits that entry.

The client receives push success. GitHub has not necessarily received the change.

The GitHub worker then:

1. Loads its checkpoint for this repository: entry 41.
2. Reads the repository manifest and discovers committed entry 42.
3. Reads entry 42 and gets the required Git objects from durable storage.
4. Pushes the change to GitHub, subject to destination state and permissions.
5. After successful delivery, saves checkpoint 42 in durable storage.

An AT Protocol worker reads the same entry but publishes a repository-event record instead of Git objects. It saves its own checkpoint independently.

Workers poll repository manifests to discover changes. Optional storage notifications reduce latency but do not replace polling or durable checkpoints.

### Failures and restarts

If GitHub is unavailable, its checkpoint stays at 41 and the worker retries with backoff. Other destinations continue independently.

If a worker stops, its replacement resumes from the durable checkpoint. It does not need the original serving process or its local files.

If delivery succeeds but checkpoint persistence fails, the replacement can attempt delivery again. Delivery is therefore at least once.

Adapters use event deduplication or Git-state reconciliation before repeating an operation. A checkpoint advances only after successful delivery or confirmation of equivalent destination state.

Workers preserve required per-repository ordering. Concurrent workers must coordinate ownership or checkpoint updates to prevent skipped work.

AnyForge exposes destination rejection and divergence rather than silently overwriting destination state.

## 6. Retention protects pending work

Log entries and required objects remain available until recovery and delivery no longer need them.

Materialization alone does not permit log deletion. Pending delivery also protects required objects from garbage collection.

Stalled destinations require an explicit retention, archive, or resynchronization policy. AnyForge never silently discards pending work.

## Open decisions

- Exact object-store requirements, log format, and manifest schema.
- Consumer ownership and checkpoint coordination.
- Retention limits and stalled-destination policy.
- Safe coalescing rules for pending ref updates.
- Git library compatibility and maintenance requirements.
