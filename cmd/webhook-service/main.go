package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gitea_jenkins_integ/internal/config"
	"gitea_jenkins_integ/internal/gitea"
	"gitea_jenkins_integ/internal/jenkins"
	"gitea_jenkins_integ/internal/processor"
	"gitea_jenkins_integ/internal/webhook"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to the service configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger := newLogger(cfg.Logging.Level)
	logger.Info("configuration loaded", slog.String("path", *configPath))

	giteaToken, err := cfg.Gitea.ResolveToken()
	if err != nil {
		logger.Error("failed to resolve gitea token", slog.String("error", err.Error()))
		os.Exit(1)
	}

	jenkinsUser, jenkinsToken, err := cfg.Jenkins.ResolveCredentials()
	if err != nil {
		logger.Error("failed to resolve jenkins credentials", slog.String("error", err.Error()))
		os.Exit(1)
	}

	giteaHTTPClient := newHTTPClient(cfg.Gitea.SkipTLSVerify, 20*time.Second)
	giteaClient, err := gitea.New(cfg.Gitea.BaseURL, giteaToken, giteaHTTPClient, logger.With(slog.String("component", "gitea_client")))
	if err != nil {
		logger.Error("failed to create gitea client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	jenkinsHTTPClient := newHTTPClient(cfg.Jenkins.SkipTLSVerify, 30*time.Second)
	jenkinsClient, err := jenkins.New(cfg.Jenkins.BaseURL, jenkinsUser, jenkinsToken, jenkinsHTTPClient, logger.With(slog.String("component", "jenkins_client")))
	if err != nil {
		logger.Error("failed to create jenkins client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	proc := processor.New(cfg, jenkinsClient, giteaClient, logger.With(slog.String("component", "processor")))

	webhookSecret := ""
	if env := cfg.Server.WebhookSecretEnv; env != "" {
		webhookSecret = strings.TrimSpace(os.Getenv(env))
		if webhookSecret == "" {
			logger.Warn("webhook secret env is set but empty", slog.String("env", env))
		}
	}

	handler := webhook.New(cfg, proc, logger.With(slog.String("component", "webhook_handler")), webhookSecret)

	mux := http.NewServeMux()
	mux.Handle("/webhook", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           loggingMiddleware(logger, mux),
		ReadTimeout:       cfg.Server.ReadTimeout.Duration,
		ReadHeaderTimeout: cfg.Server.ReadTimeout.Duration,
		WriteTimeout:      cfg.Server.WriteTimeout.Duration,
		IdleTimeout:       cfg.Server.IdleTimeout.Duration,
		ErrorLog:          log.New(os.Stderr, "http: ", log.LstdFlags),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("starting http server", slog.String("address", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", slog.String("error", err.Error()))
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("server shutdown error", slog.String("error", err.Error()))
	}

	proc.Shutdown(shutdownCtx)

	logger.Info("shutdown complete")
}

func newLogger(level string) *slog.Logger {
	lvl := parseLevel(level)
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})
	return slog.New(handler)
}

func parseLevel(lvl string) slog.Leveler {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newHTTPClient(skipTLSVerify bool, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		logger.Info("http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", ww.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (r *responseWriter) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
