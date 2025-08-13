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
		return
	}

	if err := h.DAG.AddNode(&node); err != nil {
		logger.Logger.Error("Failed to add node", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Node added successfully",
		"node":    node,
	})
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
		return
	}

	if err := h.DAG.ApproveNode(&node); err != nil {
		logger.Logger.Error("Failed to approve node", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	logger.Logger.Info("Approved new node", zap.String("node_id", node.ID), zap.Strings("parents", node.Parents))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Node approved successfully",
		"node":    node,
	})
}

// GetHighestWeightNode handles GET requests to retrieve the node with the highest weight
func (h *Handler) GetHighestWeightNode(w http.ResponseWriter, r *http.Request) {
	node, err := h.DAG.GetHighestWeightNode()
	if err != nil {
		logger.Logger.Error("Failed to get highest weight node", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "highest weighted node",
		"node":    node,
	})
}

// GetTipMCMC handles GET requests for a tip selected using MCMC
func (h *Handler) GetTipMCMC(w http.ResponseWriter, r *http.Request) {
	tip, err := h.DAG.TipSelection()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		logger.Logger.Error("Failed to select tip with MCMC", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tip)
}
