#!/usr/bin/env bash
set -euo pipefail

job_dir="${BREYTA_JOB_DIR:?missing BREYTA_JOB_DIR}"
payload_file="${BREYTA_JOB_PAYLOAD_FILE:?missing BREYTA_JOB_PAYLOAD_FILE}"
result_file="${BREYTA_JOB_RESULT_FILE:?missing BREYTA_JOB_RESULT_FILE}"
job_id="${BREYTA_JOB_ID:?missing BREYTA_JOB_ID}"
lease_token="${BREYTA_JOB_LEASE_TOKEN:?missing BREYTA_JOB_LEASE_TOKEN}"

read_payload_field() {
  local field="$1"
  python3 - "$payload_file" "$field" <<'PY'
import json
import sys

payload_path, field_name = sys.argv[1:]
with open(payload_path, "r", encoding="utf-8") as handle:
    payload = json.load(handle)

value = payload.get(field_name, "")
if value is None:
    value = ""
if not isinstance(value, str):
    value = str(value)
sys.stdout.write(value)
PY
}

write_json_file() {
  local output_path="$1"
  local json_payload="$2"
  python3 - "$output_path" "$json_payload" <<'PY'
import json
import sys

output_path, raw_payload = sys.argv[1:]
with open(output_path, "w", encoding="utf-8") as handle:
    json.dump(json.loads(raw_payload), handle)
    handle.write("\n")
PY
}

emit_progress() {
  local status="$1"
  local message="$2"
  local phase="$3"
  local findings="$4"
  local details_file="$job_dir/progress-details-${phase}.json"
  local metrics_file="$job_dir/progress-metrics-${phase}.json"
  local artifacts_file="$job_dir/progress-artifacts-${phase}.json"

  write_json_file "$details_file" "$(printf '{"surface":%s,"mode":%s,"phase":%s}' \
    "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$surface")" \
    "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$mode")" \
    "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$phase")")"
  write_json_file "$metrics_file" "$(printf '{"filesScanned":4,"findings":%s}' "$findings")"
  write_json_file "$artifacts_file" "$(printf '[{"kind":"report","label":"review-%s","contentType":"text/markdown","path":%s}]' \
    "$phase" \
    "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$report_path")")"

  if command -v breyta >/dev/null 2>&1 && [[ -n "${BREYTA_API_URL:-}" ]] && [[ -n "${BREYTA_WORKSPACE:-}" ]] && [[ -n "${BREYTA_TOKEN:-}" ]]; then
    breyta --api "$BREYTA_API_URL" \
      --workspace "$BREYTA_WORKSPACE" \
      --token "$BREYTA_TOKEN" \
      jobs progress "$job_id" \
      --lease-token "$lease_token" \
      --status "$status" \
      --message "$message" \
      --details-file "$details_file" \
      --metrics-file "$metrics_file" \
      --artifacts-file "$artifacts_file" \
      >/dev/null
  fi
}

surface="$(read_payload_field "surface")"
mode="$(read_payload_field "mode")"
surface="${surface:-flows-api}"
mode="${mode:-succeeded}"

mkdir -p "$job_dir/artifacts"
report_path="$job_dir/artifacts/review-report.md"

python3 - "$report_path" "$surface" "$mode" <<'PY'
import sys

report_path, surface, mode = sys.argv[1:]
with open(report_path, "w", encoding="utf-8") as handle:
    handle.write("# Demo agent review\n\n")
    handle.write(f"- Surface: {surface}\n")
    handle.write(f"- Mode: {mode}\n")
    handle.write("- Transport: Breyta jobs worker + CLI progress updates\n")
PY

emit_progress "started" "Inspecting exposed surface" "inspect" 0
sleep 1
emit_progress "running" "Writing review report" "report" "$( [[ "$mode" == "succeeded" ]] && printf '1' || printf '0' )"

case "$mode" in
  failed)
    write_json_file "$result_file" "$(printf '{"message":"demo review failed while parsing inputs","code":"demo_review_failed","details":{"surface":%s,"mode":%s,"phase":"report"},"artifacts":[{"kind":"report","label":"review-report","contentType":"text/markdown","path":%s}]}' \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$surface")" \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$mode")" \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$report_path")")"
    exit 7
    ;;
  no_changes)
    write_json_file "$result_file" "$(printf '{"status":"no_changes","summary":"No actionable issues found","outputs":{"surface":%s,"findingCount":0},"metrics":{"filesScanned":4,"findings":0},"artifacts":[{"kind":"report","label":"review-report","contentType":"text/markdown","path":%s}],"workerInfo":{"runner":"demo-agent-review-script","mode":%s}}' \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$surface")" \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$report_path")" \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$mode")")"
    ;;
  *)
    write_json_file "$result_file" "$(printf '{"status":"succeeded","summary":"Found one actionable issue","outputs":{"surface":%s,"findingCount":1,"recommendation":"add auth guard"},"metrics":{"filesScanned":4,"findings":1},"artifacts":[{"kind":"report","label":"review-report","contentType":"text/markdown","path":%s}],"workerInfo":{"runner":"demo-agent-review-script","mode":%s}}' \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$surface")" \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$report_path")" \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$mode")")"
    ;;
esac
