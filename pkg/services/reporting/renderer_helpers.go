package reporting

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (r *Renderer) sanitizeCell(s string) string {
	if s == "" {
		return ""
	}
	s = r.redactor.Redact(s)
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// formatDuration formats milliseconds into human readable unified duration omitting zero units.
// Examples:
//
//	6114ms   -> "6s 114ms"
//	3747003ms -> "1h 2m 27s 3ms"
//	203657ms -> "3m 23s 657ms"
//	0ms      -> "0ms"
func (r *Renderer) formatDuration(ms int64) string {
	if ms <= 0 {
		return "0ms"
	}
	d := time.Duration(ms) * time.Millisecond
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second
	d -= seconds * time.Second
	millis := d / time.Millisecond

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	if millis > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dms", millis))
	}
	return strings.Join(parts, " ")
}

func (r *Renderer) formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	diff := time.Since(t).Truncate(time.Second)
	if diff < 0 {
		diff = 0
	}
	return fmt.Sprintf("%s ago", diff.String())
}

type treeNode struct {
	name     string
	isDir    bool
	children map[string]*treeNode
}

// RenderFilesystemTree renders a slice of relative file paths as an ASCII directory tree.
func RenderFilesystemTree(files []string) string {
	if len(files) == 0 {
		return ""
	}

	root := &treeNode{
		name:     ".",
		isDir:    true,
		children: make(map[string]*treeNode),
	}

	for _, f := range files {
		f = filepath.ToSlash(strings.TrimSpace(f))
		if f == "" || f == "." {
			continue
		}
		parts := strings.Split(f, "/")
		curr := root
		for i, part := range parts {
			if part == "" {
				continue
			}
			isLast := (i == len(parts)-1)
			child, exists := curr.children[part]
			if !exists {
				child = &treeNode{
					name:     part,
					isDir:    !isLast,
					children: make(map[string]*treeNode),
				}
				curr.children[part] = child
			} else if !isLast {
				child.isDir = true
			}
			curr = child
		}
	}

	var sb strings.Builder
	sb.WriteString(".\n")

	var renderNode func(n *treeNode, prefix string)
	renderNode = func(n *treeNode, prefix string) {
		var keys []string
		for k := range n.children {
			keys = append(keys, k)
		}
		// Sort keys: directories first, then files alphabetically
		sort.Slice(keys, func(i, j int) bool {
			ni := n.children[keys[i]]
			nj := n.children[keys[j]]
			if ni.isDir != nj.isDir {
				return ni.isDir
			}
			return keys[i] < keys[j]
		})

		for i, k := range keys {
			child := n.children[k]
			isLast := (i == len(keys)-1)
			connector := "├── "
			childPrefix := "│   "
			if isLast {
				connector = "└── "
				childPrefix = "    "
			}

			dispName := child.name
			if child.isDir {
				dispName += "/"
			}
			fmt.Fprintf(&sb, "%s%s%s\n", prefix, connector, dispName)
			if child.isDir && len(child.children) > 0 {
				renderNode(child, prefix+childPrefix)
			}
		}
	}

	renderNode(root, "")
	return strings.TrimRight(sb.String(), "\n")
}
