package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Nesoriel/opspilot/internal/agent"
	"github.com/Nesoriel/opspilot/internal/tools/dnslookup"
	"github.com/Nesoriel/opspilot/internal/tools/httpprobe"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		writeJSON(os.Stdout, map[string]any{"ok": false, "error": err.Error()})
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("command is required")
	}

	switch args[0] {
	case "version":
		return writeJSON(stdout, map[string]any{"name": "opspilot", "version": version})
	case "tool":
		return runTool(ctx, args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runTool(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	registry, err := buildRegistry()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		printToolUsage(stderr)
		return errors.New("tool command is required")
	}

	switch args[0] {
	case "list":
		return writeJSON(stdout, map[string]any{"tools": registry.Definitions()})
	case "run":
		if len(args) < 2 {
			return errors.New("tool name is required")
		}
		tool, found := registry.Get(args[1])
		if !found {
			return fmt.Errorf("tool %q is not registered", args[1])
		}

		arguments := []byte(`{}`)
		if len(args) >= 3 {
			arguments = []byte(args[2])
		} else {
			stdin, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			if len(stdin) > 0 {
				arguments = stdin
			}
		}
		if !json.Valid(arguments) {
			return errors.New("tool arguments must be valid JSON")
		}

		result, err := tool.Execute(ctx, arguments)
		if err != nil {
			return err
		}
		return writeJSON(stdout, map[string]any{
			"ok":   true,
			"tool": args[1],
			"data": json.RawMessage(result),
		})
	default:
		printToolUsage(stderr)
		return fmt.Errorf("unknown tool command %q", args[0])
	}
}

func buildRegistry() (*agent.Registry, error) {
	allowPrivate, _ := strconv.ParseBool(os.Getenv("OPSPILOT_HTTP_ALLOW_PRIVATE"))
	registry := agent.NewRegistry()
	for _, tool := range []agent.Tool{
		dnslookup.New(nil),
		httpprobe.New(httpprobe.Config{
			AllowPrivateNetworks: allowPrivate,
			Timeout:              15 * time.Second,
		}),
	} {
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: opspilot <version|tool>")
}

func printToolUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: opspilot tool <list|run TOOL [JSON]>")
}
