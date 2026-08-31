// Package agentplan turns an open-ended goal into concrete public-data search
// axes by reusing an already authenticated coding-agent CLI. It deliberately
// does not know about catalogue ranking or API calls; its only output is the
// small QueryPlan input that catalog owns.
package agentplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ProviderAuto   = "auto"
	ProviderCodex  = "codex"
	ProviderClaude = "claude"
	ProviderGemini = "gemini"
	ProviderCursor = "cursor"

	StatusUsed        = "used"
	StatusUnavailable = "unavailable"
)

var providerOrder = []string{ProviderCodex, ProviderClaude, ProviderGemini, ProviderCursor}

// Plan is intentionally small and provider-neutral. Summary is explanatory,
// not an instruction to the retrieval layer.
type Plan struct {
	Provider string   `json:"provider"`
	Status   string   `json:"status"`
	Concepts []string `json:"concepts,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

type modelPlan struct {
	Concepts []string `json:"concepts"`
	Summary  string   `json:"summary,omitempty"`
}

type commandSpec struct {
	name  string
	args  []string
	stdin string
}

type resolvedProvider struct {
	provider   string
	executable string
	prefix     []string
}

// Generate invokes one installed CLI in a non-interactive, read-only mode.
// Authentication and billing remain entirely under that CLI's configuration;
// opendatactl never reads or stores its credentials.
func Generate(ctx context.Context, goal, requested string) (Plan, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return Plan{}, errors.New("검색 목표가 비어 있습니다")
	}
	candidates, automatic, err := resolveProviders(requested)
	if err != nil {
		return Plan{}, err
	}
	// Auto fallback is one user action, not four independent three-minute jobs.
	// Share the existing ceiling across all installed providers.
	overallCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var failures []string
	for _, candidate := range candidates {
		plan, runErr := generateWithProvider(overallCtx, goal, candidate)
		if runErr == nil {
			return plan, nil
		}
		if !automatic {
			return Plan{}, runErr
		}
		failures = append(failures, runErr.Error())
	}
	return Plan{}, fmt.Errorf("설치된 agent CLI가 모두 검색 계획 생성에 실패했습니다: %s", strings.Join(failures, "; "))
}

func generateWithProvider(ctx context.Context, goal string, candidate resolvedProvider) (Plan, error) {
	prompt := planningPrompt(goal)
	workDir, err := os.MkdirTemp("", "opendatactl-agent-")
	if err != nil {
		return Plan{}, fmt.Errorf("agent 임시 작업공간 생성 실패: %w", err)
	}
	defer os.RemoveAll(workDir)
	spec := providerCommand(candidate.provider, candidate.executable, candidate.prefix, workDir, prompt)

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(callCtx, spec.name, spec.args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	if spec.stdin != "" {
		cmd.Stdin = strings.NewReader(spec.stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return Plan{}, fmt.Errorf("%s 검색 계획이 3분 안에 끝나지 않았습니다", candidate.provider)
		}
		detail := commandErrorDetail(stdout.Bytes(), stderr.Bytes())
		if detail == "" {
			detail = err.Error()
		}
		return Plan{}, fmt.Errorf("%s 검색 계획 실패: %s", candidate.provider, detail)
	}
	decoded, err := decodePlan(stdout.Bytes())
	if err != nil {
		return Plan{}, fmt.Errorf("%s 검색 계획 응답 해석 실패: %w", candidate.provider, err)
	}
	return Plan{Provider: candidate.provider, Status: StatusUsed, Concepts: decoded.Concepts, Summary: decoded.Summary}, nil
}

func resolveProvider(requested string) (provider, executable string, prefix []string, err error) {
	candidates, _, err := resolveProviders(requested)
	if err != nil {
		return "", "", nil, err
	}
	first := candidates[0]
	return first.provider, first.executable, first.prefix, nil
}

func resolveProviders(requested string) ([]resolvedProvider, bool, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = ProviderAuto
	}
	if requested != ProviderAuto && !validProvider(requested) {
		return nil, false, fmt.Errorf("지원하지 않는 agent %q — auto | codex | claude | gemini | cursor 중 하나를 사용하세요", requested)
	}
	automatic := requested == ProviderAuto
	candidates := providerOrder
	if !automatic {
		candidates = []string{requested}
	}
	var resolved []resolvedProvider
	for _, candidate := range candidates {
		for _, binary := range providerBinaries(candidate) {
			path, lookErr := exec.LookPath(binary.name)
			if lookErr == nil {
				resolved = append(resolved, resolvedProvider{provider: candidate, executable: path, prefix: binary.prefix})
				break
			}
		}
	}
	if len(resolved) > 0 {
		return resolved, automatic, nil
	}
	if automatic {
		return nil, true, errors.New("사용 가능한 agent CLI가 없습니다 — codex, claude, gemini, cursor-agent 중 하나를 설치하세요")
	}
	return nil, false, fmt.Errorf("%s CLI를 찾을 수 없습니다", requested)
}

type binaryCandidate struct {
	name   string
	prefix []string
}

func providerBinaries(provider string) []binaryCandidate {
	switch provider {
	case ProviderCodex:
		return []binaryCandidate{{name: "codex"}}
	case ProviderClaude:
		return []binaryCandidate{{name: "claude"}}
	case ProviderGemini:
		return []binaryCandidate{{name: "gemini"}}
	case ProviderCursor:
		// New Cursor installs cursor-agent; the desktop binary also exposes the
		// same agent as a subcommand on supported releases.
		return []binaryCandidate{{name: "cursor-agent"}, {name: "cursor", prefix: []string{"agent"}}}
	default:
		return nil
	}
}

func validProvider(provider string) bool {
	for _, supported := range providerOrder {
		if provider == supported {
			return true
		}
	}
	return false
}

func providerCommand(provider, executable string, prefix []string, workDir, prompt string) commandSpec {
	args := append([]string{}, prefix...)
	switch provider {
	case ProviderCodex:
		args = append(args, "exec", "--ephemeral", "--ignore-rules", "--sandbox", "read-only", "--skip-git-repo-check", "--json", "-")
		return commandSpec{name: executable, args: args, stdin: prompt}
	case ProviderClaude:
		args = append(args, "--print", "--input-format", "text", "--output-format", "json", "--permission-mode", "dontAsk", "--no-session-persistence", "--safe-mode")
	case ProviderGemini:
		// An empty --prompt selects headless mode; Gemini appends stdin, where the
		// actual goal stays out of the process list.
		args = append(args, "--prompt", "", "--output-format", "json", "--approval-mode", "plan")
	case ProviderCursor:
		args = append(args, "--print", "--output-format", "json", "--mode", "ask", "--sandbox", "enabled", "--trust", "--workspace", filepath.Clean(workDir))
	}
	return commandSpec{name: executable, args: args, stdin: prompt}
}

func commandErrorDetail(stdout, stderr []byte) string {
	for _, data := range [][]byte{stderr, stdout} {
		var value any
		if json.Unmarshal(bytes.TrimSpace(data), &value) == nil {
			if detail := findErrorText(value); detail != "" {
				return compact(detail, 300)
			}
		}
		if detail := compact(string(data), 300); detail != "" {
			return detail
		}
	}
	return ""
}

func findErrorText(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"message", "result", "error"} {
			if child, ok := typed[key]; ok {
				if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
					return text
				}
				if text := findErrorText(child); text != "" {
					return text
				}
			}
		}
	case []any:
		for _, child := range typed {
			if text := findErrorText(child); text != "" {
				return text
			}
		}
	}
	return ""
}

func planningPrompt(goal string) string {
	return `당신은 대한민국 공공데이터 카탈로그의 검색 계획기다. 사용자는 정부의 분류명이나 기관 용어를 모른다.
아래 목표를 실제 데이터셋을 찾을 수 있는 서로 다른 한국어 검색축 3~8개로 바꿔라.

규칙:
- 단순 동의어를 반복하지 말고 목표에 맞는 직접 대상, 거래·가격, 선행지표, 제약·위험, 지원·인프라 관점을 고르게 검토한다.
- 목표와 무관한 관점을 억지로 채우지 않는다.
- 각 검색축은 공공기관 데이터 제목·설명에 등장할 법한 구체적인 명사구이며 40자 이하다.
- 특정 데이터셋이 실제 존재한다고 단정하지 않는다.
- 도구를 호출하거나 파일을 읽지 않는다.
- 설명문이나 마크다운 없이 JSON 객체 하나만 출력한다.

출력 형식:
{"concepts":["검색축 1","검색축 2","검색축 3"],"summary":"한 문장짜리 탐색 전략"}

사용자 목표: ` + goal
}

func decodePlan(output []byte) (modelPlan, error) {
	var decoded any
	if json.Unmarshal(output, &decoded) == nil {
		if plan, ok := findPlan(decoded); ok {
			return validatePlan(plan)
		}
	}
	// Codex --json emits JSONL events. Inspect each event independently.
	for _, line := range bytes.Split(output, []byte("\n")) {
		if json.Unmarshal(bytes.TrimSpace(line), &decoded) == nil {
			if plan, ok := findPlan(decoded); ok {
				return validatePlan(plan)
			}
		}
	}
	if plan, ok := planFromText(string(output)); ok {
		return validatePlan(plan)
	}
	return modelPlan{}, errors.New("concepts JSON 객체를 찾지 못했습니다")
}

func findPlan(value any) (modelPlan, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if concepts, ok := stringSlice(typed["concepts"]); ok {
			summary, _ := typed["summary"].(string)
			return modelPlan{Concepts: concepts, Summary: strings.TrimSpace(summary)}, true
		}
		// Provider wrappers use one of these fields. Preserve priority rather
		// than depending on Go's randomized map iteration order.
		for _, key := range []string{"structured_output", "result", "response", "text", "message", "item"} {
			if child, exists := typed[key]; exists {
				if plan, ok := findPlan(child); ok {
					return plan, true
				}
			}
		}
		for _, child := range typed {
			if plan, ok := findPlan(child); ok {
				return plan, true
			}
		}
	case []any:
		for _, child := range typed {
			if plan, ok := findPlan(child); ok {
				return plan, true
			}
		}
	case string:
		return planFromText(typed)
	}
	return modelPlan{}, false
}

func planFromText(text string) (modelPlan, bool) {
	data := []byte(strings.TrimSpace(text))
	for start := bytes.IndexByte(data, '{'); start >= 0; {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(data[start:]))
		if decoder.Decode(&value) == nil {
			if plan, ok := findPlan(value); ok {
				return plan, true
			}
		}
		next := bytes.IndexByte(data[start+1:], '{')
		if next < 0 {
			break
		}
		start += next + 1
	}
	return modelPlan{}, false
}

func stringSlice(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func validatePlan(plan modelPlan) (modelPlan, error) {
	seen := map[string]bool{}
	concepts := make([]string, 0, len(plan.Concepts))
	for _, raw := range plan.Concepts {
		concept := strings.Join(strings.Fields(raw), " ")
		if concept == "" || seen[concept] {
			continue
		}
		if utf8.RuneCountInString(concept) > 80 {
			concept = string([]rune(concept)[:80])
		}
		seen[concept] = true
		concepts = append(concepts, concept)
		if len(concepts) == 8 {
			break
		}
	}
	if len(concepts) < 2 {
		return modelPlan{}, fmt.Errorf("서로 다른 검색축이 %d개뿐입니다 (최소 2개 필요)", len(concepts))
	}
	plan.Concepts = concepts
	plan.Summary = compact(plan.Summary, 300)
	return plan, nil
}

func compact(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return value
}
