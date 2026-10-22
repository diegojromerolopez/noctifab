package templates

// BuiltinTemplates maps canonical language names to their default YAML configurations and descriptions.
var BuiltinTemplates = map[string]struct {
	Description string
	YAML        string
}{
	"go": {
		Description: "Go 1.22+ with go test, go fmt, and go vet",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 300
  test_command: go test -v ./...
  formatter_command: go fmt ./...
  linter:
    command: go vet ./...
    max_issues: 100
  allowed_commands:
    - go
    - git
    - make
    - docker
`,
	},
	"python": {
		Description: "Python 3.12+ with pytest, ruff, and mypy",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 300
  test_command: pytest
  formatter_command: ruff format .
  linter:
    command: ruff check . && mypy --strict .
    max_issues: 100
  allowed_commands:
    - python3
    - python
    - pip
    - pytest
    - coverage
    - ruff
    - mypy
    - git
    - make
`,
	},
	"rust": {
		Description: "Rust 1.80+ with cargo test, cargo fmt, and clippy",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 600
  test_command: cargo test --workspace
  formatter_command: cargo fmt
  linter:
    command: cargo fmt --check && cargo clippy -- -D warnings
    max_issues: 100
  allowed_commands:
    - cargo
    - rustc
    - gcc
    - clang
    - git
    - make
  forbidden_patterns:
    - \bunsafe\s*\{
`,
	},
	"typescript": {
		Description: "TypeScript / Node.js with npm test, prettier, eslint, and tsc",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 300
  test_command: npm test
  formatter_command: npx prettier --write .
  linter:
    command: npx eslint . && npx tsc --noEmit
    max_issues: 100
  allowed_commands:
    - npm
    - npx
    - node
    - tsc
    - vitest
    - eslint
    - prettier
    - git
    - make
`,
	},
	"c": {
		Description: "C17 / C++ with make test, make format, and make lint",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 600
  test_command: make test
  formatter_command: make format
  linter:
    command: make lint
    max_issues: 100
  allowed_commands:
    - make
    - gcc
    - clang
    - clang-format
    - clang-tidy
    - sqlite3
    - git
`,
	},
	"ruby": {
		Description: "Ruby 3.4+ with RSpec and RuboCop",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 300
  test_command: bundle exec rspec
  formatter_command: bundle exec rubocop -a
  linter:
    command: bundle exec rubocop
    max_issues: 100
  allowed_commands:
    - ruby
    - bundle
    - rspec
    - rubocop
    - git
    - make
`,
	},
	"csharp": {
		Description: "C# / .NET 9 with dotnet test, dotnet format, and dotnet build",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 300
  test_command: dotnet test
  formatter_command: dotnet format
  linter:
    command: dotnet build --warnaserror
    max_issues: 100
  allowed_commands:
    - dotnet
    - git
    - make
`,
	},
	"java": {
		Description: "Java 21 with Gradle (or Maven) and Spotless",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 600
  test_command: ./gradlew test
  formatter_command: ./gradlew spotlessApply
  linter:
    command: ./gradlew check
    max_issues: 100
  allowed_commands:
    - gradle
    - ./gradlew
    - java
    - javac
    - git
    - make
`,
	},
	"erlang": {
		Description: "Erlang/OTP 27+ with Rebar3 EUnit and Dialyzer",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 300
  test_command: rebar3 eunit
  linter:
    command: rebar3 dialyzer
    max_issues: 100
  allowed_commands:
    - rebar3
    - erl
    - erlc
    - git
    - make
`,
	},
	"ocaml": {
		Description: "OCaml 5+ with Dune runtest, dune fmt, and dune build @check",
		YAML: `sandbox:
  mode: host
  timeout_seconds: 300
  test_command: dune runtest
  formatter_command: dune fmt
  linter:
    command: dune build @check
    max_issues: 100
  allowed_commands:
    - dune
    - ocamlc
    - ocamlopt
    - git
    - make
`,
	},
}

// TemplateAliases maps common language synonyms or extensions to canonical template keys.
var TemplateAliases = map[string]string{
	"javascript": "typescript",
	"js":         "typescript",
	"ts":         "typescript",
	"golang":     "go",
	"rs":         "rust",
	"py":         "python",
	"cpp":        "c",
	"c++":        "c",
	"rb":         "ruby",
	"dotnet":     "csharp",
	"cs":         "csharp",
	"c#":         "csharp",
	"otp":        "erlang",
	"ml":         "ocaml",
}

// CanonicalTemplateName resolves any synonym/alias to the canonical template name.
func CanonicalTemplateName(name string) string {
	if canonical, ok := TemplateAliases[name]; ok {
		return canonical
	}
	return name
}
