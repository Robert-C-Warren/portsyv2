//go:build windows

package backend

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func EnsureAbletonFolderIcon(projectPath string) error {
	projectPath = filepath.Clean(projectPath)

	// Default icon location inside the project
	iconRel := filepath.Join("Ableton Project Info", "AProject.ico")
	iconPath := filepath.Join(projectPath, iconRel)

	// If no icon exists, nothing to do.
	if _, err := os.Stat(iconPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat icon: %w", err)
	}

	// Ensure desktop.ini exists with the right contents.
	iniPath := filepath.Join(projectPath, "desktop.ini")
	content := []byte(fmt.Sprintf(`[.ShellClassInfo]
	IconResource=%s,0
	IconFile=%s
	IconIndex=0
	`, filepath.ToSlash(iconRel), filepath.ToSlash(iconRel)))

	// Write or update desktop.ini only if needed.
	needWrite := true
	if b, err := os.ReadFile(iniPath); err == nil && string(b) == string(content) {
		needWrite = false
	}
	if needWrite {
		if err := os.WriteFile(iniPath, content, 0o644); err != nil {
			return fmt.Errorf("write desktop.ini: %w", err)
		}
	}

	// Set attributes:
	// - Folder must have SYSTEM for Windows to honor desktop.ini customization.
	// - desktop.ini should be HIDDEN | SYSTEM | READONLY (conventional).
	if err := setFileAttrs(projectPath, windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_SYSTEM); err != nil {
		return fmt.Errorf("set folder attrs: %w", err)
	}
	if err := setFileAttrs(iniPath, windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_SYSTEM|windows.FILE_ATTRIBUTE_READONLY); err != nil {
		return fmt.Errorf("set desktop.ini attrs: %w", err)
	}

	return nil
}

func setFileAttrs(path string, attrs uint32) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// Preserve existing attributes, OR with declared ones
	existing, err := windows.GetFileAttributes(p)
	if err != nil {
		return err
	}
	return windows.SetFileAttributes(p, existing|attrs)
}
