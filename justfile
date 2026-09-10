set shell := ["sh", "-eu", "-c"]
set positional-arguments

# List available tasks.
default:
    @just --list

# Build every subproject.
build: (all "build")

# Run every subproject's tests.
test: (all "test")

# Lint every subproject.
lint: (all "lint")

# Format every subproject.
fmt: (all "fmt")

# Run a task across libraries, then apps. Stop on the first failure.
all task *args:
    #!/bin/sh
    set -eu
    found=false
    for project in libs/*/ apps/*/; do
        [ -d "$project" ] || continue
        found=true
        just --justfile "${project}justfile" "$@"
    done
    if [ "$found" = false ]; then
        echo 'No subprojects yet.'
    fi

# Run a subproject task: just run apps/cli test (or omit the task for its default).
run project *args:
    @project="$1"; shift; just --justfile "$project/justfile" "$@"

# Check the root task routing in a temporary workspace.
self-check:
    @sh tests/tasks.sh
