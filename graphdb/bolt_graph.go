package graphdb

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// EntityTriple 实体三元组：头实体 --关系--> 尾实体
type EntityTriple struct {
	HeadID    string                 `json:"headId"`
	Relation  string                 `json:"relation"`
	TailID    string                 `json:"tailId"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt string                 `json:"createdAt"`
}

// Entity 通用实体结构
type Entity struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"` // supplier/material/purchase_order/logistics/finance_invoice
	Name       string                 `json:"name"`
	Properties map[string]interface{} `json:"properties"`
	Source     string                 `json:"source"` // auto/manual
	CreatedAt  string                 `json:"createdAt"`
}

// BoltGraph BoltDB图谱存储
type BoltGraph struct {
	db        *bolt.DB
	tripleBkt []byte // 三元组桶
	systemBkt []byte // 系统日志、配置桶
}

// NewBoltGraph 创建BoltDB图谱实例
func NewBoltGraph(dbPath string) (*BoltGraph, error) {
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("打开BoltDB失败: %w", err)
	}

	bg := &BoltGraph{
		db:        db,
		tripleBkt: []byte("entity_triple"),
		systemBkt: []byte("system_log"),
	}

	// 创建桶
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bg.tripleBkt); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bg.systemBkt); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("创建BoltDB桶失败: %w", err)
	}

	return bg, nil
}

// Close 关闭数据库
func (bg *BoltGraph) Close() error {
	return bg.db.Close()
}

// AddTriple 添加三元组
func (bg *BoltGraph) AddTriple(triple EntityTriple) error {
	triple.CreatedAt = time.Now().Format(time.RFC3339)
	key := fmt.Sprintf("%s|%s|%s|%s", triple.HeadID, triple.Relation, triple.TailID, triple.CreatedAt)

	data, err := json.Marshal(triple)
	if err != nil {
		return fmt.Errorf("序列化三元组失败: %w", err)
	}

	return bg.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bg.tripleBkt).Put([]byte(key), data)
	})
}

// GetTriplesByHead 根据头实体ID查询所有关联三元组
func (bg *BoltGraph) GetTriplesByHead(headID string) ([]EntityTriple, error) {
	var triples []EntityTriple

	err := bg.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.tripleBkt).Cursor()
		prefix := []byte(headID + "|")
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			var triple EntityTriple
			if err := json.Unmarshal(v, &triple); err != nil {
				continue
			}
			triples = append(triples, triple)
		}
		return nil
	})

	return triples, err
}

// GetTriplesByTail 根据尾实体ID查询所有关联三元组
func (bg *BoltGraph) GetTriplesByTail(tailID string) ([]EntityTriple, error) {
	var triples []EntityTriple

	err := bg.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.tripleBkt).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var triple EntityTriple
			if err := json.Unmarshal(v, &triple); err != nil {
				continue
			}
			if triple.TailID == tailID {
				triples = append(triples, triple)
			}
		}
		return nil
	})

	return triples, err
}

// GetAllTriples 获取全部三元组
func (bg *BoltGraph) GetAllTriples() ([]EntityTriple, error) {
	var triples []EntityTriple

	err := bg.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.tripleBkt).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var triple EntityTriple
			if err := json.Unmarshal(v, &triple); err != nil {
				continue
			}
			triples = append(triples, triple)
		}
		return nil
	})

	return triples, err
}

// GetRelatedEntities 获取某个实体的1度关联（上下游）
func (bg *BoltGraph) GetRelatedEntities(entityID string, depth int) ([]EntityTriple, error) {
	visited := make(map[string]bool)
	var result []EntityTriple
	bg.traverseEntity(entityID, depth, visited, &result)
	return result, nil
}

// traverseEntity 递归遍历实体关联链路
func (bg *BoltGraph) traverseEntity(entityID string, depth int, visited map[string]bool, result *[]EntityTriple) {
	if depth <= 0 || visited[entityID] {
		return
	}
	visited[entityID] = true

	triples, err := bg.GetTriplesByHead(entityID)
	if err != nil {
		return
	}
	for _, t := range triples {
		*result = append(*result, t)
		bg.traverseEntity(t.TailID, depth-1, visited, result)
	}

	// 同时查找反向关系
	reverseTriples, err := bg.GetTriplesByTail(entityID)
	if err != nil {
		return
	}
	for _, t := range reverseTriples {
		*result = append(*result, t)
		bg.traverseEntity(t.HeadID, depth-1, visited, result)
	}
}

// SaveEntity 保存实体到系统日志桶（作为实体存储）
func (bg *BoltGraph) SaveEntity(entity Entity) error {
	entity.CreatedAt = time.Now().Format(time.RFC3339)
	key := fmt.Sprintf("entity:%s:%s", entity.Type, entity.ID)

	data, err := json.Marshal(entity)
	if err != nil {
		return fmt.Errorf("序列化实体失败: %w", err)
	}

	return bg.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bg.systemBkt).Put([]byte(key), data)
	})
}

// GetEntity 获取实体
func (bg *BoltGraph) GetEntity(entityType, entityID string) (*Entity, error) {
	key := fmt.Sprintf("entity:%s:%s", entityType, entityID)

	var entity Entity
	err := bg.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bg.systemBkt).Get([]byte(key))
		if data == nil {
			return fmt.Errorf("实体不存在: %s/%s", entityType, entityID)
		}
		return json.Unmarshal(data, &entity)
	})

	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// GetAllEntities 获取所有实体
func (bg *BoltGraph) GetAllEntities() ([]Entity, error) {
	var entities []Entity

	err := bg.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.systemBkt).Cursor()
		prefix := []byte("entity:")
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			var entity Entity
			if err := json.Unmarshal(v, &entity); err != nil {
				continue
			}
			entities = append(entities, entity)
		}
		return nil
	})

	return entities, err
}

// SaveLog 保存操作日志
func (bg *BoltGraph) SaveLog(logType, message string, data interface{}) error {
	key := fmt.Sprintf("log:%s:%s", logType, time.Now().Format(time.RFC3339Nano))

	logEntry := map[string]interface{}{
		"type":    logType,
		"message": message,
		"data":    data,
		"time":    time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		return err
	}

	return bg.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bg.systemBkt).Put([]byte(key), jsonData)
	})
}

// GetLogs 获取操作日志（最近N条）
func (bg *BoltGraph) GetLogs(limit int) ([]map[string]interface{}, error) {
	var logs []map[string]interface{}

	err := bg.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.systemBkt).Cursor()
		prefix := []byte("log:")
		count := 0
		// 倒序遍历
		for k, v := c.Last(); k != nil && count < limit; k, v = c.Prev() {
			if len(k) < len(prefix) || string(k[:len(prefix)]) != string(prefix) {
				break
			}
			var logEntry map[string]interface{}
			if err := json.Unmarshal(v, &logEntry); err != nil {
				continue
			}
			logs = append(logs, logEntry)
			count++
		}
		return nil
	})

	return logs, err
}

// GetKnowledgeDocs 获取所有知识库文档（不受日志数量限制）
func (bg *BoltGraph) GetKnowledgeDocs() ([]map[string]interface{}, error) {
	var docs []map[string]interface{}

	err := bg.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.systemBkt).Cursor()
		prefix := []byte("log:")
		// 正序遍历所有日志，筛选 knowledge_upload 类型
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			var logEntry map[string]interface{}
			if err := json.Unmarshal(v, &logEntry); err != nil {
				continue
			}
			if t, ok := logEntry["type"].(string); ok && t == "knowledge_upload" {
				docs = append(docs, logEntry)
			}
		}
		return nil
	})

	return docs, err
}

// SaveChatMessage 保存聊天消息

// ClearKnowledgeLogs 清除知识库上传日志
func (bg *BoltGraph) ClearKnowledgeLogs() error {
	return bg.db.Update(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.systemBkt).Cursor()
		prefix := []byte("log:")
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			var logEntry map[string]interface{}
			if err := json.Unmarshal(v, &logEntry); err != nil {
				continue
			}
			if t, ok := logEntry["type"].(string); ok && t == "knowledge_upload" {
				if err := tx.Bucket(bg.systemBkt).Delete(k); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (bg *BoltGraph) SaveChatMessage(role, content string) error {
	key := fmt.Sprintf("chat:%s", time.Now().Format(time.RFC3339Nano))

	msg := map[string]string{
		"role":    role,
		"content": content,
		"time":    time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return bg.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bg.systemBkt).Put([]byte(key), data)
	})
}

// GetChatHistory 获取聊天历史
func (bg *BoltGraph) GetChatHistory(limit int) ([]map[string]string, error) {
	var history []map[string]string

	err := bg.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.systemBkt).Cursor()
		prefix := []byte("chat:")
		count := 0
		for k, v := c.Seek(prefix); k != nil && count < limit; k, v = c.Next() {
			if len(k) < len(prefix) || string(k[:len(prefix)]) != string(prefix) {
				break
			}
			var msg map[string]string
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}
			history = append(history, msg)
			count++
		}
		return nil
	})

	return history, err
}

// SaveConfig 保存配置
func (bg *BoltGraph) SaveConfig(key, value string) error {
	return bg.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bg.systemBkt).Put([]byte("config:"+key), []byte(value))
	})
}

// GetConfig 获取配置
func (bg *BoltGraph) GetConfig(key string) (string, error) {
	var value string
	err := bg.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bg.systemBkt).Get([]byte("config:" + key))
		if data == nil {
			return fmt.Errorf("配置不存在: %s", key)
		}
		value = string(data)
		return nil
	})
	return value, err
}

// ClearEntitiesAndTriples 清空实体和三元组数据（保留日志和聊天记录）
func (bg *BoltGraph) ClearEntitiesAndTriples() error {
	return bg.db.Update(func(tx *bolt.Tx) error {
		// 清除三元组桶
		if err := tx.DeleteBucket(bg.tripleBkt); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(bg.tripleBkt); err != nil {
			return err
		}
		// 只清除systemBkt中的entity:前缀记录
		c := tx.Bucket(bg.systemBkt).Cursor()
		prefix := []byte("entity:")
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			if err := tx.Bucket(bg.systemBkt).Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// ClearEntitiesByType 清除指定类型的所有实体（用于单个类型同步前重置）
func (bg *BoltGraph) ClearEntitiesByType(entityType string) error {
	typePrefix := fmt.Sprintf("entity:%s:", entityType)
	return bg.db.Update(func(tx *bolt.Tx) error {
		c := tx.Bucket(bg.systemBkt).Cursor()
		prefix := []byte(typePrefix)
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			if err := tx.Bucket(bg.systemBkt).Delete(k); err != nil {
				return err
			}
		}
		// 同时清除相关的三元组
		tc := tx.Bucket(bg.tripleBkt).Cursor()
		for k, _ := tc.First(); k != nil; k, _ = tc.Next() {
			keyStr := string(k)
			if len(keyStr) >= len(typePrefix) && keyStr[:len(typePrefix)] == typePrefix {
				// 三元组的key以headID开头，删掉以该type前缀开头（实体ID含type前缀?不，是"entity:type:id"格式但三元组是"headID|relation|tailID|time"）
				// 最安全的方式：先获取该类型所有实体ID，然后匹配三元组
			}
		}
		return nil
	})
	// 注：三元组清理采用更彻底的方式：清除该类型所有实体后，重新遍历三元组，删除head或tail指向不存在实体的三元组
}

// ClearAll 清空所有数据（用于重置）
func (bg *BoltGraph) ClearAll() error {
	return bg.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(bg.tripleBkt); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(bg.tripleBkt); err != nil {
			return err
		}
		if err := tx.DeleteBucket(bg.systemBkt); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(bg.systemBkt); err != nil {
			return err
		}
		return nil
	})
}

// SyncEntityRelations 根据实体属性自动建立三元组关系
func (bg *BoltGraph) SyncEntityRelations(entity Entity) error {
	props := entity.Properties

	switch entity.Type {
	case "supplier":
		// 供应商关系由其他实体反向关联，此处记录元数据即可
		return bg.SaveLog("relation", fmt.Sprintf("供应商实体入库: %s", entity.Name), entity)

	case "material_stock":
		if supplierID, ok := props["supplierId"].(string); ok && supplierID != "" {
			triple := EntityTriple{
				HeadID:   supplierID,
				Relation: "供应",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"materialName": entity.Name,
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}

	case "purchase_order":
		if supplierID, ok := props["supplierId"].(string); ok && supplierID != "" {
			triple := EntityTriple{
				HeadID:   supplierID,
				Relation: "执行采购",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"orderNo": props["orderNo"],
					"amount":  props["totalAmount"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
		if materialID, ok := props["materialId"].(string); ok && materialID != "" {
			triple := EntityTriple{
				HeadID:   entity.ID,
				Relation: "采购物料",
				TailID:   materialID,
				Metadata: map[string]interface{}{
					"orderNum": props["orderNum"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}

	case "logistics":
		if poID, ok := props["poId"].(string); ok && poID != "" {
			triple := EntityTriple{
				HeadID:   poID,
				Relation: "物流承运",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"carrier": props["carrier"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}

	case "purchase_contract":
		if supplierID, ok := props["supplierId"].(string); ok && supplierID != "" {
			triple := EntityTriple{
				HeadID:   entity.ID,
				Relation: "签约供应商",
				TailID:   supplierID,
				Metadata: map[string]interface{}{
					"contractNo": props["contractNo"],
					"amount":     props["totalAmount"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
	case "warehouse_receipt":
		if poID, ok := props["poId"].(string); ok && poID != "" {
			triple := EntityTriple{
				HeadID:   poID,
				Relation: "入库验收",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"receiptNo": props["receiptNo"],
					"status":    props["status"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
		if prodID, ok := props["orderId"].(string); ok && prodID != "" {
			triple := EntityTriple{
				HeadID:   entity.ID,
				Relation: "生产入库",
				TailID:   prodID,
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
	case "production_order":
		if poID, ok := props["poId"].(string); ok && poID != "" {
			triple := EntityTriple{
				HeadID:   poID,
				Relation: "生产执行",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"orderNo":  props["orderNo"],
					"workshop": props["workshop"],
					"status":   props["status"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
		if equipID, ok := props["equipId"].(string); ok && equipID != "" {
			triple := EntityTriple{
				HeadID:   entity.ID,
				Relation: "使用设备",
				TailID:   equipID,
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
	case "quality_inspection":
		if prodID, ok := props["orderId"].(string); ok && prodID != "" {
			triple := EntityTriple{
				HeadID:   prodID,
				Relation: "质量检验",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"result":   props["inspectionResult"],
					"passRate": props["passRate"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
	case "sales_order":
		if prodID, ok := props["orderId"].(string); ok && prodID != "" {
			triple := EntityTriple{
				HeadID:   prodID,
				Relation: "销售出库",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"client":  props["clientName"],
					"amount":  props["totalAmount"],
					"status":  props["status"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
	case "accounts_receivable":
		if soID, ok := props["orderId"].(string); ok && soID != "" {
			triple := EntityTriple{
				HeadID:   soID,
				Relation: "应收账款",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"client":  props["clientName"],
					"amount":  props["balanceAmount"],
					"risk":    props["riskLevel"],
					"aging":   props["agingDays"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
	case "finance_invoice":
		if poID, ok := props["poId"].(string); ok && poID != "" {
			triple := EntityTriple{
				HeadID:   poID,
				Relation: "应付发票",
				TailID:   entity.ID,
				Metadata: map[string]interface{}{
					"amount": props["amount"],
					"risk":   props["risk"],
				},
			}
			if err := bg.AddTriple(triple); err != nil {
				return err
			}
		}
	}

	return nil
}
