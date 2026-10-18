#!/bin/sh
# Build from cached dependencies into a new, explicit local destination.
set -eu
[ "$(uname -s)" = Darwin ] || { echo 'Local delivery requires macOS.' >&2; exit 1; }
[ "$#" -eq 1 ] || { echo 'Usage: tools/build-local.sh OUTPUT_DIRECTORY' >&2; exit 2; }
case "$1" in /*) destination=$1 ;; *) echo 'OUTPUT_DIRECTORY must be absolute.' >&2; exit 2 ;; esac
[ ! -e "$destination" ] && [ ! -L "$destination" ] || { echo 'OUTPUT_DIRECTORY must not exist.' >&2; exit 1; }
repository=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
go_binary=${GO_BIN:-/opt/homebrew/bin/go}
version=$("$go_binary" version)
case "$version" in 'go version go1.27.1 darwin/'*) ;; *) echo "Expected Go 1.27.1 on Darwin; got $version" >&2; exit 1 ;; esac
scratch=$(mktemp -d /private/tmp/skillglass-build.XXXXXX)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
cd "$repository"
GOPROXY=off GOSUMDB=off "$go_binary" build -mod=readonly -trimpath -o "$scratch/skillglass" ./cmd/skillglass
mkdir -p "$(dirname -- "$destination")"
mkdir "$destination"
install -m 0755 "$scratch/skillglass" "$destination/skillglass"
cmp "$scratch/skillglass" "$destination/skillglass"
{
 printf 'repository=%s\n' "$repository"
 printf 'candidate_head=%s\n' "$(git rev-parse HEAD)"
 if [ -z "$(git status --porcelain)" ]; then printf 'worktree_clean=true\n'; else printf 'worktree_clean=false\n'; fi
 printf 'go=%s\narchitecture=%s\n' "$version" "$(uname -m)"
 printf 'command=GOPROXY=off GOSUMDB=off %s build -mod=readonly -trimpath -o %s/skillglass ./cmd/skillglass\n' "$go_binary" "$scratch"
 printf 'delivered_path=%s/skillglass\n' "$destination"
 shasum -a 256 "$scratch/skillglass" "$destination/skillglass"
 "$go_binary" version -m "$destination/skillglass"
} > "$destination/build.txt"
printf '%s\n' "$destination/skillglass"
