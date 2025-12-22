## Local flows authoring (flows-api)

### Start flows-api (mock auth, emulator)

From `breyta/`:

```bash
./scripts/start-flows-api.sh --emulator --auth-mock
```

Notes:
- Default URL: `http://localhost:8090`
- Ctrl+C stops the server and frees the port.

### Configure breyta CLI (API mode)

In the shell/environment the agent runs in:

```bash
export BREYTA_API_URL="http://localhost:8090"
export BREYTA_WORKSPACE="ws-acme"
export BREYTA_TOKEN="dev-user-123"
```

### Workspace bootstrap (fix “not a workspace member”)

If you see `403 Access denied: not a workspace member` (often after restarting dev servers), bootstrap the workspace and membership via the dev debug endpoint:

```bash
breyta workspaces bootstrap ws-acme
```

### Activation (bind credentials for `:requires` slots)

If your flow uses `:requires` slots (including `:type :llm-provider` for LLM keys), users must activate the flow once to create a profile and bind credentials/connections.

- Sign in: `http://localhost:8090/login` → “Sign in with Google” → “Dev User”
- Activation URL: `http://localhost:8090/<workspace>/flows/<slug>/activate`
- Or print it: `breyta flows activate-url <slug>`

### Draft preview bindings

Draft runs use user-scoped draft bindings:

- Draft bindings URL: `http://localhost:8090/<workspace>/flows/<slug>/draft-bindings`
- Or print it: `breyta flows draft-bindings-url <slug>`
- Run draft: `breyta runs start --flow <slug> --source draft`

### Flow body constraints (SCI / orchestration DSL)

Flow definitions run in a constrained runtime intended for **orchestration**, not transformation:
- Many functional ops are denied in the flow body (e.g. `mapv`, `filterv`, `reduce`, etc.)
- Keep orchestration in the flow body (sequence of `step` calls)
- Do data transformation in `:code` steps (or other explicit steps)

### Input keys from `--input` (string vs keyword keys)

The CLI sends `--input` as JSON, so keys arrive as strings.

The runtime normalizes input so both string keys and keyword keys work (safe keyword aliases are added), but author flows as if you will read keyword keys (e.g. `(get input :n)`).

### Flow edit loop (pull → edit → push draft → deploy)

```bash
breyta flows list
breyta flows pull simple-http --out ./tmp/flows/simple-http.clj
# edit ./tmp/flows/simple-http.clj
breyta flows push --file ./tmp/flows/simple-http.clj
breyta flows deploy simple-http
breyta flows show simple-http
```

### Runnable smoke test (verify final output)

Create a tiny code-only flow file and run it:

```bash
mkdir -p ./tmp/flows
cat > ./tmp/flows/run-hello.clj <<'EOF'
{:slug :run-hello
 :name "Run Hello"
 :description "Simple runnable flow (returns deterministic result)"
 :tags ["draft"]
 :concurrency-config {:concurrency :singleton
                      :on-new-version :supersede}
 :requires nil
 :templates nil
 :triggers nil
 :definition
 (defflow [input]
   (let [out (step :code :make-output
                   {:type :code
                    :title "Make output"
                    :code (quote (fn [{:keys [n]}]
                                   {:ok true
                                    :message "hello"
                                    :n (or n 0)
                                    :nPlusOne (inc (or n 0))}))
                    :input input})]
     out))}
EOF

breyta flows push --file ./tmp/flows/run-hello.clj
breyta flows deploy run-hello

# Start a run and wait. Output is in:
#   data.run.resultPreview.data.result
breyta --dev runs start --flow run-hello --input '{"n":41}' --wait --timeout 30s
```
