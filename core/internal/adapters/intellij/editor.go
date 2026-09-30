package intellij

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, executable string, args ...string) error {
	return exec.CommandContext(ctx, executable, args...).Run()
}

type Editor struct {
	launcher string
	runner   CommandRunner
}

func NewEditor(launcher string, runner CommandRunner) *Editor {
	if strings.TrimSpace(launcher) == "" {
		launcher = os.Getenv("LEGACYLENS_INTELLIJ_LAUNCHER")
	}
	if runner == nil {
		runner = execRunner{}
	}
	return &Editor{launcher: launcher, runner: runner}
}

func (e *Editor) Open(ctx context.Context, location domain.Location) (application.OpenResult, error) {
	result := application.OpenResult{File: location.Path, Line: location.Line}
	if location.Path == "" || location.Line < 1 {
		return result, errors.New("editor location requires an absolute file and one-based line")
	}
	if e == nil || e.launcher == "" {
		result.Message = "Configure the IntelliJ launcher to open this file and line."
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := e.runner.Run(ctx, e.launcher, "--line", strconv.Itoa(location.Line), location.Path); err != nil {
		result.Message = "IntelliJ could not be launched; use the file and line shown."
		return result, err
	}
	result.Opened = true
	result.Message = "IntelliJ launcher completed."
	return result, nil
}
