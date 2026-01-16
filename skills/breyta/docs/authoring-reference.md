# Authoring reference
## Flow file format
- A flow file is a single EDN map literal (Clojure data), not JSON.
- The server reads it with `*read-eval*` disabled.
- Slug format: `^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`.

Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:slug` | keyword | Yes | Non-namespaced keyword (URL-safe) |
| `:name` | string | Yes | Display name |
| `:description` | string | No | Help text |
| `:concurrency` | map | Yes | See below |
| `:requires` | vector | No | Connection slots and activation inputs |
| `:templates` | vector | No | Template payloads (see `./templates.md`) |
| `:functions` | vector | No | Function templates for `:function` steps |
| `:triggers` | vector | Yes | Include a `:manual` trigger for discoverability |
| `:flow` | form | Yes | The orchestration DSL |

## `:requires`
Use `:requires` to declare connection slots and activation form inputs. Flows with `:requires` must have bindings applied and be activated.

### `:kind :connection`
Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:slot` | keyword | Yes | Used as `:connection` in steps |
| `:type` | keyword | Yes | `:http-api`, `:llm-provider`, `:database`, `:blob-storage`, `:kv-store`, `:secret` |
| `:label` | string | Yes | UI label |
| `:optional` | boolean | No | Use `flow/slot-bound?` |
| `:base-url` | string | If `:http-api` | Base URL |
| `:auth` | map | If not `:secret` | `{:type :none|:api-key|:bearer|:basic}` |
| `:oauth` | map | Optional | OAuth config |

Example:

```clojure
:requires [{:slot :crm
            :type :http-api
            :label "CRM API"
            :base-url "https://api.example.com"
            :auth {:type :bearer}}
           {:slot :ai
            :type :llm-provider
            :label "AI Provider"
            :auth {:type :api-key}
            :optional true}]
```

### `:kind :form`
Activation-only inputs (no connection created):

```clojure
{:kind :form
 :label "Activation inputs"
 :fields [{:key :region :label "Region" :field-type :select :options ["EU" "US"]}
          {:key :batch-size :label "Batch size" :field-type :number :default 500}]}
```

Form fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:key` | keyword | Yes | Input key under `:activation` |
| `:label` | string | Yes | UI label |
| `:field-type` | keyword | Yes | `:string`, `:text`, `:number`, `:boolean`, `:select`, `:date`, `:email`, `:textarea`, `:password`, `:secret` |
| `:required` | boolean | No | Default false |
| `:options` | vector | If `:select` | Options |

### `:type :secret`
Use `:type :secret` for single-value secrets (webhook signing, tokens used in custom logic). Avoid for HTTP APIs.

## `:concurrency`
Both `:type` and `:on-new-version` are required.

| Config | Description |
| --- | --- |
| `{:type :singleton :on-new-version :supersede}` | One instance at a time. New version cancels current |
| `{:type :singleton :on-new-version :drain}` | One instance at a time. Wait for current to finish |
| `{:type :singleton :on-new-version :coexist}` | One instance at a time. Both versions can run |
| `{:type :keyed :key-field :user-id :on-new-version :supersede}` | One instance per key |
| `{:type :keyed :key-field :user-id :on-new-version :drain}` | One instance per key, drain on new version |

## `:triggers`
Common types:
- `:manual` with `:label` and optional `:config`
- `:schedule` with `:config {:cron "..." :timezone "..."}`
- `:event` with `:config {:source :webhook :path "/webhooks/..." ...}`

## `:flow` rules and determinism
- Keep flow body code deterministic; avoid `rand`, current time, or external calls.
- Use `flow/step` for side effects; data transforms belong in `:function` steps.

## Functions (`:functions`)
Use `:function` steps for sandboxed transforms. For reuse, define flow-local functions.

```clojure
:functions [{:id :summarize-user
             :language :clojure
             :code "(fn [input] {:ok true :input input})"}]

(flow/step :function :summarize-user
           {:input {:user user}
            :ref :summarize-user})
```

## Input keys from `--input`
Inputs provided via `--input '{...}'` arrive as strings, but the runtime normalizes so both string and keyword keys work.
