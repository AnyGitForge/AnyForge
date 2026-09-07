# AnyForge

Route git commits from any source to any target.

A single Go binary you point your git remote at. It accepts your pushes instantly, stores them durably, and forwards them upstream when upstream is available.

> Status: pre-alpha. Nothing works yet.

## Why

Git forges go down, rate-limit you, and lock your CI to wherever your code happens to live. Meanwhile a pile of new git-storage backends are shipping — easy to spin up, hard to actually operationalize.

AnyForge is the routing layer between them.

## Use cases

1. **Redundancy** — keep working when GitHub is down. Push to AnyForge, it drains upstream on recovery.
2. **Agentic loops** — commit as fast as your agent wants. AnyForge absorbs it and coalesces before hitting upstream rate limits.
3. **CI/CD portability** — trigger a job on any service, regardless of where the code lives.

## How it works

```
git push ──> AnyForge ──drain──> GitHub
              │
              └─ durable outbox (survives restart)
```

AnyForge speaks git smart-HTTP, so it's a normal remote:

```bash
git remote add anyforge http://localhost:8080/owner/repo.git
git push anyforge main
```

Runs on your local machine, bound to loopback. It drains whenever your machine is online; the outbox survives sleep and restart, so nothing is lost in between.

## Design commitments

- **Durable outbox.** It's the only thing between you and lost work. Crash-safe, idempotent replay.
- **No secrets by default.** Uses your existing SSH agent. Nothing stored.
- **Pure Go, no `git` dependency.** Objects and protocol via `go-git`, so the binary is the whole install. Maintenance-only shell-outs (`gc`, `repack`) stay off the request path.
- **Adapters, not special cases.** GitHub is one implementation of a target, not the model.

## License

Apache 2.0
