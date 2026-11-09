package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultReservationTTL      = 15 * time.Minute
	defaultExpirationInterval  = 30 * time.Second
	defaultExpirationBatchSize = int32(100)
	defaultHTTPAddress         = ":9998"
)

type config struct {
	reservationTTL      time.Duration
	expirationInterval  time.Duration
	expirationBatchSize int32
	httpAddress         string
}

func loadConfig() (config, error) {
	reservationTTL, err := positiveDurationFromEnv(
		"INVENTORY_RESERVATION_TTL",
		defaultReservationTTL,
	)
	if err != nil {
		return config{}, err
	}
	if reservationTTL < time.Second {
		return config{}, fmt.Errorf(
			"INVENTORY_RESERVATION_TTL deve ser de pelo menos 1s",
		)
	}

	expirationInterval, err := positiveDurationFromEnv(
		"INVENTORY_EXPIRATION_INTERVAL",
		defaultExpirationInterval,
	)
	if err != nil {
		return config{}, err
	}

	expirationBatchSize, err := positiveInt32FromEnv(
		"INVENTORY_EXPIRATION_BATCH_SIZE",
		defaultExpirationBatchSize,
	)
	if err != nil {
		return config{}, err
	}

	httpAddress := os.Getenv("INVENTORY_HTTP_ADDRESS")
	if httpAddress == "" {
		httpAddress = defaultHTTPAddress
	}

	return config{
		reservationTTL:      reservationTTL,
		expirationInterval:  expirationInterval,
		expirationBatchSize: expirationBatchSize,
		httpAddress:         httpAddress,
	}, nil
}

func positiveDurationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s inválido: %w", name, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s deve ser positivo", name)
	}

	return duration, nil
}

func positiveInt32FromEnv(name string, fallback int32) (int32, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s inválido: %w", name, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s deve ser positivo", name)
	}

	return int32(parsed), nil
}
