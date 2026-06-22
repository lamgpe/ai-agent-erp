package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"zhonghonglian-erp-agent/ai"
	"zhonghonglian-erp-agent/graphdb"
	"zhonghonglian-erp-agent/vectorstore"
)

// EntityManualAPI 手动录入实体处理器
type EntityManualAPI struct {
	llmClient    *ai.LLMClient
	chromaClient *vectorstore.ChromaClient
	boltGraph    *graphdb.BoltGraph
}

// NewEntityManualAPI 创建手动录入API
func NewEntityManualAPI(llmClient *ai.LLMClient, chromaClient *vectorstore.ChromaClient, boltGraph *graphdb.BoltGraph) *EntityManualAPI {
	return &EntityManualAPI{
		llmClient:    llmClient,
		chromaClient: chromaClient,
		boltGraph:    boltGraph,
	}
}

// ManualEntityRequest 手动录入请求
type ManualEntityRequest struct {
	EntityType  string `json:"entityType"`  // supplier/material_stock/purchase_order/logistics/finance_invoice
	Description string `json:"description"` // 实体描述
	RelatedText string `json:"relatedText"` // 关联信息
}

// ManualEntityResponse 手动录入响应
type ManualEntityResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	EntityID  string `json:"entityId,omitempty"`
	EntityName string `json:"entityName,omitempty"`
}

// HandleManualCreate 处理手动创建实体
func (em *EntityManualAPI) HandleManualCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ManualEntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ManualEntityResponse{Success: false, Message: "请求格式错误"})
		return
	}

	if req.EntityType == "" || req.Description == "" {
		writeJSON(w, ManualEntityResponse{Success: false, Message: "entityType和description为必填项"})
		return
	}

	// 使用AI解析手动录入文本
	parseResult, err := em.llmClient.ParseManualEntityText(req.EntityType, req.Description, req.RelatedText)
	if err != nil {
		writeJSON(w, ManualEntityResponse{Success: false, Message: fmt.Sprintf("实体解析失败: %v", err)})
		return
	}

	entityID, _ := parseResult["id"].(string)
	if entityID == "" {
		entityID = fmt.Sprintf("manual_%s_%d", req.EntityType[:min(3, len(req.EntityType))], time.Now().UnixNano()%1000000)
	}

	entityName, _ := parseResult["name"].(string)
	if entityName == "" {
		entityName = req.Description
	}

	properties, _ := parseResult["properties"].(map[string]interface{})
	if properties == nil {
		properties = map[string]interface{}{
			"description": req.Description,
			"relatedText": req.RelatedText,
		}
	}
	properties["source"] = "manual"
	properties["createdAt"] = time.Now().Format(time.RFC3339)

	// 保存实体到BoltDB
	entity := graphdb.Entity{
		ID:         entityID,
		Type:       req.EntityType,
		Name:       entityName,
		Properties: properties,
		Source:     "manual",
	}
	if err := em.boltGraph.SaveEntity(entity); err != nil {
		writeJSON(w, ManualEntityResponse{Success: false, Message: fmt.Sprintf("保存实体失败: %v", err)})
		return
	}

	// 建立三元组关系（手动录入的也尝试建立关联）
	em.boltGraph.SyncEntityRelations(entity)

	// 向量化并存入Chroma（如果可用）
	entityText := fmt.Sprintf("[%s] %s: %s | 关联: %s", req.EntityType, entityName, req.Description, req.RelatedText)
	embedding, err := em.llmClient.GenerateEmbedding(entityText)
	if err == nil && len(embedding) > 0 {
		doc := vectorstore.ChromaDocument{
			ID:   fmt.Sprintf("%s_%s", req.EntityType, entityID),
			Text: entityText,
			Metadata: map[string]interface{}{
				"type":        req.EntityType,
				"id":          entityID,
				"name":        entityName,
				"source":      "manual",
				"description": req.Description,
			},
			Vector: embedding,
		}
		em.chromaClient.AddDocuments("erp_entities", []vectorstore.ChromaDocument{doc})
	}

	// 记录操作日志
	em.boltGraph.SaveLog("manual_entry", fmt.Sprintf("手动录入: %s - %s", req.EntityType, entityName), map[string]interface{}{
		"entityType":  req.EntityType,
		"entityId":    entityID,
		"description": req.Description,
	})

	writeJSON(w, ManualEntityResponse{
		Success:    true,
		Message:    fmt.Sprintf("成功创建%s实体: %s", req.EntityType, entityName),
		EntityID:   entityID,
		EntityName: entityName,
	})
}
