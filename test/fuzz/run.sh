#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────
# run.sh — Fuzz-test kindling generate against a list of real repos
#
# For each repo:
#   1. Shallow-clone
#   2. Run kindling generate --dry-run
#   3. Validate the generated YAML
#   4. Parse services, ports, health checks, env vars
#   5. Cross-validate networking (port mismatches, dangling refs)
#   6. Docker build each Dockerfile
#   7. Docker run + health check each container
#   8. Write structured results to results.jsonl
#
# Usage:
#   ./run.sh <repos.txt> <output-dir> [kindling-binary]
# ─────────────────────────────────────────────────────────────────
set -euo pipefail

REPOS_FILE="${1:?Usage: run.sh <repos.txt> <output-dir> [kindling-binary]}"
OUTPUT_DIR="${2:?Usage: run.sh <repos.txt> <output-dir> [kindling-binary]}"
KINDLING="${3:-kindling}"
TIMEOUT_BUILD=300   # 5 min per docker build
TIMEOUT_RUN=30      # 30s for container startup + health check

mkdir -p "$OUTPUT_DIR"
RESULTS="$OUTPUT_DIR/results.jsonl"
: > "$RESULTS"

# Counters
TOTAL=0; GENERATE_OK=0; YAML_OK=0; BUILD_OK=0; HEALTH_OK=0; NETWORK_OK=0

# ── Helpers ──────────────────────────────────────────────────────

log()  { echo "[$1] $2" >&2; }
now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

# Write a JSON result line
emit() {
  local repo="$1" stage="$2" status="$3" detail="$4" duration_ms="$5"
  local services_count="${6:-0}" issues="${7:-[]}"
  printf '{"repo":"%s","stage":"%s","status":"%s","detail":"%s","duration_ms":%s,"services_count":%s,"issues":%s}\n' \
    "$repo" "$stage" "$status" "$detail" "$duration_ms" "$services_count" "$issues" \
    >> "$RESULTS"
}

# Extract EXPOSE ports from a Dockerfile
dockerfile_ports() {
  local dockerfile="$1"
  grep -i '^EXPOSE' "$dockerfile" 2>/dev/null \
    | sed 's/EXPOSE//i' | tr -s ' /' '\n' | grep -E '^[0-9]+' | sort -u || true
}

# ── Per-repo test ────────────────────────────────────────────────

test_repo() {
  local repo_url="$1"
  local repo_name
  repo_name=$(echo "$repo_url" | sed 's|.*/||; s|\.git$||')
  local clone_dir="$OUTPUT_DIR/repos/$repo_name"
  local workflow_file="$OUTPUT_DIR/workflows/${repo_name}.yml"
  local issues="[]"

  TOTAL=$((TOTAL + 1))
  log "REPO" "[$TOTAL] $repo_url"

  # ── 1. Clone ─────────────────────────────────────────────────
  rm -rf "$clone_dir"
  mkdir -p "$clone_dir"
  local t0; t0=$(now_ms)

  if ! git clone --depth=1 --single-branch -q "$repo_url" "$clone_dir" 2>/dev/null; then
    local dur=$(( $(now_ms) - t0 ))
    emit "$repo_url" "clone" "fail" "git clone failed" "$dur"
    log "FAIL" "clone failed — skipping"
    return
  fi

  # ── 2. Generate workflow ─────────────────────────────────────
  mkdir -p "$OUTPUT_DIR/workflows"
  t0=$(now_ms)
  local gen_stderr="$OUTPUT_DIR/workflows/${repo_name}.stderr"

  if "$KINDLING" generate \
      --repo "$clone_dir" \
      --dry-run \
      --provider "${FUZZ_PROVIDER:-openai}" \
      --api-key "${FUZZ_API_KEY:-$OPENAI_API_KEY}" \
      ${FUZZ_MODEL:+--model "$FUZZ_MODEL"} \
      > "$workflow_file" 2>"$gen_stderr"; then
    local dur=$(( $(now_ms) - t0 ))
    GENERATE_OK=$((GENERATE_OK + 1))
    emit "$repo_url" "generate" "pass" "" "$dur"
    log "PASS" "generate (${dur}ms)"
  else
    local dur=$(( $(now_ms) - t0 ))
    local err
    err=$(head -5 "$gen_stderr" | tr '\n' ' ' | cut -c1-200)
    emit "$repo_url" "generate" "fail" "$err" "$dur"
    log "FAIL" "generate — $err"
    return
  fi

  # ── 3. Validate YAML ────────────────────────────────────────
  t0=$(now_ms)
  if ! python3 -c "
import yaml, sys
with open('$workflow_file') as f:
    data = yaml.safe_load(f)
if not isinstance(data, dict):
    sys.exit(1)
if 'jobs' not in data:
    sys.exit(1)
" 2>/dev/null; then
    local dur=$(( $(now_ms) - t0 ))
    emit "$repo_url" "yaml_validate" "fail" "invalid YAML or missing jobs key" "$dur"
    log "FAIL" "invalid YAML"
    return
  fi
  local dur=$(( $(now_ms) - t0 ))
  YAML_OK=$((YAML_OK + 1))
  emit "$repo_url" "yaml_validate" "pass" "" "$dur"

  # ── 4. Parse services and cross-validate networking ──────────
  t0=$(now_ms)
  local analysis
  analysis=$(python3 "$SCRIPT_DIR/analyze.py" "$workflow_file" "$clone_dir" 2>/dev/null) || true

  if [ -n "$analysis" ]; then
    local svc_count net_issues
    svc_count=$(echo "$analysis" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('services_count',0))" 2>/dev/null || echo 0)
    net_issues=$(echo "$analysis" | python3 -c "import sys,json; d=json.load(sys.stdin); print(json.dumps(d.get('issues',[])))" 2>/dev/null || echo "[]")
    local issue_count
    issue_count=$(echo "$net_issues" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)

    dur=$(( $(now_ms) - t0 ))
    if [ "$issue_count" -eq 0 ]; then
      NETWORK_OK=$((NETWORK_OK + 1))
      emit "$repo_url" "network_validate" "pass" "${svc_count} services, 0 issues" "$dur" "$svc_count" "$net_issues"
      log "PASS" "networking — ${svc_count} services, 0 issues"
    else
      emit "$repo_url" "network_validate" "warn" "${svc_count} services, ${issue_count} issues" "$dur" "$svc_count" "$net_issues"
      log "WARN" "networking — ${svc_count} services, ${issue_count} issues"
    fi
  else
    dur=$(( $(now_ms) - t0 ))
    emit "$repo_url" "network_validate" "skip" "analysis script failed" "$dur"
    log "SKIP" "networking analysis failed"
  fi

  # ── 5. Docker build each Dockerfile ──────────────────────────
  local dockerfiles
  dockerfiles=$(find "$clone_dir" -name Dockerfile -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | head -10)

  if [ -z "$dockerfiles" ]; then
    emit "$repo_url" "docker_build" "skip" "no Dockerfiles found" "0"
    log "SKIP" "no Dockerfiles"
    return
  fi

  local all_builds_ok=true
  while IFS= read -r df; do
    local df_rel="${df#$clone_dir/}"
    local ctx_dir
    ctx_dir=$(dirname "$df")
    local img_tag="fuzz-${repo_name}-$(echo "$df_rel" | tr '/' '-' | tr '[:upper:]' '[:lower:]'):test"

    t0=$(now_ms)
    if timeout "$TIMEOUT_BUILD" docker build -t "$img_tag" -f "$df" "$ctx_dir" \
        >/dev/null 2>"$OUTPUT_DIR/workflows/${repo_name}.build.log"; then
      dur=$(( $(now_ms) - t0 ))
      BUILD_OK=$((BUILD_OK + 1))
      emit "$repo_url" "docker_build" "pass" "$df_rel (${dur}ms)" "$dur"
      log "PASS" "build $df_rel (${dur}ms)"

      # ── 6. Docker run + health check ──────────────────────
      # Find the port: check workflow first, fall back to EXPOSE
      local port
      port=$(dockerfile_ports "$df" | head -1)
      if [ -z "$port" ]; then
        port="8080"  # common default
      fi

      # Find health check path from the generated workflow
      local health_path="/"
      if [ -n "$analysis" ]; then
        local hp
        hp=$(echo "$analysis" | python3 -c "
import sys, json
d = json.load(sys.stdin)
# find a service whose context matches this Dockerfile's directory
df_rel = '$df_rel'
for svc in d.get('services', []):
    ctx = svc.get('context', '')
    if df_rel.startswith(ctx) or ctx.endswith(df_rel.replace('/Dockerfile','')):
        print(svc.get('health_check_path', '/'))
        sys.exit(0)
print('/')
" 2>/dev/null || echo "/")
        health_path="$hp"
      fi

      t0=$(now_ms)
      local container_id
      container_id=$(docker run -d --rm -p "0:$port" "$img_tag" 2>/dev/null) || true

      if [ -n "$container_id" ]; then
        # Find the mapped host port
        local host_port
        host_port=$(docker port "$container_id" "$port" 2>/dev/null | head -1 | sed 's/.*://') || true

        if [ -n "$host_port" ]; then
          # Wait for health check
          local healthy=false
          for i in $(seq 1 "$TIMEOUT_RUN"); do
            if curl -sf -o /dev/null --max-time 2 "http://127.0.0.1:${host_port}${health_path}" 2>/dev/null; then
              healthy=true
              break
            fi
            sleep 1
          done

          dur=$(( $(now_ms) - t0 ))
          if $healthy; then
            HEALTH_OK=$((HEALTH_OK + 1))
            emit "$repo_url" "health_check" "pass" "$df_rel → :${port}${health_path}" "$dur"
            log "PASS" "health $df_rel → :${port}${health_path} (${dur}ms)"
          else
            emit "$repo_url" "health_check" "fail" "$df_rel → :${port}${health_path} (timeout ${TIMEOUT_RUN}s)" "$dur"
            log "FAIL" "health $df_rel → :${port}${health_path} (timeout)"
          fi
        else
          dur=$(( $(now_ms) - t0 ))
          emit "$repo_url" "health_check" "fail" "$df_rel — could not map port $port" "$dur"
          log "FAIL" "health — port map failed"
        fi

        docker stop "$container_id" >/dev/null 2>&1 || true
      else
        dur=$(( $(now_ms) - t0 ))
        emit "$repo_url" "health_check" "fail" "$df_rel — container failed to start" "$dur"
        log "FAIL" "health — container failed to start"
      fi

      # Cleanup image
      docker rmi -f "$img_tag" >/dev/null 2>&1 || true
    else
      dur=$(( $(now_ms) - t0 ))
      all_builds_ok=false
      local build_err
      build_err=$(tail -5 "$OUTPUT_DIR/workflows/${repo_name}.build.log" 2>/dev/null | tr '\n' ' ' | cut -c1-200)
      emit "$repo_url" "docker_build" "fail" "$df_rel — $build_err" "$dur"
      log "FAIL" "build $df_rel"
    fi
  done <<< "$dockerfiles"

  # Cleanup clone
  rm -rf "$clone_dir"
}

# ── Main ─────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

log "START" "Fuzz testing kindling generate"
log "INFO" "Repos: $REPOS_FILE"
log "INFO" "Output: $OUTPUT_DIR"
log "INFO" "Kindling: $($KINDLING version 2>/dev/null || echo "$KINDLING")"

while IFS= read -r line; do
  # Skip comments and blank lines
  line=$(echo "$line" | sed 's/#.*//' | xargs)
  [ -z "$line" ] && continue
  test_repo "$line"
done < "$REPOS_FILE"

# ── Summary ──────────────────────────────────────────────────────

log "DONE" "════════════════════════════════════════"
log "DONE" "Total repos:       $TOTAL"
log "DONE" "Generate OK:       $GENERATE_OK / $TOTAL"
log "DONE" "Valid YAML:        $YAML_OK / $GENERATE_OK"
log "DONE" "Networking clean:  $NETWORK_OK / $YAML_OK"
log "DONE" "Docker build OK:   $BUILD_OK"
log "DONE" "Health check OK:   $HEALTH_OK"
log "DONE" "════════════════════════════════════════"

# Write summary JSON
cat > "$OUTPUT_DIR/summary.json" <<EOF
{
  "total": $TOTAL,
  "generate_ok": $GENERATE_OK,
  "yaml_ok": $YAML_OK,
  "network_ok": $NETWORK_OK,
  "build_ok": $BUILD_OK,
  "health_ok": $HEALTH_OK,
  "generate_rate": "$(echo "scale=1; $GENERATE_OK * 100 / $TOTAL" | bc 2>/dev/null || echo "?")%",
  "network_rate": "$([ "$YAML_OK" -gt 0 ] && echo "scale=1; $NETWORK_OK * 100 / $YAML_OK" | bc 2>/dev/null || echo "?")%"
}
EOF

log "DONE" "Results: $RESULTS"
log "DONE" "Summary: $OUTPUT_DIR/summary.json"
