package templates

// TemplateSource represents where a template was resolved from.
type TemplateSource string

const (
	// SourceWorkspace indicates the template is in .noctifab/templates/
	SourceWorkspace TemplateSource = "workspace"
	// SourceGlobal indicates the template is in ~/.noctifab/templates/
	SourceGlobal TemplateSource = "global"
	// SourceBuiltin indicates the template is an embedded built-in default
	SourceBuiltin TemplateSource = "builtin"
)

// TemplateInfo contains metadata about a discovered template.
type TemplateInfo struct {
	Name        string         `json:"name"`
	Language    string         `json:"language"`
	Description string         `json:"description"`
	Source      TemplateSource `json:"source"`
	Path        string         `json:"path,omitempty"`
}
