package workers

import (
	"context"
	"errors"
	"log"
	"time"

	"e-commerce/inventory/internal/services"
)

var (
	ErrExpirationProcessorRequired = errors.New("expiration processor is required")
	ErrExpirationLoggerRequired    = errors.New("expiration worker logger is required")
	ErrInvalidExpirationInterval   = errors.New("expiration worker interval must be greater than zero")
	ErrInvalidExpirationBatchSize  = errors.New("expiration worker batch size must be greater than zero")
)

type expiredReservationsProcessor interface {
	ProcessExpiredReservations(
		ctx context.Context,
		batchSize int32,
	) (services.ExpiredReservationsBatchResult, error)
}

type ReservationExpirationWorker struct {
	processor expiredReservationsProcessor
	logger    *log.Logger
	interval  time.Duration
	batchSize int32
}

func NewReservationExpirationWorker(
	processor expiredReservationsProcessor,
	logger *log.Logger,
	interval time.Duration,
	batchSize int32,
) (ReservationExpirationWorker, error) {
	if processor == nil {
		return ReservationExpirationWorker{},
			ErrExpirationProcessorRequired
	}

	if logger == nil {
		return ReservationExpirationWorker{},
			ErrExpirationLoggerRequired
	}

	if interval <= 0 {
		return ReservationExpirationWorker{},
			ErrInvalidExpirationInterval
	}

	if batchSize <= 0 {
		return ReservationExpirationWorker{},
			ErrInvalidExpirationBatchSize
	}

	return ReservationExpirationWorker{
		processor: processor,
		logger:    logger,
		interval:  interval,
		batchSize: batchSize,
	}, nil
}

func (w ReservationExpirationWorker) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.processBatch(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			w.processBatch(ctx)
		}
	}

}

func (w ReservationExpirationWorker) processBatch(
	ctx context.Context,
) {
	result, err := w.processor.ProcessExpiredReservations(
		ctx,
		w.batchSize,
	)

	if err != nil {
		w.logger.Printf(
			"erro ao processar reservas expiradas: selected=%d processed=%d failed=%d error=%v",
			result.Selected,
			result.Processed,
			result.Failed,
			err,
		)

		return
	}

	if result.Selected == 0 {
		return
	}

	w.logger.Printf(
		"reservas expiradas processadas: selected=%d processed=%d failed=%d",
		result.Selected,
		result.Processed,
		result.Failed,
	)
}
