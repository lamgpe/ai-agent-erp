package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MockERPHandler 模拟ERP API处理器
type MockERPHandler struct {
	mockDataDir string
}

// NewMockERPHandler 创建Mock ERP处理器
func NewMockERPHandler(mockDataDir string) *MockERPHandler {
	return &MockERPHandler{
		mockDataDir: mockDataDir,
	}
}

// ServeHTTP 处理HTTP请求
func (h *MockERPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 路径格式: /mock/erp/{type} 或 /mock/erp/{type}/{id}
	path := strings.TrimPrefix(r.URL.Path, "/mock/erp/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		h.listTypes(w)
		return
	}

	entityType := parts[0]
	entityID := ""
	if len(parts) > 1 {
		entityID = parts[1]
	}

	h.handleEntityRequest(w, r, entityType, entityID)
}

// listTypes 列出所有可用的模拟数据类型
func (h *MockERPHandler) listTypes(w http.ResponseWriter) {
	types := []map[string]string{
		{"type": "supplier", "description": "供应商实体(YonSuite采购云)", "count": "20"},
		{"type": "material_stock", "description": "物料库存(YonSuite供应链云)", "count": "20"},
		{"type": "purchase_contract", "description": "采购合同(YonSuite采购云)", "count": "20"},
		{"type": "purchase_order", "description": "采购订单(YonSuite采购云)", "count": "20"},
		{"type": "warehouse_receipt", "description": "入库单(YonSuite供应链云)", "count": "20"},
		{"type": "production_order", "description": "生产工单(YonSuite制造云)", "count": "20"},
		{"type": "quality_inspection", "description": "质检报告(YonSuite质量云)", "count": "20"},
		{"type": "equipment", "description": "设备资产(YonSuite资产云)", "count": "20"},
		{"type": "logistics", "description": "物流承运(YonSuite供应链云)", "count": "20"},
		{"type": "finance_invoice", "description": "财务应付发票(YonSuite财务云)", "count": "20"},
		{"type": "sales_order", "description": "销售订单(YonSuite营销云)", "count": "20"},
		{"type": "accounts_receivable", "description": "应收账单(YonSuite财务云)", "count": "20"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"available_types": types,
		"usage":           "GET /mock/erp/{type} 获取全部数据, GET /mock/erp/{type}/{id} 获取单条",
	})
}

// handleEntityRequest 处理实体请求
func (h *MockERPHandler) handleEntityRequest(w http.ResponseWriter, r *http.Request, entityType, entityID string) {
	// 读取对应的JSON文件
	filePath := filepath.Join(h.mockDataDir, fmt.Sprintf("%s.json", entityType))

	data, err := os.ReadFile(filePath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("数据类型不存在: %s (可用: supplier/material_stock/purchase_order/logistics/finance_invoice)", entityType),
		})
		return
	}

	// 如果指定了ID，返回单条记录
	if entityID != "" {
		var items []map[string]interface{}
		if err := json.Unmarshal(data, &items); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "数据解析失败"})
			return
		}

		for _, item := range items {
			// 根据不同实体类型的ID字段名查找
			idField := getIDField(entityType)
			if id, ok := item[idField].(string); ok && id == entityID {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(item)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("记录不存在: %s/%s", entityType, entityID)})
		return
	}

	// 返回全部数据
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// getIDField 根据实体类型获取ID字段名
func getIDField(entityType string) string {
	switch entityType {
	case "supplier":
		return "supplierId"
	case "material_stock":
		return "materialId"
	case "purchase_order":
		return "poId"
	case "logistics":
		return "logId"
	case "finance_invoice":
		return "invoiceId"
	default:
		return "id"
	}
}
