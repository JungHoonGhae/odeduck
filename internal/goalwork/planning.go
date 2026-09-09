package goalwork

import _ "embed"

//go:embed planning-guide.md
var planningGuide string

// PlanningGuide is the engine-owned action contract shared by CLI planners and
// the MCP guide resource. Callers add only their transport and disclosure framing.
func PlanningGuide() string { return planningGuide }
