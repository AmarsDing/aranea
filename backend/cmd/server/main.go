package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"arenea/backend/internal/middleware"
	"arenea/backend/internal/repository"
	"arenea/backend/internal/runtime"
	"arenea/backend/internal/service"
	"arenea/backend/internal/telemetry"
	"arenea/backend/internal/transport"
	"arenea/backend/internal/util"
)

func main() {
	dbPath := getEnv("DB_PATH", "data/arenea.db")
	addr := getEnv("HTTP_ADDR", ":8080")
	telemetry.Setup("arenea-backend")
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		log.Fatalf("init repository failed: %v", err)
	}
	defer repo.Close()

	if err = repo.Migrate(); err != nil {
		log.Fatalf("migrate failed: %v", err)
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
		log.Fatalf("sync builtin plugins failed: %v", err)
	}
	runtimeAdapter.SetPluginSource(pluginSvc)
	channelSvc := service.NewChannelService(repo)
	channelSvc.SetRuntimeReloader(runtimeAdapter)
	runtimeAdapter.SetChannelSource(channelSvc)
	if err = runtimeAdapter.ReloadChannels(rootCtx); err != nil {
		log.Fatalf("load channel runtime configs failed: %v", err)
	}
	skillStorageRoot := util.ResolveSkillStorageRoot()
	log.Printf("skill storage root: %s", skillStorageRoot)
	skillSvc := service.NewSkillService(repo, runtimeAdapter, skillStorageRoot)
	toolSvc := service.NewToolService(repo)
	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		skillSvc.StartDirectorySync(rootCtx, 1)
	}()

	handler := transport.NewHTTPHandler(agentSvc, teamSvc, sessionSvc, chatSvc, auditSvc, platformSvc, usageSvc, skillSvc, toolSvc, pluginSvc, channelSvc)
	handler = middleware.CORS(handler)
	handler = middleware.RequestID(handler)
	handler = middleware.AccessLog(handler)
	handler = middleware.BasicAuth(handler)
	handler = middleware.RateLimit(60)(handler)

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("server listening on %s", addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-rootCtx.Done():
		log.Printf("shutdown signal received")
	case err = <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	if err != nil {
		log.Fatalf("server exited: %v", err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err = server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}
	stop()
	background.Wait()
}

func getEnv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
