//go:build e2e

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestE2E(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "anyforge")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-race", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}

	// ponytail: fixed port requires serial tests and an otherwise idle 8080;
	// use OS-assigned ports when the CLI supports a listen-address option.
	occupied, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		t.Fatalf("E2E tests require port 8080 to be free: %v", err)
	}
	t.Cleanup(func() { occupied.Close() })
	args := []string{"serve", "--path", "owner/repo.git", "--upstream", "git@example.invalid:owner/repo.git"}

	// Keep the port occupied: help and argument errors must precede listening.
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"help", []string{"--help"}, 0, "Usage: anyforge serve"},
		{"serve_help", []string{"serve", "--help"}, 0, "--upstream"},
		{"missing_command", nil, 1, "expected 'serve'"},
		{"unknown_command", []string{"unknown"}, 1, "expected 'serve'"},
		{"missing_path", []string{"serve", "--upstream", "git@host:repo"}, 1, "--path must"},
		{"missing_upstream", []string{"serve", "--path", "owner/repo.git"}, 1, "--upstream must"},
		{"invalid_path", []string{"serve", "--path", "../repo", "--upstream", "git@host:repo"}, 1, "--path must"},
		{"invalid_upstream", []string{"serve", "--path", "owner/repo.git", "--upstream", "not-a-remote"}, 1, "--upstream must"},
		{"unknown_flag", []string{"serve", "--unknown"}, 1, "flag provided but not defined"},
		{"occupied_port", args, 1, "listen on 127.0.0.1:8080"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			process := startE2E(t, binary, tc.args)
			process.wait(t, tc.code)
			output, err := os.ReadFile(process.logPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(output), tc.want) {
				t.Fatalf("output does not contain %q", tc.want)
			}
			if tc.code == 0 && (!strings.Contains(string(output), "--path") || !strings.Contains(string(output), "--upstream")) {
				t.Fatal("help does not document both required flags")
			}
		})
	}
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}

	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			// The reserved .invalid domain needs no upstream service or credentials.
			process := startE2E(t, binary, args)
			process.ready(t)
			client := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
			defer client.CloseIdleConnections()
			for _, path := range []string{"/", "/owner/repo.git/info/refs?service=git-upload-pack", "/unknown"} {
				response, err := client.Get("http://127.0.0.1:8080" + path)
				if err != nil {
					t.Fatal(err)
				}
				_, err = io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != http.StatusNotFound {
					t.Errorf("GET %s: status %d, want 404", path, response.StatusCode)
				}
			}
			if err := process.cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			process.wait(t, 0)
			listener, err := net.Listen("tcp", "127.0.0.1:8080")
			if err != nil {
				t.Fatalf("port not released after shutdown: %v", err)
			}
			listener.Close()
		})
	}
}

type e2eProcess struct {
	cmd     *exec.Cmd
	done    chan struct{}
	logPath string
}

func startE2E(t *testing.T, binary string, args []string) *e2eProcess {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "process.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	process := &e2eProcess{cmd: exec.Command(binary, args...), done: make(chan struct{}), logPath: logPath}
	process.cmd.Stdout = log
	process.cmd.Stderr = log
	if err := process.cmd.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	go func() {
		process.cmd.Wait()
		close(process.done)
	}()
	t.Cleanup(func() {
		select {
		case <-process.done:
		default:
			process.cmd.Process.Kill()
			select {
			case <-process.done:
			case <-time.After(3 * time.Second):
				t.Error("process did not exit after kill")
			}
		}
		log.Close()
		if t.Failed() {
			output, err := os.ReadFile(logPath)
			t.Logf("command: %q\ncombined stdout/stderr (read error: %v):\n%s", process.cmd.Args, err, output)
		}
	})
	return process
}

func (p *e2eProcess) wait(t *testing.T, code int) {
	t.Helper()
	select {
	case <-p.done:
		if got := p.cmd.ProcessState.ExitCode(); got != code {
			t.Fatalf("exit code %d, want %d", got, code)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("process did not exit within 8 seconds")
	}
}

func (p *e2eProcess) ready(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 250 * time.Millisecond, Transport: &http.Transport{DisableKeepAlives: true}}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8080/", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err == nil {
			_, readErr := io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if readErr != nil {
				t.Fatalf("read readiness response: %v", readErr)
			}
			return
		}
		select {
		case <-p.done:
			t.Fatal("process exited before HTTP readiness")
		case <-ctx.Done():
			t.Fatalf("HTTP readiness deadline exceeded: %v", err)
		case <-ticker.C:
		}
	}
}
