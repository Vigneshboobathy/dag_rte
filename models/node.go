package models

type Node struct {
	ID               string   `json:"id"`                // unique id
	Parents          []string `json:"parents"`           // parent node IDs
	Weight           int      `json:"weight"`            // direct weight based on approvals
	CumulativeWeight int64    `json:"cumulative_weight"` // total weight including indirect approvals
	CreatedAt        int64    `json:"created_at"`        // unix timestamp in ms
}
