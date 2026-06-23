package jira

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ADFNode represents a node in the Atlassian Document Format AST
type ADFNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	Content []ADFNode      `json:"content,omitempty"`
	Marks   []ADFMark      `json:"marks,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// ADFMark represents formatting applied to a text node in the ADF AST
type ADFMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// ParseADFJSON parses raw ADF JSON string and walks the tree to generate Markdown
func ParseADFJSON(adfJSON string) (string, error) {
	if strings.TrimSpace(adfJSON) == "" {
		return "", nil
	}

	var root ADFNode
	if err := json.Unmarshal([]byte(adfJSON), &root); err != nil {
		return "", fmt.Errorf("failed to unmarshal ADF JSON: %w", err)
	}

	return WalkADF(root), nil
}

// WalkADF converts the Atlassian Document Format JSON into GFM
func WalkADF(node ADFNode) string {
	var sb strings.Builder

	if node.Type == "" {
		return ""
	}

	switch node.Type {
	case "doc":
		for _, child := range node.Content {
			sb.WriteString(WalkADF(child))
		}
	case "heading":
		levelVal, ok := node.Attrs["level"]
		level := 1
		if ok {
			if f, isFloat := levelVal.(float64); isFloat {
				level = int(f)
			} else if i, isInt := levelVal.(int); isInt {
				level = i
			}
		}
		if level < 1 {
			level = 1
		} else if level > 6 {
			level = 6
		}
		fmt.Fprintf(&sb, "\n%s ", strings.Repeat("#", level))
		for _, child := range node.Content {
			sb.WriteString(WalkADF(child))
		}
		sb.WriteString("\n")
	case "paragraph":
		sb.WriteString("\n")
		for _, child := range node.Content {
			sb.WriteString(WalkADF(child))
		}
		sb.WriteString("\n")
	case "text":
		txt := node.Text
		for _, mark := range node.Marks {
			switch mark.Type {
			case "strong":
				txt = "**" + txt + "**"
			case "em":
				txt = "_" + txt + "_"
			case "code":
				txt = "`" + txt + "`"
			case "strike":
				txt = "~~" + txt + "~~"
			}
		}
		sb.WriteString(txt)
	case "bulletList":
		sb.WriteString("\n")
		for _, child := range node.Content {
			sb.WriteString("* " + WalkADF(child))
		}
	case "orderedList":
		sb.WriteString("\n")
		for i, child := range node.Content {
			fmt.Fprintf(&sb, "%d. %s", i+1, WalkADF(child))
		}
	case "listItem":
		var itemSb strings.Builder
		for _, child := range node.Content {
			itemSb.WriteString(WalkADF(child))
		}
		sb.WriteString(strings.TrimSpace(itemSb.String()) + "\n")
	case "codeBlock":
		lang := ""
		if node.Attrs != nil {
			if l, exists := node.Attrs["language"].(string); exists {
				lang = l
			}
		}
		fmt.Fprintf(&sb, "\n```%s\n", lang)
		for _, child := range node.Content {
			sb.WriteString(child.Text)
		}
		sb.WriteString("\n```\n")
	case "panel":
		sb.WriteString("\n> ")
		for _, child := range node.Content {
			sb.WriteString(WalkADF(child))
		}
		sb.WriteString("\n")
	default:
		// Lossless fallback placeholder warning for unsupported media or plugin nodes
		fmt.Fprintf(&sb, "\n[Unsupported block node: %s]\n", node.Type)
	}

	return sb.String()
}
