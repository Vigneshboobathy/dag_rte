package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"dag-project/dag"
	"dag-project/logger"
	"dag-project/models"

	"go.uber.org/zap"
)

// Handler contains the HTTP handlers for the DAG API endpoints
type Handler struct {
	DAG       *dag.DAG
	syncMutex sync.RWMutex
}

// NewHandler creates and returns a new Handler instance
func NewHandler(d *dag.DAG) *Handler {
	return &Handler{DAG: d}
}

// AddNode handles POST requests to create new nodes in the DAG
func (h *Handler) AddNode(w http.ResponseWriter, r *http.Request) {
	var node models.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		logger.Logger.Error("Failed to decode node", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid request payload",
		})
		logger.Logger.Error("Failed to decode node", zap.Error(err))
		return
	}

	if err := h.DAG.AddNode(&node); err != nil {
		logger.Logger.Error("Failed to add node", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		logger.Logger.Error("Failed to add node", zap.Error(err))
		return
	}

	// Success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Node added successfully",
		"node":    node,
	})
	logger.Logger.Info("Node added successfully", zap.String("node_id", node.ID))
}

// This endpoint creates nodes that build upon the existing DAG structure
func (h *Handler) ApproveNode(w http.ResponseWriter, r *http.Request) {
	var node models.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		logger.Logger.Error("Failed to decode approve node", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid request payload",
		})
		logger.Logger.Error("Failed to decode approve node", zap.Error(err))
		return
	}

	// Validate that approved nodes must have at least one parent
	if len(node.Parents) == 0 {
		logger.Logger.Error("Approved node must have at least one parent", zap.String("node_id", node.ID))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Approved nodes must reference at least one parent node",
		})
		logger.Logger.Error("Approved node must have at least one parent", zap.String("node_id", node.ID))
		return
	}

	if err := h.DAG.ApproveNode(&node); err != nil {
		logger.Logger.Error("Failed to approve node", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		logger.Logger.Error("Failed to approve node", zap.Error(err))
		return
	}

	logger.Logger.Info("Approved new node", zap.String("node_id", node.ID), zap.Strings("parents", node.Parents))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Node approved successfully",
		"node":    node,
	})
	logger.Logger.Info("Approved new node", zap.String("node_id", node.ID), zap.Strings("parents", node.Parents))
}

// GetHighestWeightNode handles GET requests to retrieve the node with the highest weight
func (h *Handler) GetHighestWeightNode(w http.ResponseWriter, r *http.Request) {
	node, err := h.DAG.GetHighestWeightNode()
	if err != nil {
		logger.Logger.Error("Failed to get highest weight node", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "highest weighted node",
		"node":    node,
	})
	logger.Logger.Info("Highest weighted node", zap.String("node_id", node.ID))
}

// GetHighestCumulativeWeightNode handles GET requests to retrieve the node with the highest cumulative weight
func (h *Handler) GetHighestCumulativeWeightNode(w http.ResponseWriter, r *http.Request) {
	node, err := h.DAG.GetHighestCumulativeWeightNode()
	if err != nil {
		logger.Logger.Error("Failed to get highest cumulative weight node", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "highest cumulative weighted node",
		"node":    node,
		"weight_info": map[string]interface{}{
			"direct_weight":     node.Weight,
			"cumulative_weight": node.CumulativeWeight,
		},
	})
	logger.Logger.Info("Highest cumulative weighted node", zap.String("node_id", node.ID))
}

// GetTipMCMC handles GET requests for a tip selected using MCMC
func (h *Handler) GetTipMCMC(w http.ResponseWriter, r *http.Request) {
	tip, err := h.DAG.TipSelection()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		logger.Logger.Error("Failed to select tip with MCMC", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tip)
	logger.Logger.Info("Tip selected using MCMC", zap.String("node_id", tip.ID))
}

// ValidateDAGConsistency checks if the DAG weights are consistent
func (h *Handler) ValidateDAGConsistency(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	h.syncMutex.Lock()
	defer h.syncMutex.Unlock()

	logger.Logger.Info("DAG consistency validation started")

	allNodes, err := h.DAG.GetAllNodes()
	if err != nil {
		http.Error(w, "Failed to retrieve nodes for validation", http.StatusInternalServerError)
		return
	}

	children := make(map[string][]string)
	for _, node := range allNodes {
		for _, parentID := range node.Parents {
			children[parentID] = append(children[parentID], node.ID)
		}
	}

	inconsistencies := []map[string]interface{}{}
	totalNodes := len(allNodes)
	validNodes := 0

	for _, node := range allNodes {
		expectedCumulative := int64(node.Weight)

		var calculateDescendantWeight func(string) int64
		calculateDescendantWeight = func(nID string) int64 {
			descendantWeight := int64(0)
			for _, childID := range children[nID] {
				childNode, err := h.DAG.GetNode(childID)
				if err != nil {
					continue
				}
				descendantWeight += int64(childNode.Weight)
				descendantWeight += calculateDescendantWeight(childID)
			}
			return descendantWeight
		}

		expectedCumulative += calculateDescendantWeight(node.ID)

		if node.CumulativeWeight != expectedCumulative {
			inconsistencies = append(inconsistencies, map[string]interface{}{
				"node_id":             node.ID,
				"expected_cumulative": expectedCumulative,
				"actual_cumulative":   node.CumulativeWeight,
				"difference":          expectedCumulative - node.CumulativeWeight,
			})
		} else {
			validNodes++
		}
	}

	validationResult := map[string]interface{}{
		"validation_time":    time.Now().Format(time.RFC3339),
		"total_nodes":        totalNodes,
		"valid_nodes":        validNodes,
		"inconsistent_nodes": len(inconsistencies),
		"inconsistencies":    inconsistencies,
		"duration":           time.Since(startTime).String(),
	}

	if totalNodes == 0 {
		validationResult["consistency_percentage"] = 100.0
	} else {
		validationResult["consistency_percentage"] = float64(validNodes) / float64(totalNodes) * 100
	}

	if len(inconsistencies) > 0 {
		validationResult["status"] = "inconsistent"
		w.WriteHeader(http.StatusConflict)
	} else {
		validationResult["status"] = "consistent"
		w.WriteHeader(http.StatusOK)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(validationResult)

	logger.Logger.Info("DAG consistency validation completed",
		zap.Int("total_nodes", totalNodes),
		zap.Int("valid_nodes", validNodes),
		zap.Int("inconsistencies", len(inconsistencies)))
}
