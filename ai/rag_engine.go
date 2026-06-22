package ai

import (
	"fmt"
	"strings"

	"zhonghonglian-erp-agent/graphdb"
	"zhonghonglian-erp-agent/vectorstore"
)

// RAGEngine RAG检索引擎
type RAGEngine struct {
	llmClient    *LLMClient
	chromaClient *vectorstore.ChromaClient
	boltGraph    *graphdb.BoltGraph
	collection   string
}

// NewRAGEngine 创建RAG引擎
func NewRAGEngine(llmClient *LLMClient, chromaClient *vectorstore.ChromaClient, boltGraph *graphdb.BoltGraph) *RAGEngine {
	return &RAGEngine{
		llmClient:    llmClient,
		chromaClient: chromaClient,
		boltGraph:    boltGraph,
		collection:   "erp_entities",
	}
}

// SetCollection 设置Chroma集合名
func (r *RAGEngine) SetCollection(name string) {
	r.collection = name
}

// Search 语义搜索相关实体
func (r *RAGEngine) Search(query string, topK int) ([]vectorstore.ChromaQueryResult, error) {
	// 生成查询向量
	embedding, err := r.llmClient.GenerateEmbedding(query)
	if err != nil {
		// Embedding不可用时，回退到关键词搜索
		return r.keywordSearch(query, topK), nil
	}

	// Chroma向量检索
	results, err := r.chromaClient.QueryDocuments(r.collection, embedding, topK)
	if err != nil {
		// Chroma不可用时，回退到关键词搜索
		return r.keywordSearch(query, topK), nil
	}

	return results, nil
}

// SearchCollection 搜索指定Chroma集合
func (r *RAGEngine) SearchCollection(collectionName, query string, topK int) ([]vectorstore.ChromaQueryResult, error) {
	embedding, err := r.llmClient.GenerateEmbedding(query)
	if err != nil {
		return nil, err
	}
	results, err := r.chromaClient.QueryDocuments(collectionName, embedding, topK)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// keywordSearch 关键词搜索（fallback方案，支持中文）
func (r *RAGEngine) keywordSearch(query string, topK int) []vectorstore.ChromaQueryResult {
	var results []vectorstore.ChromaQueryResult

	entities, err := r.boltGraph.GetAllEntities()
	if err != nil {
		return results
	}

	// 提取中文关键词：生成1-4字的滑动窗口作为搜索片段
	queryRunes := []rune(query)
	keywords := extractChineseKeywords(queryRunes)

	// 同时加入空格分隔的英文关键词
	spaceKeywords := strings.Fields(strings.ToLower(query))
	keywords = append(keywords, spaceKeywords...)

	// 去重
	seen := make(map[string]bool)
	var uniqueKeywords []string
	for _, kw := range keywords {
		if !seen[kw] && len(kw) >= 2 {
			seen[kw] = true
			uniqueKeywords = append(uniqueKeywords, kw)
		}
	}

	// 从查询中检测目标实体类型，大幅加权
	targetType := detectTargetEntityType(query)

	// 实体类型中文名映射（让中文查询能匹配到实体类型）
	typeNameCN := map[string]string{
		"supplier": "供应商", "material_stock": "物料库存",
		"purchase_contract": "采购合同", "purchase_order": "采购订单",
		"warehouse_receipt": "入库单", "production_order": "生产工单",
		"quality_inspection": "质检报告", "equipment": "设备资产",
		"logistics": "物流承运", "finance_invoice": "应付发票",
		"sales_order": "销售订单", "accounts_receivable": "应收账单",
	}

	for _, entity := range entities {
		// 构建实体全文（包含中文类型名，确保中文查询能匹配到类型）
		cnType := typeNameCN[entity.Type]
		if cnType == "" { cnType = entity.Type }
		entityText := fmt.Sprintf("%s %s %s %v", entity.Type, cnType, entity.Name, entity.Properties)
		entityLower := strings.ToLower(entityText)

		score := 0
		for _, kw := range uniqueKeywords {
			if strings.Contains(entityLower, kw) {
				score++
			}
		}

		// 额外加分：完整实体名匹配
		if strings.Contains(entityLower, strings.ToLower(entity.Name)) && len(entity.Name) > 0 {
			score += 2
		}

		// 实体类型加权：查询明确提到某类型时，该类型排最前
		if targetType != "" && entity.Type == targetType {
			score += 15
		}

		if score > 0 {
			results = append(results, vectorstore.ChromaQueryResult{
				ID:       entity.ID,
				Text:     entityText,
				Metadata: map[string]interface{}{"type": entity.Type, "id": entity.ID, "name": entity.Name},
				Distance: float64(1.0 / float64(score+1)),
			})
		}
	}


		// 也搜索知识库文档（从BoltDB日志中）
		logs, err := r.boltGraph.GetLogs(200)
		if err == nil {
			for _, l := range logs {
				if t, _ := l["type"].(string); t == "knowledge_upload" {
					if data, ok := l["data"].(map[string]interface{}); ok {
						kbText := fmt.Sprintf("%v %v", l["message"], data)
						kbLower := strings.ToLower(kbText)
						kbScore := 0
						for _, kw := range uniqueKeywords {
							if strings.Contains(kbLower, kw) {
								kbScore++
							}
						}
						if kbScore > 0 {
							docID := ""
							if did, ok := data["docId"].(string); ok { docID = did }
							results = append(results, vectorstore.ChromaQueryResult{
								ID:       docID,
								Text:     fmt.Sprintf("[知识库] %v", l["message"]),
								Metadata: map[string]interface{}{"type": "knowledge", "source": "upload"},
								Distance: float64(1.0 / float64(kbScore+1)),
							})
						}
					}
				}
			}
		}

	// 按匹配度排序（Distance越小越相关，score越大→Distance越小）
	sortResults(results)

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// extractChineseKeywords 从中文查询中提取关键词片段
func extractChineseKeywords(runes []rune) []string {
	var keywords []string
	n := len(runes)

	// 生成2-4字的滑动窗口
	for size := 4; size >= 2; size-- {
		for i := 0; i <= n-size; i++ {
			kw := string(runes[i : i+size])
			keywords = append(keywords, strings.ToLower(kw))
		}
	}

	// 也加入单字（如"鑫达"中的每个字也要能匹配）
	for _, r := range runes {
		keywords = append(keywords, strings.ToLower(string(r)))
	}

	return keywords
}


// detectTargetEntityType 从查询中检测用户想要查询的实体类型
func detectTargetEntityType(query string) string {
	ql := strings.ToLower(query)
	// 按优先级匹配：先匹配长词再匹配短词
	typeMap := []struct{ keyword, entityType string }{
		{"采购合同", "purchase_contract"},
		{"采购订单", "purchase_order"},
		{"入库单", "warehouse_receipt"},
		{"入库", "warehouse_receipt"},
		{"生产工单", "production_order"},
		{"工单", "production_order"},
		{"质检", "quality_inspection"},
		{"设备", "equipment"},
		{"物流", "logistics"},
		{"应付发票", "finance_invoice"},
		{"财务发票", "finance_invoice"},
		{"发票", "finance_invoice"},
		{"应收账单", "accounts_receivable"},
		{"应收", "accounts_receivable"},
		{"销售订单", "sales_order"},
		{"销售", "sales_order"},
		{"物料库存", "material_stock"},
		{"物料", "material_stock"},
		{"库存", "material_stock"},
		{"供应商", "supplier"},
	}
	for _, t := range typeMap {
		if strings.Contains(ql, t.keyword) {
			return t.entityType
		}
	}
	return ""
}

// sortResults 按Distance排序
func sortResults(results []vectorstore.ChromaQueryResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Distance > results[j].Distance {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// RetrieveWithGraph 完整的RAG+图谱检索
func (r *RAGEngine) RetrieveWithGraph(query string, topK int, graphDepth int) (*RetrievalResult, error) {
	result := &RetrievalResult{
		Query:        query,
		VectorDocs:   []string{},
		GraphTriples: []graphdb.EntityTriple{},
		RelatedEntities: []graphdb.Entity{},
	}

	// 1. 向量语义检索（实体 + 知识库）
	docs, err := r.Search(query, topK)
	// 同时检索知识库
	kbDocs, kbErr := r.SearchCollection("knowledge_base", query, topK)
	if kbErr == nil {
		for _, doc := range kbDocs {
			result.VectorDocs = append(result.VectorDocs, fmt.Sprintf("[知识库] %s", doc.Text))
		}
	}
	if err == nil {
		for _, doc := range docs {
			result.VectorDocs = append(result.VectorDocs, fmt.Sprintf("[%s] %s", doc.ID, doc.Text))
		}
	}

	// 2. 图谱遍历：对每个检索到的实体进行关联查询
	visited := make(map[string]bool)
	for _, doc := range docs {
		if doc.ID == "" {
			continue
		}
		triples, err := r.boltGraph.GetRelatedEntities(doc.ID, graphDepth)
		if err != nil {
			continue
		}
		for _, t := range triples {
			key := fmt.Sprintf("%s|%s|%s", t.HeadID, t.Relation, t.TailID)
			if !visited[key] {
				visited[key] = true
				result.GraphTriples = append(result.GraphTriples, t)
			}
		}

		// 获取实体详情
		if entityType, ok := doc.Metadata["type"].(string); ok {
			entity, err := r.boltGraph.GetEntity(entityType, doc.ID)
			if err == nil && entity != nil {
				result.RelatedEntities = append(result.RelatedEntities, *entity)
			}
		}
	}

	return result, nil
}

// RetrievalResult 检索结果
type RetrievalResult struct {
	Query           string
	VectorDocs      []string
	GraphTriples    []graphdb.EntityTriple
	RelatedEntities []graphdb.Entity
}

// FormatGraphInfo 格式化图谱信息为文本
func (r *RetrievalResult) FormatGraphInfo() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("查询: %s\n\n", r.Query))
	sb.WriteString("=== 相关实体 ===\n")
	for _, entity := range r.RelatedEntities {
		sb.WriteString(fmt.Sprintf("- [%s] %s (ID:%s)\n", entity.Type, entity.Name, entity.ID))
	}
	sb.WriteString("\n=== 实体关联链路 ===\n")
	for _, t := range r.GraphTriples {
		sb.WriteString(fmt.Sprintf("  %s --[%s]--> %s\n", t.HeadID, t.Relation, t.TailID))
	}
	return sb.String()
}

// GenerateReport 生成研判报告
func (r *RAGEngine) GenerateReport(query string, traceLog []string, chatHistory []ChatMessage) (string, error) {
	// 先分析业务查询
	analysis, err := r.llmClient.AnalyzeBusinessQuery(query)
	if err != nil {
		analysis = map[string]interface{}{"topK": 3, "graphDepth": 2}
	}

	topK := 3
	if v, ok := analysis["topK"].(float64); ok {
		topK = int(v)
	}
	graphDepth := 2
	if v, ok := analysis["graphDepth"].(float64); ok {
		graphDepth = int(v)
	}

	// 检索
	retrievalResult, err := r.RetrieveWithGraph(query, topK, graphDepth)
	if err != nil {
		retrievalResult = &RetrievalResult{Query: query}
	}

	// 生成报告
	report, err := r.llmClient.GenerateJudgmentReport(
		query,
		retrievalResult.VectorDocs,
		retrievalResult.FormatGraphInfo(),
		traceLog,
		chatHistory,
	)

	return report, err
}
