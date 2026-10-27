package handlers_test

import (
	"bytes"
	"context"
	"e-commerce/inventory/internal/database"
	"e-commerce/inventory/internal/handlers"
	"e-commerce/inventory/internal/services"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type fakeInventoryService struct {
	createInventoryItemFn        func(context.Context, services.InventoryItemInput) (uuid.UUID, error)
	getInventoryItemBySKUFn      func(context.Context, uuid.UUID) (database.GetInventoryItemBySKURow, error)
	createInventoryReservationFn func(context.Context, services.InventoryReservationInput) (database.InventoryReservation, error)
}

func (f fakeInventoryService) CreateInventoryItem(
	ctx context.Context,
	input services.InventoryItemInput,
) (uuid.UUID, error) {
	if f.createInventoryItemFn == nil {
		panic("chamada inesperada a CreateInventoryItem")
	}
	return f.createInventoryItemFn(ctx, input)
}

func (f fakeInventoryService) GetInventoryItemBySKU(
	ctx context.Context,
	id uuid.UUID,
) (database.GetInventoryItemBySKURow, error) {
	if f.getInventoryItemBySKUFn == nil {
		panic("chamada inesperada a GetInventoryItemBySKU")
	}
	return f.getInventoryItemBySKUFn(ctx, id)
}

func (f fakeInventoryService) CreateInventoryReservation(
	ctx context.Context,
	input services.InventoryReservationInput,
) (database.InventoryReservation, error) {
	if f.createInventoryReservationFn == nil {
		panic("chamada inesperada a CreateInventoryReservation")
	}
	return f.createInventoryReservationFn(ctx, input)
}

func reservationRequest(
	t *testing.T,
	body handlers.CreateInventoryReservationRequest,
) *http.Request {
	t.Helper()

	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(body); err != nil {
		t.Fatalf("codificar request de reserva: %v", err)
	}

	return httptest.NewRequest(
		http.MethodPost,
		"/inventory/reservations",
		&encoded,
	)
}

func TestCreateInventoryReservationSuccess(t *testing.T) {
	requestBody := handlers.CreateInventoryReservationRequest{
		ReservationID: uuid.New(),
		OrderID:       uuid.New(),
		SkuID:         uuid.New(),
		Quantity:      4,
	}
	wantReservation := database.InventoryReservation{
		ReservationID: requestBody.ReservationID,
		OrderID:       requestBody.OrderID,
		SkuID:         requestBody.SkuID,
		Quantity:      requestBody.Quantity,
		Status:        database.InventoryReservationStatusActive,
	}

	var gotInput services.InventoryReservationInput
	serviceCalls := 0
	service := fakeInventoryService{
		createInventoryReservationFn: func(
			_ context.Context,
			input services.InventoryReservationInput,
		) (database.InventoryReservation, error) {
			serviceCalls++
			gotInput = input
			return wantReservation, nil
		},
	}
	handler := handlers.NewInventoryHandler(service)
	recorder := httptest.NewRecorder()

	handler.CreateInventoryReservation(
		recorder,
		reservationRequest(t, requestBody),
	)

	if recorder.Code != http.StatusCreated {
		t.Errorf(
			"status HTTP = %d; esperado %d",
			recorder.Code,
			http.StatusCreated,
		)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf(
			"Content-Type = %q; esperado %q",
			contentType,
			"application/json",
		)
	}
	if serviceCalls != 1 {
		t.Errorf("chamadas ao serviço = %d; esperado 1", serviceCalls)
	}
	wantInput := services.InventoryReservationInput{
		ReservationID: requestBody.ReservationID,
		OrderID:       requestBody.OrderID,
		SkuID:         requestBody.SkuID,
		Quantity:      requestBody.Quantity,
	}
	if gotInput != wantInput {
		t.Errorf("input recebido pelo serviço = %+v; esperado %+v", gotInput, wantInput)
	}

	var response handlers.CreateInventoryReservationResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decodificar resposta de sucesso: %v", err)
	}
	wantResponse := handlers.CreateInventoryReservationResponse{
		ReservationID: wantReservation.ReservationID,
		OrderID:       wantReservation.OrderID,
		SkuID:         wantReservation.SkuID,
		Quantity:      wantReservation.Quantity,
		Status:        string(wantReservation.Status),
	}
	if response != wantResponse {
		t.Errorf("resposta = %+v; esperado %+v", response, wantResponse)
	}
}

func TestCreateInventoryReservationInvalidRequest(t *testing.T) {
	validRequest := handlers.CreateInventoryReservationRequest{
		ReservationID: uuid.New(),
		OrderID:       uuid.New(),
		SkuID:         uuid.New(),
		Quantity:      1,
	}

	tests := []struct {
		name    string
		request func(*testing.T) *http.Request
	}{
		{
			name: "JSON inválido",
			request: func(_ *testing.T) *http.Request {
				return httptest.NewRequest(
					http.MethodPost,
					"/inventory/reservations",
					bytes.NewBufferString("{"),
				)
			},
		},
		{
			name: "reservation_id vazio",
			request: func(t *testing.T) *http.Request {
				body := validRequest
				body.ReservationID = uuid.Nil
				return reservationRequest(t, body)
			},
		},
		{
			name: "order_id vazio",
			request: func(t *testing.T) *http.Request {
				body := validRequest
				body.OrderID = uuid.Nil
				return reservationRequest(t, body)
			},
		},
		{
			name: "sku_id vazio",
			request: func(t *testing.T) *http.Request {
				body := validRequest
				body.SkuID = uuid.Nil
				return reservationRequest(t, body)
			},
		},
		{
			name: "quantidade zero",
			request: func(t *testing.T) *http.Request {
				body := validRequest
				body.Quantity = 0
				return reservationRequest(t, body)
			},
		},
		{
			name: "quantidade negativa",
			request: func(t *testing.T) *http.Request {
				body := validRequest
				body.Quantity = -1
				return reservationRequest(t, body)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// O fake sem função configurada entra em panic se o handler chamar o serviço.
			handler := handlers.NewInventoryHandler(fakeInventoryService{})
			recorder := httptest.NewRecorder()

			handler.CreateInventoryReservation(recorder, test.request(t))

			if recorder.Code != http.StatusBadRequest {
				t.Errorf(
					"status HTTP = %d; esperado %d",
					recorder.Code,
					http.StatusBadRequest,
				)
			}
			if recorder.Body.Len() != 0 {
				t.Errorf("corpo da resposta = %q; esperado vazio", recorder.Body.String())
			}
		})
	}
}

func TestCreateInventoryReservationServiceErrors(t *testing.T) {
	requestBody := handlers.CreateInventoryReservationRequest{
		ReservationID: uuid.New(),
		OrderID:       uuid.New(),
		SkuID:         uuid.New(),
		Quantity:      1,
	}

	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
	}{
		{
			name:       "input inválido no serviço",
			serviceErr: services.ErrInvalidReservationInput,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "conflito de idempotência",
			serviceErr: services.ErrReservationIdempotencyConflict,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "reserva rejeitada",
			serviceErr: services.ErrInventoryReservationRejected,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "erro interno",
			serviceErr: errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serviceCalls := 0
			service := fakeInventoryService{
				createInventoryReservationFn: func(
					_ context.Context,
					_ services.InventoryReservationInput,
				) (database.InventoryReservation, error) {
					serviceCalls++
					return database.InventoryReservation{}, test.serviceErr
				},
			}
			handler := handlers.NewInventoryHandler(service)
			recorder := httptest.NewRecorder()

			handler.CreateInventoryReservation(
				recorder,
				reservationRequest(t, requestBody),
			)

			if recorder.Code != test.wantStatus {
				t.Errorf(
					"status HTTP = %d; esperado %d",
					recorder.Code,
					test.wantStatus,
				)
			}
			if serviceCalls != 1 {
				t.Errorf("chamadas ao serviço = %d; esperado 1", serviceCalls)
			}
			if recorder.Body.Len() != 0 {
				t.Errorf("corpo da resposta = %q; esperado vazio", recorder.Body.String())
			}
		})
	}
}
