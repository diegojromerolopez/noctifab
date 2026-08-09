package prompts

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

// Renderer resolves, parses, caches, and renders the effective template for
// every (agent, action) catalog key. All templates are resolved and
// test-rendered at construction time so any broken override fails fast at
// startup with a clear, file-named error — never mid-run.
type Renderer struct {
	templates map[string]*template.Template
	info      map[string]Description
}

// Description describes the effective template of one key (for CLI/list).
type Description struct {
	Agent  string
	Action string
	// Source is where the effective body came from.
	Source Source
	// AppendSource is "config" or "convention" when an append is applied to
	// the default body, empty otherwise.
	AppendSource string
	// Text is the effective template body text (placeholders unexpanded).
	Text string
}

func key(agent, action string) string { return agent + "/" + action }

// NewRenderer builds a Renderer for the given workspace directory and config
// overrides (agent -> action -> Override). Overrides may be nil. Every
// effective template is parsed and test-rendered with fixture data; the first
// failure aborts construction.
func NewRenderer(workspaceDir string, overrides map[string]map[string]Override) (*Renderer, error) {
	return newRenderer(workspaceDir, overrides, func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
	})
}

func newRenderer(workspaceDir string, overrides map[string]map[string]Override, warn func(format string, args ...any)) (*Renderer, error) {
	for agent, actions := range overrides {
		for action := range actions {
			if err := ValidateKey(agent, action); err != nil {
				return nil, fmt.Errorf("invalid prompts configuration: %w", err)
			}
		}
	}

	r := &Renderer{
		templates: make(map[string]*template.Template),
		info:      make(map[string]Description),
	}
	for _, agent := range Agents() {
		for _, action := range Actions(agent) {
			var ov Override
			if byAction, ok := overrides[agent]; ok {
				ov = byAction[action]
			}
			res, err := resolveKey(workspaceDir, agent, action, ov, warn)
			if err != nil {
				return nil, err
			}
			// missingkey=error: a typo'd placeholder in a user override must
			// fail the startup fixture render below with a clear error, never
			// silently render "<no value>" into a live prompt.
			tmpl, err := template.New(key(agent, action)).Option("missingkey=error").Parse(res.text)
			if err != nil {
				return nil, fmt.Errorf("prompt template %s/%s (source: %s) failed to parse: %w", agent, action, res.source, err)
			}
			var sb strings.Builder
			if err := tmpl.Execute(&sb, FixtureData(agent)); err != nil {
				return nil, fmt.Errorf("prompt template %s/%s (source: %s) failed to render: %w", agent, action, res.source, err)
			}
			r.templates[key(agent, action)] = tmpl
			r.info[key(agent, action)] = Description{
				Agent:        agent,
				Action:       action,
				Source:       res.source,
				AppendSource: res.appendSource,
				Text:         res.text,
			}
		}
	}
	return r, nil
}

// NewDefaultRenderer returns a Renderer that uses only the embedded default
// templates (no workspace discovery, no config overrides). It panics on
// error: the embedded defaults are compile-time assets covered by tests, so
// a failure here is a programming error.
func NewDefaultRenderer() *Renderer {
	r, err := NewRenderer("", nil)
	if err != nil {
		panic("prompts: embedded default templates are invalid: " + err.Error())
	}
	return r
}

// Rendered is the result of rendering one (agent, action) prompt: the
// overridable body and the non-overridable output contract. Callers send
// Full() to the LLM; keeping the parts separate lets the compaction layer
// skip the contract and lets multi-turn loops keep the contract at the end
// of continuation prompts.
type Rendered struct {
	// Body is the rendered (possibly user-overridden) action body.
	Body string
	// Contract is the non-overridable output contract block for the agent.
	Contract string
}

// Full returns the complete prompt: body followed by the output contract.
func (p Rendered) Full() string { return p.Body + p.Contract }

// Render renders the effective template for (agent, action) with the given
// data. The returned Rendered carries the body and the non-overridable
// output contract block for the agent.
func (r *Renderer) Render(agent, action string, data any) (Rendered, error) {
	tmpl, ok := r.templates[key(agent, action)]
	if !ok {
		return Rendered{}, ValidateKey(agent, action)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return Rendered{}, fmt.Errorf("prompt template %s/%s failed to render: %w", agent, action, err)
	}
	return Rendered{Body: sb.String(), Contract: Contract(agent)}, nil
}

// Describe returns the effective-template description for one key.
func (r *Renderer) Describe(agent, action string) (Description, error) {
	d, ok := r.info[key(agent, action)]
	if !ok {
		return Description{}, ValidateKey(agent, action)
	}
	return d, nil
}
