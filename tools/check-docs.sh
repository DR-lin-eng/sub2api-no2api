#!/bin/sh

set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$repo_root"

required_files='AGENTS.md
backend/AGENTS.md
frontend/AGENTS.md
DEV_GUIDE.md
docs/README.md
docs/FEATURES.md
docs/ARCHITECTURE.md
docs/CODE_MAP.md
docs/REQUEST_LIFECYCLES.md
backend/README.md
frontend/README.md
frontend/src/common/README.md
deploy/README.md'

maintained_docs='AGENTS.md
backend/AGENTS.md
frontend/AGENTS.md
DEV_GUIDE.md
README.md
README_CN.md
README_JA.md
docs/README.md
docs/FEATURES.md
docs/ARCHITECTURE.md
docs/CODE_MAP.md
docs/REQUEST_LIFECYCLES.md
backend/README.md
frontend/README.md
frontend/src/common/README.md
frontend/src/core/routes/README.md
frontend/src/core/stores/README.md'

for feature_readme in frontend/src/features/*/README.md; do
	maintained_docs="$maintained_docs
$feature_readme"
done
for module_readme in backend/internal/modules/*/README.md; do
	maintained_docs="$maintained_docs
$module_readme"
done

required_paths='backend/cmd/server/main.go
backend/cmd/server/wire.go
backend/ent/schema
backend/internal/application/service
backend/internal/infrastructure/repository
backend/internal/platform/config
backend/internal/transport/http/server/router.go
backend/internal/transport/http/server/routes/gateway.go
backend/internal/transport/http/server/middleware/api_key_auth.go
backend/migrations
deploy/config.example.yaml
frontend/src/common
frontend/src/core/i18n/locales
frontend/src/core/networks/client.ts
frontend/src/core/routes/index.ts
frontend/src/core/stores
frontend/src/features
frontend/src/main.ts
'

status=0

if ! grep -Fq '../frontend/src/common/README.md' docs/FEATURES.md; then
	echo "docs error: common owner is not registered in docs/FEATURES.md" >&2
	status=1
fi

for path in $required_files; do
	[ -n "$path" ] || continue
	if [ ! -f "$path" ]; then
		echo "docs error: required file is missing: $path" >&2
		status=1
	fi
done

# Every feature and vertical backend module needs a local owner README and an
# entry in the feature index. Keep this check directory-driven so new domains
# cannot silently bypass the documentation inventory.
for feature_dir in frontend/src/features/*; do
	[ -d "$feature_dir" ] || continue
	feature=$(basename "$feature_dir")
	if [ ! -f "$feature_dir/README.md" ]; then
		echo "docs error: feature README is missing: $feature_dir/README.md" >&2
		status=1
	fi
	if ! grep -Fq "../frontend/src/features/$feature/README.md" docs/FEATURES.md; then
		echo "docs error: feature is not registered in docs/FEATURES.md: $feature" >&2
		status=1
	fi
done

for module_dir in backend/internal/modules/*; do
	[ -d "$module_dir" ] || continue
	module=$(basename "$module_dir")
	if [ ! -f "$module_dir/README.md" ]; then
		echo "docs error: module README is missing: $module_dir/README.md" >&2
		status=1
	fi
	if ! grep -Fq "../backend/internal/modules/$module/README.md" docs/FEATURES.md; then
		echo "docs error: module is not registered in docs/FEATURES.md: $module" >&2
		status=1
	fi
done

for path in $required_paths; do
	[ -n "$path" ] || continue
	if [ ! -e "$path" ]; then
		echo "docs error: documented source path is missing: $path" >&2
		status=1
	fi
done

for doc in $maintained_docs; do
	[ -n "$doc" ] || continue
	doc_dir=$(dirname "$doc")
	links=$(grep -Eo '\]\([^)]*\)' "$doc" 2>/dev/null | sed -E 's/^\]\((.*)\)$/\1/' || true)
	[ -n "$links" ] || continue

	for target in $links; do
		case "$target" in
			''|'#'*|http://*|https://*|mailto:*|tel:*) continue ;;
		esac

		target=${target%%#*}
		case "$target" in
			*' '*|'<'*|'>'*)
				echo "docs error: unsupported local link syntax in $doc: $target" >&2
				status=1
				continue
				;;
		esac

		if [ ! -e "$repo_root/$doc_dir/$target" ]; then
			echo "docs error: broken local link in $doc: $target" >&2
			status=1
		fi
	done
done

stale_output=$(mktemp "${TMPDIR:-/tmp}/sub2api-docs-stale.XXXXXX")
trap 'rm -f "$stale_output"' EXIT HUP INT TERM

for readme in README.md README_CN.md README_JA.md; do
	if grep -nE 'internal/(config|model|service|handler|gateway)/' "$readme" >"$stale_output" 2>/dev/null; then
		echo "docs error: $readme still contains a legacy backend layout path:" >&2
		cat "$stale_output" >&2
		status=1
	fi
done

go_version=$(awk '$1 == "go" { print $2; exit }' backend/go.mod)
for readme in README.md README_CN.md README_JA.md; do
	if ! grep -Fq "Go-$go_version-" "$readme"; then
		echo "docs error: $readme Go badge does not match backend/go.mod ($go_version)" >&2
		status=1
	fi
	if ! grep -Fq "Go $go_version, Gin, Ent" "$readme"; then
		echo "docs error: $readme tech stack does not match backend/go.mod ($go_version)" >&2
		status=1
	fi
done

if [ "$status" -ne 0 ]; then
	exit "$status"
fi

echo "Documentation checks passed."
