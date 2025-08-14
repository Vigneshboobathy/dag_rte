package dag

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"dag-project/logger"
	"dag-project/models"
	"dag-project/repository"

	"go.uber.org/zap"
)

// DAG implements basic DAG operations and tip selection using MCMC (weighted random walk).
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
	node.CumulativeWeight = 0
	node.CreatedAt = nowMillis()
	return d.repo.PutNode(node)
}

// ApproveNode adds a new node referencing previous nodes parents
func (d *DAG) ApproveNode(node *models.Node) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	// Validate that the node doesn't reference itself as a parent
	for _, pid := range node.Parents {
		if pid == node.ID {
			return errors.New("node cannot reference itself as a parent")
		}
	}

	// Check for circular references
	if err := d.checkForCircularReferences(node.ID, node.Parents); err != nil {
		return err
	}

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

	// increase weight of parents and update cumulative weights
	err = d.propagateWeights(node.Parents)
	if err != nil {
		logger.Logger.Warn("Failed to update ancestor weights", zap.Error(err))
	}

	return nil
}

// updateParentNodeWeights recursively updates weights and cumulative weights of all parent nodes
func (d *DAG) propagateWeights(parentIDs []string) error {
	if len(parentIDs) == 0 {
		return nil
	}

	// Get all nodes to build the graph structure
	allNodes, err := d.repo.GetAllNodes()
	if err != nil {
		return err
	}

	// Build parent-child relationships
	children := make(map[string][]string)
	parents := make(map[string][]string)
	for _, n := range allNodes {
		for _, p := range n.Parents {
			children[p] = append(children[p], n.ID)
			parents[n.ID] = append(parents[n.ID], p)
		}
	}

	// Update direct weights first
	for _, pid := range parentIDs {
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

	// Now update cumulative weights for all affected nodes
	affectedNodes := make(map[string]bool)
	for _, pid := range parentIDs {
		d.markDependenciesAffected(pid, parents, affectedNodes)
	}

	// Recalculate cumulative weights for affected nodes
	for nodeID := range affectedNodes {
		if err := d.updateCumulativeWeight(nodeID, children); err != nil {
			logger.Logger.Warn("Failed to update cumulative weight",
				zap.String("node_id", nodeID), zap.Error(err))
		}
	}

	return nil
}

// checks if adding this node would create a circular reference
func (d *DAG) checkForCircularReferences(_ string, parentIDs []string) error {
	allNodes, err := d.repo.GetAllNodes()
	if err != nil {
		return err
	}

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(string) bool
	hasCycle = func(currentID string) bool {
		if recStack[currentID] {
			return true 
		}
		if visited[currentID] {
			return false 
		}

		visited[currentID] = true
		recStack[currentID] = true

		// Check if this node would be a parent of the new node
		for _, pid := range parentIDs {
			if pid == currentID {
				for _, existingNode := range allNodes {
					for _, existingParentID := range existingNode.Parents {
						if existingParentID == currentID {
							if hasCycle(existingNode.ID) {
								recStack[currentID] = false
								return true
							}
						}
					}
				}
			}
		}

		recStack[currentID] = false
		return false
	}
	for _, parentID := range parentIDs {
		if hasCycle(parentID) {
			return errors.New("circular reference detected: adding this node would create a cycle")
		}
	}

	return nil
}

// markDependenciesAffected marks all ancestor nodes as needing cumulative weight updates
func (d *DAG) markDependenciesAffected(nodeID string, parents map[string][]string, affected map[string]bool) {
	if affected[nodeID] {
		return
	}

	affected[nodeID] = true

	// Recursively mark all parents of this node
	for _, parentID := range parents[nodeID] {
		d.markDependenciesAffected(parentID, parents, affected)
	}
}

// updateCumulativeWeight calculates and stores the cumulative weight for a specific node
func (d *DAG) updateCumulativeWeight(nodeID string, children map[string][]string) error {
	node, err := d.repo.GetNode(nodeID)
	if err != nil {
		return err
	}

	// Calculate cumulative weight: direct weight + sum of all descendant weights
	cumulativeWeight := int64(node.Weight)

	// Add weights of all descendants recursively
	var calculateDescendantWeight func(string) int64
	calculateDescendantWeight = func(nID string) int64 {
		descendantWeight := int64(0)
		for _, childID := range children[nID] {
			childNode, err := d.repo.GetNode(childID)
			if err != nil {
				continue
			}
			descendantWeight += int64(childNode.Weight)
			descendantWeight += calculateDescendantWeight(childID)
		}
		return descendantWeight
	}

	cumulativeWeight += calculateDescendantWeight(nodeID)

	// Update the node's cumulative weight
	node.CumulativeWeight = cumulativeWeight
	return d.repo.PutNode(node)
}

// GetHighestWeightNode returns node with highest direct weight (unchanged)
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

// GetHighestCumulativeWeightNode returns node with highest cumulative weight
func (d *DAG) GetHighestCumulativeWeightNode() (*models.Node, error) {
	nodes, err := d.repo.GetAllNodes()
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("no nodes in DAG")
	}

	highest := nodes[0]
	for _, node := range nodes {
		if node.CumulativeWeight > highest.CumulativeWeight {
			highest = node
		}
	}

	return highest, nil
}

// TipSelection now uses an MCMC-style weighted random walk.
func (d *DAG) TipSelection() (*models.Node, error) {
	const defaultAlpha = 0.01
	const defaultMaxSteps = 10000
	return d.TipSelectionMCMC(defaultAlpha, defaultMaxSteps)
}

// TipSelectionMCMC runs a proper MCMC-style weighted random walk for tip selection.
func (d *DAG) TipSelectionMCMC(alpha float64, maxSteps int) (*models.Node, error) {
	nodes, err := d.repo.GetAllNodes()
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("no nodes in DAG")
	}

	// build maps: id -> node, parents map, children map
	nodesByID := make(map[string]*models.Node, len(nodes))
	children := make(map[string][]string)
	parents := make(map[string][]string)

	for _, n := range nodes {
		nodesByID[n.ID] = n
		for _, p := range n.Parents {
			children[p] = append(children[p], n.ID)
			parents[n.ID] = append(parents[n.ID], p)
		}
	}

	// Find all nodes with no children
	var tips []*models.Node
	for _, n := range nodes {
		if len(children[n.ID]) == 0 {
			tips = append(tips, n)
		}
	}

	if len(tips) == 0 {
		// If no tips found, return a random node
		return nodes[rand.Intn(len(nodes))], nil
	}

	// Initialize random number generator
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Start from a random tip
	currentTip := tips[rnd.Intn(len(tips))]

	// Perform MCMC walk
	for step := 0; step < maxSteps; step++ {
		currentWeight := d.calculateCumulativeWeight(currentTip.ID, children, nodesByID)
		// Propose a random selection from all tip
		proposedTip := tips[rnd.Intn(len(tips))]
		proposedWeight := d.calculateCumulativeWeight(proposedTip.ID, children, nodesByID)

		// Higher cumulative weight = higher probability of acceptance
		acceptanceProb := math.Exp(alpha * float64(proposedWeight-currentWeight))

		// Accept  the proposal
		if rnd.Float64() < acceptanceProb {
			currentTip = proposedTip
		}

		if step%100 == 0 && len(parents[currentTip.ID]) > 0 {
			// Randomly walk to a parent node
			parentID := parents[currentTip.ID][rnd.Intn(len(parents[currentTip.ID]))]
			if _, exists := nodesByID[parentID]; exists {
				if len(children[parentID]) > 0 {
					childIDs := children[parentID]
					randomChildID := childIDs[rnd.Intn(len(childIDs))]
					if _, exists := nodesByID[randomChildID]; exists {
						tipFromChild := d.walkToTip(randomChildID, children, nodesByID, rnd)
						if tipFromChild != nil {
							currentTip = tipFromChild
						}
					}
				}
			}
		}
	}

	return currentTip, nil
}

// calculateCumulativeWeight calculates the cumulative weight for a node
func (d *DAG) calculateCumulativeWeight(nodeID string, children map[string][]string, nodesByID map[string]*models.Node) int64 {
	node, exists := nodesByID[nodeID]
	if !exists {
		return 0
	}

	// Start with direct weight
	cumulativeWeight := int64(node.Weight)

	var calculateParentWeight func(string) int64
	calculateParentWeight = func(nID string) int64 {
		descendantWeight := int64(0)
		for _, childID := range children[nID] {
			if childNode, exists := nodesByID[childID]; exists {
				descendantWeight += int64(childNode.Weight)
				descendantWeight += calculateParentWeight(childID)
			}
		}
		return descendantWeight
	}

	cumulativeWeight += calculateParentWeight(nodeID)
	return cumulativeWeight
}

// walkToTip walks from a given node to one of its parent tips
func (d *DAG) walkToTip(nodeID string, children map[string][]string, nodesByID map[string]*models.Node, rnd *rand.Rand) *models.Node {
	currentID := nodeID

	for {
		childIDs := children[currentID]
		if len(childIDs) == 0 {
			if tipNode, exists := nodesByID[currentID]; exists {
				return tipNode
			}
			return nil
		}

		// Randomly choose a child 
		nextID := childIDs[rnd.Intn(len(childIDs))]
		currentID = nextID
	}
}

// GetNode retrieves a node by ID
func (d *DAG) GetNode(id string) (*models.Node, error) {
	d.mux.Lock()
	defer d.mux.Unlock()

	return d.repo.GetNode(id)
}

// GetAllNodes retrieves all nodes from the repository
func (d *DAG) GetAllNodes() ([]*models.Node, error) {
	d.mux.Lock()
	defer d.mux.Unlock()

	return d.repo.GetAllNodes()
}

// UpdateNode updates an existing node in the DAG
func (d *DAG) UpdateNode(node *models.Node) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	// Verify the node exists
	existingNode, err := d.repo.GetNode(node.ID)
	if err != nil {
		return errors.New("node does not exist")
	}

	// Preserve the original weight if the incoming node has lower weight
	if node.Weight < existingNode.Weight {
		node.Weight = existingNode.Weight
	}

	// Preserve the original creation time if the incoming node is older
	if node.CreatedAt < existingNode.CreatedAt {
		node.CreatedAt = existingNode.CreatedAt
	}

	return d.repo.PutNode(node)
}

// nowMillis returns current time in milliseconds
func nowMillis() int64 {
	return time.Now().UnixMilli()
}


