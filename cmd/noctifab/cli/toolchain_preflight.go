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
	varAssignRegex      = regexp.MustCompile(`^(?:export\s+)?([A-Za-z0-9_]+)\s*(?::=|\?=|\+=|=|!=|:::=|::=)\s*(.*)$`)
	varRefRegex         = regexp.MustCompile(`\$\(([A-Za-z0-9_]+)\)|\$\{([A-Za-z0-9_]+)\}|\$([A-Za-z0-9_])`)
	crossToolchainRegex = regexp.MustCompile(`\b([a-z0-9_]+-(?:linux|unknown|elf|darwin|gnu|musl|none|eabi|uclibc|android|windows|mingw[0-9]*)(?:-[a-z0-9_]+)?)\b`)
	includeRegex        = regexp.MustCompile(`^(?:-?include|sinclude)\s+(.+)$`)

	knownToolSuffixes = []string{
		"-as", "-ld", "-gcc", "-g++", "-c++", "-clang", "-clang++",
		"-objdump", "-objcopy", "-strip", "-ar", "-ranlib", "-nasm", "-cpp",
		"-gdb", "-size", "-nm", "-readelf",
	}
)

func hasToolSuffix(name string) bool {
	for _, suffix := range knownToolSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func normalizeMakefileLines(content string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	var current strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasSuffix(trimmed, "\\") {
			current.WriteString(strings.TrimSuffix(trimmed, "\\"))
			current.WriteString(" ")
		} else {
			current.WriteString(trimmed)
			lines = append(lines, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

// ExtractMakefileToolchains extracts all explicit toolchains, assemblers, linkers, and cross-compilers declared in a Makefile.
func ExtractMakefileToolchains(content string) []string {
	vars := make(map[string]string)
	toolchainVars := map[string]bool{
		"AS": true, "LD": true, "CC": true, "CXX": true, "CPP": true, "AR": true,
		"RANLIB": true, "OBJDUMP": true, "OBJCOPY": true, "STRIP": true,
		"NASM": true, "YASM": true, "CLANG": true, "RUSTC": true, "VALGRIND": true,
		"GOC": true, "FC": true, "DC": true, "JAVAC": true, "KOTLINC": true,
		"SCALAC": true, "SWIFTC": true, "CROSS_AS": true, "CROSS_LD": true,
		"CROSS_CC": true, "CROSS_CXX": true, "QEMU": true,
	}

	for _, line := range normalizeMakefileLines(content) {
		line = strings.TrimSpace(line)
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

	// Resolve variable references iteratively with cycle protection
	resolveVar := func(val string) string {
		for i := 0; i < 5; i++ {
			if !varRefRegex.MatchString(val) {
				break
			}
			changed := false
			val = varRefRegex.ReplaceAllStringFunc(val, func(m string) string {
				varName := strings.Trim(m, "${}()")
				if strings.HasPrefix(m, "$") && len(m) == 2 && !strings.ContainsAny(m, "{}()") {
					varName = m[1:]
				}
				if v, ok := vars[varName]; ok && v != m {
					changed = true
					return v
				}
				return m
			})
			if !changed {
				break
			}
		}
		return val
	}

	seen := make(map[string]bool)
	var toolchains []string
	addTool := func(tool string) {
		tool = strings.TrimSpace(tool)
		tool = strings.Trim(tool, "\"'`")
		if tool == "" {
			return
		}
		fields := strings.Fields(tool)
		if len(fields) == 0 {
			return
		}
		tool = strings.Trim(fields[0], "\"'`")
		tool = filepath.Base(tool)
		if tool == "" || tool == "." || tool == "/" || strings.HasPrefix(tool, "$") || strings.HasPrefix(tool, "-") {
			return
		}
		switch strings.ToLower(tool) {
		case "echo", "printf", "cd", "test", "true", "false", "exit", "mkdir", "rm", "cp", "mv", "touch", "cat", "sh", "bash", "zsh", "sudo", "sed", "awk", "grep":
			return
		}
		if !seen[tool] {
			seen[tool] = true
			toolchains = append(toolchains, tool)
		}
	}

	// 1. Check known toolchain variables
	for vName, rawVal := range vars {
		resolved := resolveVar(rawVal)
		vUpper := strings.ToUpper(vName)
		if toolchainVars[vUpper] {
			addTool(resolved)
		} else if strings.Contains(strings.ToLower(vName), "toolchain") ||
			strings.Contains(strings.ToLower(vName), "cross") ||
			strings.Contains(strings.ToLower(vName), "aarch64") ||
			strings.Contains(strings.ToLower(vName), "arm") ||
			strings.Contains(strings.ToLower(vName), "riscv") ||
			strings.Contains(strings.ToLower(vName), "triple") ||
			strings.Contains(strings.ToLower(vName), "prefix") ||
			strings.Contains(strings.ToLower(vName), "target") {
			if crossToolchainRegex.MatchString(resolved) || strings.HasSuffix(resolved, "-") {
				prefix := strings.TrimSuffix(resolved, "-")
				if vars["AS"] == "" && vars["as"] == "" {
					addTool(prefix + "-as")
				}
				if vars["LD"] == "" && vars["ld"] == "" {
					addTool(prefix + "-ld")
				}
				if vars["CC"] == "" && vars["cc"] == "" {
					addTool(prefix + "-gcc")
				}
			}
		}
	}

	// 2. Scan entire Makefile content for explicit cross-compiler invocations
	for _, match := range crossToolchainRegex.FindAllString(content, -1) {
		if hasToolSuffix(match) {
			addTool(match)
		}
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
		tool = strings.Trim(tool, "\"'`")
		if tool == "" {
			return
		}
		fields := strings.Fields(tool)
		if len(fields) == 0 {
			return
		}
		tool = strings.Trim(fields[0], "\"'`")
		tool = filepath.Base(tool)
		if tool == "" || tool == "." || tool == "/" || strings.HasPrefix(tool, "$") || strings.HasPrefix(tool, "-") {
			return
		}
		switch strings.ToLower(tool) {
		case "echo", "printf", "cd", "test", "true", "false", "exit", "mkdir", "rm", "cp", "mv", "touch", "cat", "sh", "bash", "zsh", "sudo", "sed", "awk", "grep":
			return
		}
		if !seen[tool] {
			seen[tool] = true
			list = append(list, tool)
		}
	}

	// 1. Inspect Makefile and included makefiles
	makefiles := []string{"Makefile", "makefile", "GNUmakefile", "config.mk", "Makefile.inc", "Makefile.common"}
	for _, mf := range makefiles {
		makefilePath := filepath.Join(projectDir, mf)
		if data, err := os.ReadFile(makefilePath); err == nil {
			for _, tool := range ExtractMakefileToolchains(string(data)) {
				add(tool)
			}
			// Check include directives
			for _, line := range normalizeMakefileLines(string(data)) {
				line = strings.TrimSpace(line)
				if match := includeRegex.FindStringSubmatch(line); len(match) == 2 {
					for _, incFile := range strings.Fields(match[1]) {
						incFile = strings.Trim(incFile, "\"'`")
						incPath := filepath.Join(projectDir, incFile)
						if incData, incErr := os.ReadFile(incPath); incErr == nil {
							for _, tool := range ExtractMakefileToolchains(string(incData)) {
								add(tool)
							}
						}
					}
				}
			}
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
						exe = strings.TrimSpace(exe)
						if strings.HasPrefix(exe, ".") || strings.HasPrefix(exe, "/") || strings.Contains(exe, "/") {
							continue
						}
						isKnownTool := exe == "valgrind" || exe == "docker" || exe == "gcc" ||
							exe == "g++" || exe == "clang" || exe == "clang++" || exe == "nasm" ||
							exe == "yasm" || exe == "make" || exe == "cmake" || exe == "ninja" ||
							exe == "rustc" || exe == "cargo" || exe == "go" || exe == "javac" ||
							exe == "java" || exe == "mvn" || exe == "gradle" || exe == "kotlinc" ||
							exe == "scalac" || exe == "swiftc" || exe == "dotnet" || exe == "python3" ||
							exe == "pytest" || exe == "ruby" || exe == "node" || exe == "npm" ||
							exe == "opt" || exe == "llc" || strings.HasPrefix(exe, "qemu-") ||
							strings.Contains(exe, "-") || crossToolchainRegex.MatchString(exe)
						if isKnownTool {
							add(exe)
						}
					}
				}
			}
		}
	}

	return list
}
