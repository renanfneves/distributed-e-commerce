package main

import (
	"strings"
	"testing"
	"time"
)

func TestPositiveDurationFromEnv(t *testing.T) {
	const name = "TEST_POSITIVE_DURATION"
	tests := []struct {
		name      string
		value     string
		fallback  time.Duration
		want      time.Duration
		wantError bool
	}{
		{name: "usa fallback", fallback: time.Minute, want: time.Minute},
		{name: "valor válido", value: "45s", fallback: time.Minute, want: 45 * time.Second},
		{name: "formato inválido", value: "invalid", wantError: true},
		{name: "zero", value: "0s", wantError: true},
		{name: "negativo", value: "-1s", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			t.Setenv(name, test.value)

			// Act
			got, err := positiveDurationFromEnv(name, test.fallback)

			// Assert
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), name) {
					t.Errorf("erro = %v; esperado erro contendo %q", err, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("positiveDurationFromEnv() erro = %v; esperado nil", err)
			}
			if got != test.want {
				t.Errorf("duration = %v; esperado %v", got, test.want)
			}
		})
	}
}

func TestPositiveInt32FromEnv(t *testing.T) {
	const name = "TEST_POSITIVE_INT32"
	tests := []struct {
		name      string
		value     string
		fallback  int32
		want      int32
		wantError bool
	}{
		{name: "usa fallback", fallback: 100, want: 100},
		{name: "valor válido", value: "250", fallback: 100, want: 250},
		{name: "formato inválido", value: "invalid", wantError: true},
		{name: "acima de int32", value: "2147483648", wantError: true},
		{name: "zero", value: "0", wantError: true},
		{name: "negativo", value: "-1", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			t.Setenv(name, test.value)

			// Act
			got, err := positiveInt32FromEnv(name, test.fallback)

			// Assert
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), name) {
					t.Errorf("erro = %v; esperado erro contendo %q", err, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("positiveInt32FromEnv() erro = %v; esperado nil", err)
			}
			if got != test.want {
				t.Errorf("valor = %d; esperado %d", got, test.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Arrange
	t.Setenv("INVENTORY_RESERVATION_TTL", "20m")
	t.Setenv("INVENTORY_EXPIRATION_INTERVAL", "45s")
	t.Setenv("INVENTORY_EXPIRATION_BATCH_SIZE", "250")
	t.Setenv("INVENTORY_HTTP_ADDRESS", ":8080")

	// Act
	got, err := loadConfig()

	// Assert
	if err != nil {
		t.Fatalf("loadConfig() erro = %v; esperado nil", err)
	}
	want := config{
		reservationTTL:      20 * time.Minute,
		expirationInterval:  45 * time.Second,
		expirationBatchSize: 250,
		httpAddress:         ":8080",
	}
	if got != want {
		t.Errorf("config = %+v; esperado %+v", got, want)
	}
}

func TestLoadConfigRejectsSubsecondReservationTTL(t *testing.T) {
	// Arrange
	t.Setenv("INVENTORY_RESERVATION_TTL", "500ms")

	// Act
	_, err := loadConfig()

	// Assert
	if err == nil || !strings.Contains(err.Error(), "INVENTORY_RESERVATION_TTL") {
		t.Errorf(
			"loadConfig() erro = %v; esperado erro sobre INVENTORY_RESERVATION_TTL",
			err,
		)
	}
}

func TestDatabaseConnectionString(t *testing.T) {
	t.Run("prefere DATABASE_URI", func(t *testing.T) {
		// Arrange
		const databaseURI = "postgres://user:password@database:5432/inventory"
		t.Setenv("DATABASE_URI", databaseURI)
		t.Setenv("POSTGRES_HOST", "ignored")

		// Act
		got := databaseConnectionString()

		// Assert
		if got != databaseURI {
			t.Errorf("connection string = %q; esperado %q", got, databaseURI)
		}
	})

	t.Run("monta conexão pelas variáveis PostgreSQL", func(t *testing.T) {
		// Arrange
		t.Setenv("DATABASE_URI", "")
		t.Setenv("POSTGRES_USER", "inventory_user")
		t.Setenv("POSTGRES_PASSWORD", "inventory_password")
		t.Setenv("POSTGRES_HOST", "database")
		t.Setenv("POSTGRES_PORT", "5432")
		t.Setenv("POSTGRES_DB", "inventory")

		want := "user=inventory_user password=inventory_password host=database port=5432 dbname=inventory"

		// Act
		got := databaseConnectionString()

		// Assert
		if got != want {
			t.Errorf("connection string = %q; esperado %q", got, want)
		}
	})
}
