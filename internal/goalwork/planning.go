package goalwork

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed planning-guide.md
var planningGuide string

//go:embed planning-brief.md
var planningBrief string

// PlanningGuide is the full action reference. Normal planning starts with the brief.
func PlanningGuide() string { return planningGuide }
func PlanningBrief() string { return planningBrief }

// PlanningSection reads the engine-owned reference, without duplicating contracts.
func PlanningSection(topic string) (string, error) {
	var out strings.Builder
	reference := strings.ReplaceAll(planningGuide, "\r\n", "\n")
	for _, block := range strings.Split(reference, "<!-- topic: ") {
		tag, body, found := strings.Cut(block, " -->\n")
		if found && tag == topic {
			out.WriteString(body)
		}
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("unknown guide topic %q; read the topic list in the planning brief", topic)
	}
	return out.String(), nil
}

func PlanningInstructions(view View) string {
	text := PlanningBrief()
	if view.GuideTopic != "" {
		if section, err := PlanningSection(view.GuideTopic); err == nil {
			text += "\n## Requested contract: " + view.GuideTopic + "\n" + section
		}
	}
	return text
}
