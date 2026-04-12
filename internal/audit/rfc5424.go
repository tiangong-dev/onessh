package audit

import (
	"fmt"
	"strings"
	"time"
)

func formatRFC5424Event(e Event, hostname string) string {
	ts := strings.TrimSpace(e.Time)
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(hostname) == "" {
		hostname = "-"
	}

	action := safeValue(e.Action)
	alias := safeValue(e.Alias)
	host := safeValue(e.Host)
	user := safeValue(e.User)
	result := safeValue(e.Result)
	errMsg := safeValue(e.Error)

	sd := fmt.Sprintf(
		"[onessh@32473 action=\"%s\" alias=\"%s\" host=\"%s\" user=\"%s\" result=\"%s\" error=\"%s\"]",
		escapeSD(action), escapeSD(alias), escapeSD(host), escapeSD(user), escapeSD(result), escapeSD(errMsg),
	)
	msg := fmt.Sprintf("action=%s result=%s alias=%s host=%s user=%s", action, result, alias, host, user)
	return fmt.Sprintf("<134>1 %s %s onessh - AUDIT %s %s", ts, hostname, sd, msg)
}

func parseRFC5424Event(line string) (Event, bool) {
	parts := strings.SplitN(line, " ", 7)
	if len(parts) < 7 {
		return Event{}, false
	}
	if !strings.HasPrefix(parts[0], "<") || !strings.HasSuffix(parts[0], ">1") {
		return Event{}, false
	}
	if parts[3] != "onessh" || parts[5] != "AUDIT" {
		return Event{}, false
	}

	rest := parts[6]
	sdPrefix := "[onessh@32473 "
	if !strings.HasPrefix(rest, sdPrefix) {
		return Event{}, false
	}
	closeIdx := findSDClose(rest)
	if closeIdx <= len(sdPrefix)-1 {
		return Event{}, false
	}

	sdBody := rest[len(sdPrefix):closeIdx]
	params, ok := parseSDParams(sdBody)
	if !ok {
		return Event{}, false
	}

	e := Event{
		Time:   parts[1],
		Action: normalizeDash(params["action"]),
		Alias:  normalizeDash(params["alias"]),
		Host:   normalizeDash(params["host"]),
		User:   normalizeDash(params["user"]),
		Result: normalizeDash(params["result"]),
		Error:  normalizeDash(params["error"]),
	}
	if e.Action == "" || e.Result == "" {
		return Event{}, false
	}
	return e, true
}

func findSDClose(s string) int {
	inQuotes := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuotes {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inQuotes = false
			}
			continue
		}
		if ch == '"' {
			inQuotes = true
			continue
		}
		if ch == ']' {
			return i
		}
	}
	return -1
}

func parseSDParams(body string) (map[string]string, bool) {
	params := map[string]string{}
	i := 0
	for i < len(body) {
		for i < len(body) && body[i] == ' ' {
			i++
		}
		if i >= len(body) {
			break
		}

		eq := strings.IndexByte(body[i:], '=')
		if eq < 1 {
			return nil, false
		}
		eq += i
		key := body[i:eq]
		i = eq + 1
		if i >= len(body) || body[i] != '"' {
			return nil, false
		}
		i++ // skip opening quote
		start := i
		for i < len(body) {
			if body[i] == '"' && !isEscaped(body, i) {
				break
			}
			i++
		}
		if i >= len(body) {
			return nil, false
		}
		params[key] = unescapeSD(body[start:i])
		i++ // skip closing quote
	}
	return params, true
}

func isEscaped(s string, pos int) bool {
	count := 0
	for i := pos - 1; i >= 0 && s[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

func safeValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func normalizeDash(v string) string {
	if v == "-" {
		return ""
	}
	return v
}

func escapeSD(v string) string {
	var sb strings.Builder
	sb.Grow(len(v))
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if ch == '\\' || ch == '"' || ch == ']' {
			sb.WriteByte('\\')
		}
		sb.WriteByte(ch)
	}
	return sb.String()
}

func unescapeSD(v string) string {
	var sb strings.Builder
	sb.Grow(len(v))
	escaped := false
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if escaped {
			sb.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		sb.WriteByte(ch)
	}
	if escaped {
		sb.WriteByte('\\')
	}
	return sb.String()
}
