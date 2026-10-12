package handlers

import (
	"e-commerce/inventory/internal/services"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type InventoryHandler struct {
	service services.InventoryService
}

func NewInventoryHandler(service services.InventoryService) InventoryHandler {
	return InventoryHandler{
		service: service,
	}
}

type AddItemRequest struct {
	SkuID          uuid.UUID `json:"sku_id"`
	QuantityOnHand int64     `json:"quantity_on_hand"`
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
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if errors.Is(svcErr, services.ErrInventoryItemAlreadyExists) {
			w.WriteHeader(http.StatusConflict)
			if err := json.NewEncoder(w).Encode(map[string]string{
				"message": "já existe um item de inventário para este SKU",
			}); err != nil {
				log.Printf("erro ao escrever resposta: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"id": id.String(),
	}); err != nil {
		log.Printf("erro ao escrever resposta: %v", err)
	}
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(item); err != nil {
		log.Printf("erro ao escrever resposta: %v", err)
	}
}
