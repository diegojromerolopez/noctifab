package demo

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed all:assets/*
var DemoFS embed.FS

// DemoArchetype defines the demo sample application category.
type DemoArchetype string

const (
	ArchetypeT4   DemoArchetype = "t4"
	ArchetypeCLI  DemoArchetype = "cli"
	ArchetypeREST DemoArchetype = "rest"
)

// DemoConfig encapsulates settings for the demo execution harness.
type DemoConfig struct {
	Archetype    DemoArchetype
	TargetDir    string
	ForceOffline bool
	SpeedFactor  float64
	Cleanup      bool
}

// StartRunner invokes the underlying code generation engine on the target directory.
type StartRunner func(ctx context.Context, targetDir string) error

// DemoService orchestrates demo loop execution.
type DemoService struct {
	startRunner StartRunner
}

// NewDemoService constructs a new DemoService with an optional code generation runner.
func NewDemoService(runner ...StartRunner) *DemoService {
	var r StartRunner
	if len(runner) > 0 {
		r = runner[0]
	}
	return &DemoService{startRunner: r}
}

// Run executes the end-to-end demo lifecycle.
func (s *DemoService) Run(ctx context.Context, cfg DemoConfig) error {
	if cfg.Archetype == "" {
		cfg.Archetype = ArchetypeT4
	}
	if cfg.TargetDir == "" {
		cfg.TargetDir = "demo"
	}
	if cfg.SpeedFactor <= 0 {
		cfg.SpeedFactor = 1.0
	}

	absDir, err := filepath.Abs(cfg.TargetDir)
	if err != nil {
		absDir = cfg.TargetDir
	}

	modeLabel := "Autonomous Code Generation (Live)"
	if cfg.ForceOffline || s.startRunner == nil {
		modeLabel = "Deterministic Mock (Offline)"
	}

	fmt.Printf("\033[1;36m🤖🌌 LAUNCHING NOCTIFAB ZERO-CONFIG DEMO SANDBOX\033[0m\n")
	fmt.Printf("Project: %s | Directory: %s | Mode: %s\n\n",
		cfg.Archetype,
		cfg.TargetDir,
		modeLabel,
	)

	// 1. Create target demo directory
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return fmt.Errorf("failed to create demo directory: %w", err)
	}

	if cfg.Cleanup {
		cleanupChan := make(chan os.Signal, 1)
		signal.Notify(cleanupChan, os.Interrupt, syscall.SIGTERM)
		defer func() {
			signal.Stop(cleanupChan)
			_ = os.RemoveAll(absDir)
		}()

		go func() {
			select {
			case <-cleanupChan:
				fmt.Println("\n\033[1;33m⚠️  Interrupt received. Cleaning up demo sandbox...\033[0m")
				_ = os.RemoveAll(absDir)
				os.Exit(0)
			case <-ctx.Done():
			}
		}()
	}

	// 2. Unpack project archetype
	fmt.Print("📦 [1/3] Extracting project templates & SPEC.md...")
	if err := s.unpackArchetype(cfg.Archetype, absDir); err != nil {
		fmt.Println(" ❌")
		return fmt.Errorf("failed to unpack archetype: %w", err)
	}
	fmt.Printf(" ✅ (%s)\n", cfg.TargetDir)
	s.sleep(100, cfg.SpeedFactor)

	// 3. Ensure Git repository and .noctifab workspace are initialized
	fmt.Print("🔧 [2/3] Initializing local git repository & .noctifab workspace...")
	if err := s.ensureWorkspaceAndGit(absDir, cfg.Archetype); err != nil {
		fmt.Println(" ❌")
		return fmt.Errorf("failed to initialize workspace: %w", err)
	}
	fmt.Println(" ✅")
	s.sleep(100, cfg.SpeedFactor)

	// 4. Run Code Generation Engine or Offline Mock Replay
	if !cfg.ForceOffline && s.startRunner != nil {
		fmt.Println("🚀 [3/3] Launching autonomous dark factory loop (noctifab start)...")
		fmt.Println("\033[1;32m====================================================================================\033[0m")
		if err := s.startRunner(ctx, absDir); err != nil {
			return fmt.Errorf("code generation failed: %w", err)
		}
		fmt.Println("\033[1;32m====================================================================================\033[0m")
		fmt.Println("\033[1;32m  ✨ NOCTIFAB DARK FACTORY CODE GENERATION COMPLETED!\033[0m")
		fmt.Println("\033[1;32m====================================================================================\033[0m")
	} else {
		fmt.Println("⚡ [3/3] Running deterministic offline simulation...")
		s.runOfflineSimulation(ctx, cfg)
	}

	fmt.Printf("\n📂 Generated project ready in: \033[1;36m%s\033[0m\n", cfg.TargetDir)
	if cfg.Archetype == ArchetypeT4 {
		fmt.Println("To inspect and run the generated demo project:")
		fmt.Printf("  cd %s\n", cfg.TargetDir)
		fmt.Println("  make test       # Run ISO C17 HTTP test suite")
		fmt.Println("  make build      # Compile bin/t4")
		fmt.Println("  ./bin/t4        # Launch S3-style server")
	}
	fmt.Println("\nReady to automate your own project? Run:")
	fmt.Println("  \033[1;36mnoctifab init [your-project-dir]\033[0m")
	fmt.Println("  \033[1;36mnoctifab start [your-project-dir] -i\033[0m")

	return nil
}

func (s *DemoService) ensureWorkspaceAndGit(absDir string, arch DemoArchetype) error {
	noctifabDir := filepath.Join(absDir, ".noctifab")
	if err := os.MkdirAll(filepath.Join(noctifabDir, "data"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(noctifabDir, "logs"), 0755); err != nil {
		return err
	}

	// Ensure .gitignore
	gitIgnorePath := filepath.Join(absDir, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); os.IsNotExist(err) {
		ignoreContent := ".noctifab/data/\n.noctifab/logs/\n.noctifab/worktrees/\n.noctifab/secrets.yaml\n.noctifab/noctifab.pid\nbin/\n*.db\n*.db-shm\n*.db-wal\n"
		if err := os.WriteFile(gitIgnorePath, []byte(ignoreContent), 0644); err != nil {
			return err
		}
	}

	// Initialize git if not already a repo
	gitDir := filepath.Join(absDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		cmdInit := exec.Command("git", "init", "-b", "main")
		cmdInit.Dir = absDir
		if err := cmdInit.Run(); err != nil {
			fallback := exec.Command("git", "init")
			fallback.Dir = absDir
			if fErr := fallback.Run(); fErr != nil {
				return fmt.Errorf("git init failed: %w", fErr)
			}
			_ = exec.Command("git", "-C", absDir, "checkout", "-b", "main").Run()
		}
		_ = exec.Command("git", "-C", absDir, "config", "user.name", "Noctifab Demo").Run()
		_ = exec.Command("git", "-C", absDir, "config", "user.email", "demo@noctifab.local").Run()
		_ = exec.Command("git", "-C", absDir, "add", ".").Run()
		_ = exec.Command("git", "-C", absDir, "commit", "-m", "initial demo archetype specification").Run()
	}

	return nil
}

func (s *DemoService) runOfflineSimulation(ctx context.Context, cfg DemoConfig) {
	fmt.Print("🧠 Planner Agent analyzing requirements & generating task DAG...")
	mockLLM := NewMockDemoLLMClient(cfg.SpeedFactor)
	_, _ = mockLLM.Complete(ctx, fmt.Sprintf("Role: PLANNER decompose %s", cfg.Archetype))
	fmt.Println(" ✅ (2 tasks scheduled)")
	s.sleep(200, cfg.SpeedFactor)

	fmt.Print("⚡ Generator Agent writing minimal functional code (Verification)...")
	_, _ = mockLLM.Complete(ctx, fmt.Sprintf("Role: GENERATOR write_file %s", cfg.Archetype))
	fmt.Println(" ✅ (Core implementation written)")
	s.sleep(200, cfg.SpeedFactor)

	fmt.Print("🧪 Tester Agent executing black-box tests & consensus voting...")
	_, _ = mockLLM.Complete(ctx, "Role: TESTER run_tests")
	fmt.Println(" ✅ (3/3 Consensus PASSED)")
	s.sleep(200, cfg.SpeedFactor)

	fmt.Print("🎉 Rebase queue merging validated branch into main...")
	s.sleep(200, cfg.SpeedFactor)
	fmt.Println(" ✅ (Branch merged)")

	fmt.Println("\n\033[1;32m====================================================================================\033[0m")
	fmt.Println("\033[1;32m  ✨ NOCTIFAB DARK FACTORY DEMO COMPLETED WITH 100% GREEN TEST CONSENSUS!\033[0m")
	fmt.Println("\033[1;32m====================================================================================\033[0m")
}

func (s *DemoService) unpackArchetype(arch DemoArchetype, targetDir string) error {
	subDir := fmt.Sprintf("assets/%s", arch)
	return fs.WalkDir(DemoFS, subDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(subDir, path)
		if relPath == "." {
			return nil
		}
		relPath = strings.TrimSuffix(relPath, ".tmpl")
		destPath := filepath.Join(targetDir, relPath)
		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}
		content, err := DemoFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, content, 0644)
	})
}

func (s *DemoService) sleep(ms int, speed float64) {
	if speed <= 0 {
		speed = 1.0
	}
	d := time.Duration(float64(ms)/speed) * time.Millisecond
	time.Sleep(d)
}
