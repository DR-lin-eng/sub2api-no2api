#!/bin/sh
set -eu

HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TARGET=${1:?usage: ROLLBACK.sh /absolute/path/to/independent/repository}
PATCH_FILE="$HERE/DIFF_FILE"
MODIFIED_FILE="$HERE/MODIFIED_FILE"
MODIFIED_SHA256=0725ea214b2395d5812e397e182cf5b875f7f265ac9669f5b2e2a62cecd0fd0c
ORIGINAL_SHA256=c0921e5339d2f4d12d7734b05049d1902f04cfc8f15701718215d0f01b183b60

[ -f "$PATCH_FILE" ] || {
	printf '%s\n' "rollback error: missing $PATCH_FILE" >&2
	exit 1
}
[ "$(shasum -a 256 "$MODIFIED_FILE" | awk '{print $1}')" = "$MODIFIED_SHA256" ] || {
	printf '%s\n' "rollback error: MODIFIED_FILE hash mismatch" >&2
	exit 1
}
[ -d "$TARGET/.git" ] || {
	printf '%s\n' "rollback error: target is not a git repository: $TARGET" >&2
	exit 1
}

changed=$(grep -c '^diff --git ' "$PATCH_FILE" || true)
[ "$(shasum -a 256 "$TARGET/docs/README.md" | awk '{print $1}')" = "$MODIFIED_SHA256" ] || {
	printf '%s\n' "rollback error: target is not at the modified state" >&2
	exit 1
}
git -C "$TARGET" apply --check --reverse "$PATCH_FILE"
git -C "$TARGET" apply --reverse "$PATCH_FILE"

[ "$(shasum -a 256 "$TARGET/docs/README.md" | awk '{print $1}')" = "$ORIGINAL_SHA256" ] || {
	printf '%s\n' "rollback error: original docs/README.md hash was not restored" >&2
	exit 1
}

if [ -n "$(git -C "$TARGET" status --short)" ]; then
	printf '%s\n' "rollback error: target remains dirty" >&2
	exit 1
fi

printf 'ROLLBACK restored=%s paths clean=true\n' "$changed"
