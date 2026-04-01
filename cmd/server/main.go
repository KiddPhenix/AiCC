package main

import (
	"log"
	"net/http"
	"os"

	"aihelper/internal/ai/extractor"
	"aihelper/internal/app/capture"
	"aihelper/internal/config"
	"aihelper/internal/httpapi"
	"aihelper/internal/repository"
	"aihelper/internal/service"
	"aihelper/internal/source/localjson"
	"aihelper/internal/storage"
)

func main() {
	dbPath := getenv("AIHELPER_DB_PATH", "data/aihelper.db")
	addr := getenv("AIHELPER_ADDR", ":8090")
	configPath := getenv("AIHELPER_CONFIG_PATH", "config/app.json")

	db, err := storage.OpenSQLite(dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	repo := repository.NewSQLiteSolutionRepository(db)
	solutionSvc := service.NewSolutionService(repo)
	importSvc := service.NewImportService(solutionSvc, localjson.New())
	captureApp := capture.NewService(db)
	captureSvc := service.NewCaptureService(captureApp)
	analysisSvc := service.NewCaptureAnalysisService(captureApp, solutionSvc, extractor.New(cfg.AI))
	settingsSvc := service.NewSettingsService(configPath)
	handler := httpapi.NewHandler(solutionSvc, importSvc, captureSvc, analysisSvc, settingsSvc)
	handler.SetAnalysisServiceFactory(func() *service.CaptureAnalysisService {
		freshCfg, err := config.Load(configPath)
		if err != nil {
			log.Printf("reload config for analysis service: %v", err)
			return analysisSvc
		}
		return service.NewCaptureAnalysisService(captureApp, solutionSvc, extractor.New(freshCfg.AI))
	})

	log.Printf("AI Helper listening on %s", addr)
	if err := http.ListenAndServe(addr, handler.Routes()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
