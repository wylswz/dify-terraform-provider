#!/usr/bin/env bash
# Boots the upstream Dify Docker Compose stack pinned to scripts/dify-commit
# (langgenius/dify images tagged with that commit SHA), initializes the console,
# issues a Personal Access Token, and runs `make testacc` against it.
#
# Usage:
#   bash scripts/dify-acceptance.sh [--complete] [path/to/local/dify-repo]
#
# --complete additionally runs `make testacc-complete`, which executes the
# real examples/complete graph and requires OPENAI_API_KEY and TAVILY_API_KEY
# (optionally MCP_SERVER_URL) in the environment.
#
# With a local repo argument the pinned commit's docker/ tree is extracted via
# `git archive` (the caller's checkout is never modified). Without one the
# commit is fetched from https://github.com/langgenius/dify.git into a scratch
# clone first.
#
# Environment:
#   DIFY_ACCEPTANCE_NGINX_PORT  host port for nginx (default: first free port)
#   DIFY_ACCEPTANCE_KEEP=1      keep the stack running after the script exits
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PIN_FILE="$REPO_ROOT/scripts/dify-commit"
DIFY_COMMIT="$(tr -d '[:space:]' < "$PIN_FILE")"
DIFY_REPO_URL="https://github.com/langgenius/dify.git"

for cmd in docker git python3 curl; do
	command -v "$cmd" >/dev/null 2>&1 || { echo "error: required command not found: $cmd" >&2; exit 1; }
done
docker compose version >/dev/null 2>&1 || { echo "error: docker compose v2 is required" >&2; exit 1; }

complete=0
if [ "${1:-}" = "--complete" ]; then
	complete=1
	shift
fi
if [ "$complete" = "1" ]; then
	for v in OPENAI_API_KEY TAVILY_API_KEY; do
		if [ -z "${!v:-}" ]; then
			echo "error: --complete requires $v in the environment" >&2
			exit 1
		fi
	done
fi

scratch="$(mktemp -d)"
project="difyacc$(basename "$scratch" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9' | cut -c1-16)"
echo "==> scratch dir: $scratch"
echo "==> compose project: $project"
echo "==> pinned dify commit: $DIFY_COMMIT"

compose_started=0

cleanup() {
	local status="$1"
	if [ "${DIFY_ACCEPTANCE_KEEP:-0}" = "1" ]; then
		echo "==> DIFY_ACCEPTANCE_KEEP=1 set; leaving stack '$project' and $scratch in place"
		return "$status"
	fi
	if [ "$compose_started" = "1" ]; then
		echo "==> tearing down compose project $project"
		if ! (cd "$scratch/docker" && docker compose -p "$project" down -v --remove-orphans); then
			echo "error: failed to stop compose project $project; leaving $scratch in place" >&2
			return "$((status != 0 ? status : 1))"
		fi
	fi
	# Compose down removes named volumes, not bind-mounted data owned by container UIDs.
	if [ -d "$scratch/docker/volumes" ] && ! docker run --rm --network none --read-only --user 0:0 \
		--mount "type=bind,source=$scratch/docker/volumes,target=/cleanup" \
		busybox:latest sh -ec 'rm -rf /cleanup/.[!.]* /cleanup/..?* /cleanup/*'; then
		echo "error: could not remove bind-mounted data; leaving $scratch in place" >&2
		return "$((status != 0 ? status : 1))"
	fi
	if ! rm -rf "$scratch"; then
		return "$((status != 0 ? status : 1))"
	fi
	return "$status"
}
trap 'cleanup "$?" || exit "$?"' EXIT

diagnostics() {
	echo "==> compose status (project $project):" >&2
	(cd "$scratch/docker" && docker compose -p "$project" ps) >&2 || true
	for svc in api worker web nginx plugin_daemon db; do
		echo "==> last logs: $svc" >&2
		(cd "$scratch/docker" && docker compose -p "$project" logs --tail=40 "$svc") >&2 2>/dev/null || true
	done
}

if [ $# -ge 1 ]; then
	dify_repo="$1"
	echo "==> extracting docker/ at $DIFY_COMMIT from local repo $dify_repo"
	git -C "$dify_repo" archive "$DIFY_COMMIT" docker | tar -x -C "$scratch"
else
	echo "==> fetching $DIFY_COMMIT from $DIFY_REPO_URL"
	git -C "$scratch" init -q dify-src
	git -C "$scratch/dify-src" remote add origin "$DIFY_REPO_URL"
	git -C "$scratch/dify-src" fetch -q --depth 1 origin "$DIFY_COMMIT"
	git -C "$scratch/dify-src" archive "$DIFY_COMMIT" docker | tar -x -C "$scratch"
fi

# Retag the four first-party images to the pinned commit SHA. Third-party
# images and the separately versioned plugin daemon / sandbox are untouched.
python3 - "$scratch/docker/docker-compose.yaml" "$DIFY_COMMIT" <<'PY'
import re, sys
path, sha = sys.argv[1], sys.argv[2]
src = open(path).read()
pat = re.compile(r'(langgenius/(?:dify-api|dify-web|dify-agent-backend|dify-agent-local-sandbox):)[\w.-]+')
out, n = pat.subn(lambda m: m.group(1) + sha, src)
if n < 4:
    sys.exit(f"expected >=4 image retags, got {n}")
open(path, "w").write(out)
print(f"==> retagged {n} dify images to {sha}")
PY

# Runtime configuration only: copy the upstream example env, then set the
# sandbox auth token and the nginx host port. No topology changes.
cp "$scratch/docker/.env.example" "$scratch/docker/.env"
sandbox_token="$(python3 -c 'import secrets; print(secrets.token_hex(32))')"
free_port() {
	python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}
if [ -n "${DIFY_ACCEPTANCE_NGINX_PORT:-}" ]; then
	port="$DIFY_ACCEPTANCE_NGINX_PORT"
else
	port="$(free_port)"
fi
# Upstream publishes nginx 80/443 and the plugin debugging port 5003 by
# default; bind all of them to free ports so an existing local stack cannot
# collide with the scratch one.
{
	echo "DIFY_AGENT_LOCAL_SANDBOX_AUTH_TOKEN=$sandbox_token"
	echo "EXPOSE_NGINX_PORT=$port"
	echo "EXPOSE_NGINX_SSL_PORT=$(free_port)"
	echo "EXPOSE_PLUGIN_DEBUGGING_PORT=$(free_port)"
} >> "$scratch/docker/.env"
export DIFY_HOST="http://127.0.0.1:$port"
echo "==> nginx host port: $port"

echo "==> docker compose up -d"
compose_started=1
(cd "$scratch/docker" && docker compose -p "$project" up -d)

base="$DIFY_HOST"
deadline=$(( $(date +%s) + 900 ))
echo "==> waiting for /openapi/v1/_version (up to 15m)"
while :; do
	code="$(curl -s --max-time 10 -o /dev/null -w '%{http_code}' -D "$scratch/headers" "$base/openapi/v1/_version" || true)"
	if [ "$code" = "200" ] && grep -qi '^X-Dify-Catalog:' "$scratch/headers"; then
		break
	fi
	if [ "$(date +%s)" -ge "$deadline" ]; then
		echo "error: timed out waiting for the OpenAPI (last HTTP $code)" >&2
		diagnostics
		exit 1
	fi
	sleep 5
done
echo "==> OpenAPI is up (HTTP 200, catalog header present)"

deadline=$(( $(date +%s) + 300 ))
while :; do
	body="$(curl -s --max-time 10 "$base/console/api/setup" || true)"
	case "$body" in
		*'"step"'*) break ;;
	esac
	if [ "$(date +%s)" -ge "$deadline" ]; then
		echo "error: /console/api/setup did not become ready" >&2
		diagnostics
		exit 1
	fi
	sleep 3
done
echo "==> console setup endpoint is ready"

# Bootstrap the console and mint a PAT. The token secret is written to a file
# in the scratch dir and never echoed.
python3 - "$base" "$scratch/pat" <<'PY'
import base64, http.cookiejar, json, secrets, sys, urllib.request

base, out_path = sys.argv[1], sys.argv[2]
jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

def post(path, payload, headers=None):
    req = urllib.request.Request(
        base + path,
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json", **(headers or {})},
        method="POST",
    )
    with opener.open(req, timeout=30) as resp:
        return json.loads(resp.read() or b"{}")

email = "terraform-acceptance@example.com"
password = "Acc3ptance-" + secrets.token_hex(24)

with urllib.request.urlopen(base + "/console/api/setup", timeout=30) as resp:
    status = json.loads(resp.read())
if status.get("step") == "not_started":
    post("/console/api/setup", {"email": email, "name": "tf-acceptance", "password": password})

post("/console/api/login", {
    "email": email,
    "password": base64.b64encode(password.encode()).decode(),
    "remember_me": False,
})

csrf = next((c.value for c in jar if c.name == "csrf_token"), None)
if not csrf:
    sys.exit("csrf_token cookie missing after login")

created = post("/console/api/account/personal-access-tokens",
               {"name": "terraform-acceptance", "expiration": "30_days"},
               {"X-CSRF-Token": csrf})
secret = created.get("secret")
if not secret:
    sys.exit("PAT creation response missing secret")
with open(out_path, "w") as f:
    f.write(secret)
PY

export DIFY_TOKEN="$(cat "$scratch/pat")"
if [ -n "${GITHUB_ACTIONS:-}" ]; then
	echo "::add-mask::$DIFY_TOKEN"
fi
rm -f "$scratch/pat"
echo "==> PAT issued; running acceptance tests (workspace resolves via /openapi/v1/account)"

env -u DIFY_WORKSPACE_ID make -C "$REPO_ROOT" testacc

if [ "$complete" = "1" ]; then
	echo "==> running full-parity acceptance (examples/complete)"
	env -u DIFY_WORKSPACE_ID make -C "$REPO_ROOT" testacc-complete
fi
