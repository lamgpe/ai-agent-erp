package vectorstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ChromaClient Chroma向量库HTTP客户端
type ChromaClient struct {
	baseURL    string
	httpClient *http.Client
}

// ChromaDocument 文档结构
type ChromaDocument struct {
	ID       string                 `json:"id"`
	Text     string                 `json:"text"`
	Metadata map[string]interface{} `json:"metadata"`
	Vector   []float64              `json:"vector,omitempty"`
}

// ChromaQueryResult 查询结果
type ChromaQueryResult struct {
	ID       string                 `json:"id"`
	Text     string                 `json:"text"`
	Metadata map[string]interface{} `json:"metadata"`
	Distance float64                `json:"distance"`
}

// NewChromaClient 创建Chroma客户端
func NewChromaClient(host, port string) *ChromaClient {
	return &ChromaClient{
		baseURL: fmt.Sprintf("http://%s:%s", host, port),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HealthCheck 检查Chroma服务是否可用
func (c *ChromaClient) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v2/heartbeat")
	if err != nil {
		// 尝试v1 API
		resp, err = c.httpClient.Get(c.baseURL + "/api/v1/heartbeat")
		if err != nil {
			return fmt.Errorf("Chroma服务不可用: %w", err)
		}
	}
	if resp != nil {
		resp.Body.Close()
	}
	return nil
}

// CreateCollection 创建集合
func (c *ChromaClient) CreateCollection(name string) error {
	body := map[string]interface{}{
		"name": name,
		"metadata": map[string]string{
			"description": "ERP实体向量集合",
		},
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v2/collections",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		// 如果集合已存在，忽略错误
		return nil
	}
	defer resp.Body.Close()

	// 集合已存在也算成功
	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("创建集合失败 %d: %s", resp.StatusCode, string(respBody))
}

// AddDocuments 添加文档到Chroma
func (c *ChromaClient) AddDocuments(collectionName string, docs []ChromaDocument) error {
	if len(docs) == 0 {
		return nil
	}

	var ids []string
	var texts []string
	var metadatas []map[string]interface{}

	for _, doc := range docs {
		ids = append(ids, doc.ID)
		texts = append(texts, doc.Text)
		metadatas = append(metadatas, doc.Metadata)
	}

	body := map[string]interface{}{
		"ids":       ids,
		"documents": texts,
		"metadatas": metadatas,
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v2/collections/%s/add", c.baseURL, collectionName)
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("添加文档失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("添加文档失败 %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// QueryDocuments 查询相似文档（需要先有embedding向量）
func (c *ChromaClient) QueryDocuments(collectionName string, queryEmbedding []float64, topK int) ([]ChromaQueryResult, error) {
	body := map[string]interface{}{
		"query_embeddings": [][]float64{queryEmbedding},
		"n_results":        topK,
		"include":          []string{"documents", "metadatas", "distances"},
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v2/collections/%s/query", c.baseURL, collectionName)
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("查询文档失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("查询失败 %d: %s", resp.StatusCode, string(respBody))
	}

	// 解析Chroma v2 API响应
	var result struct {
		Documents [][]string                 `json:"documents"`
		Metadatas [][]map[string]interface{} `json:"metadatas"`
		Distances [][]float64                `json:"distances"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析查询结果失败: %w", err)
	}

	var results []ChromaQueryResult
	if len(result.Documents) > 0 && len(result.Documents[0]) > 0 {
		for i := range result.Documents[0] {
			qr := ChromaQueryResult{
				Text:     result.Documents[0][i],
				Distance: 0,
			}
			if len(result.Metadatas) > 0 && i < len(result.Metadatas[0]) {
				qr.Metadata = result.Metadatas[0][i]
				if id, ok := qr.Metadata["id"].(string); ok {
					qr.ID = id
				}
			}
			if len(result.Distances) > 0 && i < len(result.Distances[0]) {
				qr.Distance = result.Distances[0][i]
			}
			results = append(results, qr)
		}
	}

	return results, nil
}

// GetCollectionCount 获取集合中文档数量
func (c *ChromaClient) GetCollectionCount(collectionName string) (int, error) {
	url := fmt.Sprintf("%s/api/v2/collections/%s", c.baseURL, collectionName)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, nil
	}

	var result struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, nil
	}

	return 0, nil // Chroma v2 doesn't easily expose count
}

// DeleteCollection 删除集合
func (c *ChromaClient) DeleteCollection(collectionName string) error {
	url := fmt.Sprintf("%s/api/v2/collections/%s", c.baseURL, collectionName)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
