# AI 智能体 ERP 场景案例

> 数据ETL / 模型推理 / RAG知识库 / 知识图谱 / 自主研判一体化案例
>
> **Palantir 模式情报研判平台**

基于 Go 语言构建的 AI Agent ERP 智能研判平台，模拟 Palantir 情报分析模式，融合大语言模型（LLM）、RAG 检索增强生成、知识图谱与实时可视化技术，实现从数据同步 → 语义检索 → 图谱遍历 → 推演预判 → 研判报告生成的全链路 AI 决策。

![平台演示](docs/demo.gif)

---

## 功能特性

| 模块 | 说明 |
|---|---|
| **数据 ETL** | 12 类 ERP 实体自动同步（供应商→采购→库存→生产→质检→物流→财务→销售），支持 Mock 数据与真实 API |
| **模型推理** | DeepSeek 大模型驱动，实现意图识别、实体抽取、业务查询分析、研判报告生成 |
| **RAG 知识库** | Chroma 向量数据库 + 关键词回退，语义搜索融合知识图谱深度遍历 |
| **知识图谱** | BoltDB 嵌入式图存储，D3.js 力导向图实时渲染，支持多度关系链发现 |
| **自主研判** | Palantir 4 步决策链路：意图识别 → 向量检索 → 图谱遍历 → 推演预判 + 7 维风险评分 |
| **实时可视化** | WebSocket 推送每一步推理过程与图谱增量，前端 D3 动态渲染 |
| **优雅降级** | Chroma 不可用→关键词搜索；LLM 不可用→规则引擎，核心功能零外部依赖 |

---

## 技术栈

| 技术 | 用途 |
|---|---|
| **Go 1.23** | 后端服务 |
| **Gorilla WebSocket** | 实时通信 |
| **BoltDB** (bbolt) | 嵌入式图数据库 |
| **Chroma** | 向量检索引擎 |
| **DeepSeek API** | 大语言模型（OpenAI 兼容协议） |
| **D3.js v7** | 知识图谱可视化 |
| **Chart.js v4** | 统计图表 |
| **net/http** | HTTP 服务（零外部框架依赖） |

---

## 快速开始

### 环境要求

- Go 1.23+
- Chroma 向量数据库（可选，不配置则降级为关键词搜索）
- DeepSeek API Key

### 环境变量配置

```bash
# LLM 配置（必填）
export LLM_API_KEY="your-deepseek-api-key"
export LLM_BASE_URL="https://api.deepseek.com"
export LLM_MODEL="deepseek-chat"

# Chroma 向量数据库（可选）
export CHROMA_HOST="localhost"
export CHROMA_PORT="8000"

# 服务配置（可选，有默认值）
export SERVER_PORT="8080"
export BOLT_DB_PATH="./data/erp_graph.db"
```

### 启动服务

```bash
# 克隆项目
git clone https://github.com/lamgpe/ai-agent-erp.git
cd ai-agent-erp-

# 安装依赖
go mod tidy

# 启动（自动加载 240 条种子数据）
go run main.go
```

服务启动后访问 **http://localhost:8080** 即可进入研判平台。

---

## 项目结构

```
├── main.go                  # 入口：初始化、路由、种子数据
├── go.mod / go.sum          # Go 模块
├── config/
│   └── config.go            # 全局配置（环境变量驱动）
├── ai/
│   ├── langchain_client.go  # LLM 客户端（DeepSeek API）
│   ├── rag_engine.go        # RAG 检索引擎
│   └── decision_trace.go    # Palantir 决策链路追踪
├── api/
│   ├── entity_auto_api.go   # 实体自动同步
│   ├── entity_manual.go     # 手工录入
│   └── mock_erp.go          # Mock ERP 接口
├── graphdb/
│   └── bolt_graph.go        # 图谱存储层（CRUD/遍历）
├── vectorstore/
│   └── chroma_client.go     # Chroma 向量存储客户端
├── ws/
│   └── ws_server.go         # WebSocket 实时推送
├── frontend/
│   ├── page.go              # 内联前端 HTML + 静态服务
│   └── app.js               # 前端 JS（D3/Chart/WebSocket）
├── mock_json/               # 12 类 × 20 条 Mock 数据
└── data/                    # BoltDB + Chroma 持久化目录
```

---

## API 端点

### 数据同步

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/sync` | POST | 单类型实体同步 |
| `/api/sync/batch` | POST | 批量同步全部 12 类实体 |
| `/mock/erp/{type}[/{id}]` | GET | Mock ERP 数据拉取 |

### 实体录入

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/manual` | POST | 手工录入（LLM 解析自然语言） |
| `/api/manual/v2` | POST | 手工录入 v2（自定义属性+关系） |

### 查询与可视化

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/entities` | GET | 获取所有已入库实体 |
| `/api/graph` | GET | 获取 D3 格式图谱数据 |
| `/api/stats` | GET | 统计信息 |
| `/api/chat` | POST | HTTP 聊天研判 |

### 知识库

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/knowledge/upload` | POST | 文档上传向量化 |
| `/api/knowledge/list` | GET | 文档列表 |
| `/api/knowledge/clear` | POST | 清空知识库 |

### 实时通信

| 端点 | 方法 | 说明 |
|---|---|---|
| `/ws` | WebSocket | 实时研判推送 |

---

## Palantir 研判流程

```
用户查询
  ↓
① 意图识别 ── LLM 分析领域/意图/检索参数
  ↓
② 向量检索 ── RAG 语义搜索 + 关键词回退，命中实体推送 D3 图谱
  ↓
③ 图谱遍历 ── 沿供应链关系链多度遍历，发现隐藏关联
  ↓
④ 推演预判 ── AI 预测下一步核查链路 + 7 维风险评分
  ↓
📄 生成研判报告
```

每一步均通过 **WebSocket** 实时推送到前端可视化展示。

---

## Mock 数据覆盖

| 序号 | 实体类型 | 业务领域 | 数量 |
|---|---|---|---|
| 1 | 供应商 (supplier) | 采购云 | 20 |
| 2 | 采购合同 (purchase_contract) | 采购云 | 20 |
| 3 | 采购订单 (purchase_order) | 采购云 | 20 |
| 4 | 物料库存 (material_stock) | 供应链云 | 20 |
| 5 | 入库单 (warehouse_receipt) | 供应链云 | 20 |
| 6 | 生产订单 (production_order) | 制造云 | 20 |
| 7 | 质检报告 (quality_inspection) | 质量云 | 20 |
| 8 | 设备台账 (equipment) | 资产云 | 20 |
| 9 | 物流运单 (logistics) | 供应链云 | 20 |
| 10 | 财务发票 (finance_invoice) | 财务云 | 20 |
| 11 | 销售订单 (sales_order) | 营销云 | 20 |
| 12 | 应收账款 (accounts_receivable) | 财务云 | 20 |

---

## 相关文档

- [AI智能体ERP场景案例_技术总结.md](./AI智能体ERP场景案例_技术总结.md) — 架构设计详解
- [Palantir模式Agent平台_总结.md](./Palantir模式Agent平台_总结.md) — Palantir 模式设计

---

## License

MIT License
