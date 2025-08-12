package models

type Node struct {
	ID        string   `json:"id"`         // unique id
	Parents   []string `json:"parents"`    // parent node IDs
	Weight    int      `json:"weight"`     // weight based on approvals
	CreatedAt int64    `json:"created_at"` // unix timestamp in ms
}
