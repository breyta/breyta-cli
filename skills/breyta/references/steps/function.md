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
- Safe helpers are exposed under `breyta.sandbox` (preferred; pure/deterministic):
  - `base64-encode` `(string|bytes) -> string` (Base64)
  - `base64-decode` `(string|bytes) -> string` (UTF-8)
  - `base64-decode-bytes` `(string|bytes) -> bytes`
  - `hex-encode` `(string|bytes) -> string`
  - `hex-decode` `(string) -> string` (UTF-8)
  - `hex-decode-bytes` `(string) -> bytes`
  - `sha256-hex` `(string|bytes) -> string` (hex digest)
  - `hmac-sha256-hex` `(key string|bytes, value string|bytes) -> string` (hex digest)
  - `uuid-from` `(string) -> uuid`
  - `uuid-from-bytes` `(string|bytes) -> uuid`
  - `parse-instant` `(string) -> java.time.Instant` (ISO-8601)
  - `format-instant` `(Instant) -> string` (ISO-8601)
  - `format-instant-pattern` `(Instant, pattern) -> string` (UTC)
  - `instant->epoch-ms` `(Instant) -> long`
  - `epoch-ms->instant` `(long) -> Instant`
  - `duration-between` `(Instant, Instant) -> Duration`
  - `truncate-instant` `(Instant, unit) -> Instant` (unit: `:seconds|:minutes|:hours|:days`)
  - `instant-plus` `(Instant, amount, unit) -> Instant` (unit: `:millis|:seconds|:minutes|:hours|:days`)
  - `instant-minus` `(Instant, amount, unit) -> Instant`
  - `url-encode` `(string) -> string` (UTF-8)
  - `url-decode` `(string) -> string` (UTF-8)
- Limited Java interop is also allowed in `:function` code (small allowlist): `java.time.*`,
  `java.time.format.DateTimeFormatter`, `java.time.temporal.{ChronoUnit,TemporalAdjusters}`,
  `java.util.{UUID,Base64}`, `java.math.{BigInteger,BigDecimal}`. Prefer `breyta.sandbox`.

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
