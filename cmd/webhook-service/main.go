package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	os.Args = os.Args[1:] // Remove command from args for flag parsing

	switch command {
	case "run":
		runCommand()
	case "check":
		checkCommand()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stdout, "Usage: webhook-service <command> [flags]\n\n")
	fmt.Fprintf(os.Stdout, "Commands:\n")
	fmt.Fprintf(os.Stdout, "  run     Run the webhook service\n")
	fmt.Fprintf(os.Stdout, "  check   Check configuration and connectivity\n\n")
	fmt.Fprintf(os.Stdout, "Use \"webhook-service <command> -h\" for more information about a command.\n")
}

func setupLogger(debug bool) *slog.Logger {
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)
	return logger
}
