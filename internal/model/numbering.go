package model

// CustomNumbering describes a fixture-defined counter shared by one or more
// workflow versions.
type CustomNumbering struct {
	ID                string  `json:"id"`
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Value             int64   `json:"value"`
	TicketNumberKeyID *string `json:"ticketNumberKeyId"`
	ResetPeriod       string  `json:"resetPeriod,omitempty"`
}

// CustomNumberingSetting configures a numbering format and issuance event for
// a workflow version.
type CustomNumberingSetting struct {
	ID                 string  `json:"id"`
	CustomNumberingID  string  `json:"customNumberingId"`
	TicketNumberFormat *string `json:"ticketNumberFormat"`
	TimingType         string  `json:"timingType"`
	SubStatusID        *string `json:"subStatusId"`
	SortOrder          int     `json:"sortOrder"`
}
