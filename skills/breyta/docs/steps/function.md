# Function step (`:function`)
Use for sandboxed transforms.

Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:code` | form/string | Yes (unless `:ref`) | Inline function body |
| `:ref` | keyword | Yes (unless `:code`) | Reference `:functions` entry |
| `:input` | any | No | Input payload |

Notes:
- Prefer `:ref` for reuse and readability.
- Keep functions deterministic; avoid time, randomness, and I/O.

Example:

```clojure
;; In the flow definition:
;; :functions [{:id :normalize
;;              :language :clojure
;;              :code "(fn [input]\n;;                       {:value (str (:value input))})"}]

(flow/step :function :normalize
           {:ref :normalize
            :input {:value 42}})
```
