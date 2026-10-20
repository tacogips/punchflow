package model

import "time"

type File struct {
	ID          string    `json:"id"`
	SignedID    string    `json:"signedId"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	ByteSize    int64     `json:"byteSize"`
	Checksum    string    `json:"checksum"`
	StorageName string    `json:"storageName"`
	UploadedBy  *string   `json:"uploadedBy"`
	CreatedAt   time.Time `json:"createdAt"`
}
type FileAttachment struct {
	FileID      string    `json:"fileId"`
	TicketID    string    `json:"ticketId"`
	CommentID   *string   `json:"commentId"`
	FormFieldID *string   `json:"formFieldId"`
	SlipItemID  *string   `json:"slipItemId"`
	CreatedAt   time.Time `json:"createdAt"`
}
