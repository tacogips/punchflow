package model

import "time"

type Category struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Folder struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	ParentID    *string   `json:"parentId"`
	Editable    bool      `json:"editable"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type Grade struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Level     int       `json:"level"`
	IsDefault bool      `json:"isDefault"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// GradeMeetsMinimum reports whether a grade is at or above the configured
// minimum grade. Higher level values represent higher grades.
func GradeMeetsMinimum(grade, minimum Grade) bool {
	return grade.Level >= minimum.Level
}
