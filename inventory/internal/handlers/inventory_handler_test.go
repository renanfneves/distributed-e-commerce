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
	"github.com/gorilla/mux"
)

type fakeInventoryService struct {
	createInventoryItemFn         func(context.Context, services.InventoryItemInput) (uuid.UUID, error)
	getInventoryItemBySKUFn       func(context.Context, uuid.UUID) (database.GetInventoryItemBySKURow, error)
	createInventoryReservationFn  func(context.Context, services.InventoryReservationInput) (database.InventoryReservation, error)
	releaseInventoryReservationFn func(context.Context, uuid.UUID) (database.InventoryReservation, error)
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

func (f fakeInventoryService) ReleaseInventoryReservation(
	ctx context.Context,
	reservationID uuid.UUID,
) (database.InventoryReservation, error) {
	if f.releaseInventoryReservationFn == nil {
		panic("chamada inesperada a ReleaseInventoryReservation")
	}
	return f.releaseInventoryReservationFn(ctx, reservationID)
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

func releaseReservationRequest(reservationID string) *http.Request {
	request := httptest.NewRequest(
		http.MethodPost,
		"/inventory/reservations/"+reservationID+"/release",
		nil,
	)
	return mux.SetURLVars(request, map[string]string{
		"id": reservationID,
	})
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

	var response handlers.InventoryReservationResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decodificar resposta de sucesso: %v", err)
	}
	wantResponse := handlers.InventoryReservationResponse{
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

func TestReleaseInventoryReservationSuccess(t *testing.T) {
	reservationID := uuid.New()
	wantReservation := database.InventoryReservation{
		ReservationID: reservationID,
		OrderID:       uuid.New(),
		SkuID:         uuid.New(),
		Quantity:      4,
		Status:        database.InventoryReservationStatusReleased,
	}

	serviceCalls := 0
	var gotReservationID uuid.UUID
	service := fakeInventoryService{
		releaseInventoryReservationFn: func(
			_ context.Context,
			gotID uuid.UUID,
		) (database.InventoryReservation, error) {
			serviceCalls++
			gotReservationID = gotID
			return wantReservation, nil
		},
	}
	handler := handlers.NewInventoryHandler(service)
	recorder := httptest.NewRecorder()

	handler.ReleaseInventoryReservation(
		recorder,
		releaseReservationRequest(reservationID.String()),
	)

	if recorder.Code != http.StatusOK {
		t.Errorf(
			"status HTTP = %d; esperado %d",
			recorder.Code,
			http.StatusOK,
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
	if gotReservationID != reservationID {
		t.Errorf(
			"reservation_id recebido pelo serviço = %v; esperado %v",
			gotReservationID,
			reservationID,
		)
	}

	var response handlers.InventoryReservationResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decodificar resposta de liberação: %v", err)
	}
	wantResponse := handlers.InventoryReservationResponse{
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

func TestReleaseInventoryReservationInvalidID(t *testing.T) {
	// O fake sem função configurada entra em panic se o handler chamar o serviço.
	handler := handlers.NewInventoryHandler(fakeInventoryService{})
	recorder := httptest.NewRecorder()

	handler.ReleaseInventoryReservation(
		recorder,
		releaseReservationRequest("id-invalido"),
	)

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
}

func TestReleaseInventoryReservationServiceErrors(t *testing.T) {
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
			name:       "reserva inexistente",
			serviceErr: services.ErrInventoryReservationNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "conflito de transição",
			serviceErr: services.ErrInventoryReservationTransitionConflict,
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
			reservationID := uuid.New()
			serviceCalls := 0
			var gotReservationID uuid.UUID
			service := fakeInventoryService{
				releaseInventoryReservationFn: func(
					_ context.Context,
					gotID uuid.UUID,
				) (database.InventoryReservation, error) {
					serviceCalls++
					gotReservationID = gotID
					return database.InventoryReservation{}, test.serviceErr
				},
			}
			handler := handlers.NewInventoryHandler(service)
			recorder := httptest.NewRecorder()

			handler.ReleaseInventoryReservation(
				recorder,
				releaseReservationRequest(reservationID.String()),
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
			if gotReservationID != reservationID {
				t.Errorf(
					"reservation_id recebido pelo serviço = %v; esperado %v",
					gotReservationID,
					reservationID,
				)
			}
			if recorder.Body.Len() != 0 {
				t.Errorf("corpo da resposta = %q; esperado vazio", recorder.Body.String())
			}
		})
	}
}
