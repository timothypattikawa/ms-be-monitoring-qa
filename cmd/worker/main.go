package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs/db"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/monitoring"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	database, err := db.NewPostgreDB(*configs.GetConfig(), false)
	if err != nil {
		log.Fatal("database unavailable")
	}
	worker := monitoring.Worker{Repo: repository.NewMonitoring(database), Connector: monitoring.NewConnector()}
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
