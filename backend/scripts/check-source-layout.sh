#!/bin/sh

set -eu

backend_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
allowlist="$backend_root/.source-layout-allowlist"
max_lines=1200
status=0

for legacy_dir in config handler middleware model payment pkg repository securityaudit server service setup util web; do
	if [ -d "$backend_root/internal/$legacy_dir" ]; then
		echo "layout error: legacy directory internal/$legacy_dir must not be restored" >&2
		status=1
	fi
done

if [ ! -f "$allowlist" ]; then
	echo "layout error: missing .source-layout-allowlist" >&2
	exit 1
fi

current=$(mktemp)
allowed=$(mktemp)
new_entries=$(mktemp)
stale_entries=$(mktemp)
trap 'rm -f "$current" "$allowed" "$new_entries" "$stale_entries"' EXIT HUP INT TERM

find "$backend_root/internal" "$backend_root/cmd" -type f -name '*.go' \
	-not -path "$backend_root/ent/*" -not -name 'wire_gen.go' -print0 \
	| xargs -0 wc -l \
	| awk -v root="$backend_root/" -v limit="$max_lines" \
		'$1 > limit && $2 != "total" {sub("^" root, "", $2); print $2}' \
	| sort -u >"$current"

awk '!/^#/ && NF {print}' "$allowlist" | sort -u >"$allowed"
comm -23 "$current" "$allowed" >"$new_entries"
comm -13 "$current" "$allowed" >"$stale_entries"

if [ -s "$new_entries" ]; then
	status=1
	while IFS= read -r relative; do
		lines=$(wc -l <"$backend_root/$relative" | tr -d ' ')
		echo "layout error: $relative has $lines lines (limit $max_lines); split by responsibility" >&2
	done <"$new_entries"
fi

if [ -s "$stale_entries" ]; then
	status=1
	while IFS= read -r relative; do
		if [ -f "$backend_root/$relative" ]; then
			lines=$(wc -l <"$backend_root/$relative" | tr -d ' ')
			echo "layout error: stale allowlist entry $relative now has $lines lines; remove it" >&2
		else
			echo "layout error: stale allowlist entry $relative (file missing)" >&2
		fi
	done <"$stale_entries"
fi

exit "$status"
