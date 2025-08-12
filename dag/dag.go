package dag

import (
	"errors"
	"sync"
	"time"

	"dag-project/logger"
	"dag-project/models"
	"dag-project/repository"

	"go.uber.org/zap"
)

type DAG struct {
	repo repository.NodeRepositoryInterface
	mux  sync.Mutex
}

func NewDAG(repo repository.NodeRepositoryInterface) *DAG {
	return &DAG{repo: repo}
}

// AddNode stores a node, with no parents initially
func (d *DAG) AddNode(node *models.Node) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	existingNode, err := d.repo.GetNode(node.ID)
	if err == nil && existingNode != nil {
		return errors.New("node with ID already exists")
	}

	node.Weight = 0
	node.CreatedAt = nowMillis()
	return d.repo.PutNode(node)
}

// ApproveNode adds a new node referencing previous node(s) (parents)
// Also increases weight of each parent by 1
func (d *DAG) ApproveNode(node *models.Node) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	// check all parents exist
	for _, pid := range node.Parents {
		_, err := d.repo.GetNode(pid)
		if err != nil {
			return errors.New("parent node " + pid + " does not exist")
		}
	}

	node.Weight = 0
	node.CreatedAt = nowMillis()

	err := d.repo.PutNode(node)
	if err != nil {
		return err
	}

	// increase weight of parents
	for _, pid := range node.Parents {
		parentNode, err := d.repo.GetNode(pid)
		if err != nil {
			logger.Logger.Warn("Parent node missing during weight update",
				zap.String("parent_id", pid))
			continue
		}
		parentNode.Weight++
		err = d.repo.PutNode(parentNode)
		if err != nil {
			logger.Logger.Warn("Failed updating parent weight",
				zap.String("parent_id", pid), zap.Error(err))
		}
	}

	return nil
}

// GetTips returns nodes with no parents (tips)
func (d *DAG) GetTips() ([]*models.Node, error) {
	nodes, err := d.repo.GetAllNodes()
	if err != nil {
		return nil, err
	}

	var tips []*models.Node
	for _, node := range nodes {
		if len(node.Parents) == 0 && node.Weight == 0 {
			tips = append(tips, node)
		}
	}
	return tips, nil
}

// GetHighestWeightNode returns node with highest weight
func (d *DAG) GetHighestWeightNode() (*models.Node, error) {
	nodes, err := d.repo.GetAllNodes()
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("no nodes in DAG")
	}

	highest := nodes[0]
	for _, node := range nodes {
		if node.Weight > highest.Weight {
			highest = node
		}
	}

	return highest, nil
}

// TipSelection returns one tip (node with no parents) with highest weight or earliest created
func (d *DAG) TipSelection() (*models.Node, error) {
	tips, err := d.GetTips()
	if err != nil {
		return nil, err
	}
	if len(tips) == 0 {
		return nil, errors.New("no tips available")
	}

	selected := tips[0]
	for _, t := range tips {
		if t.Weight > selected.Weight || (t.Weight == selected.Weight && t.CreatedAt < selected.CreatedAt) {
			selected = t
		}
	}
	return selected, nil
}

// nowMillis returns current time in milliseconds
func nowMillis() int64 {
	return time.Now().UnixMilli()
}
