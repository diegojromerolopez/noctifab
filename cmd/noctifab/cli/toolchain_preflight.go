package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/services"
)

var (
	varAssignRegex      = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*(?::=|\?=|\+=|=)\s*(.+)$`)
	varRefRegex         = regexp.MustCompile(`\$\(([A-Za-z0-9_]+)\)|\$\{([A-Za-z0-9_]+)\}`)
	crossToolchainRegex = regexp.MustCompile(`\b([a-z0-9_]+-(?:linux|unknown|elf|darwin|gnu|musl|none)-[a-z0-9_]+)\b`)
)

// ExtractMakefileToolchains extracts all explicit toolchains, assemblers, linkers, and cross-compilers declared in a Makefile.
func ExtractMakefileToolchains(content string) []string {
	vars := make(map[string]string)
	toolchainVars := map[string]bool{
		"AS": true, "LD": true, "CC": true, "CXX": true, "CPP": true, "AR": true,
		"RANLIB": true, "OBJDUMP": true, "OBJCOPY": true, "STRIP": true,
		"NASM": true, "YASM": true, "CLANG": true, "RUSTC": true, "VALGRIND": true,
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if match := varAssignRegex.FindStringSubmatch(line); len(match) == 3 {
			varName := strings.TrimSpace(match[1])
			varVal := strings.TrimSpace(match[2])
			// Strip trailing comments
			if idx := strings.Index(varVal, "#"); idx != -1 {
				varVal = strings.TrimSpace(varVal[:idx])
			}
			vars[varName] = varVal
		}
	}

	// Resolve variable references iteratively
	resolveVar := func(val string) string {
		for i := 0; i < 5; i++ {
			if !varRefRegex.MatchString(val) {
				break
			}
			val = varRefRegex.ReplaceAllStringFunc(val, func(m string) string {
				varName := strings.Trim(m, "${}()")
				if v, ok := vars[varName]; ok {
					return v
				}
				return m
			})
		}
		return val
	}

	seen := make(map[string]bool)
	var toolchains []string
	addTool := func(tool string) {
		tool = strings.TrimSpace(tool)
		if tool == "" || strings.HasPrefix(tool, "$") || strings.HasPrefix(tool, "-") || strings.HasPrefix(tool, "./") || strings.HasPrefix(tool, "/") {
			return
		}
		fields := strings.Fields(tool)
		if len(fields) > 0 {
			tool = filepath.Base(fields[0])
		}
		if tool != "" && !seen[tool] {
			seen[tool] = true
			toolchains = append(toolchains, tool)
		}
	}

	// 1. Check known toolchain variables
	for vName, rawVal := range vars {
		resolved := resolveVar(rawVal)
		if toolchainVars[vName] {
			addTool(resolved)
		} else if strings.Contains(strings.ToLower(vName), "toolchain") ||
			strings.Contains(strings.ToLower(vName), "cross") ||
			strings.Contains(strings.ToLower(vName), "aarch64") ||
			strings.Contains(strings.ToLower(vName), "arm") ||
			strings.Contains(strings.ToLower(vName), "riscv") {
			if crossToolchainRegex.MatchString(resolved) {
				if vars["AS"] == "" && vars["LD"] == "" && vars["CC"] == "" {
					addTool(resolved + "-as")
					addTool(resolved + "-ld")
					addTool(resolved + "-gcc")
				}
			}
		}
	}

	// 2. Scan entire Makefile content for explicit cross-compiler invocations
	for _, match := range crossToolchainRegex.FindAllString(content, -1) {
		addTool(match)
	}

	return toolchains
}

// DetectRequiredProjectToolchains scans project manifests, Makefiles, and story contracts for required toolchain binaries.
func DetectRequiredProjectToolchains(projectDir string) []string {
	if projectDir == "" {
		projectDir = "."
	}
	seen := make(map[string]bool)
	var list []string
	add := func(tool string) {
		tool = strings.TrimSpace(tool)
		if tool == "" || strings.HasPrefix(tool, "./") || strings.HasPrefix(tool, "/") {
			return
		}
		fields := strings.Fields(tool)
		if len(fields) > 0 {
			tool = filepath.Base(fields[0])
		}
		if tool != "" && !seen[tool] {
			seen[tool] = true
			list = append(list, tool)
		}
	}

	// 1. Inspect Makefile
	makefilePath := filepath.Join(projectDir, "Makefile")
	if data, err := os.ReadFile(makefilePath); err == nil {
		for _, tool := range ExtractMakefileToolchains(string(data)) {
			add(tool)
		}
	}

	// 2. Inspect story contracts in roadmap
	for _, dir := range []string{"roadmap/user-stories", "roadmap"} {
		roadmapDir := filepath.Join(projectDir, dir)
		entries, err := os.ReadDir(roadmapDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			fpath := filepath.Join(roadmapDir, entry.Name())
			content, err := os.ReadFile(fpath)
			if err != nil {
				continue
			}
			contract, err := services.ParseStoryContract(entry.Name(), string(content))
			if err == nil {
				for _, pc := range contract.PublicContracts {
					for _, exe := range pc.AllowedExecutables {
						if !strings.HasPrefix(exe, ".") && !strings.Contains(exe, "/") {
							if exe == "valgrind" || exe == "docker" || exe == "gcc" ||
								exe == "clang" || exe == "nasm" || strings.Contains(exe, "-") {
								add(exe)
							}
						}
					}
				}
			}
		}
	}

	return list
}
