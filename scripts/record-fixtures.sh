#!/usr/bin/env bash
# Record read-only Mantis REST responses into internal/mantis/testdata/,
# scrubbing personal data (emails, names, issue text, project names).
#
#   MANTIS_URL=https://mantis.williamhleucka.com MANTIS_TOKEN="$MANTIS_WH" \
#     scripts/record-fixtures.sh [issue-id]
#
# Only GET requests are made. The token is never written to disk.
set -euo pipefail

: "${MANTIS_URL:?set MANTIS_URL (without /api/rest)}"
: "${MANTIS_TOKEN:?set MANTIS_TOKEN}"
ISSUE_ID="${1:-33}"
OUT="$(cd "$(dirname "$0")/.." && pwd)/internal/mantis/testdata"
API="${MANTIS_URL%/}/api/rest"

get() { curl -sf --max-redirs 0 -H "Authorization: $MANTIS_TOKEN" "$API/$1"; }

# Scrub: users → userN, projects → project-N, free text → placeholder,
# string history values → placeholder.
SCRUB='
def user: if type == "object" and has("id") then
    (if has("name") then .name = "user\(.id)" else . end)
  | (if has("real_name") then .real_name = "User \(.id)" else . end)
  | (if has("email") then .email = "user\(.id)@example.test" else . end)
  else . end;
def text($k): if type == "string" then "Sample \($k)" else . end;
walk(
  if type == "object" then
      (if has("real_name") or has("email") then user else . end)
    | (if has("reporter") then .reporter |= user else . end)
    | (if has("handler") then .handler |= user else . end)
    | (if has("user") then .user |= user else . end)
    | (if has("project") and (.project|type) == "object" and (.project.id // 0) > 0 then .project.name = "project-\(.project.id)" else . end)
    | (if has("summary") then .summary |= text("summary") else . end)
    | (if has("description") then .description |= text("description") else . end)
    | (if has("steps_to_reproduce") then .steps_to_reproduce |= text("steps") else . end)
    | (if has("additional_information") then .additional_information |= text("additional info") else . end)
    | (if has("text") then .text |= text("note text") else . end)
    | (if has("change") and (.change|type) == "string" and .change != "" then .change = "old value => new value" else . end)
    | (if has("message") and (.message|type) == "string" and (.message|startswith("File ")) then .message |= sub(": .*"; ": file.txt") else . end)
    | (if has("old_value") and (.old_value|type) == "string" then .old_value = "old value" else . end)
    | (if has("new_value") and (.new_value|type) == "string" then .new_value = "new value" else . end)
    | (if has("filename") then .filename = "file-\(.id).txt" else . end)
    | (if has("timezone") then .timezone = "UTC" else . end)
  else . end
)
| if has("projects") then .projects |= map(.name = "project-\(.id)") else . end
'

record() { # name endpoint
  get "$2" | jq "$SCRUB" > "$OUT/$1.json"
  echo "recorded $1.json"
}

mkdir -p "$OUT"
record me                "users/me"
record projects          "projects"
record issues_list       "issues?page_size=3&page=1"
record issue             "issues/$ISSUE_ID"
record config_enums      "config?option[]=status_enum_string&option[]=priority_enum_string&option[]=severity_enum_string&option[]=reproducibility_enum_string&option[]=resolution_enum_string&option[]=status_colors"
PROJECT_ID="$(jq '.issues[0].project.id' "$OUT/issue.json")"
record project           "projects/$PROJECT_ID"
record project_users     "projects/$PROJECT_ID/users"
