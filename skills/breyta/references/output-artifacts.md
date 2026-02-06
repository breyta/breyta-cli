# Flow output artifacts (final output viewers)

Flows always produce a final output: the value returned by the `:flow` form.

The Breyta UI has a dedicated run **Output** page that renders this final output as a user-facing artifact (separate from the debug panel). This is important for end-user installations, where users only see the final output.

This document describes how flow authors can shape the final output for good presentation.

## The viewer envelope (recommended)

Return an **envelope map** with these namespaced keys:

- `:breyta.viewer/kind` — the viewer to use (allowlisted)
- `:breyta.viewer/value` — the value to render
- `:breyta.viewer/options` — optional config (title, alt text, etc.)

Example: Markdown report

```clojure
{:breyta.viewer/kind :markdown
 :breyta.viewer/options {:title "Summary"}
 :breyta.viewer/value "# Report\n\nHello."}
```

Example: image

```clojure
{:breyta.viewer/kind :image
 :breyta.viewer/options {:title "Screenshot" :alt "Screenshot"}
 :breyta.viewer/value "https://example.com/image.png"}
```

Example: audio/video

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

### Multi-part output (group)

Use `:group` when you want to return multiple artifacts in one run:

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

## Inference (optional, no envelope)

If you don’t use an envelope, the UI may infer a media viewer from common shapes:

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

## JSON compatibility

If the final output is JSON (string keys), the UI also recognizes:

- `"breyta.viewer/kind"`
- `"breyta.viewer/value"`
- `"breyta.viewer/options"`
- `"breyta.viewer/items"`

## Guidance

- Prefer explicit envelopes for end-user-facing flows.
- Keep outputs reasonably sized; the UI truncates large raw outputs by default.
- For media, prefer URLs (or resource URLs produced by persistence/storage) over huge inline strings. Data URIs can work for small demos.

