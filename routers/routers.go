package routers

import (
	"dag-project/handlers"

	"github.com/gorilla/mux"
)

// RegisterRoutes sets up all the HTTP routes for the DAG
func RegisterRoutes(r *mux.Router, h *handlers.Handler) {

	// Creates a new node in the DAG with no parents initially
	r.HandleFunc("/nodes", h.AddNode).Methods("POST")

	// Approves a new node that references existing nodes as parents
	r.HandleFunc("/nodes/approve", h.ApproveNode).Methods("POST")

	// Used for identifying the most referenced/important nodes in the graph
	r.HandleFunc("/nodes/highest-weight", h.GetHighestWeightNode).Methods("GET")

	// Used for identifying the most important nodes including indirect approvals
	r.HandleFunc("/nodes/highest-cumulative-weight", h.GetHighestCumulativeWeightNode).Methods("GET")

	// Retrieves a tip using the MCMC algorithm
	r.HandleFunc("/nodes/tip-selection", h.GetTipMCMC).Methods("GET")

	// Used for identifying DAG state synchronization 
	r.HandleFunc("/sync/validate", h.ValidateDAGConsistency).Methods("GET")
}

	



