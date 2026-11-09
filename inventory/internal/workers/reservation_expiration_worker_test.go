package workers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"e-commerce/inventory/internal/services"
)

type processorCall struct {
	ctx       context.Context
	batchSize int32
}

type fakeExpiredReservationsProcessor struct {
	calls  chan processorCall
	result services.ExpiredReservationsBatchResult
	err    error
}

func (f fakeExpiredReservationsProcessor) ProcessExpiredReservations(
	ctx context.Context,
	batchSize int32,
) (services.ExpiredReservationsBatchResult, error) {
	f.calls <- processorCall{
		ctx:       ctx,
		batchSize: batchSize,
	}

	return f.result, f.err
}

func TestNewReservationExpirationWorkerSuccess(t *testing.T) {
	// Arrange
	processor := fakeExpiredReservationsProcessor{}
	logger := log.New(io.Discard, "", 0)

	// Act
	worker, err := NewReservationExpirationWorker(processor, logger, time.Minute, 100)

	// Assert
	if err != nil {
		t.Fatalf(
			"NewReservationExpirationWorker() erro = %v; esperado nil",
			err,
		)
	}
	if worker.processor != processor {
		t.Errorf("processor do worker não corresponde ao processor informado")
	}

	if worker.logger != logger {
		t.Errorf("logger do worker não corresponde ao logger informado")
	}

	if worker.interval != time.Minute {
		t.Errorf(
			"interval = %v; esperado %v",
			worker.interval,
			time.Minute,
		)
	}

	if worker.batchSize != 100 {
		t.Errorf(
			"batchSize = %d; esperado 100",
			worker.batchSize,
		)
	}
}

func TestNewReservationExpirationWorkerMissingProcessor(t *testing.T) {
	// Arrange
	logger := log.New(io.Discard, "", 0)

	// Act
	worker, err := NewReservationExpirationWorker(nil, logger, time.Minute, 100)

	// Assert
	if !errors.Is(err, ErrExpirationProcessorRequired) {
		t.Errorf(
			"NewReservationExpirationWorker() erro = %v; esperado %v",
			err,
			ErrExpirationProcessorRequired,
		)
	}

	if worker != (ReservationExpirationWorker{}) {
		t.Errorf(
			"worker = %+v; esperado valor zero",
			worker,
		)
	}
}

func TestNewReservationExpirationWorkerMissingLogger(t *testing.T) {
	// Arrange
	processor := fakeExpiredReservationsProcessor{}

	// Act
	worker, err := NewReservationExpirationWorker(processor, nil, time.Minute, 100)

	// Assert
	if !errors.Is(err, ErrExpirationLoggerRequired) {
		t.Errorf(
			"NewReservationExpirationWorker() erro = %v; esperado %v",
			err,
			ErrExpirationLoggerRequired,
		)
	}

	if worker != (ReservationExpirationWorker{}) {
		t.Errorf(
			"worker = %+v; esperado valor zero",
			worker,
		)
	}
}

func TestNewReservationExpirationWorkerInvalidInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{
			name:     "zero",
			interval: 0,
		},
		{
			name:     "negativo",
			interval: -time.Second,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			processor := fakeExpiredReservationsProcessor{}
			logger := log.New(io.Discard, "", 0)

			// Act
			worker, err := NewReservationExpirationWorker(processor, logger, test.interval, 100)

			// Assert
			if !errors.Is(err, ErrInvalidExpirationInterval) {
				t.Errorf(
					"NewReservationExpirationWorker() erro = %v; esperado %v",
					err,
					ErrInvalidExpirationInterval,
				)
			}
			if worker != (ReservationExpirationWorker{}) {
				t.Errorf(
					"worker = %+v; esperado valor zero",
					worker,
				)
			}
		})
	}
}

func TestNewReservationExpirationWorkerInvalidBatchSize(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int32
	}{
		{
			name:      "zero",
			batchSize: 0,
		},
		{
			name:      "negativo",
			batchSize: -1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			processor := fakeExpiredReservationsProcessor{}
			logger := log.New(io.Discard, "", 0)

			// Act
			worker, err := NewReservationExpirationWorker(processor, logger, time.Minute, test.batchSize)

			// Assert
			if !errors.Is(err, ErrInvalidExpirationBatchSize) {
				t.Errorf(
					"NewReservationExpirationWorker() erro = %v; esperado %v",
					err,
					ErrInvalidExpirationBatchSize,
				)
			}
			if worker != (ReservationExpirationWorker{}) {
				t.Errorf(
					"worker = %+v; esperado valor zero",
					worker,
				)
			}
		})
	}
}

func TestReservationExpirationWorkerRunProcessesImmediately(
	t *testing.T,
) {
	// Arrange
	processor := fakeExpiredReservationsProcessor{
		calls: make(chan processorCall, 1),
	}
	logger := log.New(io.Discard, "", 0)
	worker, err := NewReservationExpirationWorker(processor, logger, time.Hour, 25)
	if err != nil {
		t.Fatalf(
			"NewReservationExpirationWorker() erro = %v; esperado nil",
			err,
		)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	// Act
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()
	// Assert
	select {
	case call := <-processor.calls:
		if call.batchSize != 25 {
			t.Errorf(
				"batchSize recebido = %d; esperado 25",
				call.batchSize,
			)
		}

		if call.ctx != ctx {
			t.Errorf("contexto recebido não corresponde ao contexto informado")
		}

	case <-time.After(time.Second):
		t.Fatal("tempo esgotado aguardando processamento imediato")
	}
	cancel()
	select {
	case <-done:

	case <-time.After(time.Second):
		t.Fatal("worker não terminou após cancelamento")
	}
}

func TestReservationExpirationWorkerRunWithCanceledContext(t *testing.T) {
	// Arrange
	processor := fakeExpiredReservationsProcessor{
		calls: make(chan processorCall, 1),
	}
	worker, err := NewReservationExpirationWorker(
		processor,
		log.New(io.Discard, "", 0),
		time.Hour,
		25,
	)
	if err != nil {
		t.Fatalf("NewReservationExpirationWorker() erro = %v; esperado nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	worker.Run(ctx)

	// Assert
	select {
	case call := <-processor.calls:
		t.Errorf("processor chamado com batchSize=%d; esperado nenhuma chamada", call.batchSize)
	default:
	}
}

func TestReservationExpirationWorkerRunProcessesOnTicksAndStops(t *testing.T) {
	// Arrange
	processor := fakeExpiredReservationsProcessor{
		calls: make(chan processorCall, 4),
		err:   errors.New("temporary database error"),
	}
	worker, err := NewReservationExpirationWorker(
		processor,
		log.New(io.Discard, "", 0),
		20*time.Millisecond,
		10,
	)
	if err != nil {
		t.Fatalf("NewReservationExpirationWorker() erro = %v; esperado nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	// Act
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	// Assert
	for callNumber := 1; callNumber <= 2; callNumber++ {
		select {
		case call := <-processor.calls:
			if call.batchSize != 10 {
				t.Errorf("chamada %d batchSize = %d; esperado 10", callNumber, call.batchSize)
			}
		case <-time.After(time.Second):
			cancel()
			t.Fatalf("tempo esgotado aguardando chamada %d", callNumber)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker não terminou após cancelamento")
	}
}

func TestReservationExpirationWorkerProcessBatchLogsResult(t *testing.T) {
	tests := []struct {
		name         string
		result       services.ExpiredReservationsBatchResult
		processorErr error
		wantLog      string
	}{
		{
			name:    "lote vazio não gera log",
			wantLog: "",
		},
		{
			name: "sucesso",
			result: services.ExpiredReservationsBatchResult{
				Selected:  3,
				Processed: 3,
			},
			wantLog: "reservas expiradas processadas: selected=3 processed=3 failed=0",
		},
		{
			name: "erro parcial",
			result: services.ExpiredReservationsBatchResult{
				Selected:  3,
				Processed: 2,
				Failed:    1,
			},
			processorErr: errors.New("release failed"),
			wantLog:      "erro ao processar reservas expiradas: selected=3 processed=2 failed=1 error=release failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			var logs bytes.Buffer
			processor := fakeExpiredReservationsProcessor{
				calls:  make(chan processorCall, 1),
				result: test.result,
				err:    test.processorErr,
			}
			worker, err := NewReservationExpirationWorker(
				processor,
				log.New(&logs, "", 0),
				time.Minute,
				10,
			)
			if err != nil {
				t.Fatalf("NewReservationExpirationWorker() erro = %v; esperado nil", err)
			}

			// Act
			worker.processBatch(context.Background())

			// Assert
			gotLog := strings.TrimSpace(logs.String())
			if gotLog != test.wantLog {
				t.Errorf("log = %q; esperado %q", gotLog, test.wantLog)
			}
		})
	}
}
