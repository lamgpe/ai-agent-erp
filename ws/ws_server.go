package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// 消息类型常量
const (
	MsgTypeChat        = "chat"        // 聊天消息
	MsgTypeGraph       = "graph"       // 图谱更新
	MsgTypeTrace       = "trace"       // AI推理步骤
	MsgTypeEntity      = "entity"      // 实体新增
	MsgTypeSystem      = "system"      // 系统消息
	MsgTypeReport      = "report"      // 研判报告
)

// WSMessage WebSocket消息结构
type WSMessage struct {
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
	Time    string      `json:"time,omitempty"`
}

// Client WebSocket客户端
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub WebSocket连接中心
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	onMessage  func(messageType string, data json.RawMessage) // 消息回调
}

// NewHub 创建WebSocket Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// SetMessageHandler 设置消息处理器
func (h *Hub) SetMessageHandler(handler func(messageType string, data json.RawMessage)) {
	h.onMessage = handler
}

// Run 启动Hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
				// 广播离线后的在线人数
				go h.BroadcastClientCount()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast 广播消息给所有客户端
func (h *Hub) Broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket消息序列化失败: %v", err)
		return
	}
	h.broadcast <- data
}

// BroadcastJSON 广播任意数据
func (h *Hub) BroadcastJSON(msgType string, content interface{}) {
	h.Broadcast(WSMessage{Type: msgType, Content: content})
}

// BroadcastClientCount 广播当前在线客户端数
func (h *Hub) BroadcastClientCount() {
	count := h.ClientCount()
	h.BroadcastJSON(MsgTypeSystem, fmt.Sprintf("在线客户端: %d", count))
}

// ClientCount 获取当前客户端数
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
}

// HandleWebSocket 处理WebSocket连接
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	// 同步注册客户端（避免异步导致的计数滞后）
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	// 发送欢迎消息
	welcome := WSMessage{
		Type:    MsgTypeSystem,
		Content: fmt.Sprintf("已连接到AI智能体ERP场景案例-数据ETL/模型推理/RAG知识库/知识图谱/自主研判一体化案例 (在线客户端: %d)", h.ClientCount()),
	}
	welcomeData, _ := json.Marshal(welcome)
	client.send <- welcomeData

	// 广播在线人数
	h.BroadcastClientCount()

	// 启动读写协程
	go client.writePump()
	go client.readPump()
}

// writePump 写协程
func (c *Client) writePump() {
	defer c.conn.Close()
	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

// readPump 读协程
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		// 解析消息
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		// 如果有消息处理器，调用它
		if c.hub.onMessage != nil {
			data, _ := json.Marshal(msg.Content)
			c.hub.onMessage(msg.Type, data)
		}
	}
}

// ChatRequest WebSocket聊天请求
type ChatRequest struct {
	Query string `json:"query"`
}
