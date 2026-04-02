package tui

import (
	"context"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
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

func copyToClipboardCmd(text string, count int) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		ctx := context.Background()
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.CommandContext(ctx, "pbcopy")
		case "windows":
			cmd = exec.CommandContext(ctx, "clip")
		default:
			cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
		}
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
		return clipboardDoneMsg{count: count}
	}
}
