#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
export GOCACHE="${GOCACHE:-$repo_root/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$repo_root/.cache/go-mod}"
export GOTOOLCHAIN=auto
mkdir -p .local
run_dir="$(mktemp -d "$repo_root/.local/e2e.XXXXXX")"
target="$(go env GOOS)-$(go env GOARCH)"
binary="$repo_root/dist/$target/readerhub"
"$binary" serve --listen 127.0.0.1:18848 --data "$run_dir/data" >"$run_dir/hub.log" 2>&1 &
hub_pid=$!
trap 'kill "$hub_pid" 2>/dev/null || true; wait "$hub_pid" 2>/dev/null || true' EXIT
export READERHUB_TEST_URL=http://127.0.0.1:18848
export READERHUB_TEST_TOKEN_FILE="$run_dir/data/access-token"
export READERHUB_CLI_CONFIG="$run_dir/client.json"
export READERHUB_TEST_CLI="$repo_root/dist/$target/readerhub-cli"
python3 - <<'PY'
import json, os, subprocess, time, urllib.request
from pathlib import Path
for attempt in range(50):
    try:
        urllib.request.urlopen(os.environ['READERHUB_TEST_URL']+'/health', timeout=1).close()
        break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('E2E Hub did not start; check isolated Hub log')
env=os.environ.copy();env['READERHUB_TOKEN']=Path(env['READERHUB_TEST_TOKEN_FILE']).read_text()
r=subprocess.run([env['READERHUB_TEST_CLI'],'add','tests/fixtures/reading/ReaderHub使用指南.md','--hub',env['READERHUB_TEST_URL'],'--project','ReaderHub'],env=env,capture_output=True,text=True)
if r.returncode:
    raise SystemExit(r.stderr)
receipt=json.loads(r.stdout)
assert receipt['images'] == 1
print('E2E guide uploaded through real CLI')
PY
cd web
npm run test:e2e -- "$@"
