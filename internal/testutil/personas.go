package testutil

// Persona identifies one of the API identities seeded by fixtures/default.
type Persona string

const (
	Admin       Persona = "Admin"
	Manager     Persona = "Manager"
	Finance     Persona = "Finance"
	Employee    Persona = "Employee"
	Integration Persona = "Integration"
)

func (p Persona) token() string {
	switch p {
	case Admin:
		return "pf_pat_dev_admin"
	case Manager:
		return "pf_pat_dev_manager"
	case Finance:
		return "pf_pat_dev_finance"
	case Employee:
		return "pf_pat_dev_employee"
	case Integration:
		return "pf_sat_dev_integration"
	default:
		return ""
	}
}
