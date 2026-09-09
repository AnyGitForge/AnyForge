package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestArguments(t *testing.T) {
	for _, remote := range []string{
		"git@github.com:owner/repo.git",
		"github.com:owner/repo.git",
		"ssh://git@example.invalid:2222/owner/repo.git",
		"https://example.invalid/owner/repo.git",
		"http://localhost:3000/owner/repo.git",
		"git://example.invalid/owner/repo.git",
	} {
		opts, err := parseArgs([]string{"serve", "--path", "owner/repo.git", "--upstream", remote}, io.Discard)
		if err != nil || opts.path != "owner/repo.git" || opts.upstream != remote {
			t.Fatalf("remote %q: options=%+v, error=%v", remote, opts, err)
		}
	}
	for _, path := range []string{"", ".", "..", "/owner/repo.git", "owner//repo", "owner/../repo", "owner/./repo", "owner/repo/", `owner\repo`, "owner/%2e%2e", "owner/repo?x", "owner/repo#x", "owner/re po", "owner/repo\n"} {
		if _, err := parseArgs([]string{"serve", "--path", path, "--upstream", "git@github.com:owner/repo.git"}, io.Discard); err == nil {
			t.Errorf("accepted invalid path %q", path)
		}
	}
	for _, remote := range []string{"", "github.com", "/tmp/repo", "git@github.com:", "https://", "https://github.com", "https://github.com/", "ssh://host:bad/repo", "file:///tmp/repo", "https://host/repo?token=secret", "https://host/repo#x", "git@host:repo\n"} {
		if _, err := parseArgs([]string{"serve", "--path", "owner/repo.git", "--upstream", remote}, io.Discard); err == nil {
			t.Errorf("accepted invalid upstream %q", remote)
		}
	}
	for _, args := range [][]string{
		nil, {"unknown"}, {"serve"}, {"serve", "--unknown"},
		{"serve", "--path"},
		{"serve", "--path", "owner/repo.git", "--upstream", "git@host:repo", "extra"},
	} {
		if err := run(context.Background(), args, io.Discard); err == nil {
			t.Errorf("accepted invalid arguments %q", args)
		}
	}
	for _, args := range [][]string{{"--help"}, {"-h"}, {"serve", "--help"}} {
		if err := run(context.Background(), args, io.Discard); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("help %q: %v", args, err)
		}
	}
}

func TestHTTPAndShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer listener.Close()
	result := make(chan error, 1)
	go func() { result <- serve(ctx, listener, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	for _, path := range []string{"/", "/owner/repo.git/info/refs?service=git-upload-pack", "/unknown"} {
		response, err := client.Get("http://" + listener.Addr().String() + path)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status %d", path, response.StatusCode)
		}
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("server did not stop after cancellation")
	}
	if connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		connection.Close()
		t.Fatal("listener remained open after shutdown")
	}
}
