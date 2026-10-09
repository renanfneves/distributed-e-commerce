package main

import (
	"context"
	"e-commerce/product/internal/database"
	"e-commerce/product/internal/handlers"
	"e-commerce/product/internal/services"
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

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	poolErr := pool.Ping(ctx)
	if poolErr != nil {
		log.Fatal(poolErr)
	}

	ps := services.NewProductService(pool, database.New(pool))
	ph := handlers.NewProductHandler(ps)

	r := mux.NewRouter()
	r.HandleFunc("/products", ph.ListProducts).Methods("GET")
	r.HandleFunc("/products/{id}", ph.GetProductById).Methods("GET")
	r.HandleFunc("/products", ph.CreateProduct).Methods("POST")
	log.Fatal(http.ListenAndServe(":9999", r), nil)
}
