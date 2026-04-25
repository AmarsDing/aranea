// Package server centralises the Aranea backend bootstrap so that both
// the standalone `aranea-server` binary (cmd/server) and the in-process
// web SubLauncher used by `aranea web` can share an identical wiring
// path. Anything that touches global resources (database, telemetry,
// background goroutines) lives here so the CLI never has to duplicate
// it.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"arenea/backend/internal/middleware"
	"arenea/backend/internal/repository"
	"arenea/backend/internal/runtime"
	"arenea/backend/internal/service"
	"arenea/backend/internal/telemetry"
	"arenea/backend/internal/transport"
	"arenea/backend/internal/util"
)

// Options controls a Run invocation. Zero values fall back to the same
// environment variables (DB_PATH, HTTP_ADDR) that the legacy main()
// honoured so existing deployments keep working unchanged.
type Options struct {
	// DBPath is the SQLite database file to open. When empty we honour
	// DB_PATH and finally fall back to data/arenea.db.
	DBPath string
	// Addr is the listen address (host:port). Empty → HTTP_ADDR → :8080.
	Addr string
	// TelemetryService is the service name reported through telemetry.
	// Empty → "arenea-backend".
	TelemetryService string
	// SkipTelemetry disables the global telemetry init. Useful for the
	// in-process web SubLauncher which inherits the parent CLI's setup.
	SkipTelemetry bool
	// Ready is closed once the HTTP server is bound and accepting
	// connections. Callers can use it to print a banner or open a
	// browser tab without racing the listener.
	Ready chan<- ListenInfo
	// Logger overrides the default *log.Logger. Pass a discard logger to
	// silence the embedded server when running inside the CLI.
	Logger *log.Logger
}

// ListenInfo is published on Options.Ready as soon as the listener is
// bound, exposing the *actual* address the server runs on (handy when
// callers asked for ":0" to pick a random port).
type ListenInfo struct {
	Addr string
}

// Run boots the backend and blocks until ctx is cancelled or the HTTP
// listener fails. It is the single source of truth for service wiring,
// shared between the standalone server and the embedded web launcher.
//
// The function performs (in order):
//  1. telemetry setup (unless SkipTelemetry)
//  2. SQLite open + migrate
//  3. service / runtime / plugin construction
//  4. middleware chain assembly
//  5. listener bind + http.Serve
//  6. graceful shutdown driven by ctx
//
// All background goroutines (cron runner, skill directory sync) honour
// ctx so a single cancellation drives a clean teardown of everything.
func Run(ctx context.Context, opts Options) error {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	dbPath := firstNonEmpty(opts.DBPath, os.Getenv("DB_PATH"), "data/arenea.db")
	addr := firstNonEmpty(opts.Addr, os.Getenv("HTTP_ADDR"), ":8080")
	if !opts.SkipTelemetry {
		telemetry.Setup(firstNonEmpty(opts.TelemetryService, "arenea-backend"))
	}

	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		return fmt.Errorf("init repository: %w", err)
	}
	defer repo.Close()
	if err = repo.Migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	runtimeAdapter := runtime.NewADKRuntimeAdapter()
	agentSvc := service.NewAgentService(repo)
	teamSvc := service.NewTeamService(repo)
	sessionSvc := service.NewSessionService(repo)
	chatSvc := service.NewChatService(repo, runtimeAdapter)
	auditSvc := service.NewAuditService(repo)
	platformSvc := service.NewPlatformService(repo)
	usageSvc := service.NewUsageService(repo)
	pluginSvc := service.NewPluginService(repo)
	if err = pluginSvc.SyncBuiltins(); err != nil {
		return fmt.Errorf("sync builtin plugins: %w", err)
	}
	runtimeAdapter.SetPluginSource(pluginSvc)
	channelSvc := service.NewChannelService(repo)
	channelSvc.SetRuntimeReloader(runtimeAdapter)
	runtimeAdapter.SetChannelSource(channelSvc)
	if err = runtimeAdapter.ReloadChannels(ctx); err != nil {
		return fmt.Errorf("load channel runtime configs: %w", err)
	}
	skillStorageRoot := util.ResolveSkillStorageRoot()
	logger.Printf("skill storage root: %s", skillStorageRoot)
	skillSvc := service.NewSkillService(repo, runtimeAdapter, skillStorageRoot)
	toolSvc := service.NewToolService(repo)
	cronRunner := service.NewCronRunner(repo, chatSvc)

	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		skillSvc.StartDirectorySync(ctx, 1)
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		cronRunner.Start(ctx, time.Minute)
	}()

	handler := transport.NewHTTPHandler(transport.Services{
		Agent:    agentSvc,
		Team:     teamSvc,
		Session:  sessionSvc,
		Chat:     chatSvc,
		Audit:    auditSvc,
		Platform: platformSvc,
		Usage:    usageSvc,
		Skill:    skillSvc,
		Tool:     toolSvc,
		Plugin:   pluginSvc,
		Channel:  channelSvc,
	})
	wrapped := middleware.RateLimit(60)(
		middleware.BasicAuth(
			middleware.AccessLog(
				middleware.RequestID(
					middleware.CORS(handler),
				),
			),
		),
	)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	if opts.Ready != nil {
		select {
		case opts.Ready <- ListenInfo{Addr: listener.Addr().String()}:
		default:
		}
	}

	srv := &http.Server{Handler: wrapped, ReadHeaderTimeout: 30 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		logger.Printf("server listening on %s", listener.Addr().String())
		serveErr <- srv.Serve(listener)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Printf("shutdown signal received")
	case e := <-serveErr:
		if e != nil && !errors.Is(e, http.ErrServerClosed) {
			runErr = e
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil && runErr == nil {
		runErr = fmt.Errorf("shutdown: %w", shutdownErr)
	}
	background.Wait()
	return runErr
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
