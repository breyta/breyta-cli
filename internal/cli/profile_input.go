package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"olympos.io/encoding/edn"
)

type profilePayload struct {
	ProfileType string
	AutoUpgrade *bool
	Inputs      map[string]any
}

func parseProfileArg(raw string) (*profilePayload, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("missing profile input")
	}
	var data []byte
	format := "edn"
	if strings.HasPrefix(trimmed, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, "@"))
		if path == "" {
			return nil, errors.New("missing profile file path")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read profile file: %w", err)
		}
		data = b
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".edn":
			format = "edn"
		case ".json":
			format = "json"
		default:
			format = "edn"
		}
	} else {
		data = []byte(trimmed)
		format = "edn"
	}
	payload, err := decodeProfilePayload(data, format)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeProfilePayload(data []byte, format string) (*profilePayload, error) {
	var raw map[string]any
	switch format {
	case "edn":
		var v any
		if err := edn.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("invalid EDN format: %w", err)
		}
		m, err := normalizeEDNMap(v)
		if err != nil {
			return nil, err
		}
		raw = m
	default:
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("invalid JSON format: %w", err)
		}
	}
	if raw == nil {
		return nil, errors.New("profile payload must be a map")
	}
	return buildProfilePayload(raw)
}

func normalizeEDNMap(v any) (map[string]any, error) {
	switch typed := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range typed {
			nv, err := normalizeEDNValue(val)
			if err != nil {
				return nil, err
			}
			out[k] = nv
		}
		return out, nil
	case map[any]any:
		out := map[string]any{}
		for k, val := range typed {
			ks, ok := ednKeyToString(k)
			if !ok || strings.TrimSpace(ks) == "" {
				return nil, fmt.Errorf("invalid map key type %T", k)
			}
			nv, err := normalizeEDNValue(val)
			if err != nil {
				return nil, err
			}
			out[ks] = nv
		}
		return out, nil
	default:
		return nil, errors.New("profile payload must be a map")
	}
}

func normalizeEDNValue(v any) (any, error) {
	switch typed := v.(type) {
	case map[string]any, map[any]any:
		return normalizeEDNMap(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			nv, err := normalizeEDNValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, nv)
		}
		return out, nil
	default:
		return typed, nil
	}
}

func ednKeyToString(k any) (string, bool) {
	switch typed := k.(type) {
	case string:
		return typed, true
	case edn.Keyword:
		return string(typed), true
	case edn.Symbol:
		return string(typed), true
	default:
		return "", false
	}
}

func ednifyKeys(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := map[edn.Keyword]any{}
		for k, val := range typed {
			key := strings.TrimPrefix(k, ":")
			out[edn.Keyword(key)] = ednifyKeys(val)
		}
		return out
	case map[edn.Keyword]any:
		out := map[edn.Keyword]any{}
		for k, val := range typed {
			out[k] = ednifyKeys(val)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, ednifyKeys(item))
		}
		return out
	default:
		return v
	}
}

func buildProfilePayload(raw map[string]any) (*profilePayload, error) {
	payload := &profilePayload{
		ProfileType: "",
		Inputs:      map[string]any{},
	}
	for key, value := range raw {
		switch key {
		case "profile":
			if err := applyProfileSection(payload, value); err != nil {
				return nil, err
			}
		case "bindings":
			if err := applyBindingsSection(payload, value); err != nil {
				return nil, err
			}
		case "activation":
			if err := applyActivationSection(payload, value); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unexpected top-level key: %s", key)
		}
	}
	return payload, nil
}

func applyProfileSection(payload *profilePayload, value any) error {
	section, ok := value.(map[string]any)
	if !ok {
		return errors.New("profile must be a map")
	}
	for key, val := range section {
		switch key {
		case "type":
			str := strings.TrimSpace(strings.TrimPrefix(toString(val), ":"))
			if str == "" {
				return errors.New("profile.type must be a string")
			}
			switch strings.ToLower(str) {
			case "prod", "production", "active":
				payload.ProfileType = "prod"
			case "draft":
				payload.ProfileType = "draft"
			default:
				return fmt.Errorf("invalid profile.type: %s", str)
			}
		case "autoUpgrade", "auto-upgrade":
			b, ok := val.(bool)
			if !ok {
				return errors.New("profile.autoUpgrade must be a boolean")
			}
			payload.AutoUpgrade = &b
		default:
			return fmt.Errorf("unexpected profile key: %s", key)
		}
	}
	return nil
}

func applyBindingsSection(payload *profilePayload, value any) error {
	bindings, ok := value.(map[string]any)
	if !ok {
		return errors.New("bindings must be a map")
	}
	for slot, raw := range bindings {
		slotName := strings.TrimSpace(slot)
		if slotName == "" {
			return errors.New("binding slot name must be a string")
		}
		slotMap, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("bindings.%s must be a map", slotName)
		}
		for field, fieldValue := range slotMap {
			if fieldValue == nil || isPlaceholder(fieldValue) {
				continue
			}
			canon, ok := canonicalBindingField(field)
			if !ok {
				return fmt.Errorf("bindings.%s has unknown field: %s", slotName, field)
			}
			payload.Inputs[canon+"-"+slotName] = fieldValue
		}
	}
	return nil
}

func isPlaceholder(value any) bool {
	str := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(toString(value)), ":"))
	return str == "redacted" || str == "generate"
}

func parseSetAssignments(items []string) (map[string]any, error) {
	out := map[string]any{}
	for _, item := range items {
		raw := strings.TrimSpace(item)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --set %q (expected key=value)", raw)
		}
		key := strings.TrimSpace(parts[0])
		valRaw := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid --set %q (empty key)", raw)
		}
		if strings.HasPrefix(key, "activation.") {
			field := strings.TrimSpace(strings.TrimPrefix(key, "activation."))
			if field == "" {
				return nil, fmt.Errorf("invalid --set %q (missing activation field)", raw)
			}
			val := parseSetValue(valRaw)
			if isPlaceholder(val) {
				continue
			}
			out["form-"+field] = val
			continue
		}
		slot, field, ok := strings.Cut(key, ".")
		if !ok {
			return nil, fmt.Errorf("invalid --set %q (use activation.<field> or <slot>.<field>)", raw)
		}
		slot = strings.TrimSpace(slot)
		field = strings.TrimSpace(field)
		if slot == "" || field == "" {
			return nil, fmt.Errorf("invalid --set %q (empty slot or field)", raw)
		}
		canon, ok := canonicalBindingField(field)
		if !ok {
			return nil, fmt.Errorf("invalid --set %q (unknown field %q)", raw, field)
		}
		val := parseSetValue(valRaw)
		if isPlaceholder(val) {
			continue
		}
		out[canon+"-"+slot] = val
	}
	return out, nil
}

func parseSetValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if looksLikeJSONValue(raw) {
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err == nil {
			return v
		}
	}
	return raw
}

func looksLikeJSONValue(raw string) bool {
	switch strings.ToLower(raw) {
	case "true", "false", "null":
		return true
	}
	switch raw[0] {
	case '{', '[', '"', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	default:
		return false
	}
}

func applyActivationSection(payload *profilePayload, value any) error {
	activation, ok := value.(map[string]any)
	if !ok {
		return errors.New("activation must be a map")
	}
	for key, val := range activation {
		field := strings.TrimSpace(key)
		if field == "" {
			return errors.New("activation keys must be strings")
		}
		if val == nil || isPlaceholder(val) {
			continue
		}
		payload.Inputs["form-"+field] = val
	}
	return nil
}

func canonicalBindingField(field string) (string, bool) {
	switch field {
	case "name", "url", "apikey", "provider", "backend", "connstr", "dsn", "host", "port", "database", "user", "pass", "project", "conn", "secret",
		"header", "prefix", "location", "param-name", "query-param", "db-param":
		return field, true
	case "apiKey", "api-key":
		return "apikey", true
	case "param", "paramName":
		return "param-name", true
	case "queryParam":
		return "query-param", true
	case "dbParam":
		return "db-param", true
	case "baseUrl", "baseURL", "base-url", "baseurl":
		return "url", true
	case "password":
		return "pass", true
	case "connection":
		return "conn", true
	case "serviceAccountJson", "service-account-json", "serviceaccountjson", "svcacct":
		return "svcacct", true
	default:
		lower := strings.ToLower(field)
		switch lower {
		case "name", "url", "apikey", "provider", "backend", "connstr", "dsn", "host", "port", "database", "user", "pass", "project", "conn", "secret",
			"header", "prefix", "location", "param", "param-name", "query-param", "db-param":
			if lower == "param" {
				return "param-name", true
			}
			return lower, true
		case "baseurl", "base-url":
			return "url", true
		case "password":
			return "pass", true
		case "connection":
			return "conn", true
		case "serviceaccountjson", "service-account-json", "svcacct":
			return "svcacct", true
		default:
			return "", false
		}
	}
}

func buildProfileTemplate(requirements []any, profileType string, bindingValues map[string]any) (map[string]any, error) {
	activation := map[string]any{}
	bindings := map[string]any{}

	for _, raw := range requirements {
		req, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := strings.ToLower(toString(req["kind"]))
		if kind == "form" {
			if err := addFormFields(activation, req); err != nil {
				return nil, err
			}
			continue
		}
		slot := strings.TrimSpace(toString(req["slot"]))
		if slot == "" {
			continue
		}
		slotKey := strings.TrimPrefix(slot, ":")
		slotMap := map[string]any{}
		label := strings.TrimSpace(toString(req["label"]))
		if label != "" {
			slotMap["name"] = label
		}
		reqType := strings.ToLower(toString(req["type"]))
		hasOAuth := req["oauth"] != nil
		authType := strings.ToLower(toString(getNested(req, "auth", "type")))
		baseURL := strings.TrimSpace(toString(req["base-url"]))
		switch reqType {
		case "http-api":
			if baseURL != "" {
				slotMap["url"] = baseURL
			}
			if authType != "none" && !hasOAuth {
				slotMap["apikey"] = edn.Keyword("redacted")
			}
		case "llm-provider":
			slotMap["provider"] = "openai"
			slotMap["apikey"] = edn.Keyword("redacted")
		case "database":
			slotMap["backend"] = ""
			slotMap["connstr"] = ""
		case "secret":
			slotMap["secret"] = edn.Keyword("generate")
		default:
			if authType != "none" && !hasOAuth {
				slotMap["apikey"] = edn.Keyword("redacted")
			}
		}
		if bindingValues != nil {
			if value, ok := bindingValues[slotKey]; ok {
				slotMap = map[string]any{"conn": value}
			} else if value, ok := bindingValues[slot]; ok {
				slotMap = map[string]any{"conn": value}
			}
		}
		if len(slotMap) > 0 {
			bindings[slotKey] = slotMap
		}
	}

	template := map[string]any{
		"profile": map[string]any{
			"type":        profileType,
			"autoUpgrade": false,
		},
		"bindings":   bindings,
		"activation": activation,
	}
	return template, nil
}

func addFormFields(activation map[string]any, req map[string]any) error {
	fieldsAny, ok := req["fields"].([]any)
	if !ok {
		return nil
	}
	for _, raw := range fieldsAny {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key := strings.TrimSpace(toString(field["key"]))
		if key == "" {
			continue
		}
		if def, ok := field["default"]; ok {
			activation[key] = def
		} else {
			activation[key] = ""
		}
	}
	return nil
}

func getNested(m map[string]any, keys ...string) any {
	var cur any = m
	for _, key := range keys {
		next, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = next[key]
	}
	return cur
}

func toString(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}
