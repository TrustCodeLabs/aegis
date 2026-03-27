package notes

import "time"

const (
	ModuleName      = "notes"
	ResourceName    = "note-data"
	OperationCreate = "notes.create"
	OperationList   = "notes.list"
	OperationGet    = "notes.get"
	OperationUpdate = "notes.update"
	OperationDelete = "notes.delete"
)

type Note struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Internal  string    `json:"internal"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Summary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Internal  string    `json:"internal"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Index struct {
	Notes []Summary `json:"notes"`
}

type CreateInput struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Internal string `json:"internal,omitempty"`
}

type UpdateInput struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Internal string `json:"internal,omitempty"`
}

type LookupInput struct {
	ID string `json:"id"`
}

type ListInput struct{}

type Output struct {
	Note Note `json:"note"`
}

type ListOutput struct {
	Notes []Summary `json:"notes"`
}

type DeleteOutput struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}
