package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
)

type options struct {
	path     string
	upstream string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "anyforge:", err)
		os.Exit(1)
	}
}

func parseArgs(args []string, output io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("anyforge serve", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&opts.path, "path", "", "served repository path, such as owner/repo.git (required)")
	flags.StringVar(&opts.upstream, "upstream", "", "upstream Git remote (required; no connection at startup)")
	flags.Usage = func() {
		fmt.Fprintln(output, "Usage: anyforge serve --path owner/repo.git --upstream git@github.com:owner/repo.git")
		flags.PrintDefaults()
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		flags.Usage()
		return opts, flag.ErrHelp
	}
	if len(args) == 0 || args[0] != "serve" {
		return opts, errors.New("expected 'serve'; use --help for usage")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, errors.New("unexpected positional arguments; use --help for usage")
	}
	if !fs.ValidPath(opts.path) || opts.path == "." || strings.ContainsAny(opts.path, `\%?#`) || hasSpaceOrControl(opts.path) {
		return opts, errors.New("--path must be a relative repository path without empty, '.' or '..' segments, URL escapes, spaces, or query characters")
	}
	if !validUpstream(opts.upstream) {
		return opts, errors.New("--upstream must be an SSH, HTTPS, HTTP, or Git URL, or [user@]host:path")
	}
	return opts, nil
}

func hasSpaceOrControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

var scpRemote = regexp.MustCompile(`^([^@/:]+@)?[a-zA-Z0-9][a-zA-Z0-9.-]*:.+$`)

func validUpstream(remote string) bool {
	if remote == "" || hasSpaceOrControl(remote) || strings.Contains(remote, `\`) {
		return false
	}
	if !strings.Contains(remote, "://") {
		return scpRemote.MatchString(remote)
	}
	u, err := url.Parse(remote)
	if err != nil || u.Hostname() == "" || u.Path == "" || u.Path == "/" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return false
	}
	switch u.Scheme {
	case "ssh", "https", "http", "git":
		return true
	default:
		return false
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	opts, err := parseArgs(args, output)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		return fmt.Errorf("listen on 127.0.0.1:8080: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(output, nil))
	// Do not log the upstream: URLs can contain credentials.
	logger.Info("HTTP server started", "address", listener.Addr().String(), "path", opts.path)
	return serve(ctx, listener, logger)
}

func serve(ctx context.Context, listener net.Listener, logger *slog.Logger) error {
	defer listener.Close()
	server := &http.Server{
		Handler:           http.NotFoundHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			server.Close()
			<-result
			return fmt.Errorf("HTTP shutdown: %w", err)
		}
		err := <-result
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		logger.Info("HTTP server stopped")
		return nil
	}
}
