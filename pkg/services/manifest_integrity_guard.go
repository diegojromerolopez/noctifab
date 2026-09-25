package services

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// ManifestIntegrityGuard ensures generated file targets observe workspace path isolation
// and verifies that third-party code imports match pinned project dependencies.
type ManifestIntegrityGuard struct {
	goStdLib map[string]bool
	pyStdLib map[string]bool
}

// NewManifestIntegrityGuard initializes the guard with standard library indices.
func NewManifestIntegrityGuard() *ManifestIntegrityGuard {
	return &ManifestIntegrityGuard{
		goStdLib: map[string]bool{
			"archive/tar": true, "archive/zip": true, "bufio": true, "bytes": true,
			"compress/bzip2": true, "compress/flate": true, "compress/gzip": true,
			"context": true, "crypto": true, "crypto/aes": true, "crypto/cipher": true,
			"crypto/des": true, "crypto/dsa": true, "crypto/ecdsa": true, "crypto/ed25519": true,
			"crypto/elliptic": true, "crypto/hmac": true, "crypto/md5": true, "crypto/rand": true,
			"crypto/rc4": true, "crypto/rsa": true, "crypto/sha1": true, "crypto/sha256": true,
			"crypto/sha512": true, "crypto/subtle": true, "crypto/tls": true, "crypto/x509": true,
			"database/sql": true, "database/sql/driver": true, "debug/dwarf": true, "debug/elf": true,
			"encoding": true, "encoding/base32": true, "encoding/base64": true, "encoding/binary": true,
			"encoding/csv": true, "encoding/gob": true, "encoding/hex": true, "encoding/json": true,
			"encoding/pem": true, "encoding/xml": true, "errors": true, "expvar": true,
			"flag": true, "fmt": true, "hash": true, "hash/adler32": true, "hash/crc32": true,
			"hash/fnv": true, "html": true, "html/template": true, "image": true, "image/color": true,
			"image/gif": true, "image/jpeg": true, "image/png": true, "io": true, "io/fs": true,
			"io/ioutil": true, "log": true, "log/syslog": true, "math": true, "math/big": true,
			"math/bits": true, "math/cmplx": true, "math/rand": true, "mime": true, "mime/multipart": true,
			"net": true, "net/http": true, "net/http/cgi": true, "net/http/cookiejar": true,
			"net/http/httptest": true, "net/http/httptrace": true, "net/http/httputil": true,
			"net/mail": true, "net/rpc": true, "net/smtp": true, "net/url": true, "os": true,
			"os/exec": true, "os/signal": true, "os/user": true, "path": true, "path/filepath": true,
			"plugin": true, "reflect": true, "regexp": true, "regexp/syntax": true, "runtime": true,
			"runtime/debug": true, "runtime/pprof": true, "runtime/trace": true, "sort": true,
			"strconv": true, "strings": true, "sync": true, "sync/atomic": true, "syscall": true,
			"testing": true, "testing/fstest": true, "testing/iotest": true, "testing/quick": true,
			"text/scanner": true, "text/tabwriter": true, "text/template": true, "time": true,
			"unicode": true, "unicode/utf16": true, "unicode/utf8": true, "unsafe": true,
		},
		pyStdLib: pyStdLib,
	}
}

// ValidatePathIsolation verifies that targetPath resides strictly within projectRoot
// and does not attempt directory traversal or modification of protected directories.
func (g *ManifestIntegrityGuard) ValidatePathIsolation(projectRoot, targetPath string) error {
	cleanRoot := filepath.Clean(projectRoot)
	var fullPath string
	if filepath.IsAbs(targetPath) {
		fullPath = filepath.Clean(targetPath)
	} else {
		fullPath = filepath.Clean(filepath.Join(cleanRoot, targetPath))
	}

	rel, err := filepath.Rel(cleanRoot, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." && fullPath != cleanRoot {
		return fmt.Errorf("path isolation violation: path %q attempts to escape project root %q", targetPath, projectRoot)
	}

	// Forbidden directories
	cleanRel := filepath.ToSlash(rel)
	if cleanRel == ".git" || strings.HasPrefix(cleanRel, ".git/") {
		return fmt.Errorf("path isolation violation: modifying .git directory is forbidden: %s", targetPath)
	}
	if cleanRel == ".noctifab" || strings.HasPrefix(cleanRel, ".noctifab/") {
		return fmt.Errorf("path isolation violation: modifying internal .noctifab directory is forbidden: %s", targetPath)
	}

	return nil
}

// ValidateImports verifies that all external packages imported by fileContent are either
// standard library modules or declared in declaredDependencies.
func (g *ManifestIntegrityGuard) ValidateImports(filePath string, fileContent string, declaredDependencies []string, projectModule string) error {
	declaredMap := make(map[string]bool)
	for _, dep := range declaredDependencies {
		declaredMap[strings.ToLower(strings.TrimSpace(dep))] = true
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return g.validateGoImports(filePath, fileContent, declaredMap, projectModule)
	case ".py":
		return g.validatePythonImports(filePath, fileContent, declaredMap)
	}

	return nil
}

func (g *ManifestIntegrityGuard) validateGoImports(filePath, content string, declared map[string]bool, projectModule string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, content, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("failed to parse Go imports in %s: %w", filePath, err)
	}

	for _, imp := range f.Imports {
		pkg := strings.Trim(imp.Path.Value, `"`)
		if g.goStdLib[pkg] {
			continue
		}
		if projectModule != "" && (pkg == projectModule || strings.HasPrefix(pkg, projectModule+"/")) {
			continue
		}

		// Check if package matches any declared dependency prefix
		found := false
		for dec := range declared {
			if pkg == dec || strings.HasPrefix(pkg, dec+"/") {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("hallucinated import %q in %s: package not declared in go.mod", pkg, filePath)
		}
	}
	return nil
}

var (
	pyImportRE = regexp.MustCompile(`(?m)^\s*import\s+([a-zA-Z0-9_\.]+)|^\s*from\s+([a-zA-Z0-9_\.]+)\s+import`)
)

func (g *ManifestIntegrityGuard) validatePythonImports(filePath, content string, declared map[string]bool) error {
	matches := pyImportRE.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		pkg := m[1]
		if pkg == "" {
			pkg = m[2]
		}
		rootPkg := strings.Split(pkg, ".")[0]
		if rootPkg == "" || g.pyStdLib[rootPkg] {
			continue
		}
		// Relative imports
		if strings.HasPrefix(pkg, ".") {
			continue
		}

		found := false
		for dec := range declared {
			normDec := strings.ToLower(strings.ReplaceAll(dec, "-", "_"))
			if strings.ToLower(rootPkg) == normDec {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("hallucinated import %q in %s: package not declared in pyproject.toml/requirements.txt", rootPkg, filePath)
		}
	}
	return nil
}
