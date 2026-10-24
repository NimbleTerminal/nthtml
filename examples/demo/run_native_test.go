//go:build !js

package main

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml"
)

func TestShutdownDisablesMouseBeforeQuit(t *testing.T) {
	a := app{viewer: nthtml.New(nil), cols: 80}
	if a.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("mouse should be enabled while running")
	}
	next, cmd := a.Update(tea.KeyPressMsg{Code: 'q'})
	closing := next.(app)
	if !closing.closing || cmd == nil {
		t.Fatal("quit must begin cleanup")
	}
	if closing.View().MouseMode != tea.MouseModeNone {
		t.Fatal("mouse must be disabled before input stops")
	}
	// Input in transit must not navigate, launch links, or restart shutdown.
	for _, msg := range []tea.Msg{
		tea.KeyPressMsg{Code: 'q'}, tea.KeyPressMsg{Code: 't'},
		tea.MouseClickMsg{Button: tea.MouseLeft},
		tea.MouseWheelMsg{Button: tea.MouseWheelDown},
		tea.WindowSizeMsg{Width: 120, Height: 40},
		nthtml.LinkMsg{Owner: closing.viewer},
	} {
		next, cmd = closing.Update(msg)
		if cmd != nil {
			t.Fatalf("late input %T produced a command", msg)
		}
		closing = next.(app)
		if !closing.closing || closing.viewer.LayoutMode() != nthtml.LayoutTypeset || closing.cols != 80 {
			t.Fatal("late input changed state")
		}
	}
	_, cmd = closing.Update(shutdownReadyMsg{})
	if cmd == nil {
		t.Fatal("missing final quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("shutdown should finish with Quit")
	}
}

type shutdownOutput struct {
	mu                      sync.Mutex
	data                    string
	enabled, disabled       chan struct{}
	sawEnabled, sawDisabled bool
}

func (w *shutdownOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data += string(p)
	if !w.sawEnabled && strings.Contains(w.data, "\x1b[?1002h") {
		w.sawEnabled = true
		close(w.enabled)
	}
	if w.sawEnabled && !w.sawDisabled && strings.Contains(string(p), "\x1b[?1002l") {
		w.sawDisabled = true
		close(w.disabled)
	}
	return len(p), nil
}
func TestProgramConsumesLateMouseInputDuringShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	input, send := io.Pipe()
	defer input.Close()
	defer send.Close()
	output := &shutdownOutput{enabled: make(chan struct{}), disabled: make(chan struct{})}
	p := tea.NewProgram(app{viewer: nthtml.New(nil), cols: 80}, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithEnvironment([]string{"TERM=xterm-256color"}), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	select {
	case <-output.enabled:
	case err := <-done:
		t.Fatalf("exited before mouse enabled: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := io.WriteString(send, "q"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-output.disabled:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Write a terminal SGR mouse report after reporting has been disabled.
	// The reader must still be alive to consume this already-in-flight event.
	consumed := make(chan error, 1)
	go func() { _, err := io.WriteString(send, "\x1b[<0;10;5M\x1b[<0;10;5m"); consumed <- err }()
	select {
	case err := <-consumed:
		if err != nil {
			t.Fatal(err)
		}
	case <-done:
		t.Fatal("stopped reading before late report was consumed")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
