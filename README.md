# AnyForge

Store Git durably. Distribute changes to other forges.

AnyForge is a service for Git storage, serving, and distribution. It accepts pushes into durable storage, then independently delivers Git changes and repository events to configured destinations.

> Status: pre-alpha. The CLI starts an HTTP server. Git storage and distribution are not implemented.

## Why

Git forges can fail, rate-limit requests, and tie CI to where your code lives.

AnyForge accepts pushes without waiting for other forges, even while those forges are offline. AnyForge and its durable storage must remain available.

## Intended use cases

1. **Forge outages** — accept pushes while a destination is unavailable, then deliver after recovery.
2. **Agentic workflows** — absorb bursts of pushes and deliver within destination rate limits.
3. **CI/CD portability** — publish repository events for integrations that trigger jobs independently of the destination forge.

## Architecture

See [Architecture decisions](ARCHITECTURE.md) for storage, distribution, deployment roles, and open decisions.

## Development

The monorepo is language-agnostic. Linux and macOS are the initial development platforms.

- `apps/`: applications, primarily the CLI, with room for a web interface.
- `libs/`: shared libraries.
- `mise.toml`: pinned development tools, including Go for the CLI.
- `justfile`: root task entry points.

Install [mise](https://mise.jdx.dev/getting-started.html), then run:

```sh
mise trust
mise install
mise exec -- just
```

With mise activated in your shell, use `just` directly. All project tasks use justfiles, not mise tasks.

```sh
just build                     # Build all subprojects
just test                      # Run all subproject tests
just lint                      # Lint all subprojects
just fmt                       # Format all subprojects
just all <task> [args...]       # Run any task across all subprojects
just run apps/<name> [args...]  # Run a task or the subproject default
just run libs/<name> [args...]
just self-check                # Check root task routing
```

`apps/cli/` contains the Go CLI. `libs/` has no shared libraries yet.

Run the compiled-binary E2E tests separately from `just test`:

```sh
just run apps/cli e2e
```

The suite requires port `8080` to be free. Tests run serially and cover CLI errors, HTTP responses, and process shutdown.
No upstream service or credentials are required.

### Run the HTTP skeleton

Build the binary:

```sh
just build
```

Start the server:

```sh
apps/cli/bin/anyforge serve \
  --path owner/repo.git \
  --upstream git@github.com:owner/repo.git
```

The server listens on `127.0.0.1:8080` and returns HTTP 404 for all requests. Git endpoints are not implemented yet.

Both flags are required. The CLI accepts one repository mapping per process, without a config file.
The path must be relative, without empty segments, `.` or `..`, URL escapes, backslashes, whitespace, or query characters.
The `.git` suffix is optional. Upstreams accept SSH, HTTPS, HTTP, and Git URLs, or `[user@]host:path` syntax.

Startup validates the arguments but does not connect to the upstream or create repository storage.
Invalid arguments and bind failures produce an error and a nonzero exit status.
Ctrl+C and SIGTERM stop the server, with up to five seconds for active requests to finish.

Show the available flags:

```sh
apps/cli/bin/anyforge serve --help
```

### Adding a subproject

Create a directory directly under `apps/` or `libs/`. Give it its own `justfile` with `build`, `test`, `lint`, and `fmt` recipes.
Each recipe runs from its subproject directory. Add the required toolchains to `mise.toml`.

Aggregate tasks run libraries first, then apps, in shell glob order. They stop on the first failure, including missing justfiles or recipes.
They do not resolve dependencies between subprojects. Each subproject owns its build dependencies.

GitHub Actions runs the CLI E2E suite on pushes and pull requests.

## License

Apache 2.0
