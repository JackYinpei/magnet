package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	apphttp "magnet-player/internal/http"
	"magnet-player/internal/config"
	"magnet-player/internal/downloader"
	"magnet-player/internal/repository/sqlite"
	"magnet-player/internal/service"
	"magnet-player/internal/storage"
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	if strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		logger.Fatalf("auth jwt secret is required")
	}
	if strings.TrimSpace(cfg.Auth.RegisterPassword) == "" {
		logger.Fatalf("auth registration password is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sqlite.Open(cfg.Database.Path)
	if err != nil {
		logger.Fatalf("open database: %v", err)
	}
	defer db.Close()

	taskRepo := sqlite.NewTaskRepository(db)
	fileRepo := sqlite.NewTaskFileRepository(db)
	userRepo := sqlite.NewUserRepository(db)

	if err := taskRepo.Init(ctx); err != nil {
		logger.Fatalf("init task repository: %v", err)
	}
	if err := fileRepo.Init(ctx); err != nil {
		logger.Fatalf("init file repository: %v", err)
	}
	if err := userRepo.Init(ctx); err != nil {
		logger.Fatalf("init user repository: %v", err)
	}

	taskService := service.NewTaskService(taskRepo, fileRepo)
	userService := service.NewUserService(userRepo, cfg.Auth.RegisterPassword)

	storageSvc, err := buildStorage(ctx, cfg, logger)
	if err != nil {
		logger.Fatalf("setup storage: %v", err)
	}

	manager := downloader.NewManager(downloader.Config{
		DownloadRoot:   cfg.Download.DataDir,
		MaxConcurrent:  3,
		StatusInterval: 2 * time.Second,
		UploadOptions: storage.UploadOptions{
			Bucket:    cfg.Storage.Bucket,
			KeyPrefix: cfg.Storage.KeyPrefix,
		},
		Logger: logger,
	}, taskService, storageSvc)

	if err := manager.Start(ctx); err != nil {
		logger.Fatalf("start manager: %v", err)
	}
	if err := manager.Resume(ctx); err != nil {
		logger.Warnf("resume tasks: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	handler := apphttp.NewHandler(
		taskService,
		manager,
		storageSvc,
		cfg.Storage.Bucket,
		cfg.Download.DataDir,
		userService,
		cfg.Auth.JWTSecret,
		time.Duration(cfg.Auth.TokenTTLMinutes)*time.Minute,
	)
	handler.RegisterRoutes(router)

	srv := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: router,
	}

	go func() {
		logger.Infof("listening on %s", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warnf("http shutdown: %v", err)
	}
	manager.Shutdown()

	logger.Info("bye")
}

func buildStorage(ctx context.Context, cfg config.Config, logger *logrus.Logger) (storage.Service, error) {
	if cfg.Storage.Bucket == "" {
		return nil, fmt.Errorf("storage bucket is required")
	}

	loadOpts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.Storage.Region),
	}
	if cfg.AWS.Profile != "" {
		loadOpts = append(loadOpts, awscfg.WithSharedConfigProfile(cfg.AWS.Profile))
	}

	awsCfg, err := awscfg.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Storage.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Storage.Endpoint)
			o.UsePathStyle = true
		}
	})
	logger.Infof("using s3 bucket %s (region %s)", cfg.Storage.Bucket, cfg.Storage.Region)
	return storage.NewS3Service(client), nil
}
