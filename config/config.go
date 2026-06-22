package config

import (
	"os"
)

// Config 全局配置结构体
type Config struct {
	// 云端LLM配置（兼容OpenAI协议：通义千问/DeepSeek/OpenAI）
	LLMAPIKey      string // API密钥
	LLMBaseURL     string // API基础地址，如 https://dashscope.aliyuncs.com/compatible-mode/v1
	LLMModel       string // 对话模型名，如 qwen-turbo / gpt-3.5-turbo
	EmbeddingModel string // 向量模型名，如 text-embedding-v3 / text-embedding-ada-002

	// Chroma向量库配置
	ChromaHost string // Chroma服务地址，默认 127.0.0.1
	ChromaPort string // Chroma服务端口，默认 8000

	// 服务端口
	ServerPort string // HTTP服务端口，默认 8080

	// BoltDB文件路径
	BoltDBPath string // BoltDB数据库文件路径
}

// DefaultConfig 返回默认配置（优先从环境变量读取）
func DefaultConfig() *Config {
	return &Config{
		LLMAPIKey:      getEnv("LLM_API_KEY", "sk-89fde58dcf244d5d84ef119f899935aa"),
		LLMBaseURL:     getEnv("LLM_BASE_URL", "https://api.deepseek.com"),
		LLMModel:       getEnv("LLM_MODEL", "deepseek-chat"),
		EmbeddingModel: getEnv("EMBEDDING_MODEL", "text-embedding-v3"),
		ChromaHost:     getEnv("CHROMA_HOST", "127.0.0.1"),
		ChromaPort:     getEnv("CHROMA_PORT", "8000"),
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		BoltDBPath:     getEnv("BOLT_DB_PATH", "data/erp_graph.db"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
