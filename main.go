package main

import (
	"bytes"
	"encoding/json"
	"time"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"zhonghonglian-erp-agent/ai"
	"zhonghonglian-erp-agent/api"
	"zhonghonglian-erp-agent/config"
	"zhonghonglian-erp-agent/frontend"
	"zhonghonglian-erp-agent/graphdb"
	"zhonghonglian-erp-agent/vectorstore"
	"zhonghonglian-erp-agent/ws"
)

func main() {
	// 打印Banner
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println("  AI智能体ERP场景案例-数据ETL/模型推理/RAG知识库/知识图谱/自主研判一体化案例 — Palantir情报研判平台")
	fmt.Println("  Go + Langchaingo | 轻量化完整落地方案")
	fmt.Println("  一键可测")
	fmt.Println("═══════════════════════════════════════════════════════")

	// 1. 加载配置
	cfg := config.DefaultConfig()
	fmt.Println("\n配置信息:")
	fmt.Printf("   LLM提供商: DeepSeek\n")
	fmt.Printf("   LLM模型: %s\n", cfg.LLMModel)
	fmt.Printf("   API地址: %s\n", cfg.LLMBaseURL)
	fmt.Printf("   Embedding模型: %s (注: DeepSeek不支持Embedding，将使用关键词检索)\n", cfg.EmbeddingModel)
	fmt.Printf("   Chroma地址: %s:%s\n", cfg.ChromaHost, cfg.ChromaPort)
	fmt.Printf("   服务端口: %s\n", cfg.ServerPort)
	fmt.Printf("   BoltDB路径: %s\n", cfg.BoltDBPath)

	// 确保data目录存在
	ensureDir("data")
	ensureDir("data/chroma-data")

	// 2. 初始化BoltDB图谱存储
	fmt.Println("\n初始化存储层...")
	boltGraph, err := graphdb.NewBoltGraph(cfg.BoltDBPath)
	if err != nil {
		log.Fatalf("初始化BoltDB失败: %v", err)
	}
	defer boltGraph.Close()
	fmt.Println("   [OK] BoltDB已就绪 (数据文件: " + cfg.BoltDBPath + ")")

	// 3. 初始化Chroma向量库客户端
	chromaClient := vectorstore.NewChromaClient(cfg.ChromaHost, cfg.ChromaPort)
	if err := chromaClient.HealthCheck(); err != nil {
		fmt.Printf("   [WARN] Chroma服务未启动 (%v)\n", err)
		fmt.Println("   提示: 请在另一个终端执行 chroma run --path ./data/chroma-data --host 127.0.0.1 --port 8000")
		fmt.Println("   系统将以降级模式运行（使用关键词搜索替代向量检索）")
	} else {
		fmt.Println("   [OK] Chroma向量服务已连接")
		// 创建ERP实体集合
		if err := chromaClient.CreateCollection("erp_entities"); err != nil {
			fmt.Printf("   [WARN] 创建Chroma集合失败: %v\n", err)
		} else {
			fmt.Println("   [OK] Chroma集合 'erp_entities' 已就绪")

			// 创建知识库集合
			if err := chromaClient.CreateCollection("knowledge_base"); err != nil {
				fmt.Printf("   [WARN] 创建Chroma集合 'knowledge_base' 失败: %v\n", err)
			} else {
				fmt.Println("   [OK] Chroma集合 'knowledge_base' 已就绪")
			}
		}
	}

	// 4. 初始化AI客户端
	fmt.Println("\n初始化AI层...")
	llmClient := ai.NewLLMClient(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel)
	embedClient := ai.NewLLMClient(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.EmbeddingModel)

	if cfg.LLMAPIKey == "" {
		fmt.Println("   [WARN] LLM API Key未设置")
		fmt.Println("   提示: 设置环境变量 export LLM_API_KEY=your-key")
		fmt.Println("   系统将以规则引擎模式运行（AI功能受限）")
	} else {
		fmt.Println("   [OK] LLM客户端已配置 (DeepSeek: " + cfg.LLMModel + ")")
		fmt.Println("   [INFO] DeepSeek不支持Embedding API，向量检索自动降级为关键词匹配")
	}

	// 使用embedClient进行向量化，llmClient进行对话
	// 将embedding模型设置到llmClient中用于向量化场景
	_ = embedClient // embedding专用客户端

	// 5. 初始化RAG引擎
	ragEngine := ai.NewRAGEngine(llmClient, chromaClient, boltGraph)

	// 6. 初始化API处理器
	mockDir := filepath.Join(".", "mock_json")
	// 检查mock_json目录
	if _, err := os.Stat(mockDir); os.IsNotExist(err) {
		// 尝试绝对路径
		execPath, _ := os.Executable()
		mockDir = filepath.Join(filepath.Dir(execPath), "mock_json")
	}
	fmt.Printf("\nMock数据目录: %s\n", mockDir)

	mockHandler := api.NewMockERPHandler(mockDir)
	entityAutoAPI := api.NewEntityAutoAPI(llmClient, chromaClient, boltGraph, mockDir)
	entityManualAPI := api.NewEntityManualAPI(llmClient, chromaClient, boltGraph)

	// 7. 初始化WebSocket Hub
	wsHub := ws.NewHub()
	go wsHub.Run()

	// 设置WebSocket消息处理器
	wsHub.SetMessageHandler(func(messageType string, data json.RawMessage) {
		switch messageType {
		case "chat":
			var chatReq ws.ChatRequest
			if err := json.Unmarshal(data, &chatReq); err != nil {
				return
			}
			// 异步处理研判请求
			go processJudgment(chatReq.Query, llmClient, ragEngine, boltGraph, wsHub)
		}
	})

	fmt.Println("   [OK] WebSocket Hub已启动")

	// 7.5 启动时自动加载Mock数据（如果数据库为空，同步执行）
	seedMockData(boltGraph, llmClient, chromaClient, mockDir)

	// 8. 注册HTTP路由
	mux := http.NewServeMux()

	// 前端页面
	mux.HandleFunc("/", frontend.GetFrontendHandler())
	// 前端JS文件
	mux.HandleFunc("/app.js", frontend.GetJSHandler())

	// Mock ERP API
	mux.Handle("/mock/erp/", mockHandler)

	// 手动录入v2（增强版）
	mux.HandleFunc("/api/manual/v2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != "POST" { http.Error(w, "POST only", 405); return }
		var req struct {
			EntityType string                   `json:"entityType"`
			Name       string                   `json:"name"`
			Properties map[string]interface{}   `json:"properties"`
			Relations  []map[string]interface{} `json:"relations"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		entityID := fmt.Sprintf("man_%s_%d", req.EntityType[:3], time.Now().UnixNano()%1000000)
		entity := graphdb.Entity{
			ID: entityID, Type: req.EntityType, Name: req.Name,
			Properties: req.Properties, Source: "manual",
		}
		boltGraph.SaveEntity(entity)
		// 建立自定义关系
		for _, rel := range req.Relations {
			relation, _ := rel["relation"].(string)
			targetID, _ := rel["targetId"].(string)
			if relation != "" && targetID != "" {
				boltGraph.AddTriple(graphdb.EntityTriple{
					HeadID: entityID, Relation: relation, TailID: targetID,
				})
			}
		}
		boltGraph.SaveLog("manual_entry_v2", req.Name, map[string]interface{}{"entityId": entityID})
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "已创建: " + req.Name, "entityId": entityID})
	})

	// 实体API
	mux.HandleFunc("/api/sync", entityAutoAPI.HandleSync)
	mux.HandleFunc("/api/sync/batch", entityAutoAPI.HandleBatchSync)
	mux.HandleFunc("/api/manual", entityManualAPI.HandleManualCreate)
	mux.HandleFunc("/api/entities", entityAutoAPI.HandleGetEntities)
	mux.HandleFunc("/api/graph", entityAutoAPI.HandleGetGraph)

	// Chat API (HTTP fallback)
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var chatReq ws.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误"})
			return
		}

		// 同步处理（HTTP模式下）
		report, steps, err := processJudgmentSync(chatReq.Query, llmClient, ragEngine, boltGraph)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"report": report,
			"steps":  steps,
		})
	})

	// 统计API
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		entities, _ := boltGraph.GetAllEntities()
		triples, _ := boltGraph.GetAllTriples()
		chatCount := 0
		// 统计聊天消息数量
		if history, err := boltGraph.GetChatHistory(1000); err == nil {
			chatCount = len(history)
		}
		// 统计知识库文档数
		kbDocCount := 0
		if logs, err := boltGraph.GetLogs(1000); err == nil {
			for _, l := range logs {
				if t, _ := l["type"].(string); t == "knowledge_upload" {
					kbDocCount++
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"totalEntities": len(entities),
			"totalTriples":  len(triples),
			"totalDocs":     len(entities) + kbDocCount,
			"chatSessions":  chatCount / 2,
		})
	})

	// 低代码平台API
	mux.HandleFunc("/api/lowcode/entities", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		entities, _ := boltGraph.GetAllEntities()
		// 按类型分组，每类型取1条展示属性
		typeAttrs := map[string]map[string]interface{}{}
		for _, e := range entities {
			if _, ok := typeAttrs[e.Type]; !ok {
				typeAttrs[e.Type] = map[string]interface{}{
					"typeName":   e.Type,
					"sampleId":   e.ID,
					"sampleName": e.Name,
					"attributes": getAttributeList(e.Properties),
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"entityTypes": typeAttrs})
	})

	mux.HandleFunc("/api/lowcode/generate-form", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		var req struct {
			EntityType string   `json:"entityType"`
			Attributes []string `json:"attributes"`
			FormName   string   `json:"formName"`
			Approvers  []string `json:"approvers"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		// 生成表单JSON
		form := map[string]interface{}{
			"formId":      fmt.Sprintf("form_%d", time.Now().UnixNano()),
			"formName":    req.FormName,
			"entityType":  req.EntityType,
			"fields":      buildFormFields(req.EntityType, req.Attributes, boltGraph),
			"workflow":    buildApprovalFlow(req.Approvers),
			"generatedAt": time.Now().Format("2006-01-02 15:04:05"),
		}
		// 存储到BoltDB
		boltGraph.SaveLog("generated_form", req.FormName, form)
		json.NewEncoder(w).Encode(form)
	})

	// 低代码表单列表
	mux.HandleFunc("/api/lowcode/forms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		logs, _ := boltGraph.GetLogs(100)
		var forms []map[string]interface{}
		for _, l := range logs {
			if t, _ := l["type"].(string); t == "generated_form" {
				forms = append(forms, l)
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"forms": forms, "count": len(forms)})
	})

	// 知识库API
	mux.HandleFunc("/api/knowledge/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != "POST" { http.Error(w, "POST only", 405); return }
		r.ParseMultipartForm(10 << 20) // 10MB
		file, header, err := r.FormFile("file")
		if err != nil { json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); return }
		defer file.Close()
		buf := new(bytes.Buffer)
		buf.ReadFrom(file)
		content := buf.String()
		docID := fmt.Sprintf("kb_%d", time.Now().UnixNano())
		// 向量化（尝试）
		if embedding, err := llmClient.GenerateEmbedding(content); err == nil && len(embedding) > 0 {
			chromaClient.AddDocuments("knowledge_base", []vectorstore.ChromaDocument{{
				ID: docID, Text: content,
				Metadata: map[string]interface{}{"filename": header.Filename, "source": "upload", "type": "knowledge"},
				Vector: embedding,
			}})
		}
		// 同时存到BoltDB
		boltGraph.SaveLog("knowledge_upload", header.Filename, map[string]interface{}{"docId": docID, "size": len(content)})
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "docId": docID, "filename": header.Filename, "size": len(content)})
	})


	mux.HandleFunc("/api/knowledge/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		docs, _ := boltGraph.GetKnowledgeDocs()
		json.NewEncoder(w).Encode(map[string]interface{}{"documents": docs, "count": len(docs)})
	})


		// 知识库清空
		mux.HandleFunc("/api/knowledge/clear", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			if r.Method != "POST" { http.Error(w, "POST only", 405); return }
			chromaClient.DeleteCollection("knowledge_base")
			chromaClient.CreateCollection("knowledge_base")
			boltGraph.ClearKnowledgeLogs()
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "知识库已清空"})
		})

	// WebSocket
	mux.HandleFunc("/ws", wsHub.HandleWebSocket)

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "ok",
			"service":     "AI智能体ERP场景案例-数据ETL/模型推理/RAG知识库/知识图谱/自主研判一体化案例",
			"ws_clients":  wsHub.ClientCount(),
			"bolt_db":     "connected",
			"llm_api_key": cfg.LLMAPIKey != "",
		})
	})

	// 9. 启动HTTP服务
	fmt.Println("\n" + "═" + "══════════════════════════════════════════════════════")
	fmt.Printf("服务启动: http://127.0.0.1:%s\n", cfg.ServerPort)
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("快速开始:")
	fmt.Println("   1. 浏览器打开: http://127.0.0.1:" + cfg.ServerPort)
	fmt.Println("   2. 启动自动加载240条YonSuite全链路演示数据（12类实体）")
	fmt.Println("   3. 在Chat中输入业务问题，体验12层全链路研判")
	fmt.Println()
	fmt.Println("Mock API示例:")
	fmt.Println("   curl http://127.0.0.1:" + cfg.ServerPort + "/mock/erp/supplier")
	fmt.Println("   curl http://127.0.0.1:" + cfg.ServerPort + "/mock/erp/purchase_order")
	fmt.Println()
	fmt.Println("不依赖Chroma也能运行（规则引擎降级模式）")
	fmt.Println("═══════════════════════════════════════════════════════")

	if err := http.ListenAndServe(":"+cfg.ServerPort, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// processJudgment 异步处理研判请求（WebSocket模式）
func processJudgment(query string, llmClient *ai.LLMClient, ragEngine *ai.RAGEngine, boltGraph *graphdb.BoltGraph, wsHub *ws.Hub) {
	// 发送思考中状态
	wsHub.BroadcastJSON(ws.MsgTypeSystem, fmt.Sprintf("正在研判: %s", query))

	// 创建决策追踪
	traceID := fmt.Sprintf("trace_%d", len(query))
	trace := ai.NewDecisionTrace(traceID, query)

	// 设置每步回调，实时推送到前端
	trace.SetCallback(func(step ai.DecisionStep) {
		wsHub.BroadcastJSON(ws.MsgTypeTrace, step)
	})

	// 设置图谱更新回调，每个Palantir阶段实时推送图谱增量
	trace.SetGraphCallback(func(update ai.GraphUpdate) {
		wsHub.BroadcastJSON(ws.MsgTypeGraph, update)
		// 同时推送阶段描述到聊天区
		phaseLabel := map[string]string{
			"retrieval":  "[检索阶段]",
			"traversal":  "[遍历阶段]",
			"prediction": "[预判阶段]",
		}
		label := phaseLabel[update.Phase]
		if label == "" {
			label = "[图谱更新]"
		}
		wsHub.BroadcastJSON(ws.MsgTypeSystem,
			fmt.Sprintf("%s %s", label, update.Description))
	})

	// 运行完整研判流程
	report, steps, err := trace.RunFullDecisionPipeline(ragEngine, query)
	if err != nil {
		wsHub.BroadcastJSON(ws.MsgTypeSystem, fmt.Sprintf("[ERR] 研判出错: %v", err))
		return
	}

	// 发送报告到前端（仅report类型，前端自动markdown渲染）
	wsHub.BroadcastJSON(ws.MsgTypeReport, report)

	// 保存聊天记录
	boltGraph.SaveChatMessage("user", query)
	boltGraph.SaveChatMessage("assistant", report)

	log.Printf("研判完成: %s, 共%d步", query, len(steps))
}

// processJudgmentSync 同步处理研判请求（HTTP模式）
func processJudgmentSync(query string, llmClient *ai.LLMClient, ragEngine *ai.RAGEngine, boltGraph *graphdb.BoltGraph) (string, []ai.DecisionStep, error) {
	traceID := fmt.Sprintf("trace_http_%d", len(query))
	trace := ai.NewDecisionTrace(traceID, query)

	report, steps, err := trace.RunFullDecisionPipeline(ragEngine, query)
	if err != nil {
		return "", nil, err
	}

	// 保存聊天记录
	boltGraph.SaveChatMessage("user", query)
	boltGraph.SaveChatMessage("assistant", report)

	return report, steps, nil
}

// ensureDir 确保目录存在
func ensureDir(path string) {
	if err := os.MkdirAll(path, 0755); err != nil {
		log.Printf("创建目录失败 %s: %v", path, err)
	}
}

// seedMockData 启动时自动加载Mock数据（如果数据库为空）
func seedMockData(boltGraph *graphdb.BoltGraph, llmClient *ai.LLMClient, chromaClient *vectorstore.ChromaClient, mockDir string) {
	entities, _ := boltGraph.GetAllEntities()
	if len(entities) > 0 {
		fmt.Printf("   [OK] 数据库已有 %d 条实体，跳过自动加载\n", len(entities))
		return
	}

	fmt.Println("   自动加载Mock数据...")
	allTypes := []string{"supplier", "material_stock", "purchase_contract", "purchase_order", "warehouse_receipt", "production_order", "quality_inspection", "equipment", "logistics", "finance_invoice", "sales_order", "accounts_receivable"}
	totalEntities := 0
	totalTriples := 0

	for _, entityType := range allTypes {
		filePath := filepath.Join(mockDir, entityType+".json")
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("   [WARN] 读取 %s 失败: %v\n", filePath, err)
			continue
		}

		var items []map[string]interface{}
		if err := json.Unmarshal(data, &items); err != nil {
			fmt.Printf("   [WARN] 解析 %s 失败: %v\n", filePath, err)
			continue
		}

		for _, item := range items {
			entityID := getEntityIDFromItem(entityType, item)
			entityName := getEntityNameFromItem(entityType, item)

			entity := graphdb.Entity{
				ID:         entityID,
				Type:       entityType,
				Name:       entityName,
				Properties: item,
				Source:     "auto",
			}
			if err := boltGraph.SaveEntity(entity); err != nil {
				continue
			}
			totalEntities++

			if err := boltGraph.SyncEntityRelations(entity); err == nil {
				totalTriples++
			}
		}
	}

	boltGraph.SaveLog("auto_seed", "启动自动加载Mock数据", map[string]interface{}{
		"entities": totalEntities,
		"triples":  totalTriples,
	})

	fmt.Printf("   [OK] 自动加载完成: %d 条实体, %d 条关系\n", totalEntities, totalTriples)
}

// getEntityIDFromItem 从mock数据条目中提取实体ID
func getEntityIDFromItem(entityType string, item map[string]interface{}) string {
	keyMap := map[string]string{
		"supplier": "supplierId", "material_stock": "materialId",
		"purchase_contract": "contractId", "purchase_order": "poId",
		"warehouse_receipt": "receiptId", "production_order": "orderId",
		"quality_inspection": "inspectionId", "equipment": "equipId",
		"logistics": "logId", "finance_invoice": "invoiceId",
		"sales_order": "orderId", "accounts_receivable": "receivableId",
	}
	key := keyMap[entityType]
	if key == "" {
		key = "id"
	}
	if v, ok := item[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return fmt.Sprintf("unknown_%d", len(item))
}

// getEntityNameFromItem 从mock数据条目中提取实体名称
// getAttributeList 从属性map中提取字段名列表
func getAttributeList(props map[string]interface{}) []map[string]string {
	var attrs []map[string]string
	for k, v := range props {
		attrs = append(attrs, map[string]string{"name": k, "sampleValue": fmt.Sprintf("%v", v)})
	}
	return attrs
}

// buildFormFields 构建表单字段定义
func buildFormFields(entityType string, selectedAttrs []string, boltGraph *graphdb.BoltGraph) []map[string]interface{} {
	var fields []map[string]interface{}
	entities, _ := boltGraph.GetAllEntities()
	for _, e := range entities {
		if e.Type == entityType {
			for _, attr := range selectedAttrs {
				if val, ok := e.Properties[attr]; ok {
					fieldType := "text"
					switch val.(type) {
					case float64: fieldType = "number"
					case bool: fieldType = "checkbox"
					}
					fields = append(fields, map[string]interface{}{
						"name": attr, "label": attr, "type": fieldType, "sampleValue": val,
					})
				}
			}
			break
		}
	}
	return fields
}

// buildApprovalFlow 构建审批流程定义
func buildApprovalFlow(approvers []string) map[string]interface{} {
	nodes := []map[string]interface{}{}
	for i, a := range approvers {
		role := "审批节点"
		if i == 0 { role = "提交人" }
		if i == len(approvers)-1 { role = "终审人" }
		nodes = append(nodes, map[string]interface{}{
			"step": i + 1, "role": role, "approver": a, "action": "approve_or_reject",
		})
	}
	return map[string]interface{}{"nodes": nodes, "totalSteps": len(nodes)}
}

func getEntityNameFromItem(entityType string, item map[string]interface{}) string {
	nameMap := map[string]string{
		"supplier": "name", "material_stock": "name",
		"purchase_contract": "contractNo", "purchase_order": "orderNo",
		"warehouse_receipt": "receiptNo", "production_order": "orderNo",
		"quality_inspection": "inspectionNo", "equipment": "name",
		"logistics": "carrier", "finance_invoice": "invoiceNo",
		"sales_order": "orderNo", "accounts_receivable": "receivableNo",
	}
	key := nameMap[entityType]
	if key == "" {
		key = "name"
	}
	if v, ok := item[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return "unknown"
}
