package main

import (
	"context"
	"os/exec"
	"runtime"
)

// App struct holds the Wails application context and provides
// methods that can be called from the frontend.
type App struct {
	ctx context.Context
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// OpenInFileManager opens the system file manager at the specified path.
// This can be called from the frontend to reveal files/folders.
func (a *App) OpenInFileManager(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path) // -R reveals in Finder
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	default: // Linux
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// OpenFolder opens a folder in the system file manager.
func (a *App) OpenFolder(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	default: // Linux
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
