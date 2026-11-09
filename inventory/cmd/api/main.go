package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"e-commerce/inventory/internal/handlers"
	"e-commerce/inventory/internal/services"
	"e-commerce/inventory/internal/workers"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const (
	databasePingTimeout = 5 * time.Second
	shutdownTimeout     = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("carregar .env: %w", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	appCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	pool, err := pgxpool.New(appCtx, databaseConnectionString())
	if err != nil {
		return fmt.Errorf("criar pool PostgreSQL: %w", err)
	}
	defer pool.Close()

	pingCtx, cancelPing := context.WithTimeout(appCtx, databasePingTimeout)
	err = pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		return fmt.Errorf("conectar ao PostgreSQL: %w", err)
	}

	inventoryService := services.NewInventoryService(pool, cfg.reservationTTL)
	inventoryHandler := handlers.NewInventoryHandler(inventoryService)
	expirationWorker, err := workers.NewReservationExpirationWorker(
		inventoryService,
		log.Default(),
		cfg.expirationInterval,
		cfg.expirationBatchSize,
	)
	if err != nil {
		return fmt.Errorf("configurar worker de expiração: %w", err)
	}

	router := mux.NewRouter()
	router.HandleFunc("/inventory", inventoryHandler.AddItemToInventory).Methods(http.MethodPost)
	router.HandleFunc("/inventory/reservations", inventoryHandler.CreateInventoryReservation).Methods(http.MethodPost)
	router.HandleFunc("/inventory/reservations/{id}/release", inventoryHandler.ReleaseInventoryReservation).Methods(http.MethodPost)
	router.HandleFunc("/inventory/reservations/{id}/confirm", inventoryHandler.ConfirmInventoryReservation).Methods(http.MethodPost)
	router.HandleFunc("/inventory/{id}", inventoryHandler.GetInventoryItemBySKU).Methods(http.MethodGet)

	server := &http.Server{
		Addr:              cfg.httpAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		expirationWorker.Run(appCtx)
	}()

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	var serveErr error
	select {
	case <-appCtx.Done():
	case serveErr = <-serverErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = fmt.Errorf("servidor HTTP: %w", serveErr)
		} else {
			serveErr = nil
		}
	}

	stop()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	shutdownErr := server.Shutdown(shutdownCtx)
	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
		if shutdownErr == nil {
			shutdownErr = fmt.Errorf("encerrar worker: %w", shutdownCtx.Err())
		}
	}

	if serveErr != nil {
		return serveErr
	}
	if shutdownErr != nil {
		return fmt.Errorf("encerramento gracioso: %w", shutdownErr)
	}
	return nil
}

func databaseConnectionString() string {
	if databaseURI := os.Getenv("DATABASE_URI"); databaseURI != "" {
		return databaseURI
	}

	return fmt.Sprintf(
		"user=%s password=%s host=%s port=%s dbname=%s",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_DB"),
	)
}
