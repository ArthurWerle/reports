package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/ArthurWerle/reports/internal/config"
	"github.com/ArthurWerle/reports/internal/handler"
	"github.com/ArthurWerle/reports/internal/migrations"
	"github.com/ArthurWerle/reports/internal/repository"
	"github.com/ArthurWerle/reports/internal/service"
	"github.com/ArthurWerle/reports/internal/templates"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	logger := setupLogger(cfg.Log.Level)
	logger.Info("starting reports service")

	db, err := setupDatabase(cfg, logger)
	if err != nil {
		logger.Error("failed to setup database", "error", err)
		os.Exit(1)
	}

	if err := migrations.RunMigrations(db, logger); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	reportingLoc, err := time.LoadLocation(cfg.Reporting.Timezone)
	if err != nil {
		logger.Error("invalid REPORTING_TIMEZONE", "timezone", cfg.Reporting.Timezone, "error", err)
		os.Exit(1)
	}

	reportRepo := repository.NewReportRepository(db)
	execRepo := repository.NewExecutionRepository(db)

	txClient := service.NewTransactionsClient(cfg.Services.TransactionsBaseURL)
	insightsClient := service.NewInsightsClient(cfg.Services.AIInternalBaseURL)
	mailer := service.NewMailer(cfg.SMTP)
	enq := service.NewEventsClient(cfg.Services.EventsBaseURL)
	generator := service.NewGenerator(execRepo, reportRepo, txClient, insightsClient, mailer, *cfg, reportingLoc, logger)

	reportHandler := handler.NewReportHandler(reportRepo, execRepo, enq, cfg.Services.CallbackBaseURL, reportingLoc, logger)
	executionHandler := handler.NewExecutionHandler(execRepo, logger)
	jobHandler := handler.NewJobHandler(execRepo, generator, logger)

	router := setupRouter(cfg, reportHandler, executionHandler, jobHandler, logger)

	// Scheduler runs in the background until the shutdown context is cancelled.
	schedulerCtx, cancelScheduler := context.WithCancel(context.Background())
	if cfg.Scheduler.Enabled {
		scheduler := service.NewScheduler(reportRepo, execRepo, enq, cfg.Services.CallbackBaseURL, reportingLoc, logger)
		go scheduler.Run(schedulerCtx)
	} else {
		logger.Info("scheduler disabled (SCHEDULER_ENABLED=false)")
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Server.Port),
		Handler: router,
	}

	go func() {
		logger.Info("starting HTTP server", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")
	cancelScheduler()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}

	logger.Info("server exited")
}

func setupRouter(
	cfg *config.Config,
	reportHandler *handler.ReportHandler,
	executionHandler *handler.ExecutionHandler,
	jobHandler *handler.JobHandler,
	logger *slog.Logger,
) *gin.Engine {
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	router.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	router.GET("/", func(c *gin.Context) {
		html, err := templates.UIHTML()
		if err != nil {
			logger.Error("failed to load ui", "error", err)
			c.String(http.StatusInternalServerError, "failed to load ui")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", html)
	})

	v1 := router.Group("/api/v1")
	{
		v1.GET("/reports", reportHandler.List)
		v1.POST("/reports", reportHandler.Create)
		v1.PUT("/reports/:id", reportHandler.Update)
		v1.DELETE("/reports/:id", reportHandler.Delete)
		v1.POST("/reports/:id/run", reportHandler.Run)

		v1.GET("/executions", executionHandler.List)
		v1.GET("/executions/:id/html", executionHandler.HTML)
		v1.GET("/executions/:id/charts/:name", executionHandler.Chart)

		v1.GET("/jobs/generate", jobHandler.Generate)
	}

	return router
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: logLevel}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func setupDatabase(cfg *config.Config, logger *slog.Logger) (*gorm.DB, error) {
	gormLogLevel := gormLogger.Silent
	if cfg.Log.Level == "debug" {
		gormLogLevel = gormLogger.Info
	}

	gormCfg := &gorm.Config{
		Logger:         gormLogger.Default.LogMode(gormLogLevel),
		TranslateError: true,
	}

	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	logger.Info("database connection established")
	return db, nil
}
