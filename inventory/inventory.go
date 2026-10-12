package main

import (
	"context"
	"e-commerce/inventory/internal/database"
	"e-commerce/inventory/internal/handlers"
	"e-commerce/inventory/internal/services"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		panic(err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_DB"),
	))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	poolErr := pool.Ping(ctx)
	if poolErr != nil {
		log.Fatal(poolErr)
	}

	is := services.NewInventoryService(database.New(pool))
	ih := handlers.NewInventoryHandler(is)

	r := mux.NewRouter()
	r.HandleFunc("/inventory/{id}", ih.GetInventoryItemBySKU).Methods("GET")
	r.HandleFunc("/inventory", ih.AddItemToInventory).Methods("POST")
	log.Fatal(http.ListenAndServe(":9998", r))

}
