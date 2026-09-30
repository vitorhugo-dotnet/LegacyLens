package intellij

import (
	"context"
	"reflect"
	"testing"

	"legacylens/core/internal/domain"
)

type recordingRunner struct {
	executable string
	args       []string
	err        error
}

func (r *recordingRunner) Run(_ context.Context, executable string, args ...string) error {
	r.executable = executable
	r.args = append([]string(nil), args...)
	return r.err
}

func TestOpenLocationArguments(t *testing.T) {
	runner := &recordingRunner{}
	editor := NewEditor("idea", runner)
	location := domain.Location{Path: `C:\Legacy App\src\ação.java`, Line: 42}
	result, err := editor.Open(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--line", "42", location.Path}
	if runner.executable != "idea" || !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("launcher = %q args = %#v; want %q %#v", runner.executable, runner.args, "idea", want)
	}
	if !result.Opened || result.File != location.Path || result.Line != 42 {
		t.Fatalf("result = %#v; want opened file and line fallback", result)
	}
}

func TestMissingLauncherReturnsFallback(t *testing.T) {
	editor := NewEditor("", &recordingRunner{})
	location := domain.Location{Path: `/project/src/Main.java`, Line: 7}
	result, err := editor.Open(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if result.Opened || result.File != location.Path || result.Line != location.Line || result.Message == "" {
		t.Fatalf("result = %#v; want file/line fallback", result)
	}
}

func TestLauncherCanBeConfiguredViaEnvironment(t *testing.T) {
	t.Setenv("LEGACYLENS_INTELLIJ_LAUNCHER", `C:\Program Files\JetBrains\IntelliJ IDEA\bin\idea64.exe`)
	runner := &recordingRunner{}
	location := domain.Location{Path: `C:\Legacy App\Main.java`, Line: 42}
	result, err := NewEditor("", runner).Open(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Opened || runner.executable != `C:\Program Files\JetBrains\IntelliJ IDEA\bin\idea64.exe` {
		t.Fatalf("launcher = %q, result = %#v", runner.executable, result)
	}
}
