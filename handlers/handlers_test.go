package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"dag-project/dag"
	"dag-project/handlers"
	"dag-project/logger"
	"dag-project/models"
	"dag-project/repository"
	"dag-project/routers"
)

type mockRepo struct {
	mu    sync.Mutex
	nodes map[string]*models.Node
}

func newMockRepo() *mockRepo {
	return &mockRepo{nodes: make(map[string]*models.Node)}
}

func (m *mockRepo) PutNode(node *models.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *node
	m.nodes[node.ID] = &copy
	return nil
}

func (m *mockRepo) GetNode(id string) (*models.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	// return a copy to simulate DB retrieval
	copy := *n
	return &copy, nil
}

func (m *mockRepo) GetAllNodes() ([]*models.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]*models.Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		copy := *n
		res = append(res, &copy)
	}
	return res, nil
}

func testServer() (*mux.Router, *mockRepo) {
	logger.Logger = zap.NewNop()

	mockRepo := newMockRepo()
	var repoInterface repository.NodeRepositoryInterface = mockRepo
	dag := dag.NewDAG(repoInterface)
	handler := handlers.NewHandler(dag)
	router := mux.NewRouter()
	routers.RegisterRoutes(router, handler)
	return router, mockRepo
}

func TestAddNode_Success(t *testing.T) {
	router, mockRepo := testServer()

	body := map[string]interface{}{
		"id":      "A",
		"parents": []string{},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(bodyJSON))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body: %s", res.Code, res.Body.String())
	}

	got, err := mockRepo.GetNode("A")
	if err != nil {
		t.Fatalf("expected node stored, got error: %v", err)
	}
	if got.Weight != 0 {
		t.Fatalf("expected weight 0, got %d", got.Weight)
	}
}

func TestAddNode_Duplicate(t *testing.T) {
	router, _ := testServer()

	body := map[string]interface{}{
		"id":      "A",
		"parents": []string{},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(bodyJSON))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected first add 201, got %d", res.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(bodyJSON))
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected duplicate 409, got %d, body: %s", w2.Code, w2.Body.String())
	}
}

func TestApproveNode_SuccessAndParentWeightIncrement(t *testing.T) {
	router, mockRepo := testServer()

	// Create parent node
	parentNode := map[string]interface{}{
		"id":      "P1",
		"parents": []string{},
	}
	parentNodeJSON, _ := json.Marshal(parentNode)
	parentRequest := httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(parentNodeJSON))
	parentResponseRecorder := httptest.NewRecorder()
	router.ServeHTTP(parentResponseRecorder, parentRequest)
	if parentResponseRecorder.Code != http.StatusCreated {
		t.Fatalf("adding parent failed, code=%d body=%s", parentResponseRecorder.Code, parentResponseRecorder.Body.String())
	}

	// Create child node with parent "P1"
	childNode := map[string]interface{}{
		"id":      "C1",
		"parents": []string{"P1"},
	}
	childNodeJSON, _ := json.Marshal(childNode)
	childRequest := httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(childNodeJSON))
	childResponseRecorder := httptest.NewRecorder()
	router.ServeHTTP(childResponseRecorder, childRequest)
	if childResponseRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", childResponseRecorder.Code, childResponseRecorder.Body.String())
	}

	// Verify parent weight increment
	parentFromRepo, err := mockRepo.GetNode("P1")
	if err != nil {
		t.Fatalf("parent missing: %v", err)
	}
	if parentFromRepo.Weight != 1 {
		t.Fatalf("expected parent weight 1, got %d", parentFromRepo.Weight)
	}
}

func TestApproveNode_MissingParent(t *testing.T) {
	router, _ := testServer()

	invalidChildNode := map[string]interface{}{
		"id":      "C2",
		"parents": []string{"NOPE"}, // Non-existent parent ID
	}

	childNodeJSON, _ := json.Marshal(invalidChildNode)

	request := httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(childNodeJSON))
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", responseRecorder.Code, responseRecorder.Body.String())
	}
}
func TestGetHighestWeightNode(t *testing.T) {
	// Start test server
	router, _ := testServer()

	// Create node A (no parents)
	nodeA := map[string]interface{}{"id": "A", "parents": []string{}}
	nodeAJSON, _ := json.Marshal(nodeA)
	respCreateA := httptest.NewRecorder()
	router.ServeHTTP(respCreateA, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(nodeAJSON)))
	if respCreateA.Code != http.StatusCreated {
		t.Fatalf("Failed to add node A: %d", respCreateA.Code)
	}

	// Create node B (no parents)
	nodeB := map[string]interface{}{"id": "B", "parents": []string{}}
	nodeBJSON, _ := json.Marshal(nodeB)
	respCreateB := httptest.NewRecorder()
	router.ServeHTTP(respCreateB, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(nodeBJSON)))
	if respCreateB.Code != http.StatusCreated {
		t.Fatalf("Failed to add node B: %d", respCreateB.Code)
	}

	// Approve node C (parent B)
	nodeC := map[string]interface{}{"id": "C", "parents": []string{"B"}}
	nodeCJSON, _ := json.Marshal(nodeC)
	respApproveC := httptest.NewRecorder()
	router.ServeHTTP(respApproveC, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeCJSON)))
	if respApproveC.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node C: %d", respApproveC.Code)
	}

	// Retrieve the highest-weight node
	respHighestWeight := httptest.NewRecorder()
	router.ServeHTTP(respHighestWeight, httptest.NewRequest(http.MethodGet, "/nodes/highest-weight", nil))
	if respHighestWeight.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d, body: %s", respHighestWeight.Code, respHighestWeight.Body.String())
	}

	// Parse response JSON
	var highestWeightResponse map[string]interface{}
	if err := json.Unmarshal(respHighestWeight.Body.Bytes(), &highestWeightResponse); err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}

	// Extract node info
	highestWeightNode, ok := highestWeightResponse["node"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing 'node' in response: %v", highestWeightResponse)
	}

	// Validate highest-weight node is B
	if highestWeightNode["id"] != "B" {
		t.Fatalf("Expected highest-weight node B, got %v", highestWeightNode["id"])
	}
}
func TestGetTipMCMC(t *testing.T) {
	// Start test server
	router, _ := testServer()

	// Create node A (no parents)
	nodeA := map[string]interface{}{"id": "A", "parents": []string{}}
	nodeAJSON, _ := json.Marshal(nodeA)
	respCreateA := httptest.NewRecorder()
	router.ServeHTTP(respCreateA, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(nodeAJSON)))
	if respCreateA.Code != http.StatusCreated {
		t.Fatalf("Failed to add node A: %d", respCreateA.Code)
	}

	// Approve node B (parent A)
	nodeB := map[string]interface{}{"id": "B", "parents": []string{"A"}}
	nodeBJSON, _ := json.Marshal(nodeB)
	respApproveB := httptest.NewRecorder()
	router.ServeHTTP(respApproveB, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeBJSON)))
	if respApproveB.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node B: %d", respApproveB.Code)
	}

	// Approve node C (parent B)
	nodeC := map[string]interface{}{"id": "C", "parents": []string{"B"}}
	nodeCJSON, _ := json.Marshal(nodeC)
	respApproveC := httptest.NewRecorder()
	router.ServeHTTP(respApproveC, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeCJSON)))
	if respApproveC.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node C: %d", respApproveC.Code)
	}

	// Tip selection should return node C
	respTipSelection := httptest.NewRecorder()
	router.ServeHTTP(respTipSelection, httptest.NewRequest(http.MethodGet, "/nodes/tip-selection", nil))
	if respTipSelection.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d, body: %s", respTipSelection.Code, respTipSelection.Body.String())
	}

	var selectedTip models.Node
	if err := json.Unmarshal(respTipSelection.Body.Bytes(), &selectedTip); err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}
	if selectedTip.ID != "C" {
		t.Fatalf("Expected tip C, got %s", selectedTip.ID)
	}
}
func TestGetTipMCMC_NoNodes(t *testing.T) {
	router, _ := testServer()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/nodes/tip-selection", nil))
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body: %s", resp.Code, resp.Body.String())
	}
}
