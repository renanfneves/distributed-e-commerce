package handlers

import (
	"context"
	"e-commerce/inventory/internal/database"
	"e-commerce/inventory/internal/services"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if body == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("erro ao escrever resposta JSON: %v", err)
	}
}

type inventoryService interface {
	CreateInventoryItem(
		ctx context.Context,
		input services.InventoryItemInput,
	) (uuid.UUID, error)

	GetInventoryItemBySKU(
		ctx context.Context,
		id uuid.UUID,
	) (database.GetInventoryItemBySKURow, error)

	CreateInventoryReservation(
		ctx context.Context,
		input services.InventoryReservationInput,
	) (database.InventoryReservation, error)
}

type InventoryHandler struct {
	service inventoryService
}

func NewInventoryHandler(service inventoryService) InventoryHandler {
	return InventoryHandler{
		service: service,
	}
}

type AddItemRequest struct {
	SkuID          uuid.UUID `json:"sku_id"`
	QuantityOnHand int64     `json:"quantity_on_hand"`
}

type AddItemResponse struct {
	InventoryItemID uuid.UUID `json:"id"`
}

func (h InventoryHandler) AddItemToInventory(w http.ResponseWriter, r *http.Request) {
	var body AddItemRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SkuID == uuid.Nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	id, svcErr := h.service.CreateInventoryItem(r.Context(), services.InventoryItemInput{
		SkuID:          body.SkuID,
		QuantityOnHand: body.QuantityOnHand,
	})
	if svcErr != nil {
		log.Printf("erro ao adicionar item ao inventário: %v", svcErr)
		if errors.Is(svcErr, services.ErrInvalidQuantity) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"message": services.ErrInvalidQuantity.Error(),
			})

			return
		}
		if errors.Is(svcErr, services.ErrInventoryItemAlreadyExists) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"message": "já existe um item de inventário para este SKU",
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated,
		AddItemResponse{
			InventoryItemID: id,
		})

}

func (h InventoryHandler) GetInventoryItemBySKU(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	skuID, err := uuid.Parse(vars["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	item, svcErr := h.service.GetInventoryItemBySKU(r.Context(), skuID)
	if svcErr != nil {
		log.Printf("erro buscar item no inventário: %v", svcErr)

		if errors.Is(svcErr, services.ErrInventoryItemNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, item)
}

type CreateInventoryReservationRequest struct {
	ReservationID uuid.UUID `json:"reservation_id"`
	OrderID       uuid.UUID `json:"order_id"`
	SkuID         uuid.UUID `json:"sku_id"`
	Quantity      int64     `json:"quantity"`
}

type CreateInventoryReservationResponse struct {
	ReservationID uuid.UUID `json:"reservation_id"`
	OrderID       uuid.UUID `json:"order_id"`
	SkuID         uuid.UUID `json:"sku_id"`
	Quantity      int64     `json:"quantity"`
	Status        string    `json:"status"`
}

func (h InventoryHandler) CreateInventoryReservation(w http.ResponseWriter, r *http.Request) {
	var body CreateInventoryReservationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		body.ReservationID == uuid.Nil ||
		body.OrderID == uuid.Nil ||
		body.SkuID == uuid.Nil ||
		body.Quantity <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	response, err := h.service.CreateInventoryReservation(r.Context(), services.InventoryReservationInput{
		ReservationID: body.ReservationID,
		OrderID:       body.OrderID,
		SkuID:         body.SkuID,
		Quantity:      body.Quantity,
	})
	if err != nil {
		if errors.Is(err, services.ErrInvalidReservationInput) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if errors.Is(err, services.ErrReservationIdempotencyConflict) ||
			errors.Is(err, services.ErrInventoryReservationRejected) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("erro ao criar reserva de inventário: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, CreateInventoryReservationResponse{
		ReservationID: response.ReservationID,
		OrderID:       response.OrderID,
		SkuID:         response.SkuID,
		Quantity:      response.Quantity,
		Status:        string(response.Status),
	})

}
