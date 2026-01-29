# Common failures

## Missing token
Symptom: `missing token (run "breyta auth login")`
Fix: Run `breyta auth login`, then `breyta auth whoami`

## Wrong workspace
Symptom: `Access denied: not a workspace member` or empty lists
Fix: Run `breyta workspaces list`, then pass `--workspace <id>` or set it in config
Note: `breyta workspaces use` is not implemented

## API unreachable
Symptom: `dial tcp` or `no such host`
Fix: Check the API URL and workspace. For local dev, enable dev mode and set `BREYTA_API_URL`

## Local vs prod mismatch
Symptom: Commands hit mock data or a local server after using the TUI
Fix: Switch to prod in the TUI or set the API URL to `https://flows.breyta.ai`
