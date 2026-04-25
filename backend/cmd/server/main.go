package main

import (
	"context"
	"log"
	"net/http"
	"os"

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
	skillStorageRoot := util.ResolveSkillStorageRoot()
	log.Printf("skill storage root: %s", skillStorageRoot)
	skillSvc := service.NewSkillService(repo, runtimeAdapter, skillStorageRoot)
	toolSvc := service.NewToolService(repo)
	go skillSvc.StartDirectorySync(context.Background(), 1)

	handler := transport.NewHTTPHandler(agentSvc, teamSvc, sessionSvc, chatSvc, auditSvc, platformSvc, usageSvc, skillSvc, toolSvc, pluginSvc)
	handler = middleware.CORS(handler)
	handler = middleware.RequestID(handler)
	handler = middleware.AccessLog(handler)
	handler = middleware.BasicAuth(handler)
	handler = middleware.RateLimit(60)(handler)

	log.Printf("server listening on %s", addr)
	if err = http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func getEnv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
