package model

import (
	"github.com/google/uuid"
	"time"
)

type Media struct {
	URL       string            `json:"url"`
	UUID      uuid.UUID         `json:"uuid"`
	Kind      string            `json:"kind"`
	MIME      string            `json:"mime"`
	Size      int64             `json:"size"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Alt       map[string]string `json:"alt"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type MediaList struct {
	Items  []Media `json:"items"`
	Total  int64   `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type MediaInput struct {
	Alt map[string]string `json:"alt"`
}
