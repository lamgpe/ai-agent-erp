package ai

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"zhonghonglian-erp-agent/graphdb"
)

// DecisionStep AI推理步骤
type DecisionStep struct {
	StepID      int      `json:"stepId"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"` // pending/running/done/error
	StartTime   string   `json:"startTime,omitempty"`
	EndTime     string   `json:"endTime,omitempty"`
	Result      string   `json:"result,omitempty"`
	NextPlan    []string `json:"nextPlan,omitempty"`   // AI计划的后续步骤
	BranchOpts  []string `json:"branchOpts,omitempty"` // 多分支可选路径
}

// GraphUpdate 图谱增量更新（推送到前端D3渲染）
type GraphUpdate struct {
	Phase       string      `json:"phase"`       // retrieval/traversal/prediction
	FocusNodeID string      `json:"focusNodeId"` // 当前聚焦的实体ID
	Description string      `json:"description"` // 当前阶段描述
	Nodes       []GraphNode `json:"nodes"`       // 本次新增/更新的节点
	Links       []GraphLink `json:"links"`       // 本次新增/更新的连线
}

// GraphNode 图谱节点
type GraphNode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Group   int    `json:"group"`
	IsNew   bool   `json:"isNew"`
	IsFocus bool   `json:"isFocus"`
}

// GraphLink 图谱连线
type GraphLink struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
	LineType string `json:"lineType"` // solid（实线-已确认）/ dashed（虚线-AI预判）
	IsNew    bool   `json:"isNew"`
}

// DecisionTrace AI推理决策链路追踪器
type DecisionTrace struct {
	mu            sync.RWMutex
	TraceID       string         `json:"traceId"`
	Query         string         `json:"query"`
	Steps         []DecisionStep `json:"steps"`
	stepIdx       int
	callback      func(DecisionStep)   // 每步完成时的回调（用于WebSocket推送trace）
	graphCallback func(GraphUpdate)     // 图谱更新回调
	seenEntities  map[string]bool       // 已推送过的实体（去重）
}

// NewDecisionTrace 创建决策链路追踪器
func NewDecisionTrace(traceID, query string) *DecisionTrace {
	return &DecisionTrace{
		TraceID:      traceID,
		Query:        query,
		Steps:        make([]DecisionStep, 0),
		seenEntities: make(map[string]bool),
	}
}

// SetCallback 设置每步完成的回调
func (dt *DecisionTrace) SetCallback(cb func(DecisionStep)) {
	dt.callback = cb
}

// SetGraphCallback 设置图谱更新回调
func (dt *DecisionTrace) SetGraphCallback(cb func(GraphUpdate)) {
	dt.graphCallback = cb
}

// pushGraph 推送图谱更新，自动去重
func (dt *DecisionTrace) pushGraph(update GraphUpdate) {
	if dt.graphCallback == nil {
		return
	}
	// 过滤已推送过的节点
	var freshNodes []GraphNode
	for _, n := range update.Nodes {
		key := n.ID
		if !dt.seenEntities[key] {
			dt.seenEntities[key] = true
			n.IsNew = true
			freshNodes = append(freshNodes, n)
		} else {
			n.IsNew = false
			freshNodes = append(freshNodes, n)
		}
	}
	update.Nodes = freshNodes
	dt.graphCallback(update)
}

// AddStep 添加推理步骤，返回步骤完成函数
func (dt *DecisionTrace) AddStep(name, description string) func(status, result string) {
	dt.mu.Lock()
	dt.stepIdx++
	step := DecisionStep{
		StepID:      dt.stepIdx,
		Name:        name,
		Description: description,
		Status:      "running",
		StartTime:   time.Now().Format(time.RFC3339),
	}
	dt.Steps = append(dt.Steps, step)
	dt.mu.Unlock()

	if dt.callback != nil {
		dt.callback(step)
	}

	return func(status, result string) {
		dt.mu.Lock()
		step.Status = status
		step.EndTime = time.Now().Format(time.RFC3339)
		step.Result = result
		dt.Steps[step.StepID-1] = step
		dt.mu.Unlock()

		if dt.callback != nil {
			dt.callback(step)
		}
	}
}

// GetAllSteps 获取所有推理步骤
func (dt *DecisionTrace) GetAllSteps() []DecisionStep {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return dt.Steps
}

// GetTraceLog 获取推理链路文本日志
func (dt *DecisionTrace) GetTraceLog() []string {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	var logs []string
	for _, step := range dt.Steps {
		logs = append(logs, fmt.Sprintf("[%s] %s: %s -> %s",
			step.Status, step.Name, step.Description, step.Result))
	}
	return logs
}

// guessEntityType 根据ID前缀猜测实体类型
func guessEntityType(id string) string {
	if len(id) < 3 { return "" }
	prefix := id[:3]
	switch prefix {
	case "sup": return "supplier"
	case "mat": return "material_stock"
	case "con": return "purchase_contract"
	case "po0", "po_": return "purchase_order"
	case "wh0", "wh_": return "warehouse_receipt"
	case "prd": return "production_order"
	case "qc0", "qc_": return "quality_inspection"
	case "equ": return "equipment"
	case "log": return "logistics"
	case "fin": return "finance_invoice"
	case "so0", "so_": return "sales_order"
	case "ar0", "ar_": return "accounts_receivable"
	default: return ""
	}
}

// getGroup 实体类型→D3分组颜色
func getGroupByType(entityType string) int {
	switch entityType {
	case "supplier":
		return 1
	case "material_stock":
		return 2
	case "purchase_contract":
		return 6
	case "purchase_order":
		return 3
	case "warehouse_receipt":
		return 7
	case "production_order":
		return 8
	case "quality_inspection":
		return 9
	case "equipment":
		return 10
	case "logistics":
		return 4
	case "finance_invoice":
		return 5
	case "sales_order":
		return 11
	case "accounts_receivable":
		return 12
	default:
		return 0
	}
}

// entityFromTriple 从三元组和实体列表中提取节点+连线
func buildGraphFromRetrieval(retrieval *RetrievalResult, focusEntityID string, phase string) GraphUpdate {
	update := GraphUpdate{
		Phase:       phase,
		FocusNodeID: focusEntityID,
	}

	seenNodes := make(map[string]bool)

	// 1. 从RelatedEntities提取节点
	for _, e := range retrieval.RelatedEntities {
		if !seenNodes[e.ID] {
			seenNodes[e.ID] = true
			update.Nodes = append(update.Nodes, GraphNode{
				ID:      e.ID,
				Name:    e.Name,
				Type:    e.Type,
				Group:   getGroupByType(e.Type),
				IsFocus: e.ID == focusEntityID,
			})
		}
	}

	// 2. 从GraphTriples提取连线和补充节点
	for _, t := range retrieval.GraphTriples {
		// 确保head和tail节点都存在
		if !seenNodes[t.HeadID] {
			seenNodes[t.HeadID] = true
			// 尝试从metadata获取名称
			name := t.HeadID
			if t.Metadata != nil {
				if n, ok := t.Metadata["materialName"].(string); ok {
					name = n
				}
			}
			update.Nodes = append(update.Nodes, GraphNode{
				ID:      t.HeadID,
				Name:    name,
				Type:    "",
				Group:   0,
				IsFocus: t.HeadID == focusEntityID,
			})
		}
		if !seenNodes[t.TailID] {
			seenNodes[t.TailID] = true
			name := t.TailID
			update.Nodes = append(update.Nodes, GraphNode{
				ID:      t.TailID,
				Name:    name,
				Type:    "",
				Group:   0,
				IsFocus: t.TailID == focusEntityID,
			})
		}

		update.Links = append(update.Links, GraphLink{
			Source:   t.HeadID,
			Target:   t.TailID,
			Relation: t.Relation,
			LineType: "solid",
		})
	}

	// 为没有名字的节点从entities中补充
	for i, n := range update.Nodes {
		if n.Name == n.ID {
			for _, e := range retrieval.RelatedEntities {
				if e.ID == n.ID && e.Name != "" {
					update.Nodes[i].Name = e.Name
					update.Nodes[i].Type = e.Type
					update.Nodes[i].Group = getGroupByType(e.Type)
				}
			}
		}
	}

	return update
}

// RunFullDecisionPipeline 运行完整的Palantir研判流程
func (dt *DecisionTrace) RunFullDecisionPipeline(
	engine *RAGEngine,
	query string,
) (string, []DecisionStep, error) {

	// Step 1: 意图分析
	done1 := dt.AddStep("意图识别", "分析用户查询的业务领域和决策意图")
	var analysis map[string]interface{}
	var err error
	analysis, err = engine.llmClient.AnalyzeBusinessQuery(query)
	if err != nil {
		done1("done", fmt.Sprintf("使用默认分析: %v", err))
		analysis = map[string]interface{}{
			"domain": "general", "topK": float64(3), "graphDepth": float64(2),
		}
	} else {
		// 翻译领域为中文并给出推理理由
		domainCN := map[string]string{
			"supply_chain":"供应链", "procurement":"采购管理", "finance":"财务管理",
			"inventory":"库存管理", "logistics":"物流管理", "general":"通用",
		}
		dcn := domainCN[fmt.Sprintf("%v", analysis["domain"])]
		if dcn == "" { dcn = fmt.Sprintf("%v", analysis["domain"]) }
		keywords := ""
		if kw, ok := analysis["searchKeywords"].([]interface{}); ok && len(kw) > 0 {
			var kwStrs []string
			for _, k := range kw { kwStrs = append(kwStrs, fmt.Sprintf("%v", k)) }
			keywords = strings.Join(kwStrs, "、")
		}
		reason := fmt.Sprintf("判断为「%s」领域问题", dcn)
		if keywords != "" { reason += fmt.Sprintf("，关键检索词：%s", keywords) }
		reason += fmt.Sprintf("，设定检索范围TopK=%d、图谱遍历深度=%d",
			int(analysis["topK"].(float64)), int(analysis["graphDepth"].(float64)))
		step := dt.Steps[dt.stepIdx-1]
	step.NextPlan = []string{"根据意图参数启动语义检索", "按TopK限制查询Chroma向量库", "匹配相关实体类型"}
	done1("done", reason)
	}

	// Step 2: 向量语义检索 + 推送初始图谱
	done2 := dt.AddStep("向量语义检索", "从Chroma向量库检索相关ERP实体文档")
	topK := 3
	if v, ok := analysis["topK"].(float64); ok {
		topK = int(v)
	}
	retrievalResult, err := engine.RetrieveWithGraph(query, topK, 1)
	if err != nil {
		done2("done", fmt.Sprintf("检索到0条结果（%v）", err))
	} else {
		// 统计实体类型分布
		typeCount := map[string]int{}
		for _, e := range retrievalResult.RelatedEntities {
			typeCount[e.Type]++
		}
		var typeParts []string
		for t, c := range typeCount {
			typeParts = append(typeParts, fmt.Sprintf("%s(%d条)", t, c))
		}
		typeSummary := strings.Join(typeParts, "、")
		if typeSummary == "" { typeSummary = "无" }
		reason := fmt.Sprintf("命中%d条文档（%s），包含%d条图谱关联",
			len(retrievalResult.VectorDocs), typeSummary, len(retrievalResult.GraphTriples))
		step := dt.Steps[dt.stepIdx-1]
	step.NextPlan = []string{"对检索到的实体执行深度图谱遍历", "挖掘上下游关联链路", "识别间接关联的隐藏风险节点"}
	step.BranchOpts = []string{"按风险维度深挖（逾期/库存/质量）", "按链路维度全量展开（供应链全景）", "聚焦单一实体做关联分析"}
	done2("done", reason)

		// 📊 推送第1批图谱：检索到的核心实体
		focusID := ""
		if len(retrievalResult.RelatedEntities) > 0 {
			focusID = retrievalResult.RelatedEntities[0].ID
		}
		graph1 := buildGraphFromRetrieval(retrievalResult, focusID, "retrieval")
		graph1.Description = fmt.Sprintf("语义检索命中%d个实体", len(graph1.Nodes))
		dt.pushGraph(graph1)

		// 短暂停顿让前端渲染
		time.Sleep(200 * time.Millisecond)
	}

	// Step 3: 图谱深度遍历 + 推送扩展图谱
	done3 := dt.AddStep("图谱深度遍历", "遍历实体上下游关联链路")
	graphDepth := 2
	if v, ok := analysis["graphDepth"].(float64); ok {
		graphDepth = int(v)
	}

	if retrievalResult != nil && len(retrievalResult.VectorDocs) > 0 {
		// 对检索到的每个实体做深度遍历
		for _, entity := range retrievalResult.RelatedEntities {
			triples, err := engine.boltGraph.GetRelatedEntities(entity.ID, graphDepth)
			if err != nil {
				continue
			}
			// 合并新发现的三元组
			existingKeys := make(map[string]bool)
			for _, t := range retrievalResult.GraphTriples {
				k := fmt.Sprintf("%s|%s|%s", t.HeadID, t.Relation, t.TailID)
				existingKeys[k] = true
			}
			for _, t := range triples {
				k := fmt.Sprintf("%s|%s|%s", t.HeadID, t.Relation, t.TailID)
				if !existingKeys[k] {
					existingKeys[k] = true
					retrievalResult.GraphTriples = append(retrievalResult.GraphTriples, t)
				}
			}
			// 获取关联实体详情（从三元组tail端）
			for _, t := range triples {
				// 尝试获取tail实体
				entityType := guessEntityType(t.TailID)
				if entityType != "" {
					entity, err := engine.boltGraph.GetEntity(entityType, t.TailID)
					if err == nil && entity != nil {
						found := false
						for _, e := range retrievalResult.RelatedEntities {
							if e.ID == entity.ID { found = true; break }
						}
						if !found {
							retrievalResult.RelatedEntities = append(retrievalResult.RelatedEntities, *entity)
						}
					}
				}
			}
		}

		// 总结关键关系类型
		relCount := map[string]int{}
		for _, t := range retrievalResult.GraphTriples {
			relCount[t.Relation]++
		}
		var relParts []string
		for r, c := range relCount {
			relParts = append(relParts, fmt.Sprintf("%s(%d条)", r, c))
		}
		relSummary := strings.Join(relParts, "、")
		if relSummary == "" { relSummary = "无关联" }
		reason := fmt.Sprintf("%d度遍历完成，发现%d条链路（%s），实体上下游已串联",
			graphDepth, len(retrievalResult.GraphTriples), relSummary)
		step := dt.Steps[dt.stepIdx-1]
	step.NextPlan = []string{"基于链路数据运行AI推演预判", "标记高风险节点生成虚线预警", "计算下一步待核查实体优先级"}
	done3("done", reason)

		// 📊 推送第2批图谱：扩展关联实体（增量）
		graph2 := buildGraphFromRetrieval(retrievalResult, "", "traversal")
		graph2.Description = fmt.Sprintf("深度遍历%d度关联，新增%d条链路", graphDepth, len(graph2.Links))
		dt.pushGraph(graph2)

		time.Sleep(200 * time.Millisecond)
	} else {
		done3("done", "无检索结果，跳过图谱遍历")
	}


	// Step 4: 推演预判 + AI预测虚线链路
	done4 := dt.AddStep("推演预判", "AI分析现有链路，预判下一步需检索的实体")
	prejudgeResult := "无特殊预判"

	if retrievalResult != nil && len(retrievalResult.GraphTriples) > 0 {
		// 检查是否有延期或风险
		riskFound := false
		for _, triple := range retrievalResult.GraphTriples {
			if triple.Relation == "应付发票" {
				if risk, ok := triple.Metadata["risk"].(string); ok && risk != "正常" {
					prejudgeResult = fmt.Sprintf("[RISK] 发现风险: %s - %v", risk, triple.Metadata)
					riskFound = true
					break
				}
			}
		}

		// 🤖 AI预测：基于现有图谱推演下一跳
		predictedLinks := dt.predictNextEntities(engine, retrievalResult)
		if len(predictedLinks) > 0 {
			// 构建待核查链路详情列表
			var linkDetails []string
			for i, pl := range predictedLinks {
				detail := fmt.Sprintf("%d. %s --[%s]--> %s", i+1, pl.HeadID, pl.Relation, pl.TailID)
				// 附加核查意见
				if pl.Metadata != nil {
					metaParts := []string{}
					if risk, ok := pl.Metadata["risk"].(string); ok && risk != "" { metaParts = append(metaParts, fmt.Sprintf("风险:%s", risk)) }
					if amt, ok := pl.Metadata["amount"].(float64); ok { metaParts = append(metaParts, fmt.Sprintf("金额:%.0f", amt)) }
					if gap, ok := pl.Metadata["stockGap"].(float64); ok { metaParts = append(metaParts, fmt.Sprintf("缺口:%.0f", gap)) }
					if delay, ok := pl.Metadata["delay"].(float64); ok { metaParts = append(metaParts, fmt.Sprintf("延误:%.0f天", delay)) }
					if len(metaParts) > 0 { detail += " [" + strings.Join(metaParts, ",") + "]" }
				}
				linkDetails = append(linkDetails, detail)
			}
			linksSummary := strings.Join(linkDetails, "；")
			if riskFound {
				prejudgeResult += fmt.Sprintf(" | AI预判%d条待核查链路：%s", len(predictedLinks), linksSummary)
			} else {
				prejudgeResult = fmt.Sprintf("AI预判%d条待核查链路：%s", len(predictedLinks), linksSummary)
			}

			// 📊 推送第3批图谱：AI预判虚线链路
			graph3 := GraphUpdate{
				Phase:       "prediction",
				Description: fmt.Sprintf("AI预判%d条待核查链路（虚线）", len(predictedLinks)),
			}
			for _, pl := range predictedLinks {
				// 将核查意见编码到relation中
				relWithMeta := pl.Relation
				if pl.Metadata != nil {
					metaParts := []string{}
					if risk, ok := pl.Metadata["risk"].(string); ok && risk != "" { metaParts = append(metaParts, "风险:"+risk) }
					if amt, ok := pl.Metadata["amount"].(float64); ok { metaParts = append(metaParts, fmt.Sprintf("金额:%.0f元", amt)) }
					if gap, ok := pl.Metadata["stockGap"].(float64); ok { metaParts = append(metaParts, fmt.Sprintf("缺货:%.0f", gap)) }
					if delay, ok := pl.Metadata["delay"].(float64); ok { metaParts = append(metaParts, fmt.Sprintf("延误:%.0f天", delay)) }
					if len(metaParts) > 0 { relWithMeta += "|" + strings.Join(metaParts, "|") }
				}
				graph3.Links = append(graph3.Links, GraphLink{
					Source:   pl.HeadID,
					Target:   pl.TailID,
					Relation: relWithMeta,
					LineType: "dashed",
					IsNew:    true,
				})
			}
			dt.pushGraph(graph3)
		}
	}
	// 补充详细推理说明
	predReason := prejudgeResult
	if strings.Contains(predReason, "无特殊预判") && len(retrievalResult.GraphTriples) == 0 {
		predReason = "未发现实体关联链路，无法进行推演预判——建议先同步更多数据"
	}
	step := dt.Steps[dt.stepIdx-1]
	step.NextPlan = []string{"汇总所有阶段数据生成结构化报告", "调用LLM撰写专业研判文本", "输出问题分析+数据举证+风险评估+行动建议"}
	done4("done", predReason)

	// Step 6: 生成研判报告
	done5 := dt.AddStep("生成研判报告", "汇总所有信息，生成结构化研判报告")
	// 获取最近聊天历史作为上下文
	chatHistory, _ := engine.boltGraph.GetChatHistory(6)
	var chatMsgs []ChatMessage
	for _, h := range chatHistory {
		chatMsgs = append(chatMsgs, ChatMessage{Role: h["role"], Content: h["content"]})
	}

	report, err := engine.GenerateReport(query, dt.GetTraceLog(), chatMsgs)
	if err != nil {
		done5("done", "报告已生成（LLM不可用，使用规则引擎回退方案）")
	} else {
		step := dt.Steps[dt.stepIdx-1]
	step.NextPlan = []string{"研判流程全部完成，可将报告导出或继续追问"}
	done5("done", "已生成结构化研判报告，包含问题分析、数据举证、关联链路、风险评估、行动建议五部分")
	}

	return report, dt.GetAllSteps(), nil
}


// scoreEntityRisks 对检索到的实体进行多维风险打分
func (dt *DecisionTrace) scoreEntityRisks(retrieval *RetrievalResult) []map[string]interface{} {
	var cards []map[string]interface{}
	if retrieval == nil { return cards }

	for _, entity := range retrieval.RelatedEntities {
		props := entity.Properties
		score := 0.0
		reasons := []string{}

		// 维度1: 供应商延期风险 (0-30分)
		if entity.Type == "supplier" {
			if dc, ok := props["delayCount"].(float64); ok {
				s := dc * 8
				if s > 30 { s = 30 }
				score += s
				if dc >= 3 { reasons = append(reasons, fmt.Sprintf("延期%d次(+%.0f)", int(dc), s)) }
			}
			if rl, ok := props["riskLevel"].(string); ok {
				if rl == "高" { score += 20; reasons = append(reasons, "风险等级:高(+20)") } else if rl == "中" { score += 10; reasons = append(reasons, "风险等级:中(+10)") }
			}
		}

		// 维度2: 库存风险 (0-25分)
		if entity.Type == "material_stock" {
			if cur, ok1 := props["currentStock"].(float64); ok1 {
				if safe, ok2 := props["safeStock"].(float64); ok2 && safe > 0 {
					ratio := cur / safe
					if ratio < 0.3 { score += 25; reasons = append(reasons, fmt.Sprintf("库存仅%.0f/安全%.0f(严重不足+25)", cur, safe)) } else if ratio < 0.5 { score += 15; reasons = append(reasons, fmt.Sprintf("库存%.0f/安全%.0f(不足+15)", cur, safe)) } else if ratio < 0.8 { score += 8; reasons = append(reasons, "库存偏低+8") }
				}
			}
		}

		// 维度3: 财务逾期风险 (0-25分)
		if entity.Type == "finance_invoice" || entity.Type == "accounts_receivable" {
			if risk, ok := props["risk"].(string); ok {
				if strings.Contains(risk, "逾期") { score += 25; reasons = append(reasons, "财务逾期风险(+25)") } else if risk == "关注" { score += 12; reasons = append(reasons, "财务关注(+12)") }
			}
			if rl, ok := props["riskLevel"].(string); ok {
				if rl == "高风险" { score += 25; reasons = append(reasons, "高风险等级(+25)") } else if rl == "逾期风险" { score += 20; reasons = append(reasons, "逾期风险(+20)") }
			}
			if aging, ok := props["agingDays"].(float64); ok {
				s := aging * 0.5
				if s > 20 { s = 20 }
				score += s
				if aging > 30 { reasons = append(reasons, fmt.Sprintf("账龄%.0f天(+%.0f)", aging, s)) }
			}
		}

		// 维度4: 合同风险 (0-10分)
		if entity.Type == "purchase_contract" {
			if rc, ok := props["riskClause"].(string); ok && rc != "" {
				score += 8; reasons = append(reasons, "含风险条款(+8)")
			}
		}

		// 维度5: 质检风险 (0-15分)
		if entity.Type == "quality_inspection" {
			if result, ok := props["inspectionResult"].(string); ok {
				if result == "退货" { score += 15; reasons = append(reasons, "质检退货(+15)") } else if result == "让步接收" { score += 8; reasons = append(reasons, "让步接收(+8)") }
			}
			if pr, ok := props["passRate"].(float64); ok {
				if pr < 0.9 { score += 10; reasons = append(reasons, fmt.Sprintf("合格率仅%.0f%%(+10)", pr*100)) }
			}
		}

		// 维度6: 设备风险 (0-10分)
		if entity.Type == "equipment" {
			if status, ok := props["status"].(string); ok {
				if status == "维修中" || status == "待维护" { score += 10; reasons = append(reasons, fmt.Sprintf("设备%s(+10)", status)) }
			}
			if ur, ok := props["utilizationRate"].(float64); ok {
				if ur > 0.9 { score += 5; reasons = append(reasons, "利用率过高(+5)") }
			}
		}

		// 维度7: 物流延误 (0-10分)
		if entity.Type == "logistics" {
			if delay, ok := props["arriveDelayDay"].(float64); ok {
				s := delay * 1.5
				if s > 10 { s = 10 }
				score += s
				if delay > 3 { reasons = append(reasons, fmt.Sprintf("延误%.0f天(+%.0f)", delay, s)) }
			}
		}

		if score > 0 || len(reasons) > 0 {
			cards = append(cards, map[string]interface{}{
				"entityId":   entity.ID,
				"entityName": entity.Name,
				"entityType": entity.Type,
				"score":      score,
				"level":      riskLevel(score),
				"reasons":    reasons,
			})
		}
	}
	return cards
}

func riskLevel(score float64) string {
	if score >= 70 { return "严重" }
	if score >= 40 { return "高风险" }
	if score >= 20 { return "中等" }
	return "关注"
}

// predictNextEntities AI预测下一跳实体（生成虚线链路）
func (dt *DecisionTrace) predictNextEntities(engine *RAGEngine, retrieval *RetrievalResult) []graphdb.EntityTriple {
	var predicted []graphdb.EntityTriple

	if retrieval == nil || len(retrieval.RelatedEntities) == 0 {
		return predicted
	}

	// 已有实体ID集合
	existingIDs := make(map[string]bool)
	for _, e := range retrieval.RelatedEntities {
		existingIDs[e.ID] = true
	}
	for _, t := range retrieval.GraphTriples {
		existingIDs[t.HeadID] = true
		existingIDs[t.TailID] = true
	}

	// 构建上下文供给LLM预测
	var ctxParts []string
	ctxParts = append(ctxParts, "已知实体:")
	for _, e := range retrieval.RelatedEntities {
		ctxParts = append(ctxParts, fmt.Sprintf("  [%s] %s (ID:%s)", e.Type, e.Name, e.ID))
	}
	ctxParts = append(ctxParts, "\n已知关系:")
	for _, t := range retrieval.GraphTriples[:min(10, len(retrieval.GraphTriples))] {
		ctxParts = append(ctxParts, fmt.Sprintf("  %s --[%s]--> %s", t.HeadID, t.Relation, t.TailID))
	}

	contextStr := strings.Join(ctxParts, "\n")

	// 用LLM预测（如果失败就用规则预测）
	predictionStr, err := engine.llmClient.Chat([]ChatMessage{
		{Role: "system", Content: `你是一个ERP供应链分析专家。基于已知实体链路，预测下一步需要检索核查的实体关系。
请返回JSON数组，每个预测包含headId, relation, tailId。
格式：[{"headId":"实体ID","relation":"关系名","tailId":"目标ID或?"}]

预测规则：
- 如果发现高风险供应商，预测关联的采购订单、发票
- 如果库存低于安全线，预测上游供应商、在途物流
- 如果有逾期发票，预测关联的物流状态、采购订单`},
		{Role: "user", Content: contextStr},
	}, 0.3, 300)

	if err == nil && predictionStr != "" {
		// 尝试从LLM回复中提取预测
		predictionStr = extractJSON(predictionStr)
		// 简单解析：找实体ID引用
		for _, e := range retrieval.RelatedEntities {
			for _, t := range retrieval.GraphTriples {
				if !existingIDs[t.TailID] {
					predicted = append(predicted, graphdb.EntityTriple{
						HeadID:   e.ID,
						Relation: "预判关联",
						TailID:   t.TailID,
					})
					existingIDs[t.TailID] = true
				}
			}
		}
	}

	// 规则预测作为兜底
	if len(predicted) == 0 {
		predicted = dt.ruleBasedPredict(retrieval, engine, existingIDs)
	}

	return predicted
}

// ruleBasedPredict 基于规则的预测 + 自动核查（LLM不可用时的兜底）
func (dt *DecisionTrace) ruleBasedPredict(retrieval *RetrievalResult, engine *RAGEngine, existingIDs map[string]bool) []graphdb.EntityTriple {
	var predicted []graphdb.EntityTriple

	for _, e := range retrieval.RelatedEntities {
		switch e.Type {
		case "supplier":
			if risk, ok := e.Properties["riskLevel"].(string); ok && (risk == "高" || risk == "中") {
				for _, t := range retrieval.GraphTriples {
					if t.HeadID == e.ID && t.Relation == "执行采购" {
						// 自动核查该采购订单的发票
						invoices := dt.findLinkedEntities(engine, t.TailID, "finance_invoice", "应付发票")
						for _, inv := range invoices {
							predicted = append(predicted, graphdb.EntityTriple{
								HeadID: t.TailID, Relation: "待核查发票",
								TailID: inv.ID,
								Metadata: map[string]interface{}{"risk": inv.Properties["risk"], "amount": inv.Properties["amount"]},
							})
						}
						if len(invoices) == 0 {
							predicted = append(predicted, graphdb.EntityTriple{
								HeadID: t.TailID, Relation: "待核查发票", TailID: "未找到关联发票",
							})
						}
						// 自动核查物流
						logistics := dt.findLinkedEntities(engine, t.TailID, "logistics", "物流承运")
						for _, lg := range logistics {
							predicted = append(predicted, graphdb.EntityTriple{
								HeadID: t.TailID, Relation: "待核查物流",
								TailID: lg.ID,
								Metadata: map[string]interface{}{"status": lg.Properties["currentLocation"], "delay": lg.Properties["arriveDelayDay"]},
							})
						}
					}
				}
			}
		case "purchase_order":
			if status, ok := e.Properties["status"].(string); ok && strings.Contains(status, "延期") {
				// 自动核查物流
				logistics := dt.findLinkedEntities(engine, e.ID, "logistics", "物流承运")
				for _, lg := range logistics {
					if !existingIDs[lg.ID] {
						predicted = append(predicted, graphdb.EntityTriple{
							HeadID: e.ID, Relation: "待确认物流状态",
							TailID: lg.ID,
							Metadata: map[string]interface{}{"status": lg.Properties["currentLocation"], "delay": lg.Properties["arriveDelayDay"]},
						})
					}
				}
				// 自动核查发票
				invoices := dt.findLinkedEntities(engine, e.ID, "finance_invoice", "应付发票")
				for _, inv := range invoices {
					if !existingIDs[inv.ID] {
						predicted = append(predicted, graphdb.EntityTriple{
							HeadID: e.ID, Relation: "待核查发票",
							TailID: inv.ID,
							Metadata: map[string]interface{}{"risk": inv.Properties["risk"], "amount": inv.Properties["amount"]},
						})
					}
				}
			}
		case "material_stock":
			if stock, ok := e.Properties["currentStock"].(float64); ok {
				if safe, ok2 := e.Properties["safeStock"].(float64); ok2 && stock < safe {
					if supplierID, ok3 := e.Properties["supplierId"].(string); ok3 {
						gap := safe - stock
						predicted = append(predicted, graphdb.EntityTriple{
							HeadID: supplierID, Relation: "紧急备货",
							TailID: e.ID,
							Metadata: map[string]interface{}{"stockGap": gap, "currentStock": stock, "safeStock": safe},
						})
					}
				}
			}
		}
	}

	return predicted
}

// findLinkedEntities 通过BoltDB自动查找与某实体关联的指定类型实体
func (dt *DecisionTrace) findLinkedEntities(engine *RAGEngine, sourceID, targetType, relation string) []graphdb.Entity {
	var results []graphdb.Entity
	triples, err := engine.boltGraph.GetRelatedEntities(sourceID, 1)
	if err != nil { return results }
	for _, t := range triples {
		if t.Relation == relation {
			entity, err := engine.boltGraph.GetEntity(targetType, t.TailID)
			if err == nil && entity != nil { results = append(results, *entity) }
		}
	}
	// 也查反向关系
	for _, t := range triples {
		if t.TailID == sourceID {
			entity, err := engine.boltGraph.GetEntity(targetType, t.HeadID)
			if err == nil && entity != nil { results = append(results, *entity) }
		}
	}
	return results
}
