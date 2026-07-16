package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/Nesoriel/opspilot/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func runMCP(ctx context.Context, args []string, stderr io.Writer) error {
	if len(args) != 1 || args[0] != "stdio" {
		printMCPUsage(stderr)
		return errors.New("mcp stdio command is required")
	}

	registry, err := buildRegistry()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	server, err := mcpserver.New(registry, mcpserver.Config{
		Version:     version,
		ToolTimeout: 15 * time.Second,
		Logger:      logger,
	})
	if err != nil {
		return fmt.Errorf("initialize MCP server: %w", err)
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

func isMCPCommand(args []string) bool {
	return len(args) > 0 && args[0] == "mcp"
}

func printMCPUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: opspilot mcp stdio")
}
