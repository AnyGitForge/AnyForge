#!/bin/sh
set -eu

workspace=$(mktemp -d)
trap 'rm -rf "$workspace"' EXIT HUP INT TERM
cp justfile "$workspace/justfile"
cd "$workspace"
mkdir apps libs

[ "$(just build)" = 'No subprojects yet.' ]
mkdir -p libs/example apps/example
for project in libs/example apps/example; do
    printf '%s\n' "$project" > "$project/identity"
    cat > "$project/justfile" <<'JUST'
set positional-arguments

default:
    @cat identity

build:
    @cat identity

test *args:
    @printf '<%s>\n' "$@"

fail:
    @exit 1
JUST
done

[ "$(just build)" = "$(printf 'libs/example\napps/example')" ]
[ "$(just run apps/example)" = 'apps/example' ]
[ "$(just run apps/example test 'two words' 'a"b')" = "$(printf '<two words>\n<a"b>')" ]
[ "$(just all test 'two words')" = "$(printf '<two words>\n<two words>')" ]

if just all fail >/dev/null 2>&1; then
    echo 'A failed task must fail the aggregate.' >&2
    exit 1
fi
if just lint >/dev/null 2>&1; then
    echo 'A missing task must fail the aggregate.' >&2
    exit 1
fi
rm libs/example/justfile
if just build >/dev/null 2>&1; then
    echo 'A missing justfile must fail the aggregate.' >&2
    exit 1
fi

echo 'Task routing checks passed.'
