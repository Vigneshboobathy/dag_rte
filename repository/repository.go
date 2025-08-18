package repository

import (
	"dag-project/db"
	"dag-project/models"
	"encoding/json"
)

// It abstracts the storage layer from the business logic
type NodeRepositoryInterface interface {
	PutNode(node *models.Node) error
	GetNode(id string) (*models.Node, error)
	GetAllNodes() ([]*models.Node, error)
	PutCheckpoint(cp *models.Checkpoint) error
	GetLatestCheckpoint() (*models.Checkpoint, error)
}

// NodeRepository implements the NodeRepositoryInterface using LevelDB as the storage backend
type NodeRepository struct {
	db *db.LevelDB
}

// NewNodeRepository creates and returns a new NodeRepository instance
func NewNodeRepository(db *db.LevelDB) *NodeRepository {
	return &NodeRepository{db: db}
}

// PutNode stores a node in the LevelDB storage
func (r *NodeRepository) PutNode(node *models.Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}
	return r.db.Put([]byte(node.ID), data)
}

// GetNode retrieves a node from LevelDB storage by its ID
func (r *NodeRepository) GetNode(id string) (*models.Node, error) {
	data, err := r.db.Get([]byte(id))
	if err != nil {
		return nil, err
	}
	var node models.Node
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

// GetAllNodes retrieves all nodes from the LevelDB storage
func (r *NodeRepository) GetAllNodes() ([]*models.Node, error) {
	iter := r.db.NewIterator()
	defer iter.Release()

	var nodes []*models.Node
	for iter.Next() {
		var node models.Node
		if err := json.Unmarshal(iter.Value(), &node); err != nil {
			return nil, err
		}
		nodes = append(nodes, &node)
	}
	return nodes, iter.Error()
}

// Creates a new checkpoint by storing the current state of the DAG
func (r *NodeRepository) PutCheckpoint(cp *models.Checkpoint) error {
	data, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	key := []byte("checkpoint:" + cp.ID)
	return r.db.Put(key, data)
}

// Retrieves the most recent checkpoint to restore the DAG state
func (r *NodeRepository) GetLatestCheckpoint() (*models.Checkpoint, error) {
	iter := r.db.NewIterator()
	defer iter.Release()

	var latest *models.Checkpoint
	for iter.Next() {
		key := string(iter.Key())
		if len(key) >= 11 && key[:11] == "checkpoint:" {
			var cp models.Checkpoint
			if err := json.Unmarshal(iter.Value(), &cp); err != nil {
				return nil, err
			}
			if latest == nil || cp.Timestamp > latest.Timestamp {
				latest = &cp
			}
		}
	}
	return latest, iter.Error()
}
	