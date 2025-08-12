package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"dag-project/dag"
	"dag-project/db"
	"dag-project/handlers"
	"dag-project/logger"
	"dag-project/repository"
	"dag-project/routers"
)

func main() {
	// Load config
	viper.SetConfigFile("config/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Config file error:", err)
		os.Exit(1)
	}

	appLogFile := viper.GetString("log.app_log_file")
	logLevel := viper.GetString("log.level")

	if err := logger.InitLogger(appLogFile, logLevel); err != nil {
		fmt.Println("Failed to initialize logger:", err)
		os.Exit(1)
	}

	logger.Logger.Info("Starting DAG server...")

	// Connect to LevelDB
	leveldbPath := viper.GetString("leveldb.path")
	ldb, err := db.NewLevelDB(leveldbPath)
	if err != nil {
		logger.Logger.Fatal("Failed to open leveldb", zap.Error(err))
	}
	defer ldb.Close()

	// Initialize repository
	nodeRepo := repository.NewNodeRepository(ldb)

	// Initialize DAG service with repository
	d := dag.NewDAG(nodeRepo)

	// Initialize HTTP handlers
	h := handlers.NewHandler(d)

	// Setup router
	r := mux.NewRouter()
	routers.RegisterRoutes(r, h)

	// HTTP Server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", viper.GetInt("server.port")),
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			logger.Logger.Info("Server stopped", zap.Error(err))
		}
	}()

	logger.Logger.Info("Server running on port", zap.Int("port", viper.GetInt("server.port")))

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Logger.Info("Shutdown signal received, exiting...")
	srv.Close()
}
