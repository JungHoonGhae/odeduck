package goalwork

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// GoalContract is the planner's explicit interpretation of the original user
// goal, frozen before acquisition. Meeting it is not evidence that this
// interpretation or the real-world meaning of joined identifiers is correct.
type GoalContract struct {
	Outcome      string                   `json:"outcome"`
	Region       string                   `json:"region"`
	Period       string                   `json:"period"`
	TimeWindow   *DateWindow              `json:"timeWindow,omitempty"`
	Coverage     string                   `json:"coverage"` // sample or population
	Roles        []RoleRequirement        `json:"roles"`    // all required; bridging roles may be discovered later
	Outputs      []OutputRequirement      `json:"outputs"`
	Explanations []ExplanationRequirement `json:"explanations,omitempty"`
}

type RoleRequirement struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type OutputRequirement struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Role        string `json:"role"`
	Type        string `json:"type"` // string, number, boolean
}

type RoleBinding struct {
	Role        string `json:"role"`
	Observation string `json:"observation"`
}

type OutputBinding struct {
	Output string `json:"output"`
	Field  string `json:"field"`
}

type RequirementCheck struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type GoalEvaluation struct {
	Status              string                `json:"status"` // partial or requirements_met, never a claim of real-world goal completion
	CompositionID       string                `json:"compositionId"`
	ExecutionRevision   int                   `json:"executionRevision,omitempty"` // omitted by historical writers; never the current exploration revision
	Checks              []RequirementCheck    `json:"checks"`
	NeedsSemanticReview bool                  `json:"needsSemanticReview"`
	ReviewReason        string                `json:"reviewReason"`
	Temporal            TemporalEvaluation    `json:"temporal"`
	Explanations        []EvidenceExplanation `json:"explanations,omitempty"`
	Review              *ResultReview         `json:"review,omitempty"`
	FullScope           *FullScopeContext     `json:"fullScope,omitempty"`
}

func validateContract(c GoalContract) error {
	if c.TimeWindow != nil {
		if _, err := parseDateWindow(*c.TimeWindow); err != nil {
			return err
		}
	}
	if strings.TrimSpace(c.Outcome) == "" || len(c.Outcome) > 2000 || strings.TrimSpace(c.Region) == "" || len(c.Region) > 500 || strings.TrimSpace(c.Period) == "" || len(c.Period) > 500 {
		return fmt.Errorf("contract needs bounded outcome (2000 bytes), region and period (500 bytes each)")
	}
	if c.Coverage != "sample" && c.Coverage != "population" {
		return fmt.Errorf("contract coverage must be sample or population; preserve what the user actually requested")
	}
	if len(c.Roles) < 1 || len(c.Roles) > 8 || len(c.Outputs) < 1 || len(c.Outputs) > 16 {
		return fmt.Errorf("contract needs 1–8 required roles and 1–16 required outputs")
	}
	roles := map[string]bool{}
	for _, r := range c.Roles {
		if !validRequirementID(r.ID) || roles[r.ID] || strings.TrimSpace(r.Description) == "" || len(r.Description) > 1000 {
			return fmt.Errorf("roles need unique short IDs and descriptions")
		}
		roles[r.ID] = true
	}
	outputs := map[string]bool{}
	for _, o := range c.Outputs {
		if !validRequirementID(o.ID) || outputs[o.ID] || !roles[o.Role] || strings.TrimSpace(o.Description) == "" || len(o.Description) > 1000 {
			return fmt.Errorf("outputs need unique short IDs, descriptions and an existing required role")
		}
		outputs[o.ID] = true
		if o.Type != "string" && o.Type != "number" && o.Type != "boolean" {
			return fmt.Errorf("output type must be string, number or boolean")
		}
	}
	if len(c.Explanations) > 8 {
		return fmt.Errorf("at most 8 evidence explanations are supported")
	}
	for _, r := range c.Explanations {
		if r.Basis != "" && r.Basis != "execution" && r.Basis != "source" {
			return fmt.Errorf("explanation basis must be execution or source")
		}
		if !validRequirementID(r.ID) || outputs[r.ID] || !validExplanationTopic(r.Topic) || strings.TrimSpace(r.Description) == "" || len(r.Description) > 1000 {
			return fmt.Errorf("explanations need unique IDs distinct from data outputs, a supported evidence topic and a description")
		}
		outputs[r.ID] = true
	}
	return nil
}

func validRequirementID(id string) bool {
	return strings.TrimSpace(id) == id && id != "" && len(id) <= 100 && !strings.ContainsAny(id, ".\n\r\t")
}

func evaluateRequirements(c GoalContract, p Composition, rows []Row, observations []Observation, nodes []Node, fullScope bool) GoalEvaluation {
	result := GoalEvaluation{Status: "requirements_met", CompositionID: p.ID, NeedsSemanticReview: true, ReviewReason: "Contract interpretation, regional/temporal scope, identifier meaning and measure units remain explicit review items; structural checks do not prove causality, identity or usefulness."}
	check := func(kind, id string, passed bool, detail string) {
		result.Checks = append(result.Checks, RequirementCheck{Kind: kind, ID: id, Passed: passed, Detail: detail})
		if !passed {
			result.Status = "partial"
		}
	}
	byID := map[string]Observation{}
	for _, o := range observations {
		byID[o.ID] = o
	}
	used := compositionSources(p, byID)
	bindings := map[string]string{}
	counts := map[string]int{}
	for _, b := range p.Roles {
		bindings[b.Role] = b.Observation
		counts[b.Role]++
	}
	validRoles := map[string]bool{}
	for _, r := range c.Roles {
		id := bindings[r.ID]
		obs, exists := byID[id]
		searchedForRole := false
		for _, n := range nodes {
			if n.Hit.PK == obs.PK {
				for _, role := range n.Roles {
					searchedForRole = searchedForRole || role == r.ID
				}
			}
		}
		ok := counts[r.ID] == 1 && exists && used[id] && searchedForRole
		validRoles[r.ID] = ok
		detail := "bound observed source participates in the executed composition; semantic role assignment remains proposed"
		if !ok {
			detail = "required role needs one participating observation from a source searched under this role ID"
		}
		check("role", r.ID, ok, detail)
	}
	outputBindings := map[string]string{}
	outputCounts := map[string]int{}
	for _, b := range p.Outputs {
		outputBindings[b.Output] = b.Field
		outputCounts[b.Output]++
	}
	for _, o := range c.Outputs {
		field := outputBindings[o.ID]
		origin := field
		for _, a := range p.Aggregates {
			if a.As == field {
				origin = a.Field
			}
		}
		for _, m := range p.Measures {
			if m.As == origin {
				origin = measureSourceField(m)
			}
		}
		// An output must originate in the observation assigned to its required
		// role. Merely naming a population value "shelter capacity" cannot pass.
		ok := validRoles[o.Role] && outputCounts[o.ID] == 1 && outputMatchesRole(origin, bindings[o.Role], byID) && len(rows) > 0
		for _, row := range rows {
			if !hasOutputType(row[field], o.Type) {
				ok = false
			}
		}
		detail := "all artifact rows contain the required typed value from the role's observation"
		if !ok {
			detail = "missing, null, mistyped or misattributed output; bind an actual selected field or aggregate from its required role"
		}
		check("output", o.ID, ok, detail)
	}
	for _, requirement := range c.Explanations {
		present := requirement.Basis != "source" || slices.ContainsFunc(p.Explanations, func(d ExplanationDraft) bool { return d.ID == requirement.ID })
		check("explanation", requirement.ID, present, "execution notes are generated separately; a source-based requirement needs a cited proposal whose support and goal fit remain review items")
	}
	if c.Coverage == "population" && fullScope {
		result.FullScope = fullScopeExtents(observations, used)
		check("coverage", c.Coverage, result.FullScope.Eligible, "acquisition extent eligibility only; every original source's applicability to the full requested scope requires separate review")
	} else {
		check("coverage", c.Coverage, c.Coverage == "sample", "acquired bounded samples cannot establish population coverage")
	}
	if c.TimeWindow != nil {
		check("time_window", "requested", p.Time != nil && p.Time.Window != nil && *p.Time.Window == *c.TimeWindow, "composition must use the immutable requested inclusive date window and observed record time fields")
	}
	return result
}

func hasOutputType(value any, want string) bool {
	switch value.(type) {
	case string:
		return want == "string" && strings.TrimSpace(value.(string)) != ""
	case json.Number, float64:
		return want == "number"
	case bool:
		return want == "boolean"
	}
	return false
}
