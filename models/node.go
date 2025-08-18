package models

type Node struct {
	ID               string   `json:"id"`                // unique id
	Parents          []string `json:"parents"`           // parent node IDs
	Weight           int      `json:"weight"`            // direct weight based on approvals
	CumulativeWeight int64    `json:"cumulative_weight"` // total weight including indirect approvals
	CreatedAt        int64    `json:"created_at"`        // unix timestamp in ms
}

type Checkpoint struct {
	ID        string `json:"checkpoint_id"` // checkpoint ID
	Timestamp int64  `json:"timestamp"`     // when the checkpoint was created
	RootHash  string `json:"root_hash"`     // Merkle root / hash of DAG state
	NodeCount int    `json:"node_count"`    // how many nodes up to this checkpoint
}

type SyncState struct {
	LatestCheckpoint *Checkpoint `json:"latest_checkpoint,omitempty"`
	NodeCount        int         `json:"node_count"`
	TipCount         int         `json:"tip_count"`
	RootHash         string      `json:"root_hash"`
	Timestamp        int64       `json:"timestamp"`
}
