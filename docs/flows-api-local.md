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

### Flow edit loop (pull → edit → push draft → deploy)

```bash
breyta flows list --pretty
breyta flows pull simple-http --out ./tmp/flows/simple-http.clj --pretty
# edit ./tmp/flows/simple-http.clj
breyta flows push --file ./tmp/flows/simple-http.clj --pretty
breyta flows deploy simple-http --pretty
breyta flows show simple-http --pretty
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

breyta flows push --file ./tmp/flows/run-hello.clj --pretty
breyta flows deploy run-hello --pretty

# Start a run and wait. Output is in:
#   data.run.resultPreview.data.result
breyta --dev runs start --flow run-hello --input '{"n":41}' --wait --timeout 30s --pretty
```
