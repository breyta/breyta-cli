package cli

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/breyta/breyta-cli/internal/state"
	"github.com/spf13/cobra"
)

var publishMediaFlagNames = []string{
	"publish-media-type",
	"publish-media-source-kind",
	"publish-media-source",
	"publish-media-source-file",
	"publish-media-poster-kind",
	"publish-media-poster",
	"publish-media-alt",
	"clear-publish-media",
}

var publishMediaUploadFileResource = jobsWorkerUploadFileResource

func publishMediaFlagsChanged(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	for _, name := range publishMediaFlagNames {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func publishMediaPayloadValue(media *state.FlowPublishMedia) map[string]any {
	if media == nil {
		return nil
	}
	item := map[string]any{
		"type": media.Type,
	}
	if source := publishMediaSourcePayloadValue(media.Source); len(source) > 0 {
		item["source"] = source
	}
	if poster := publishMediaSourcePayloadValue(media.PosterSource); len(poster) > 0 {
		item["posterSource"] = poster
	}
	if alt := normalizeOptionalText(media.Alt); alt != "" {
		item["alt"] = alt
	}
	return item
}

func publishMediaSourcePayloadValue(source *state.FlowPublishMediaSource) map[string]any {
	if source == nil {
		return nil
	}
	item := map[string]any{
		"kind": source.Kind,
	}
	switch source.Kind {
	case "https-url":
		item["url"] = source.URL
	case "flow-resource":
		item["uri"] = source.URI
	}
	return item
}

func resolvePublishMediaInput(
	cmd *cobra.Command,
	app *App,
	publishMediaType string,
	publishMediaSourceKind string,
	publishMediaSource string,
	publishMediaSourceFile string,
	publishMediaPosterKind string,
	publishMediaPoster string,
	publishMediaAlt string,
	clearPublishMedia bool,
) (bool, *state.FlowPublishMedia, error) {
	provided := clearPublishMedia || publishMediaFlagsChanged(cmd)
	if !provided {
		return false, nil, nil
	}

	if clearPublishMedia {
		if publishMediaNonClearFlagsChanged(cmd) {
			return false, nil, errors.New("--clear-publish-media cannot be combined with other publish-media flags")
		}
		return true, nil, nil
	}

	typeValue := strings.ToLower(strings.TrimSpace(publishMediaType))
	if typeValue != "image" && typeValue != "video" {
		return false, nil, errors.New("publish media updates require --publish-media-type image|video")
	}

	source, err := resolvePublishMediaPrimarySource(cmd, app, publishMediaSourceKind, publishMediaSource, publishMediaSourceFile)
	if err != nil {
		return false, nil, err
	}

	posterProvided := cmd.Flags().Changed("publish-media-poster-kind") || cmd.Flags().Changed("publish-media-poster")
	var posterSource *state.FlowPublishMediaSource
	if posterProvided {
		if typeValue != "video" {
			return false, nil, errors.New("poster media is only supported for video publish media")
		}
		posterSource, err = resolvePublishMediaSource(
			"--publish-media-poster-kind",
			"--publish-media-poster",
			publishMediaPosterKind,
			publishMediaPoster,
		)
		if err != nil {
			return false, nil, err
		}
	}

	media := &state.FlowPublishMedia{
		Type:   typeValue,
		Source: source,
		Alt:    normalizeOptionalText(publishMediaAlt),
	}
	if posterSource != nil {
		media.PosterSource = posterSource
	}
	return true, media, nil
}

func resolvePublishMediaPrimarySource(cmd *cobra.Command, app *App, publishMediaSourceKind string, publishMediaSource string, publishMediaSourceFile string) (*state.FlowPublishMediaSource, error) {
	if cmd != nil && cmd.Flags().Changed("publish-media-source-file") {
		if cmd.Flags().Changed("publish-media-source-kind") || cmd.Flags().Changed("publish-media-source") {
			return nil, errors.New("--publish-media-source-file cannot be combined with --publish-media-source-kind or --publish-media-source")
		}
		if apiFlagExplicit(cmd) && strings.TrimSpace(app.APIURL) == "" {
			return nil, errors.New("--publish-media-source-file requires API mode (set BREYTA_API_URL)")
		}
		if err := requireAPI(app); err != nil {
			return nil, errors.New("--publish-media-source-file requires API mode")
		}
		path := strings.TrimSpace(publishMediaSourceFile)
		if path == "" {
			return nil, errors.New("--publish-media-source-file requires a local file path")
		}
		filename := strings.TrimSpace(filepath.Base(path))
		uploadResult, err := publishMediaUploadFileResource(cmd.Context(), app, path, filename, "")
		if err != nil {
			return nil, err
		}
		uri := firstNonBlankString(uploadResult["resourceUri"], uploadResult["uri"])
		if uri == "" {
			return nil, errors.New("publish media upload response missing resource URI")
		}
		return &state.FlowPublishMediaSource{Kind: "flow-resource", URI: uri}, nil
	}

	return resolvePublishMediaSource(
		"--publish-media-source-kind",
		"--publish-media-source",
		publishMediaSourceKind,
		publishMediaSource,
	)
}

func publishMediaNonClearFlagsChanged(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	for _, name := range publishMediaFlagNames {
		if name == "clear-publish-media" {
			continue
		}
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func resolvePublishMediaSource(kindFlag string, valueFlag string, rawKind string, rawValue string) (*state.FlowPublishMediaSource, error) {
	kind := strings.ToLower(strings.TrimSpace(rawKind))
	value := strings.TrimSpace(rawValue)
	if kind == "" || value == "" {
		return nil, fmt.Errorf("publish media updates require both %s and %s", kindFlag, valueFlag)
	}

	switch kind {
	case "https-url":
		if !isHTTPSURL(value) {
			return nil, fmt.Errorf("%s must be an https URL when %s=https-url", valueFlag, kindFlag)
		}
		return &state.FlowPublishMediaSource{Kind: kind, URL: value}, nil
	case "flow-resource":
		if !strings.HasPrefix(value, "res://") {
			return nil, fmt.Errorf("%s must be a res:// URI when %s=flow-resource", valueFlag, kindFlag)
		}
		return &state.FlowPublishMediaSource{Kind: kind, URI: value}, nil
	default:
		return nil, fmt.Errorf("%s must be https-url or flow-resource", kindFlag)
	}
}

func isHTTPSURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") && strings.TrimSpace(parsed.Host) != ""
}
