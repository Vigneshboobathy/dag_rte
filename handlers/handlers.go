package handlers

import (
	"encoding/json"
	"net/http"

	"dag-project/dag"
	"dag-project/logger"
	"dag-project/models"

	"go.uber.org/zap"
)

// Handler contains the HTTP handlers for the DAG API endpoints
type Handler struct {
	DAG *dag.DAG
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

// CreateCheckpoint handles POST requests to create a new checkpoint
func (h *Handler) CreateCheckpoint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	cp, err := h.DAG.CreateCheckpoint(body.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cp)
}

// Get LatestCheckpoint handles GET requests for the latest checkpoint
func (h *Handler) GetLatestCheckpoint(w http.ResponseWriter, r *http.Request) {
	cp, err := h.DAG.GetLatestCheckpoint()
	if err != nil || cp == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "No checkpoint found"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cp)
}

// GetSyncState handles GET requests for the current DAG sync state
func (h *Handler) GetSyncState(w http.ResponseWriter, r *http.Request) {
	state, err := h.DAG.GetSyncState()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(state)
}
