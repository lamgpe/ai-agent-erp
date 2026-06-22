package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMClient LLM客户端（兼容OpenAI协议）
type LLMClient struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// ChatMessage 对话消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest LLM请求
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatResponse LLM响应
type ChatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// EmbeddingRequest 向量化请求
type EmbeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// EmbeddingResponse 向量化响应
type EmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewLLMClient 创建LLM客户端
func NewLLMClient(apiKey, baseURL, model string) *LLMClient {
	// 确保baseURL以/v1结尾
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	return &LLMClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Chat 对话接口
func (c *LLMClient) Chat(messages []ChatMessage, temperature float64, maxTokens int) (string, error) {
	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("LLM请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("解析LLM响应失败: %w, body: %s", err, string(body))
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("LLM返回错误: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM返回空响应")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// GenerateEmbedding 生成文本向量
func (c *LLMClient) GenerateEmbedding(text string) ([]float64, error) {
	req := EmbeddingRequest{
		Model: c.model,
		Input: text,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Embedding使用专门的端点
	embedURL := strings.Replace(c.baseURL, "/v1", "/v1", 1)
	httpReq, err := http.NewRequest("POST", embedURL+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Embedding请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var embedResp EmbeddingResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("解析Embedding响应失败: %w", err)
	}

	if embedResp.Error != nil {
		return nil, fmt.Errorf("Embedding返回错误: %s", embedResp.Error.Message)
	}

	if len(embedResp.Data) == 0 {
		return nil, fmt.Errorf("Embedding返回空数据")
	}

	return embedResp.Data[0].Embedding, nil
}

// ExtractEntityFromJSON 从JSON数据中提取ERP实体（使用LLM结构化解析）
func (c *LLMClient) ExtractEntityFromJSON(entityType string, jsonData string) (map[string]interface{}, error) {
	systemPrompt := fmt.Sprintf(`你是一个ERP系统实体抽取专家。请分析以下%s类型的JSON数据，提取关键实体信息。

请返回严格的JSON格式，包含以下字段：
{
  "entities": [
    {
      "type": "实体类型(supplier/material_stock/purchase_order/logistics/finance_invoice)",
      "id": "实体唯一ID",
      "name": "实体名称",
      "properties": {  // 所有原始属性保留 },
      "relations": [
        {"relation": "关系名", "targetId": "关联实体ID", "targetType": "关联实体类型"}
      ]
    }
  ]
}`, entityType)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: jsonData},
	}

	response, err := c.Chat(messages, 0.3, 2000)
	if err != nil {
		// LLM不可用时，使用规则引擎
		return c.extractByRules(entityType, jsonData)
	}

	// 尝试提取JSON部分
	response = extractJSON(response)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		// LLM返回格式不对时使用规则引擎
		return c.extractByRules(entityType, jsonData)
	}

	return result, nil
}

// extractByRules 基于规则提取实体（LLM不可用时的fallback）
func (c *LLMClient) extractByRules(entityType string, jsonData string) (map[string]interface{}, error) {
	var rawData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &rawData); err != nil {
		// 尝试解析JSON数组
		var arrData []map[string]interface{}
		if err2 := json.Unmarshal([]byte(jsonData), &arrData); err2 != nil {
			return nil, fmt.Errorf("无法解析JSON: %w", err)
		}
		if len(arrData) > 0 {
			rawData = arrData[0]
		} else {
			return nil, fmt.Errorf("空数组")
		}
	}

	entity := map[string]interface{}{
		"type":       entityType,
		"properties": rawData,
	}

	// 根据类型提取ID和名称
	switch entityType {
	case "supplier":
		if id, ok := rawData["supplierId"]; ok {
			entity["id"] = id
		}
		if name, ok := rawData["name"]; ok {
			entity["name"] = name
		}
	case "material_stock":
		if id, ok := rawData["materialId"]; ok {
			entity["id"] = id
		}
		if name, ok := rawData["name"]; ok {
			entity["name"] = name
		}
	case "purchase_order":
		if id, ok := rawData["poId"]; ok {
			entity["id"] = id
		}
		if name, ok := rawData["orderNo"]; ok {
			entity["name"] = name
		}
	case "logistics":
		if id, ok := rawData["logId"]; ok {
			entity["id"] = id
		}
		if carrier, ok := rawData["carrier"]; ok {
			entity["name"] = carrier
		}
	case "finance_invoice":
		if id, ok := rawData["invoiceId"]; ok {
			entity["id"] = id
		}
		if no, ok := rawData["invoiceNo"]; ok {
			entity["name"] = no
		}
	default:
		entity["id"] = fmt.Sprintf("%v", rawData["id"])
		entity["name"] = fmt.Sprintf("%v", rawData["name"])
	}

	// 提取关联关系
	var relations []map[string]string
	switch entityType {
	case "material_stock":
		if supplierID, ok := rawData["supplierId"].(string); ok {
			relations = append(relations, map[string]string{
				"relation":   "供应",
				"targetId":   supplierID,
				"targetType": "supplier",
			})
		}
	case "purchase_order":
		if supplierID, ok := rawData["supplierId"].(string); ok {
			relations = append(relations, map[string]string{
				"relation":   "执行采购",
				"targetId":   supplierID,
				"targetType": "supplier",
			})
		}
		if materialID, ok := rawData["materialId"].(string); ok {
			relations = append(relations, map[string]string{
				"relation":   "采购物料",
				"targetId":   materialID,
				"targetType": "material_stock",
			})
		}
	case "logistics":
		if poID, ok := rawData["poId"].(string); ok {
			relations = append(relations, map[string]string{
				"relation":   "物流承运",
				"targetId":   poID,
				"targetType": "purchase_order",
			})
		}
	case "finance_invoice":
		if poID, ok := rawData["poId"].(string); ok {
			relations = append(relations, map[string]string{
				"relation":   "应付发票",
				"targetId":   poID,
				"targetType": "purchase_order",
			})
		}
	}

	entity["relations"] = relations

	result := map[string]interface{}{
		"entities": []interface{}{entity},
	}
	return result, nil
}

// AnalyzeBusinessQuery 分析业务查询（意图识别+领域分类）
func (c *LLMClient) AnalyzeBusinessQuery(query string) (map[string]interface{}, error) {
	systemPrompt := `你是一个ERP业务分析专家。分析用户的业务问题，识别业务领域，并给出检索策略。

请返回严格的JSON格式：
{
  "domain": "业务领域(supply_chain/procurement/finance/inventory/logistics)",
  "intent": "用户意图描述",
  "searchKeywords": ["关键词1", "关键词2"],
  "topK": 检索数量建议(注意：如果用户问"列出所有/有多少/全部/每家"，请设置topK=20或更大；一般分析类问题设置3-5即可),
  "graphDepth": 图谱遍历深度(1-3),
  "riskAlert": true/false 是否需要风险预警
}`

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: query},
	}

	response, err := c.Chat(messages, 0.3, 500)
	if err != nil {
		// 返回默认分析结果（根据查询类型调整topK）
		defaultTopK := 3
		ql := strings.ToLower(query)
		if strings.Contains(ql, "所有") || strings.Contains(ql, "全部") ||
			strings.Contains(ql, "列出") || strings.Contains(ql, "多少") ||
			strings.Contains(ql, "每家") || strings.Contains(ql, "有哪些") {
			defaultTopK = 20
		}
		return map[string]interface{}{
			"domain":         "general",
			"intent":         query,
			"searchKeywords": []string{query},
			"topK":           float64(defaultTopK),
			"graphDepth":     2,
			"riskAlert":      false,
		}, nil
	}

	response = extractJSON(response)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		defaultTopK := 3
		ql := strings.ToLower(query)
		if strings.Contains(ql, "所有") || strings.Contains(ql, "全部") ||
			strings.Contains(ql, "列出") || strings.Contains(ql, "多少") ||
			strings.Contains(ql, "每家") || strings.Contains(ql, "有哪些") {
			defaultTopK = 20
		}
		return map[string]interface{}{
			"domain":         "general",
			"intent":         query,
			"searchKeywords": []string{query},
			"topK":           float64(defaultTopK),
			"graphDepth":     2,
			"riskAlert":      false,
		}, nil
	}

	return result, nil
}

// GenerateJudgmentReport 生成研判报告
func (c *LLMClient) GenerateJudgmentReport(query string, vectorDocs []string, graphInfo string, traceLog []string, chatHistory []ChatMessage) (string, error) {
	systemPrompt := `你是一个用友 YouSuite ERP 分析专家。请根据用户问题的类型，选择最合适的回答方式：

【判断问题类型】
- 如果用户问的是"有哪些/列出/多少个/谁/哪家"等事实查询 → 直接给出数据清单，列出数量+明细
- 如果用户问的是"分析/评估/风险/建议/怎么办"等研判查询 → 使用报告格式

【事实查询回答格式】
先给出总数，再用表格或列表列出每一项的关键信息（名称、ID、核心属性）
示例：
"共找到3家供应铝合金板材6061-T6的供应商：

| 序号 | 供应商 | ID | 合同 | 风险 |
|------|--------|-----|------|------|
| 1 | 鑫达原材料 | sup001 | HT-PUR-2026001 | 高 |
| 2 | ... | ... | ... | ... |

关键数据：..."

【研判报告回答格式】
1. **问题分析**：用户问题的核心关切
2. **数据举证**：从数据中提取的关键信息
3. **关联链路**：涉及的实体上下游关系
4. **风险评估**：风险点及严重程度
5. **行动建议**：具体建议

请用专业、准确的语言，直接回答问题，不要添加不必要的格式修饰。`

	context := fmt.Sprintf(`用户问题: %s

检索到的相关实体:
%s

知识图谱关联链路:
%s`, query, strings.Join(vectorDocs, "\n"), graphInfo)

	// 构建完整消息列表：system + history + current context
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
	}
	// 插入最近6条历史对话（3轮问答）
	if len(chatHistory) > 0 {
		start := 0
		if len(chatHistory) > 6 {
			start = len(chatHistory) - 6
		}
		for _, h := range chatHistory[start:] {
			messages = append(messages, h)
		}
	}
	messages = append(messages, ChatMessage{Role: "user", Content: context})

	response, err := c.Chat(messages, 0.5, 2000)
	if err != nil {
		return c.generateFallbackReport(query, vectorDocs, graphInfo), nil
	}

	return response, nil
}

// generateFallbackReport 生成回退报告（LLM不可用时）
func (c *LLMClient) generateFallbackReport(query string, vectorDocs []string, graphInfo string) string {
	return fmt.Sprintf(`## 查询结果

针对查询「%s」，系统进行了语义检索和图谱分析，共检索到 %d 条相关实体记录。

### 数据详情
%s

注：当前未配置LLM API Key，无法进行AI智能分析。配置DeepSeek等API后即可获得完整研判报告。

---
*本报告由AI智能体ERP场景案例-数据ETL/模型推理/RAG知识库/知识图谱/自主研判一体化案例自动生成*`, query, len(vectorDocs), graphInfo)
}

// extractJSON 从LLM响应中提取JSON部分
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	// 移除markdown代码块标记
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
	}

	return strings.TrimSpace(text)
}

// ParseManualEntityText 解析手动录入的实体文本
func (c *LLMClient) ParseManualEntityText(entityType, description, relatedText string) (map[string]interface{}, error) {
	systemPrompt := fmt.Sprintf(`你是一个ERP实体录入助手。从以下文本中提取%s类型实体信息。

请返回严格的JSON格式：
{
  "type": "%s",
  "id": "自动生成唯一ID（格式：类型缩写+6位数字）",
  "name": "实体名称",
  "properties": {  // 提取的所有属性键值对 }
}`, entityType, entityType)

	userInput := fmt.Sprintf("实体描述: %s\n关联信息: %s", description, relatedText)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userInput},
	}

	response, err := c.Chat(messages, 0.3, 1000)
	if err != nil {
		// 回退：使用规则生成
		return map[string]interface{}{
			"type": entityType,
			"id":   fmt.Sprintf("manual_%s_%d", entityType[:3], time.Now().UnixNano()%1000000),
			"name": description,
			"properties": map[string]interface{}{
				"description": description,
				"relatedText": relatedText,
				"source":      "manual",
			},
		}, nil
	}

	response = extractJSON(response)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return map[string]interface{}{
			"type": entityType,
			"id":   fmt.Sprintf("manual_%s_%d", entityType[:3], time.Now().UnixNano()%1000000),
			"name": description,
			"properties": map[string]interface{}{
				"description": description,
				"relatedText": relatedText,
				"source":      "manual",
			},
		}, nil
	}

	return result, nil
}
