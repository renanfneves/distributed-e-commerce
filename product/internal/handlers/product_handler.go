package handlers

import (
	"e-commerce/product/internal/services"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type ProductHandler struct {
	service services.ProductService
}

func NewProductHandler(service services.ProductService) ProductHandler {
	return ProductHandler{
		service: service,
	}
}

type CreateProductInput struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Code         string `json:"code"`
	PriceInCents int64  `json:"price_in_cents"`
	Currency     string `json:"currency"`
}

func (p ProductHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var body CreateProductInput
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	input := services.ProductInput{
		Name:         body.Name,
		Description:  body.Description,
		Code:         body.Code,
		PriceInCents: body.PriceInCents,
		Currency:     body.Currency,
	}

	id, svcErr := p.service.CreateProduct(r.Context(), input)
	if svcErr != nil {
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

func (p ProductHandler) GetProductById(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	product, svcErr := p.service.GetProductById(r.Context(), id)
	if svcErr != nil {
		if errors.Is(svcErr, services.ErrProductNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(product); err != nil {
		log.Printf("erro ao escrever resposta: %v", err)
	}
}

func (p ProductHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	offset := int32(0)
	rawOffset := r.URL.Query().Get("offset")
	if rawOffset != "" {
		parsed, err := strconv.ParseInt(rawOffset, 10, 32)
		if err != nil || parsed < 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		offset = int32(parsed)
	}

	limit := int32(20)
	rawLimit := r.URL.Query().Get("limit")
	if rawLimit != "" {
		parsed, err := strconv.ParseInt(rawLimit, 10, 32)
		if err != nil || parsed < 1 || parsed > 100 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		limit = int32(parsed)
	}

	products, err := p.service.ListProducts(r.Context(), int32(offset), int32(limit))
	if err != nil {
		log.Printf("erro ao listar produtos: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(products) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(products); err != nil {
		log.Printf("erro ao escrever resposta: %v", err)
	}
}
