#!/bin/sh
# Exercise the preparation script without network access or real credentials.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM
mkdir -p "$test_dir/bin"

cat > "$test_dir/bin/curl" <<'SH'
#!/bin/sh
set -eu
case "$2" in
    */docker-compose.local.yml) source=docker-compose.local.yml ;;
    */.env.example) source=.env.example ;;
    *) exit 1 ;;
esac
cp "$DEPLOY_TEST_FIXTURE_ROOT/$source" "$4"
SH

cat > "$test_dir/bin/openssl" <<'SH'
#!/bin/sh
set -eu
if [ "$3" = 6 ]; then
    case "$DEPLOY_TEST_OPENSSL_MODE" in
        failure) exit 1 ;;
        empty) exit 0 ;;
    esac
    printf '%s\n' abcdef123456
else
    printf '%s\n' 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
fi
SH
chmod +x "$test_dir/bin/curl" "$test_dir/bin/openssl"
export PATH="$test_dir/bin:$PATH"
export DEPLOY_TEST_FIXTURE_ROOT="$repo_root/deploy"

for mode in success failure empty; do
    mkdir "$test_dir/$mode"
    if (cd "$test_dir/$mode" && DEPLOY_TEST_OPENSSL_MODE="$mode" bash "$repo_root/deploy/docker-deploy.sh" > output.txt 2>&1); then
        [ "$mode" = success ] || { echo "Unexpected success: $mode"; exit 1; }
        grep -q '^ADMIN_EMAIL=admin-abcdef123456@sub2api.local$' "$test_dir/$mode/.env"
        [ -d "$test_dir/$mode/data" ]
    else
        [ "$mode" != success ] || { echo "Unexpected preparation failure"; exit 1; }
        [ ! -e "$test_dir/$mode/.env" ] || { echo "Credentials written after RNG failure"; exit 1; }
    fi
done
echo 'Docker deployment admin credential tests passed.'
