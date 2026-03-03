package onboard

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/grasberg/sofia/cmd/sofia/internal"
	"github.com/grasberg/sofia/pkg/config"
)

func onboard() {
	configPath := internal.GetConfigPath()

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists at %s\n", configPath)
		fmt.Print("Overwrite? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" {
			fmt.Println("Aborted.")
			return
		}
	}

	cfg := config.DefaultConfig()
	if err := config.SaveConfig(configPath, cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	workspace := cfg.WorkspacePath()
	createWorkspaceTemplates(workspace)

	err := installAntigravityKit()
	if err != nil {
		fmt.Printf("Warning: Could not auto-install antigravity-kit: %v\n", err)
	}

	fmt.Printf("%s sofia is ready!\n", internal.Logo)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Add your API key to", configPath)
	fmt.Println("")
	fmt.Println("     Recommended:")
	fmt.Println("     - OpenRouter: https://openrouter.ai/keys (access 100+ models)")
	fmt.Println("     - Ollama:     https://ollama.com (local, free)")
	fmt.Println("")
	fmt.Println("     See README.md for 17+ supported providers.")
	fmt.Println("")
	fmt.Println("  2. Chat: sofia agent -m \"Hello!\"")
}

func installAntigravityKit() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	targetDir := filepath.Join(home, ".sofia", "antigravity-kit")

	// If it already exists, let's just abort and say success, so we don't overwrite user's custom edits
	if _, err := os.Stat(targetDir); err == nil {
		return nil
	}

	// Where is it located relative to us?
	// It's checked into the repo at third_party/antigravity-kit
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	sourceDir := filepath.Join(wd, "third_party", "antigravity-kit")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		// Maybe relative to the executable path
		exePath, err := os.Executable()
		if err == nil {
			sourceDir = filepath.Join(filepath.Dir(exePath), "..", "third_party", "antigravity-kit")
			if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
				return fmt.Errorf("could not find third_party/antigravity-kit in %s or %s", wd, filepath.Dir(exePath))
			}
		} else {
			return fmt.Errorf("could not find third_party/antigravity-kit in %s", wd)
		}
	}

	fmt.Printf("Copying antigravity-kit templates to %s...\n", targetDir)
	return copyDir(sourceDir, targetDir)
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, info.Mode())
	})
}

func createWorkspaceTemplates(workspace string) {
	err := copyEmbeddedToTarget(workspace)
	if err != nil {
		fmt.Printf("Error copying workspace templates: %v\n", err)
	}
}

func copyEmbeddedToTarget(targetDir string) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("Failed to create target directory: %w", err)
	}

	// Walk through all files in embed.FS
	err := fs.WalkDir(embeddedFiles, "workspace", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Read embedded file
		data, err := embeddedFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("Failed to read embedded file %s: %w", path, err)
		}

		new_path, err := filepath.Rel("workspace", path)
		if err != nil {
			return fmt.Errorf("Failed to get relative path for %s: %v\n", path, err)
		}

		// Build target file path
		targetPath := filepath.Join(targetDir, new_path)

		// Ensure target file's directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("Failed to create directory %s: %w", filepath.Dir(targetPath), err)
		}

		// Write file
		if err := os.WriteFile(targetPath, data, 0o644); err != nil {
			return fmt.Errorf("Failed to write file %s: %w", targetPath, err)
		}

		return nil
	})

	return err
}
