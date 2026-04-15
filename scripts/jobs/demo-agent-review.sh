#!/usr/bin/env bash
set -euo pipefail

job_dir="${BREYTA_JOB_DIR:?missing BREYTA_JOB_DIR}"
payload_file="${BREYTA_JOB_PAYLOAD_FILE:?missing BREYTA_JOB_PAYLOAD_FILE}"

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

emit_progress() {
  local status="$1"
  local message="$2"
  local phase="$3"
  local findings="$4"

  breyta jobs worker progress \
    --status "$status" \
    --message "$message" \
    --detail "surface=$surface" \
    --detail "mode=$mode" \
    --detail "phase=$phase" \
    --metric filesScanned=4 \
    --metric "findings=$findings" \
    >/dev/null
}

surface="$(read_payload_field "surface")"
mode="$(read_payload_field "mode")"
surface="${surface:-flows-api}"
mode="${mode:-succeeded}"

mkdir -p "$job_dir/artifacts"
report_path="$job_dir/artifacts/review-report.md"
findings_path="$job_dir/artifacts/findings.json"

cat >"$report_path" <<EOF
# Demo agent review

- Surface: $surface
- Mode: $mode
- Contract: breyta jobs worker helper commands
EOF

emit_progress "started" "Inspecting exposed surface" "inspect" 0
sleep 1
emit_progress "running" "Writing review report" "report" "$( [[ "$mode" == "succeeded" ]] && printf '1' || printf '0' )"

report_uri="$(breyta jobs worker attach-file \
  --file "$report_path" \
  --label review-report \
  --kind report \
  --print-uri)"

finding_count=1
severity=high
recommendation="add auth guard"
finding_id="auth-guard"
review_status="open"

case "$mode" in
  failed)
    finding_count=0
    severity=error
    recommendation="review failed before producing actionable findings"
    finding_id="review-error"
    review_status="failed"
    ;;
  no_changes)
    finding_count=0
    severity=info
    recommendation="no actionable issues found"
    finding_id="review-summary"
    review_status="none"
    ;;
esac

cat >"$findings_path" <<EOF
[
  {
    "finding_id": "$finding_id",
    "surface": "$surface",
    "mode": "$mode",
    "severity": "$severity",
    "status": "$review_status",
    "recommendation": "$recommendation",
    "finding_count": $finding_count
  }
]
EOF

summary_uri="$(breyta jobs worker attach-kv \
  --label review-summary \
  --field "surface=$surface" \
  --field "mode=$mode" \
  --field "findingCount=$finding_count" \
  --field "severity=$severity" \
  --field "recommendation=$recommendation" \
  --field "reportResourceUri=$report_uri" \
  --print-uri)"

findings_uri="$(breyta jobs worker attach-table \
  --label findings \
  --table security-findings \
  --rows-file "$findings_path" \
  --write-mode upsert \
  --key-field finding_id \
  --index-field severity \
  --print-uri)"

case "$mode" in
  failed)
    breyta jobs worker fail \
      --message "demo review failed while parsing inputs" \
      --code demo_review_failed \
      --detail "surface=$surface" \
      --detail "mode=$mode" \
      --detail phase=report \
      >/dev/null
    ;;
  no_changes)
    breyta jobs worker finish \
      --status no_changes \
      --summary "No actionable issues found" \
      --output "surface=$surface" \
      --output "findingCount=$finding_count" \
      --output "reportResourceUri=$report_uri" \
      --output "summaryResourceUri=$summary_uri" \
      --output "findingsTableUri=$findings_uri" \
      --metric filesScanned=4 \
      --metric findings=0 \
      >/dev/null
    ;;
  *)
    breyta jobs worker finish \
      --summary "Found one actionable issue" \
      --output "surface=$surface" \
      --output "findingCount=$finding_count" \
      --output "recommendation=add auth guard" \
      --output "reportResourceUri=$report_uri" \
      --output "summaryResourceUri=$summary_uri" \
      --output "findingsTableUri=$findings_uri" \
      --metric filesScanned=4 \
      --metric findings=1 \
      >/dev/null
    ;;
esac
