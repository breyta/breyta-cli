# Google Drive folder sync (service account)

This repo ships a production flow that continuously syncs files from a Google Drive folder into Breyta blob storage, and writes per-file metadata docs (KV-backed-by-Firestore) for querying.

## Why service account auth (not user OAuth)
Google OAuth user auth is not suitable for **continuous**, unattended syncing (e.g. every 2 hours), because it requires the user to re-authenticate periodically. For this integration we use a **Google service account** (SA) so the flow can mint OAuth access tokens on a schedule without user interaction.

## Flow + schedule
- Flow slug: `google-drive-folder-sync`
- Trigger: schedule, every 2 hours (`cron: 0 */2 * * *`, `timezone: UTC`)
- Folder input: a Drive folder URL or raw folder ID

## Required secret
The flow uses `:auth {:type :google-service-account ...}` on `:http` steps.

- Secret slot / ref: `google-drive-service-account`
- Secret value: a JSON service account key (the full JSON payload)

Bind it via profiles (prod):
1) `breyta flows bindings template google-drive-folder-sync --out profile.edn`
2) Edit `profile.edn` and set the secret slot:
   - `{:bindings {:google-drive-service-account {:secret "<SERVICE_ACCOUNT_JSON>"}}}`
3) `breyta flows bindings apply google-drive-folder-sync @profile.edn`
4) `breyta flows activate google-drive-folder-sync --version latest`

## Run now (manual)
### Incremental run (uses the shared cursor)
This is the safest way to “run now”, because it uses the same cursor the schedule uses.

```bash
breyta runs start \
  --flow google-drive-folder-sync \
  --input '{"folder":"https://drive.google.com/drive/folders/<id>?usp=sharing","cursor_key":"google-drive-sync:cursor"}' \
  --wait --pretty
```

### Isolated/forced scan (separate cursor)
Use a unique `cursor_key` to force a “fresh” scan without disturbing the schedule cursor.

```bash
breyta runs start \
  --flow google-drive-folder-sync \
  --input '{"folder":"https://drive.google.com/drive/folders/<id>?usp=sharing","cursor_key":"google-drive-sync:cursor:manual-check:20260101T000000Z"}' \
  --wait --pretty
```

Notes:
- For singleton flows, repeated manual starts may reuse a stable workflow id; prefer reading results by `runs list` / `runs show` timestamps rather than assuming unique workflow ids.
- Avoid passing `--profile-id` unless you explicitly want that profile’s KV namespace; scheduled runs use the flow’s active prod profile.

## What gets written
### Blobs
Downloads are persisted as blobs, with filenames like:
- `gdrive/<folder-id>/<name>.<ext>`

Google Docs/Sheets are exported to:
- Docs → `text/plain` (`.txt`)
- Sheets → `xlsx`
- Other google-apps types → `pdf`

### Per-file metadata docs (KV-backed-by-Firestore)
For each downloaded file the flow writes a KV doc:
- Key format: `google-drive-sync:meta:<folder-id>:<file-id>`
- Value includes:
  - `:folder-id`, `:file-id`, `:blob-filename`
  - Drive list fields like `:name`, `:mimeType`, `:modifiedTime`, `:size`, `:driveId`

## How to verify a run
After a run completes:
- Check `download-files` step has `successes` and `failures` as expected.
- Check there are `meta-<file-id>` KV steps with `{:success true, :key "...", :created? ...}`.
- Check the cursor KV write `set-cursor` updates `google-drive-sync:cursor` (or your custom cursor key).

