# AnyForge

Store Git durably. Distribute changes to other forges.

AnyForge is a Go service for Git storage, serving, and distribution. It accepts pushes into durable storage, then independently delivers Git changes and repository events to configured destinations.

> Status: pre-alpha. Nothing works yet. This document describes the intended design.

## Why

Git forges can fail, rate-limit requests, and tie CI to where your code lives.

AnyForge accepts pushes without waiting for other forges, even while those forges are offline. AnyForge and its durable storage must remain available.

## Intended use cases

1. **Forge outages** — accept pushes while a destination is unavailable, then deliver after recovery.
2. **Agentic workflows** — absorb bursts of pushes and deliver within destination rate limits.
3. **CI/CD portability** — publish repository events for integrations that trigger jobs independently of the destination forge.

## Architecture

See [Architecture decisions](ARCHITECTURE.md) for storage, distribution, deployment roles, and open decisions.

## License

Apache 2.0
