package goalwork

import "github.com/JungHoonGhae/odeduck/internal/dataset"

// SourceDeclaration is bounded publisher metadata, NOT a verified record
// namespace, period or geographic scope. Missing values remain unknown.
type SourceDeclaration struct {
	Status           string   `json:"status"`
	SourceURL        string   `json:"sourceUrl,omitempty"`
	Name             string   `json:"name,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	Description      string   `json:"description,omitempty"`
	SpatialCoverage  string   `json:"spatialCoverage,omitempty"`
	TemporalCoverage string   `json:"temporalCoverage,omitempty"`
	ModifiedAt       string   `json:"modifiedAt,omitempty"`
	Notices          []string `json:"notices,omitempty"`
	Truncated        bool     `json:"truncated,omitempty"`
}

func sourceDeclarations(result *dataset.InspectionResult) map[string]SourceDeclaration {
	out := map[string]SourceDeclaration{}
	if result.File != nil {
		f := result.File
		d := SourceDeclaration{Status: "publisher_declared"}
		clip := func(s string, n int) string {
			if len(s) > n {
				d.Truncated = true
			}
			return bounded(s, n)
		}
		d.SourceURL, d.Name, d.Provider = clip(f.SourceURL, 1000), clip(f.Name, 400), clip(f.Provider, 400)
		d.ModifiedAt = clip(f.ModifiedAt, 100)
		description := f.Metadata["description"]
		if description == "" {
			description = f.Metadata["설명"]
		}
		d.Description = clip(description, 2400)
		coverage := func(official, html string) string {
			value := f.Metadata[official]
			if value == "" {
				return clip(f.Metadata[html], 600)
			}
			if other := f.Metadata[html]; other != "" && other != value {
				d.Notices = append(d.Notices, clip(html+" (HTML alternative declaration): "+other, 900))
			}
			return clip(value, 600)
		}
		d.SpatialCoverage = coverage("spatialCoverage", "공간범위")
		d.TemporalCoverage = coverage("temporalCoverage", "시간범위")
		for _, label := range []string{"데이터 한계", "기타 유의사항"} {
			if text := f.Metadata[label]; text != "" {
				d.Notices = append(d.Notices, clip(label+": "+text, 1200))
			}
		}
		out["file"] = d
	}
	if result.Standard != nil {
		s := result.Standard
		out["standard"] = SourceDeclaration{Status: "publisher_declared", Name: bounded(s.Name, 400), SourceURL: bounded(s.SourceURL, 1000), Truncated: len(s.Name) > 400 || len(s.SourceURL) > 1000}
	}
	if result.API != nil {
		a := result.API
		out["api"] = SourceDeclaration{Status: "publisher_declared", Name: bounded(a.DataName, 400), Truncated: len(a.DataName) > 400}
	}
	return out
}
