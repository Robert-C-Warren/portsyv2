//go:build windows

package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func EnsureAbletonFolderIcon(projectPath string) error {
	// Accept either desktop.ini or Desktop.ini
	var iniPath string
	for _, name := range []string{"desktop.ini", "Desktop.ini"} {
		p := filepath.Join(projectPath, name)
		if _, err := os.Stat(p); err == nil {
			iniPath = p
			break
		}
	}
	ico := filepath.Join(projectPath, "Ableton Project Info", "AProject.ico")

	// If no desktop.ini but the icon exists, write a minimal ini
	if iniPath == "" {
		if _, err := os.Stat(ico); err != nil {
			// No icon available; nothing to do
			return nil
		}
		iniPath = filepath.Join(projectPath, "desktop.ini")
		body := strings.Join([]string{
			"[.ShellClassInfo]",
			"IconFile=Ableton Project Info\\AProject.ico",
			"IconIndex=0",
			"ConfirmFileOp=0",
			"NoSharing=0",
			"", // Trailing new line
		}, "\r\n")
		if err := os.WriteFile(iniPath, []byte(body), 0644); err != nil {
			return fmt.Errorf("write desktop.ini: %w", err)
		}
	}

	// Mark the folder as System and the ini as Hidden+System
	_ = exec.Command("attrib", "+s", projectPath).Run()
	_ = exec.Command("attrib", "+h", "+s", iniPath).Run()
	return nil
}
