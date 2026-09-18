#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 BASELINE_FILE TARGET_COPY" >&2
  exit 64
fi

baseline=$1
target=$2
cp "$baseline" "$target"
expected=$(shasum -a 256 "$baseline" | awk '{print $1}')
actual=$(shasum -a 256 "$target" | awk '{print $1}')
test "$actual" = "$expected"

if sed -n '/func (a \\*Account) IsCustomBaseURLEnabled/,/return false/p' "$target" | grep -q 'IsOpenAIOAuth'; then
  echo "rollback verification failed: OpenAI OAuth custom relay support remains" >&2
  exit 1
fi

printf 'restored_behavior=anthropic_oauth_setup_token_only_custom_relay\n'
printf 'restored_status=openai_oauth_custom_relay_absent\n'
printf 'restored_sha256=%s\n' "$actual"
