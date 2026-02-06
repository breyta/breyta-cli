# Flow output artifacts (viewers)

Flows always produce a final output: the value returned by the `:flow` form. In the Breyta UI, that output is stored as a canonical “result resource” and shown on the run’s **Output** page.

This page documents how flow authors can shape the final output so it renders well for both:
- technical flow authors, and
- end-users who will only see the final artifact (no debug panel).

## Quick examples

### 1) Default: raw EDN/JSON

If you return any normal value (map/vector/string/number/etc.), the UI will render it as raw output by default.

### 2) Force a viewer with a small envelope

Return a map with these keys:
- `:breyta.viewer/kind` — one of the supported kinds (allowlisted)
- `:breyta.viewer/value` — the value to render
- `:breyta.viewer/options` — optional viewer options (title, alt text, etc.)

Markdown:

```clojure
{:breyta.viewer/kind :markdown
 :breyta.viewer/options {:title "Summary"}
 :breyta.viewer/value "# Report\n\nHello."}
```

Image (URL, data URI, or signed URL):

```clojure
{:breyta.viewer/kind :image
 :breyta.viewer/options {:title "Screenshot" :alt "Screenshot"}
 :breyta.viewer/value "https://example.com/image.png"}
```

Audio/video (URL, data URI, or signed URL):

```clojure
{:breyta.viewer/kind :audio
 :breyta.viewer/options {:title "Audio"}
 :breyta.viewer/value "https://example.com/audio.wav"}
```

```clojure
{:breyta.viewer/kind :video
 :breyta.viewer/options {:title "Video"}
 :breyta.viewer/value "https://example.com/video.mp4"}
```

### Private media (no public URL required)

`<img>`, `<audio>`, and `<video>` ultimately need a `src` URL that the browser can fetch, but it does **not** need to be public.

Recommended pattern: persist the media as a blob and request a signed URL:

```clojure
(let [download (flow/step :http :download-video
                          {:connection :api
                           :path "/video.mp4"
                           :method :get
                           :persist {:type :blob
                                     :content-type "video/mp4"
                                     :signed-url true}})
      video-url (:url download)]
  {:breyta.viewer/kind :video
   :breyta.viewer/options {:title "Video"}
   :breyta.viewer/value video-url})
```

Tip: if you return the persisted result map directly (it includes `:url` + `:content-type`), the UI can often infer the correct media viewer without an explicit envelope.

### 3) Multi-part output (group)

Use `:group` to return multiple outputs (e.g. a report + a chart + a download link):

```clojure
{:breyta.viewer/kind :group
 :breyta.viewer/items
 [{:breyta.viewer/kind :markdown
   :breyta.viewer/options {:title "Summary"}
   :breyta.viewer/value "# Hello"}

  {:breyta.viewer/kind :image
   :breyta.viewer/options {:title "Image"}
   :breyta.viewer/value "https://example.com/image.png"}

  {:breyta.viewer/kind :raw
   :breyta.viewer/options {:title "Raw"}
   :breyta.viewer/value {:ok true :data [1 2 3]}}]}
```

## Inference (optional)

If you do **not** use an envelope, the UI can still infer media viewers from common shapes:

```clojure
{:url "https://example.com/file.png" :content-type "image/png"}
```

```clojure
{:signed-url "https://example.com/file.wav" :content-type "audio/wav"}
```

If inference doesn’t match what you want, wrap the value in an explicit envelope.

## Supported viewers (currently)

- `:raw` (fallback for everything)
- `:text`
- `:markdown`
- `:image`
- `:audio`
- `:video`
- `:group`

## Notes / constraints

- Viewer kinds are allowlisted; unknown kinds fall back to `:raw`.
- Large output is truncated by default, with an option to expand.
- For JSON outputs, the UI also accepts string keys (e.g. `"breyta.viewer/kind"`).
