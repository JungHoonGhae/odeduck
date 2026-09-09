package goalwork

import "strings"

// ColumnProfile describes acquired value shapes, never value examples. It is
// scoped to one bounded observation and cannot certify geographic grain,
// identifier meaning, units, population coverage or a conversion rule.
type ColumnProfile struct {
	Version        string `json:"version"`
	Missing        int    `json:"missing,omitempty"`
	Null           int    `json:"null,omitempty"`
	Strings        int    `json:"strings,omitempty"`
	Blank          int    `json:"blank,omitempty"`
	Trimmed        int    `json:"trimmed,omitempty"`
	MinTokens      int    `json:"minTokens,omitempty"`
	MaxTokens      int    `json:"maxTokens,omitempty"`
	Decimal        int    `json:"decimal,omitempty"`
	GroupedDecimal int    `json:"groupedDecimal,omitempty"`
}

func profileColumns(rows []Row, names []string) map[string]ColumnProfile {
	profiles := make(map[string]ColumnProfile, len(names))
	for _, name := range names {
		p := ColumnProfile{Version: "shape_v1"}
		for _, row := range rows {
			value, exists := row[name]
			if !exists {
				p.Missing++
				continue
			}
			if value == nil {
				p.Null++
				continue
			}
			text, ok := value.(string)
			if !ok {
				continue
			}
			p.Strings++
			trimmed := strings.TrimSpace(text)
			if trimmed != text {
				p.Trimmed++
			}
			if trimmed == "" {
				p.Blank++
				continue
			}
			tokens := len(strings.Fields(trimmed))
			if p.MinTokens == 0 || tokens < p.MinTokens {
				p.MinTokens = tokens
			}
			if tokens > p.MaxTokens {
				p.MaxTokens = tokens
			}
			// Counts explicitly describe trim=true lexical forms, not numeric
			// evidence or an automatic choice to strip identifier zeroes.
			if len(text) <= 256 {
				if plainMeasure.MatchString(trimmed) {
					p.Decimal++
				}
				if groupedMeasure.MatchString(trimmed) {
					p.GroupedDecimal++
				}
			}
		}
		profiles[name] = p
	}
	return profiles
}
