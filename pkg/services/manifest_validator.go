package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// UndeclaredDepViolation represents a source code import missing from project manifests.
type UndeclaredDepViolation struct {
	File         string `json:"file"`
	Line         int    `json:"line"`
	Package      string `json:"package"`
	Manifest     string `json:"manifest"`
	Diagnostic   string `json:"diagnostic"`
	SuggestedFix string `json:"suggested_fix"`
}

// ManifestValidationResult captures the outcome of manifest parsing and undeclared dependency checks.
type ManifestValidationResult struct {
	Valid        bool                     `json:"valid"`
	ManifestType string                   `json:"manifest_type"`
	ManifestPath string                   `json:"manifest_path"`
	DeclaredDeps map[string]string        `json:"declared_deps"`
	ScannedFiles []string                 `json:"scanned_files"`
	Violations   []UndeclaredDepViolation `json:"violations"`
	SyntaxErrors []string                 `json:"syntax_errors"`
	Diagnostic   string                   `json:"diagnostic"`
}

// Python standard library module index (top-level modules).
var pyStdLib = map[string]bool{
	"abc": true, "aifc": true, "argparse": true, "array": true, "ast": true,
	"asynchat": true, "asyncio": true, "asyncore": true, "atexit": true, "base64": true,
	"bdb": true, "binascii": true, "binhex": true, "bisect": true, "builtins": true,
	"bz2": true, "calendar": true, "cgi": true, "cgitb": true, "chunk": true,
	"cmath": true, "cmd": true, "code": true, "codecs": true, "codeop": true,
	"collections": true, "colorsys": true, "compileall": true, "concurrent": true,
	"configparser": true, "contextlib": true, "contextvars": true, "copy": true,
	"copyreg": true, "cProfile": true, "crypt": true, "csv": true, "ctypes": true,
	"curses": true, "dataclasses": true, "datetime": true, "dbm": true, "decimal": true,
	"difflib": true, "dis": true, "distutils": true, "doctest": true, "email": true,
	"encodings": true, "enum": true, "errno": true, "faulthandler": true, "fcntl": true,
	"filecmp": true, "fileinput": true, "fnmatch": true, "fractions": true, "ftplib": true,
	"functools": true, "gc": true, "getopt": true, "getpass": true, "gettext": true,
	"glob": true, "graphlib": true, "grp": true, "gzip": true, "hashlib": true,
	"heapq": true, "hmac": true, "html": true, "http": true, "imaplib": true,
	"imghdr": true, "imp": true, "importlib": true, "inspect": true, "io": true,
	"ipaddress": true, "itertools": true, "json": true, "keyword": true, "linecache": true,
	"locale": true, "logging": true, "lzma": true, "mailbox": true, "mailcap": true,
	"marshal": true, "math": true, "mimetypes": true, "mmap": true, "modulefinder": true,
	"msilib": true, "msvcrt": true, "multiprocessing": true, "netrc": true, "nis": true,
	"nntplib": true, "numbers": true, "operator": true, "optparse": true, "os": true,
	"ossaudiodev": true, "parser": true, "pathlib": true, "pdb": true, "pickle": true,
	"pickletools": true, "pipes": true, "pkgutil": true, "platform": true, "plistlib": true,
	"poplib": true, "posix": true, "posixpath": true, "pprint": true, "profile": true,
	"pstats": true, "pty": true, "pwd": true, "py_compile": true, "pyclbr": true,
	"pydoc": true, "queue": true, "quopri": true, "random": true, "re": true,
	"readline": true, "reprlib": true, "resource": true, "rlcompleter": true, "runpy": true,
	"sched": true, "secrets": true, "select": true, "selectors": true, "shelve": true,
	"shlex": true, "shutil": true, "signal": true, "site": true, "smtpd": true,
	"smtplib": true, "sndhdr": true, "socket": true, "socketserver": true, "spwd": true,
	"sqlite3": true, "ssl": true, "stat": true, "statistics": true, "string": true,
	"stringprep": true, "struct": true, "subprocess": true, "sunau": true, "symbol": true,
	"symtable": true, "sys": true, "sysconfig": true, "syslog": true, "tabnanny": true,
	"tarfile": true, "telnetlib": true, "tempfile": true, "termios": true, "test": true,
	"textwrap": true, "threading": true, "time": true, "timeit": true, "tkinter": true,
	"token": true, "tokenize": true, "tomllib": true, "trace": true, "traceback": true,
	"tracemalloc": true, "tty": true, "turtle": true, "turtledemo": true, "types": true,
	"typing": true, "unicodedata": true, "unittest": true, "urllib": true, "uu": true,
	"uuid": true, "venv": true, "warnings": true, "wave": true, "weakref": true,
	"webbrowser": true, "winreg": true, "winsound": true, "wsgiref": true, "xdrlib": true,
	"xml": true, "xmlrpc": true, "zipapp": true, "zipfile": true, "zipimport": true,
	"zlib": true, "zoneinfo": true, "__future__": true,
}

// Rust built-in root crate prefixes.
var rustStdCrates = map[string]bool{
	"std": true, "core": true, "alloc": true, "crate": true, "super": true, "self": true,
}

// Node built-in modules.
var nodeBuiltins = map[string]bool{
	"assert": true, "async_hooks": true, "buffer": true, "child_process": true,
	"cluster": true, "console": true, "constants": true, "crypto": true, "dgram": true,
	"diagnostics_channel": true, "dns": true, "domain": true, "events": true, "fs": true,
	"http": true, "http2": true, "https": true, "inspector": true, "module": true,
	"net": true, "os": true, "path": true, "perf_hooks": true, "process": true,
	"punycode": true, "querystring": true, "readline": true, "repl": true, "stream": true,
	"string_decoder": true, "timers": true, "tls": true, "trace_events": true,
	"tty": true, "url": true, "util": true, "v8": true, "vm": true, "wasi": true,
	"worker_threads": true, "zlib": true,
}

// ParseCargoToml extracts declared dependencies from Cargo.toml.
func ParseCargoToml(content string) (map[string]string, error) {
	deps := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))
	currentSection := ""
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[] \t")
			continue
		}

		isDepSection := currentSection == "dependencies" ||
			currentSection == "dev-dependencies" ||
			currentSection == "build-dependencies" ||
			strings.HasPrefix(currentSection, "dependencies.") ||
			strings.HasPrefix(currentSection, "target.")

		if isDepSection {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				crate := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				crateNormalized := strings.ReplaceAll(crate, "-", "_")
				deps[crateNormalized] = val
				deps[crate] = val
			}
		}
	}
	return deps, scanner.Err()
}

// ParsePyprojectToml extracts declared dependencies from pyproject.toml.
func ParsePyprojectToml(content string) (map[string]string, error) {
	deps := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))
	currentSection := ""
	inArray := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[] \t")
			inArray = false
			continue
		}

		if inArray {
			if strings.Contains(line, "]") {
				inArray = false
			}
			clean := strings.Trim(line, "\",[] \t")
			if clean != "" {
				pkg := extractPyPkgName(clean)
				deps[normalizePyPkg(pkg)] = clean
			}
			continue
		}

		if (currentSection == "project" || currentSection == "tool.poetry") && strings.HasPrefix(line, "dependencies") {
			if strings.Contains(line, "[") {
				inArray = true
				if strings.Contains(line, "]") {
					inArray = false
				}
				parts := strings.Split(line, "[")
				if len(parts) > 1 {
					sub := strings.Trim(parts[1], "[] \t")
					for _, item := range strings.Split(sub, ",") {
						itemClean := strings.Trim(item, "\"' \t")
						if itemClean != "" {
							pkg := extractPyPkgName(itemClean)
							deps[normalizePyPkg(pkg)] = itemClean
						}
					}
				}
			}
		} else if strings.HasPrefix(currentSection, "tool.poetry.dependencies") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				pkg := strings.TrimSpace(parts[0])
				deps[normalizePyPkg(pkg)] = strings.TrimSpace(parts[1])
			}
		}
	}
	return deps, scanner.Err()
}

// ParseRequirementsTxt extracts dependencies from requirements.txt.
func ParseRequirementsTxt(content string) (map[string]string, error) {
	deps := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		pkg := extractPyPkgName(line)
		if pkg != "" {
			deps[normalizePyPkg(pkg)] = line
		}
	}
	return deps, scanner.Err()
}

// ParseGoMod extracts the module name and require entries from go.mod.
func ParseGoMod(content string) (string, map[string]string, error) {
	deps := make(map[string]string)
	module := ""
	scanner := bufio.NewScanner(strings.NewReader(content))
	inRequire := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "module ") {
			module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			continue
		}
		if line == "require (" {
			inRequire = true
			continue
		}
		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				deps[parts[0]] = parts[1]
			}
			continue
		}
		if strings.HasPrefix(line, "require ") {
			parts := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(parts) >= 2 {
				deps[parts[0]] = parts[1]
			}
		}
	}
	return module, deps, scanner.Err()
}

// ParsePackageJSON extracts dependencies from package.json.
func ParsePackageJSON(content string) (map[string]string, error) {
	var raw struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("invalid package.json syntax: %w", err)
	}
	deps := make(map[string]string)
	for k, v := range raw.Dependencies {
		deps[k] = v
	}
	for k, v := range raw.DevDependencies {
		deps[k] = v
	}
	return deps, nil
}

func extractPyPkgName(entry string) string {
	for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<", ";", "["} {
		if idx := strings.Index(entry, sep); idx != -1 {
			entry = entry[:idx]
		}
	}
	return strings.TrimSpace(entry)
}

func normalizePyPkg(pkg string) string {
	pkg = strings.ToLower(strings.TrimSpace(pkg))
	return strings.ReplaceAll(pkg, "-", "_")
}

var (
	pyImportRe = regexp.MustCompile(`^\s*(?:from\s+([a-zA-Z0-9_\.]+)\s+import|import\s+([a-zA-Z0-9_,\.\s]+))`)
	rustUseRe  = regexp.MustCompile(`^\s*(?:pub\s+)?use\s+([a-zA-Z0-9_]+)::`)
	jsImportRe = regexp.MustCompile(`^\s*(?:import\s+.*?from\s+['"]([^'"]+)['"]|const\s+.*?=\s*require\(['"]([^'"]+)['"]\))`)
)

// ImportOccurrence records an imported package and its source line.
type ImportOccurrence struct {
	Package string
	Line    int
}

// ExtractImportsFromFile extracts external library dependencies imported in a given source file.
func ExtractImportsFromFile(filePath string, content string, goModule string, localModules map[string]bool) []ImportOccurrence {
	ext := strings.ToLower(filepath.Ext(filePath))
	var occurrences []ImportOccurrence
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		switch ext {
		case ".py":
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			if matches := pyImportRe.FindStringSubmatch(trimmed); len(matches) > 0 {
				candidate := matches[1]
				if candidate == "" {
					candidate = matches[2]
				}
				for _, sub := range strings.Split(candidate, ",") {
					sub = strings.TrimSpace(sub)
					if idx := strings.Index(sub, " as "); idx != -1 {
						sub = strings.TrimSpace(sub[:idx])
					}
					rootPkg := strings.Split(sub, ".")[0]
					if rootPkg != "" && !pyStdLib[rootPkg] && !localModules[rootPkg] && !strings.HasPrefix(sub, ".") {
						occurrences = append(occurrences, ImportOccurrence{Package: rootPkg, Line: lineNo})
					}
				}
			}

		case ".rs":
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if matches := rustUseRe.FindStringSubmatch(trimmed); len(matches) > 1 {
				crate := matches[1]
				if !rustStdCrates[crate] && !localModules[crate] {
					occurrences = append(occurrences, ImportOccurrence{Package: crate, Line: lineNo})
				}
			}

		case ".go":
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
				pkg := strings.Trim(trimmed, "\"")
				if strings.Contains(strings.Split(pkg, "/")[0], ".") {
					if goModule == "" || !strings.HasPrefix(pkg, goModule) {
						occurrences = append(occurrences, ImportOccurrence{Package: pkg, Line: lineNo})
					}
				}
			}

		case ".js", ".ts", ".jsx", ".tsx":
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if matches := jsImportRe.FindStringSubmatch(trimmed); len(matches) > 0 {
				spec := matches[1]
				if spec == "" && len(matches) > 2 {
					spec = matches[2]
				}
				if spec != "" && !strings.HasPrefix(spec, ".") && !strings.HasPrefix(spec, "/") {
					parts := strings.Split(spec, "/")
					root := parts[0]
					if strings.HasPrefix(root, "@") && len(parts) > 1 {
						root = parts[0] + "/" + parts[1]
					}
					if !nodeBuiltins[root] {
						occurrences = append(occurrences, ImportOccurrence{Package: root, Line: lineNo})
					}
				}
			}
		}
	}
	return occurrences
}

// DiscoverLocalModules detects project internal module roots (e.g. Python filenames and subdirs).
func DiscoverLocalModules(projectPath string) map[string]bool {
	local := make(map[string]bool)
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return local
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			local[name] = true
		} else if strings.HasSuffix(name, ".py") {
			local[strings.TrimSuffix(name, ".py")] = true
		} else if strings.HasSuffix(name, ".rs") {
			local[strings.TrimSuffix(name, ".rs")] = true
		}
	}
	srcDir := filepath.Join(projectPath, "src")
	if srcEntries, err := os.ReadDir(srcDir); err == nil {
		for _, entry := range srcEntries {
			name := entry.Name()
			if entry.IsDir() {
				local[name] = true
			} else if strings.HasSuffix(name, ".py") {
				local[strings.TrimSuffix(name, ".py")] = true
			} else if strings.HasSuffix(name, ".rs") {
				local[strings.TrimSuffix(name, ".rs")] = true
			}
		}
	}
	return local
}
