package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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
	mu          sync.Mutex
	nodes       map[string]*models.Node
	checkpoints map[string]*models.Checkpoint
}

func newMockRepo() *mockRepo {
	return &mockRepo{nodes: make(map[string]*models.Node), checkpoints: make(map[string]*models.Checkpoint)}
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

func (m *mockRepo) PutCheckpoint(cp *models.Checkpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *cp
	m.checkpoints[cp.ID] = &copy
	return nil
}

func (m *mockRepo) GetLatestCheckpoint() (*models.Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.checkpoints) == 0 {
		return nil, fmt.Errorf("no checkpoint")
	}
	var latest *models.Checkpoint
	for _, cp := range m.checkpoints {
		if latest == nil || cp.Timestamp > latest.Timestamp {
			c := *cp
			latest = &c
		}
	}
	return latest, nil
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
		"parents": []string{"NOPE"},
	}

	childNodeJSON, _ := json.Marshal(invalidChildNode)

	request := httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(childNodeJSON))
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", responseRecorder.Code, responseRecorder.Body.String())
	}
}

func TestApproveNode_NoParents(t *testing.T) {
	router, _ := testServer()

	nodeWithoutParents := map[string]interface{}{
		"id":      "INVALID",
		"parents": []string{},
	}

	nodeJSON, _ := json.Marshal(nodeWithoutParents)

	request := httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeJSON))
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for node without parents, got %d", responseRecorder.Code)
	}

	var errorResponse map[string]string
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &errorResponse); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errorResponse["error"] != "Approved nodes must reference at least one parent node" {
		t.Fatalf("expected error about missing parents, got %s", errorResponse["error"])
	}
}

func TestApproveNode_SelfReference(t *testing.T) {
	router, _ := testServer()

	selfReferencingNode := map[string]interface{}{
		"id":      "SELF_REF",
		"parents": []string{"SELF_REF"}, // Self-reference
	}

	nodeJSON, _ := json.Marshal(selfReferencingNode)
	request := httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeJSON))
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for self-referencing node, got %d", responseRecorder.Code)
	}

	var errorResponse map[string]string
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &errorResponse); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errorResponse["error"] != "node cannot reference itself as a parent" {
		t.Fatalf("expected error about self-reference, got %s", errorResponse["error"])
	}
}

func TestGetHighestWeightNode(t *testing.T) {
	router, _ := testServer()

	nodeA := map[string]interface{}{"id": "A", "parents": []string{}}
	nodeAJSON, _ := json.Marshal(nodeA)
	respCreateA := httptest.NewRecorder()
	router.ServeHTTP(respCreateA, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(nodeAJSON)))
	if respCreateA.Code != http.StatusCreated {
		t.Fatalf("Failed to add node A: %d", respCreateA.Code)
	}

	nodeB := map[string]interface{}{"id": "B", "parents": []string{}}
	nodeBJSON, _ := json.Marshal(nodeB)
	respCreateB := httptest.NewRecorder()
	router.ServeHTTP(respCreateB, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(nodeBJSON)))
	if respCreateB.Code != http.StatusCreated {
		t.Fatalf("Failed to add node B: %d", respCreateB.Code)
	}

	nodeC := map[string]interface{}{"id": "C", "parents": []string{"B"}}
	nodeCJSON, _ := json.Marshal(nodeC)
	respApproveC := httptest.NewRecorder()
	router.ServeHTTP(respApproveC, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeCJSON)))
	if respApproveC.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node C: %d", respApproveC.Code)
	}

	respHighestWeight := httptest.NewRecorder()
	router.ServeHTTP(respHighestWeight, httptest.NewRequest(http.MethodGet, "/nodes/highest-weight", nil))
	if respHighestWeight.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d, body: %s", respHighestWeight.Code, respHighestWeight.Body.String())
	}

	var highestWeightResponse map[string]interface{}
	if err := json.Unmarshal(respHighestWeight.Body.Bytes(), &highestWeightResponse); err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}

	highestWeightNode, ok := highestWeightResponse["node"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing 'node' in response: %v", highestWeightResponse)
	}

	if highestWeightNode["id"] != "B" {
		t.Fatalf("Expected highest-weight node B, got %v", highestWeightNode["id"])
	}
}
func TestGetTipMCMC(t *testing.T) {
	router, _ := testServer()

	nodeA := map[string]interface{}{"id": "A", "parents": []string{}}
	nodeAJSON, _ := json.Marshal(nodeA)
	respCreateA := httptest.NewRecorder()
	router.ServeHTTP(respCreateA, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(nodeAJSON)))
	if respCreateA.Code != http.StatusCreated {
		t.Fatalf("Failed to add node A: %d", respCreateA.Code)
	}

	nodeB := map[string]interface{}{"id": "B", "parents": []string{"A"}}
	nodeBJSON, _ := json.Marshal(nodeB)
	respApproveB := httptest.NewRecorder()
	router.ServeHTTP(respApproveB, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeBJSON)))
	if respApproveB.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node B: %d", respApproveB.Code)
	}

	nodeC := map[string]interface{}{"id": "C", "parents": []string{"B"}}
	nodeCJSON, _ := json.Marshal(nodeC)
	respApproveC := httptest.NewRecorder()
	router.ServeHTTP(respApproveC, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeCJSON)))
	if respApproveC.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node C: %d", respApproveC.Code)
	}

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

func TestCumulativeWeight_ChainScenario(t *testing.T) {
	router, mockRepo := testServer()

	node1 := map[string]interface{}{"id": "1", "parents": []string{}}
	node1JSON, _ := json.Marshal(node1)
	resp1 := httptest.NewRecorder()
	router.ServeHTTP(resp1, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(node1JSON)))
	if resp1.Code != http.StatusCreated {
		t.Fatalf("Failed to create node 1: %d", resp1.Code)
	}

	node2 := map[string]interface{}{"id": "2", "parents": []string{"1"}}
	node2JSON, _ := json.Marshal(node2)
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(node2JSON)))
	if resp2.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node 2: %d", resp2.Code)
	}

	node3 := map[string]interface{}{"id": "3", "parents": []string{"2"}}
	node3JSON, _ := json.Marshal(node3)
	resp3 := httptest.NewRecorder()
	router.ServeHTTP(resp3, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(node3JSON)))
	if resp3.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node 3: %d", resp3.Code)
	}

	node1FromRepo, err := mockRepo.GetNode("1")
	if err != nil {
		t.Fatalf("Node 1 not found: %v", err)
	}
	if node1FromRepo.Weight != 1 {
		t.Fatalf("Expected node 1 direct weight 1, got %d", node1FromRepo.Weight)
	}
	if node1FromRepo.CumulativeWeight != 2 {
		t.Fatalf("Expected node 1 cumulative weight 2, got %d", node1FromRepo.CumulativeWeight)
	}

	node2FromRepo, err := mockRepo.GetNode("2")
	if err != nil {
		t.Fatalf("Node 2 not found: %v", err)
	}
	if node2FromRepo.Weight != 1 {
		t.Fatalf("Expected node 2 direct weight 1, got %d", node2FromRepo.Weight)
	}
	if node2FromRepo.CumulativeWeight != 1 {
		t.Fatalf("Expected node 2 cumulative weight 1, got %d", node2FromRepo.CumulativeWeight)
	}

	node3FromRepo, err := mockRepo.GetNode("3")
	if err != nil {
		t.Fatalf("Node 3 not found: %v", err)
	}
	if node3FromRepo.Weight != 0 {
		t.Fatalf("Expected node 3 direct weight 0, got %d", node3FromRepo.Weight)
	}
	if node3FromRepo.CumulativeWeight != 0 {
		t.Fatalf("Expected node 3 cumulative weight 0, got %d", node3FromRepo.CumulativeWeight)
	}

	respCumulative := httptest.NewRecorder()
	router.ServeHTTP(respCumulative, httptest.NewRequest(http.MethodGet, "/nodes/highest-cumulative-weight", nil))
	if respCumulative.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d, body: %s", respCumulative.Code, respCumulative.Body.String())
	}

	var cumulativeResponse map[string]interface{}
	if err := json.Unmarshal(respCumulative.Body.Bytes(), &cumulativeResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	highestNode, ok := cumulativeResponse["node"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing 'node' in response: %v", cumulativeResponse)
	}

	if highestNode["id"] != "1" {
		t.Fatalf("Expected highest cumulative weight node 1, got %v", highestNode["id"])
	}

	weightInfo, ok := cumulativeResponse["weight_info"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing 'weight_info' in response: %v", cumulativeResponse)
	}

	if weightInfo["direct_weight"] != float64(1) {
		t.Fatalf("Expected direct weight 1, got %v", weightInfo["direct_weight"])
	}

	if weightInfo["cumulative_weight"] != float64(2) {
		t.Fatalf("Expected cumulative weight 2, got %v", weightInfo["cumulative_weight"])
	}
}

func TestGetTipMCMC_WeightBasedSelection(t *testing.T) {
	router, _ := testServer()
	nodeA := map[string]interface{}{"id": "A", "parents": []string{}}
	nodeAJSON, _ := json.Marshal(nodeA)
	respCreateA := httptest.NewRecorder()
	router.ServeHTTP(respCreateA, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(nodeAJSON)))
	if respCreateA.Code != http.StatusCreated {
		t.Fatalf("Failed to add node A: %d", respCreateA.Code)
	}

	nodeB := map[string]interface{}{"id": "B", "parents": []string{"A"}}
	nodeBJSON, _ := json.Marshal(nodeB)
	respApproveB := httptest.NewRecorder()
	router.ServeHTTP(respApproveB, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeBJSON)))
	if respApproveB.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node B: %d", respApproveB.Code)
	}

	nodeC := map[string]interface{}{"id": "C", "parents": []string{"B"}}
	nodeCJSON, _ := json.Marshal(nodeC)
	respApproveC := httptest.NewRecorder()
	router.ServeHTTP(respApproveC, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeCJSON)))
	if respApproveC.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node C: %d", respApproveC.Code)
	}

	nodeD := map[string]interface{}{"id": "D", "parents": []string{"A"}}
	nodeDJSON, _ := json.Marshal(nodeD)
	respApproveD := httptest.NewRecorder()
	router.ServeHTTP(respApproveD, httptest.NewRequest(http.MethodPost, "/nodes/approve", bytes.NewReader(nodeDJSON)))
	if respApproveD.Code != http.StatusCreated {
		t.Fatalf("Failed to approve node D: %d", respApproveD.Code)
	}

	tipSelections := make(map[string]int)
	numSelections := 20

	for i := 0; i < numSelections; i++ {
		respTipSelection := httptest.NewRecorder()
		router.ServeHTTP(respTipSelection, httptest.NewRequest(http.MethodGet, "/nodes/tip-selection", nil))
		if respTipSelection.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d, body: %s", respTipSelection.Code, respTipSelection.Body.String())
		}

		var selectedTip models.Node
		if err := json.Unmarshal(respTipSelection.Body.Bytes(), &selectedTip); err != nil {
			t.Fatalf("Invalid JSON response: %v", err)
		}

		tipSelections[selectedTip.ID]++
	}

	// Verify that both tips C and D were selected (since they're both valid tips)
	if tipSelections["C"] == 0 {
		t.Fatalf("Tip C was never selected, got selections: %v", tipSelections)
	}
	if tipSelections["D"] == 0 {
		t.Fatalf("Tip D was never selected, got selections: %v", tipSelections)
	}

	// The MCMC should show some variation in selection, not always pick the same tip
	if tipSelections["C"] == numSelections || tipSelections["D"] == numSelections {
		t.Logf("Warning: MCMC always selected the same tip, selections: %v", tipSelections)
		t.Logf("This might indicate the algorithm is not properly randomizing, but it's not necessarily wrong")
	}

	t.Logf("Tip selection distribution: %v", tipSelections)
}

func TestCreateCheckpoint_Success(t *testing.T) {
	router, _ := testServer()

	// create two nodes
	for _, id := range []string{"A", "B"} {
		body := map[string]interface{}{"id": id, "parents": []string{}}
		b, _ := json.Marshal(body)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(b)))
		if resp.Code != http.StatusCreated {
			t.Fatalf("failed to create node %s: %d", id, resp.Code)
		}
	}

	// create checkpoint
	cpReqBody := map[string]string{"id": "cp1"}
	cpReqJSON, _ := json.Marshal(cpReqBody)
	respCP := httptest.NewRecorder()
	router.ServeHTTP(respCP, httptest.NewRequest(http.MethodPost, "/checkpoints", bytes.NewReader(cpReqJSON)))
	if respCP.Code != http.StatusCreated {
		t.Fatalf("expected 201 for checkpoint, got %d, body: %s", respCP.Code, respCP.Body.String())
	}

	var cp models.Checkpoint
	if err := json.Unmarshal(respCP.Body.Bytes(), &cp); err != nil {
		t.Fatalf("invalid checkpoint response: %v", err)
	}
	if cp.ID != "cp1" {
		t.Fatalf("expected checkpoint id cp1, got %s", cp.ID)
	}
	if cp.NodeCount != 2 {
		t.Fatalf("expected node_count 2, got %d", cp.NodeCount)
	}
	if cp.RootHash == "" {
		t.Fatalf("expected non-empty root_hash")
	}

	// latest should be cp1
	respLatest := httptest.NewRecorder()
	router.ServeHTTP(respLatest, httptest.NewRequest(http.MethodGet, "/checkpoints/latest", nil))
	if respLatest.Code != http.StatusOK {
		t.Fatalf("expected 200 for latest, got %d, body: %s", respLatest.Code, respLatest.Body.String())
	}
	var latest models.Checkpoint
	if err := json.Unmarshal(respLatest.Body.Bytes(), &latest); err != nil {
		t.Fatalf("invalid latest checkpoint response: %v", err)
	}
	if latest.ID != "cp1" {
		t.Fatalf("expected latest checkpoint cp1, got %s", latest.ID)
	}
}

func TestCreateCheckpoint_InvalidBody(t *testing.T) {
	router, _ := testServer()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/checkpoints", bytes.NewReader([]byte(`{}`))))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid body, got %d", resp.Code)
	}
}

func TestGetLatestCheckpoint_NoCheckpoint(t *testing.T) {
	router, _ := testServer()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/checkpoints/latest", nil))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when no checkpoints, got %d", resp.Code)
	}
}

func TestGetLatestCheckpoint_ReturnsLatest(t *testing.T) {
	router, _ := testServer()

	// create a node so node_count > 0
	body := map[string]interface{}{"id": "A", "parents": []string{}}
	b, _ := json.Marshal(body)
	respNode := httptest.NewRecorder()
	router.ServeHTTP(respNode, httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewReader(b)))
	if respNode.Code != http.StatusCreated {
		t.Fatalf("failed to create node: %d", respNode.Code)
	}

	// cp1
	cp1 := httptest.NewRecorder()
	router.ServeHTTP(cp1, httptest.NewRequest(http.MethodPost, "/checkpoints", bytes.NewReader([]byte(`{"id":"cp1"}`))))
	if cp1.Code != http.StatusCreated {
		t.Fatalf("expected 201 for cp1, got %d", cp1.Code)
	}

	// ensure different timestamp
	time.Sleep(2 * time.Millisecond)

	// cp2
	cp2 := httptest.NewRecorder()
	router.ServeHTTP(cp2, httptest.NewRequest(http.MethodPost, "/checkpoints", bytes.NewReader([]byte(`{"id":"cp2"}`))))
	if cp2.Code != http.StatusCreated {
		t.Fatalf("expected 201 for cp2, got %d", cp2.Code)
	}

	// latest should be cp2
	latest := httptest.NewRecorder()
	router.ServeHTTP(latest, httptest.NewRequest(http.MethodGet, "/checkpoints/latest", nil))
	if latest.Code != http.StatusOK {
		t.Fatalf("expected 200 for latest, got %d, body: %s", latest.Code, latest.Body.String())
	}
	var got models.Checkpoint
	if err := json.Unmarshal(latest.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid latest checkpoint response: %v", err)
	}
	if got.ID != "cp2" {
		t.Fatalf("expected latest checkpoint cp2, got %s", got.ID)
	}
}
