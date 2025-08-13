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

// TipSelection now uses an MCMC-style weighted random walk.
func (d *DAG) TipSelection() (*models.Node, error) {
	const defaultAlpha = 0.01
	const defaultMaxSteps = 10000
	return d.TipSelectionMCMC(defaultAlpha, defaultMaxSteps)
}

// TipSelectionMCMC runs a weighted random walk (MCMC-like) biased by cumulative weight.
func (d *DAG) TipSelectionMCMC(alpha float64, maxSteps int) (*models.Node, error) {
	// fetch all nodes
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

	// compute cumulative weights with memoized DFS
	cumWeight := make(map[string]int64)
	var computeCum func(id string) int64
	var computeMux sync.Mutex
	computeCum = func(id string) int64 {
		computeMux.Lock()
		if v, ok := cumWeight[id]; ok {
			computeMux.Unlock()
			return v
		}
		computeMux.Unlock()

		n, ok := nodesByID[id]
		if !ok {
			return 0
		}

		var sum int64 = int64(n.Weight)
		for _, childID := range children[id] {
			sum += computeCum(childID)
		}

		computeMux.Lock()
		cumWeight[id] = sum
		computeMux.Unlock()
		return sum
	}

	for id := range nodesByID {
		_ = computeCum(id)
	}

	// find a start node: pick the earliest-created node that has children (a "past" node)
	var start *models.Node
	var earliest int64 = 1<<63 - 1
	for _, n := range nodes {
		if len(children[n.ID]) == 0 {
			// skip tips
			continue
		}
		if n.CreatedAt < earliest {
			earliest = n.CreatedAt
			start = n
		}
	}
	// if no node with children found (very small graph), pick a random node
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	if start == nil {
		start = nodes[rnd.Intn(len(nodes))]
	}

	// perform weighted random walk forward along children until we hit a tip
	cur := start
	steps := 0
	for {
		steps++
		if steps > maxSteps {
			return nil, errors.New("mcmc tip selection exceeded max steps")
		}

		ch := children[cur.ID]
		// if no children -> reached a tip
		if len(ch) == 0 {
			return cur, nil
		}

		// compute weights using exp(alpha * cumulativeWeight)
		weights := make([]float64, len(ch))
		var total float64
		for i, cid := range ch {
			cw := cumWeight[cid]
			// Use exp to bias selection: higher cumulative weight = higher probability
			w := math.Exp(alpha * float64(cw))
			weights[i] = w
			total += w
		}

		// choose child by weights
		if total <= 0 {
			// fallback uniform
			cur = nodesByID[ch[rnd.Intn(len(ch))]]
			continue
		}
		p := rnd.Float64() * total
		acc := 0.0
		chosen := ch[0]
		for i, w := range weights {
			acc += w
			if p <= acc {
				chosen = ch[i]
				break
			}
		}
		cur = nodesByID[chosen]
	}
}

// nowMillis returns current time in milliseconds
func nowMillis() int64 {
	return time.Now().UnixMilli()
}
