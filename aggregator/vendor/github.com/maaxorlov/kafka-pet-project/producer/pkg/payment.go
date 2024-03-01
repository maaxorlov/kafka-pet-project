package pkg

import "github.com/google/uuid"

type Payment struct {
	ID           uuid.UUID `json:"id"`
	IsSuccessful bool      `json:"is_successful"`
	Amount       float64   `json:"amount"`
}
