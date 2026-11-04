package model

import "time"

type GeneralMaster struct {
	ID                        string    `json:"id"`
	Code                      string    `json:"code"`
	Name                      string    `json:"name"`
	Description               *string   `json:"description"`
	DefaultSortBy             string    `json:"defaultSortBy"`
	InitialDisplayCode        bool      `json:"initialDisplayCode"`
	InitialDisplayCreatedAt   bool      `json:"initialDisplayCreatedAt"`
	InitialDisplayDescription bool      `json:"initialDisplayDescription"`
	CreatedAt                 time.Time `json:"createdAt"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}
type GeneralMasterField struct {
	ID              string    `json:"id"`
	GeneralMasterID string    `json:"generalMasterId"`
	Code            string    `json:"code"`
	Title           string    `json:"title"`
	Description     *string   `json:"description"`
	FieldType       string    `json:"fieldType"`
	Required        bool      `json:"required"`
	FieldOrder      int       `json:"fieldOrder"`
	Visible         bool      `json:"visible"`
	InitialDisplay  bool      `json:"initialDisplay"`
	Options         []string  `json:"options,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
type GeneralMasterItem struct {
	ID              string            `json:"id"`
	GeneralMasterID string            `json:"generalMasterId"`
	Code            string            `json:"code"`
	Name            string            `json:"name"`
	Description     *string           `json:"description"`
	StartsOn        *string           `json:"startsOn"`
	EndsOn          *string           `json:"endsOn"`
	Inputs          map[string]any    `json:"inputs"`
	InputIDs        map[string]string `json:"inputIds"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}
