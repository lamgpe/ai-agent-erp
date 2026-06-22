package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"zhonghonglian-erp-agent/ai"
	"zhonghonglian-erp-agent/graphdb"
	"zhonghonglian-erp-agent/vectorstore"
)

// EntityAutoAPI 自动API拉取生成实体处理器
type EntityAutoAPI struct {
	llmClient    *ai.LLMClient
	chromaClient *vectorstore.ChromaClient
	boltGraph    *graphdb.BoltGraph
	mockDir      string
}

// NewEntityAutoAPI 创建自动实体API
func NewEntityAutoAPI(llmClient *ai.LLMClient, chromaClient *vectorstore.ChromaClient, boltGraph *graphdb.BoltGraph, mockDir string) *EntityAutoAPI {
	return &EntityAutoAPI{
		llmClient:    llmClient,
		chromaClient: chromaClient,
		boltGraph:    boltGraph,
		mockDir:      mockDir,
	}
}

// SyncRequest 同步请求
type SyncRequest struct {
	EntityType string `json:"entityType"` // supplier/material_stock/purchase_order/logistics/finance_invoice
	SourceURL  string `json:"sourceUrl"`  // 可选，自定义API地址
}

// SyncResponse 同步响应
type SyncResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	EntityCount  int    `json:"entityCount"`
	TripleCount  int    `json:"tripleCount"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// HandleSync 处理自动同步请求
func (ea *EntityAutoAPI) HandleSync(w http.ResponseWriter, r *http.Request) {
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

	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, SyncResponse{Success: false, ErrorMessage: "请求格式错误"})
		return
	}

	if req.EntityType == "" {
		writeJSON(w, SyncResponse{Success: false, ErrorMessage: "请指定entityType"})
		return
	}

	// 从本地mock JSON文件读取数据（不再依赖HTTP端口）
	var body []byte
	var err error
	if req.SourceURL != "" {
		resp, httpErr := http.Get(req.SourceURL)
		if httpErr == nil {
			defer resp.Body.Close()
			body, _ = io.ReadAll(resp.Body)
		}
	}
	if body == nil {
		filePath := filepath.Join(ea.mockDir, req.EntityType+".json")
		body, err = os.ReadFile(filePath)
		if err != nil {
			writeJSON(w, SyncResponse{
				Success:      false,
				Message:      fmt.Sprintf("读取本地数据文件失败: %v", err),
				ErrorMessage: err.Error(),
			})
			return
		}
	}

	jsonStr := string(body)

	// 使用AI解析实体（尝试LLM增强解析，失败不影响流程）
	_, err = ea.llmClient.ExtractEntityFromJSON(req.EntityType, jsonStr)
	if err != nil {
		// LLM解析失败不影响后续处理，使用规则引擎作为fallback
		log.Printf("LLM实体解析失败(将使用规则引擎): %v", err)
	}

	// 解析为数组（支持单条和多条）
	var rawItems []map[string]interface{}
	if err := json.Unmarshal(body, &rawItems); err != nil {
		// 尝试作为单条解析
		var singleItem map[string]interface{}
		if err2 := json.Unmarshal(body, &singleItem); err2 != nil {
			writeJSON(w, SyncResponse{Success: false, ErrorMessage: "JSON解析失败"})
			return
		}
		rawItems = []map[string]interface{}{singleItem}
	}

	entityCount := 0
	tripleCount := 0

	for _, rawItem := range rawItems {
		// 获取实体ID和名称
		entityID := getEntityID(req.EntityType, rawItem)
		entityName := getEntityName(req.EntityType, rawItem)

		// 保存实体到BoltDB
		entity := graphdb.Entity{
			ID:         entityID,
			Type:       req.EntityType,
			Name:       entityName,
			Properties: rawItem,
			Source:     "auto",
		}
		if err := ea.boltGraph.SaveEntity(entity); err != nil {
			continue
		}
		entityCount++

		// 建立三元组关系
		if err := ea.boltGraph.SyncEntityRelations(entity); err == nil {
			tripleCount++
		}

		// 向量化并存入Chroma（如果可用）
		entityText := fmt.Sprintf("[%s] %s: %v", req.EntityType, entityName, rawItem)
		embedding, err := ea.llmClient.GenerateEmbedding(entityText)
		if err == nil && len(embedding) > 0 {
			doc := vectorstore.ChromaDocument{
				ID:   fmt.Sprintf("%s_%s", req.EntityType, entityID),
				Text: entityText,
				Metadata: map[string]interface{}{
					"type":   req.EntityType,
					"id":     entityID,
					"name":   entityName,
					"source": "auto",
				},
				Vector: embedding,
			}
			ea.chromaClient.AddDocuments("erp_entities", []vectorstore.ChromaDocument{doc})
		}
	}

	// 记录操作日志
	ea.boltGraph.SaveLog("entity_sync", fmt.Sprintf("API同步: %s", req.EntityType), map[string]interface{}{
		"entityType": req.EntityType,
		"count":      entityCount,
	})

	writeJSON(w, SyncResponse{
		Success:     true,
		Message:     fmt.Sprintf("成功同步 %s: %d条实体, %d条关系", req.EntityType, entityCount, tripleCount),
		EntityCount: entityCount,
		TripleCount: tripleCount,
	})
}

// getEntityID 从原始数据提取实体ID
func getEntityID(entityType string, data map[string]interface{}) string {
	switch entityType {
	case "supplier":
		return getStringField(data, "supplierId")
	case "material_stock":
		return getStringField(data, "materialId")
	case "purchase_order":
		return getStringField(data, "poId")
	case "logistics":
		return getStringField(data, "logId")
	case "finance_invoice":
		return getStringField(data, "invoiceId")
	default:
		return getStringField(data, "id")
	}
}

// getEntityName 从原始数据提取实体名称
func getEntityName(entityType string, data map[string]interface{}) string {
	switch entityType {
	case "supplier":
		return getStringField(data, "name")
	case "material_stock":
		return getStringField(data, "name")
	case "purchase_order":
		return getStringField(data, "orderNo")
	case "logistics":
		return getStringField(data, "carrier")
	case "finance_invoice":
		return getStringField(data, "invoiceNo")
	default:
		return getStringField(data, "name")
	}
}

func getStringField(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// HandleBatchSync 批量同步所有实体（同步前清除旧数据，非累加）
func (ea *EntityAutoAPI) HandleBatchSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 清除旧实体和三元组（保留日志和聊天记录）
	if err := ea.boltGraph.ClearEntitiesAndTriples(); err != nil {
		writeJSON(w, map[string]interface{}{"success": false, "error": "清除旧数据失败: " + err.Error()})
		return
	}

	allTypes := []string{"supplier", "material_stock", "purchase_contract", "purchase_order", "warehouse_receipt", "production_order", "quality_inspection", "equipment", "logistics", "finance_invoice", "sales_order", "accounts_receivable"}

	var results []SyncResponse
	totalEntities := 0

	for _, entityType := range allTypes {

			// 直接从本地mock文件读取
			filePath := filepath.Join(ea.mockDir, entityType+".json")
			body, readErr := os.ReadFile(filePath)

			if readErr != nil {
				results = append(results, SyncResponse{Success: false, Message: entityType, ErrorMessage: "读取文件失败: " + readErr.Error()})
				continue
			}


		var rawItems []map[string]interface{}
		if err := json.Unmarshal(body, &rawItems); err != nil {
			results = append(results, SyncResponse{Success: false, Message: entityType, ErrorMessage: "JSON解析失败"})
			continue
		}

		count := 0
		for _, rawItem := range rawItems {
			entityID := getEntityID(entityType, rawItem)
			entityName := getEntityName(entityType, rawItem)

			entity := graphdb.Entity{
				ID:         entityID,
				Type:       entityType,
				Name:       entityName,
				Properties: rawItem,
				Source:     "auto",
			}
			if err := ea.boltGraph.SaveEntity(entity); err != nil {
				continue
			}
			ea.boltGraph.SyncEntityRelations(entity)
			count++
		}
		totalEntities += count
		results = append(results, SyncResponse{
			Success:     true,
			Message:     fmt.Sprintf("%s同步完成", entityType),
			EntityCount: count,
		})
	}

	ea.boltGraph.SaveLog("batch_sync", "批量同步完成", map[string]interface{}{"totalEntities": totalEntities})

	writeJSON(w, map[string]interface{}{
		"success":       true,
		"totalEntities": totalEntities,
		"details":       results,
	})
}

// HandleGetEntities 获取已入库的实体列表
func (ea *EntityAutoAPI) HandleGetEntities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	entities, err := ea.boltGraph.GetAllEntities()
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{
		"entities": entities,
		"count":    len(entities),
	})
}

// HandleGetGraph 获取知识图谱数据（供D3前端渲染）
func (ea *EntityAutoAPI) HandleGetGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	entities, err := ea.boltGraph.GetAllEntities()
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	triples, err := ea.boltGraph.GetAllTriples()
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	// 构建D3图数据格式
	nodes := []map[string]interface{}{}
	links := []map[string]interface{}{}
	nodeSet := make(map[string]bool)

	for _, entity := range entities {
		if !nodeSet[entity.ID] {
			nodeSet[entity.ID] = true
			nodes = append(nodes, map[string]interface{}{
				"id":    entity.ID,
				"name":  entity.Name,
				"type":  entity.Type,
				"group": getGroup(entity.Type),
			})
		}
	}

	for _, triple := range triples {
		links = append(links, map[string]interface{}{
			"source":   triple.HeadID,
			"target":   triple.TailID,
			"relation": triple.Relation,
			"type":     "solid", // 实线：已入库关系
		})
	}

	writeJSON(w, map[string]interface{}{
		"nodes": nodes,
		"links": links,
	})
}

// getGroup 根据实体类型返回D3分组编号
func getGroup(entityType string) int {
	switch entityType {
	case "supplier":
		return 1 // 红
	case "material_stock":
		return 2 // 绿
	case "purchase_contract":
		return 6 // 青
	case "purchase_order":
		return 3 // 蓝
	case "warehouse_receipt":
		return 7 // 橙
	case "production_order":
		return 8 // 粉
	case "quality_inspection":
		return 9 // 灰
	case "equipment":
		return 10 // 棕
	case "logistics":
		return 4 // 黄
	case "finance_invoice":
		return 5 // 紫
	case "sales_order":
		return 11 // 深绿
	case "accounts_receivable":
		return 12 // 深红
	default:
		return 0
	}
}

// Contains 辅助函数
func Contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
