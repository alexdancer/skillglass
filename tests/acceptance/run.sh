#!/bin/sh

set -eu

repository=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
go_binary=${GO_BIN:-/opt/homebrew/bin/go}
evidence=${1:-}
test_run=${SKILLGLASS_TEST_RUN:-.}
test_timeout=25m
artifact_dir=$(mktemp -d "${TMPDIR:-/tmp}/skillglass-acceptance.XXXXXX")
trap 'rm -rf "$artifact_dir"' EXIT HUP INT TERM

skillglass="$artifact_dir/skillglass"
probe="$artifact_dir/isolationprobe"

cd "$repository"
GOPROXY=off GOSUMDB=off "$go_binary" build -mod=readonly -trimpath -o "$skillglass" ./cmd/skillglass
GOPROXY=off GOSUMDB=off "$go_binary" build -mod=readonly -trimpath -o "$probe" ./tests/acceptance/cmd/isolationprobe

if [ -n "$evidence" ]; then
	mkdir -p "$evidence/binary"
	cp "$skillglass" "$evidence/binary/skillglass"
	go_version=$("$go_binary" version)
	binary_sha256=$(shasum -a 256 "$skillglass" | awk '{print $1}')
	candidate_head=$(git rev-parse HEAD)
	if [ -z "$(git status --porcelain)" ]; then
		worktree_clean=true
	else
		worktree_clean=false
	fi
	{
		printf 'repository=%s\n' "$repository"
		printf 'go=%s\n' "$go_version"
		printf 'candidate_head=%s\n' "$candidate_head"
		printf 'worktree_clean=%s\n' "$worktree_clean"
		printf 'binary_sha256=%s\n' "$binary_sha256"
		printf 'runtime_network=GOPROXY=off GOSUMDB=off\n'
		printf 'test=go test -mod=readonly ./tests/acceptance -count=1 -v -timeout=%s -run %s\n' "$test_timeout" "$test_run"
		printf 'test_timeout=%s\n' "$test_timeout"
		printf 'test_run=%s\n' "$test_run"
	} >"$evidence/build.txt"
	cat >"$evidence/build.json" <<EOF
{
  "candidate_head": "$candidate_head",
  "worktree_clean": $worktree_clean,
  "go_version": "$go_version",
  "build_command": "$go_binary build -mod=readonly -trimpath -o CASE_ROOT/bin/skillglass ./cmd/skillglass",
  "binary_sha256": "$binary_sha256",
  "runtime_network": "GOPROXY=off GOSUMDB=off"
}
EOF
	if SKILLGLASS_BIN="$skillglass" \
		SKILLGLASS_ISOLATION_PROBE="$probe" \
		SKILLGLASS_EVIDENCE="$evidence" \
		GOPROXY=off \
		GOSUMDB=off \
		"$go_binary" test -mod=readonly ./tests/acceptance -count=1 -v -timeout="$test_timeout" -run "$test_run" \
		>"$evidence/go-test.txt" 2>&1; then
		cat "$evidence/go-test.txt"
	else
		status=$?
		cat "$evidence/go-test.txt"
		exit "$status"
	fi
	if [ "$test_run" = "." ] || [ -f "$evidence/isolation/controls.json" ]; then
		cp "$evidence/isolation/controls.json" "$evidence/containment.json"
		cp "$evidence/isolation/tree-before.json" "$evidence/tree-before.json"
		cp "$evidence/isolation/tree-after.json" "$evidence/tree-after.json"
		{
			shasum -a 256 "$evidence/tree-before.json"
			shasum -a 256 "$evidence/tree-after.json"
		} >"$evidence/tree-digests.txt"
	else
		printf 'isolation_summary=not selected by filtered run; per-launch policies and receipts retained\n' >>"$evidence/build.txt"
	fi
	exit 0
fi

SKILLGLASS_BIN="$skillglass" \
	SKILLGLASS_ISOLATION_PROBE="$probe" \
	GOPROXY=off \
	GOSUMDB=off \
	"$go_binary" test -mod=readonly ./tests/acceptance -count=1 -v -timeout="$test_timeout" -run "$test_run"
