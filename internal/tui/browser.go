package tui

import (
	"context"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		ctx := context.Background()
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.CommandContext(ctx, "open", url)
		case "windows":
			cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.CommandContext(ctx, "xdg-open", url)
		}
		_ = cmd.Start()
		return nil
	}
}
