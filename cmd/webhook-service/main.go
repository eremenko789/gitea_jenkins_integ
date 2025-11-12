// Package main предоставляет точку входа для вебхук-сервиса интеграции Gitea и Jenkins.
// Сервис обрабатывает события pull request из Gitea и отслеживает соответствующие задачи в Jenkins.
package main

import (
	"fmt"
	"log/slog"
	"os"
)

// main является точкой входа приложения. Обрабатывает аргументы командной строки
// и запускает соответствующую команду (run или check).
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

// printUsage выводит информацию об использовании программы в стандартный вывод.
func printUsage() {
	fmt.Fprintf(os.Stdout, "Usage: webhook-service <command> [flags]\n\n")
	fmt.Fprintf(os.Stdout, "Commands:\n")
	fmt.Fprintf(os.Stdout, "  run     Run the webhook service\n")
	fmt.Fprintf(os.Stdout, "  check   Check configuration and connectivity\n\n")
	fmt.Fprintf(os.Stdout, "Use \"webhook-service <command> -h\" for more information about a command.\n")
}

// setupLogger создает и настраивает логгер с указанным уровнем логирования.
// Если debug равен true, устанавливается уровень Debug, иначе - Info.
// Возвращает настроенный логгер и устанавливает его как логгер по умолчанию.
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
