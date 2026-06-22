package frontend

import (
	"net/http"
	"os"
)

func GetFrontendHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(frontendHTML))
	}
}

func GetJSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		js, err := os.ReadFile("frontend/app.js")
		if err != nil {
			http.Error(w, "JS not found", 404)
			return
		}
		w.Write(js)
	}
}

const frontendHTML = "<!DOCTYPE html>\n<html lang=\"zh-CN\">\n" +
	headContent + bodyContent + "</html>"

const headContent = `<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>AI智能体ERP场景案例-数据ETL/模型推理/RAG知识库/知识图谱/自主研判一体化案例</title>
<script src="https://d3js.org/d3.v7.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;background:#f0f4f8;color:#1e293b;height:100vh;overflow:hidden;font-size:16px;display:flex;flex-direction:column}
#topbar{text-align:center;padding:18px 20px;background:#fff;border-bottom:1px solid #e2e8f0;flex-shrink:0;box-shadow:0 1px 3px rgba(0,0,0,.06)}
#topbar h1{font-size:20px;color:#2563eb;font-weight:800;letter-spacing:1px;margin-bottom:4px}
#topbar .sub{font-size:14px;color:#64748b;letter-spacing:.5px}
#app{display:flex;flex:1;gap:12px;background:#f0f4f8;padding:12px;overflow:hidden}
/* Card system */
.crd{background:#fff;border:1px solid #e2e8f0;border-radius:12px;display:flex;flex-direction:column;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,.04),0 1px 2px rgba(0,0,0,.06)}
.crd-hd{padding:16px 20px;background:#f8fafc;border-bottom:1px solid #e2e8f0;display:flex;align-items:center;gap:10px}
.crd-hd h2{font-size:16px;font-weight:700;color:#334155;letter-spacing:.5px}
.crd-bd{flex:1;overflow-y:auto;padding:16px}
.crd-bd::-webkit-scrollbar{width:6px}.crd-bd::-webkit-scrollbar-thumb{background:#cbd5e1;border-radius:3px}
#left-panel{display:flex;flex-direction:column;gap:12px}
.stat-row{display:flex;gap:0;background:#f1f5f9;border-radius:10px;overflow:hidden}
.stat-box{flex:1;text-align:center;padding:16px 8px;background:#fff}
.stat-box .val{display:block;font-size:32px;font-weight:800;color:#2563eb;line-height:1.1}
.stat-box .lbl{display:block;font-size:13px;color:#64748b;margin-top:6px;letter-spacing:.3px;font-weight:500}
/* Chat */
#chat-crd{flex:1;min-height:0}
.msg{padding:12px 16px;border-radius:10px;max-width:93%;font-size:16px;line-height:1.7;animation:fadeIn .2s ease;margin-bottom:8px}
.msg.user{align-self:flex-end;background:#2563eb;color:#fff}
.msg.assistant,.msg.report{align-self:flex-start;background:#f8fafc;color:#334155;border:1px solid #e2e8f0}
.msg.system{align-self:center;background:#ecfdf5;color:#065f46;font-size:11px;padding:6px 14px;border-radius:20px;border:1px solid #a7f3d0}
.msg.report{border-left:4px solid #2563eb}
.msg p{margin:3px 0}.msg h2{font-size:20px;color:#1e40af;margin:8px 0 6px;border-bottom:2px solid #e2e8f0;padding-bottom:5px}
.msg h3{font-size:18px;color:#1e40af;margin:6px 0 4px}.msg h4{font-size:16px;color:#1e40af;margin:5px 0 3px}
.msg ul,.msg ol{margin:4px 0 4px 20px}.msg li{margin:2px 0}
.msg strong{color:#0f172a}.msg code{background:#f1f5f9;padding:2px 6px;border-radius:4px;font-size:14px;color:#2563eb}
.msg table{width:100%;border-collapse:collapse;margin:6px 0;font-size:15px}
.msg th{background:#f1f5f9;color:#2563eb;padding:8px 12px;border:1px solid #e2e8f0;font-weight:600}
.msg td{padding:6px 12px;border:1px solid #e2e8f0;color:#334155}
.msg hr{border:none;border-top:1px solid #e2e8f0;margin:8px 0}
.msg pre{background:#f1f5f9;padding:10px;border-radius:6px;overflow-x:auto;font-size:14px;border:1px solid #e2e8f0}
@keyframes fadeIn{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:translateY(0)}}
.report-expand{display:inline-block;margin-left:6px;cursor:pointer;color:#64748b;font-size:14px}.report-expand:hover{color:#2563eb}
/* Input */
.inp-row{display:flex;flex-direction:column;gap:8px}
#chat-input{width:100%;height:80px;background:#fff;border:2px solid #e2e8f0;border-radius:10px;color:#1e293b;padding:10px 14px;resize:none;font-size:16px;font-family:inherit;outline:none;transition:border .2s}
#chat-input:focus{border-color:#2563eb;box-shadow:0 0 0 3px rgba(37,99,235,.1)}
.btn{padding:10px 20px;border:none;border-radius:8px;cursor:pointer;font-size:15px;font-weight:600;letter-spacing:.3px;transition:all .15s;white-space:nowrap}
.btn-p{background:#2563eb;color:#fff}.btn-p:hover{background:#1d4ed8;box-shadow:0 2px 8px rgba(37,99,235,.25)}
.btn-g{background:#fff;color:#64748b;border:2px solid #e2e8f0}.btn-g:hover{color:#ef4444;border-color:#ef4444}
#ws-status.on{color:#10b981;font-size:14px;font-weight:600}#ws-status.off{color:#ef4444;font-size:14px;font-weight:600}
/* Center: Graph */
#center-panel{flex:1;display:flex;flex-direction:column;gap:12px;min-width:0}
#graph-card{flex:1;position:relative;overflow:hidden;background:#fff}
#graph-card svg{width:100%;height:100%}
#phase-bar{position:absolute;top:0;left:0;right:0;z-index:15;pointer-events:none;opacity:0;transition:opacity .3s}
#phase-bar-inner{height:4px;background:linear-gradient(90deg,#2563eb,#10b981);width:0;transition:width .4s}
#phase-box{background:rgba(255,255,255,.97);border-bottom:1px solid #e2e8f0;padding:10px 16px;display:flex;align-items:center;gap:10px}
#phase-dot{width:8px;height:8px;border-radius:50%;background:#2563eb;animation:pulse-dot 1s infinite}
@keyframes pulse-dot{0%,100%{opacity:1}50%{opacity:.3}}
#phase-text{font-size:14px;color:#64748b}#phase-step{color:#2563eb;font-weight:700}
#graph-legend{position:absolute;bottom:12px;left:50%;transform:translateX(-50%);background:rgba(255,255,255,.96);padding:10px 14px;border-radius:8px;font-size:13px;display:flex;flex-wrap:wrap;gap:4px 12px;border:1px solid #e2e8f0;z-index:5;max-width:calc(100% - 24px);box-shadow:0 2px 8px rgba(0,0,0,.06)}
.legend-item{display:flex;align-items:center;gap:4px;white-space:nowrap;font-size:12px}.legend-dot{width:8px;height:8px;border-radius:50%}
#phase-overlay{position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);pointer-events:none;opacity:0;z-index:10;transition:opacity .3s}
#phase-overlay>div{background:rgba(255,255,255,.97);border:2px solid #2563eb;border-radius:10px;padding:16px 24px;text-align:center;box-shadow:0 4px 20px rgba(37,99,235,.12)}
.phase-title{color:#2563eb;font-size:20px;font-weight:800}.phase-desc{color:#64748b;font-size:13px;margin-top:4px}
.graph-tooltip{position:absolute;background:rgba(15,23,42,.96);color:#f1f5f9;padding:8px 12px;border-radius:6px;font-size:13px;pointer-events:none;z-index:100;border:1px solid #334155;max-width:240px}
.divider{width:4px;min-width:4px;background:#e2e8f0;cursor:col-resize;transition:background .2s;border-radius:2px}
.divider:hover,.divider.active{width:4px;min-width:4px;background:#2563eb}
/* Right panel */
#right-panel{display:flex;flex-direction:column;gap:12px}
.r-tabs{display:flex;gap:0;background:#f1f5f9;border-radius:8px;overflow:hidden;margin-bottom:10px;padding:3px}
.r-tab{flex:1;padding:8px 4px;text-align:center;cursor:pointer;font-size:14px;color:#64748b;background:transparent;border:none;transition:all .15s;font-weight:600;border-radius:6px}
.r-tab.active{background:#2563eb;color:#fff;font-weight:700;box-shadow:0 1px 3px rgba(0,0,0,.1)}
.tab-pane{display:none}.tab-pane.active{display:block}
.form-g{margin-bottom:10px}
.form-g label{display:block;font-size:13px;color:#64748b;margin-bottom:4px;font-weight:600;letter-spacing:.3px}
.form-g select,.form-g input,.form-g textarea{width:100%;background:#fff;border:2px solid #e2e8f0;border-radius:8px;color:#1e293b;padding:8px 10px;font-size:14px;outline:none;transition:border .2s}
.form-g select:focus,.form-g input:focus,.form-g textarea:focus{border-color:#2563eb;box-shadow:0 0 0 3px rgba(37,99,235,.08)}
.fbtn-row{display:flex;gap:6px;flex-wrap:wrap}.fbtn-row .btn{flex:1;text-align:center;font-size:14px;padding:10px 8px}
.r-msg{margin-top:6px;font-size:13px;line-height:1.6;max-height:120px;overflow-y:auto}
.r-msg .ok{color:#10b981}.r-msg .err{color:#ef4444}
.mini-lst{max-height:120px;overflow-y:auto;font-size:13px;color:#475569;margin-top:6px}
/* Trace */
.trace-step{font-size:14px;padding:8px 10px;margin:3px 0;border-radius:6px}
.trace-step.running{background:#fef3c7;border-left:4px solid #f59e0b}
.trace-step.done{background:#ecfdf5;border-left:4px solid #10b981;color:#065f46}
.trace-step .sn{background:#2563eb;color:#fff;min-width:20px;height:20px;border-radius:50%;display:inline-flex;align-items:center;justify-content:center;font-weight:700;font-size:11px;margin-right:8px;vertical-align:middle}
.trace-row{display:flex;gap:8px}.trace-body{flex:1;min-width:0}.trace-name{font-weight:700;color:#334155;margin-bottom:2px;font-size:14px}
.trace-desc{color:#64748b;font-size:13px}.trace-result{color:#10b981;font-size:12px;margin-top:4px;padding:4px 8px;background:rgba(16,185,129,.06);border-radius:4px;line-height:1.5}
.trace-plan{margin-top:5px;padding:6px 8px;background:rgba(37,99,235,.04);border-radius:4px;border-left:3px solid #2563eb}
.trace-branch{margin-top:4px;padding:6px 8px;background:rgba(245,158,11,.04);border-radius:4px;border-left:3px solid #f59e0b}
.plan-label,.branch-label{font-size:11px;font-weight:700;display:block;margin-bottom:3px}
.plan-label{color:#2563eb}.branch-label{color:#f59e0b}
.plan-item,.branch-item{font-size:11px;color:#64748b;display:block;padding:2px 0 2px 10px}
.active-dot{display:inline-block;width:6px;height:6px;border-radius:50%;background:#f59e0b;animation:pulse-dot .8s infinite;vertical-align:middle;margin-right:4px}
@keyframes slideIn{from{opacity:0;transform:translateX(-8px)}to{opacity:1;transform:translateX(0)}}
/* Modal */
.modal-overlay{position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(15,23,42,.6);z-index:999;display:flex;align-items:center;justify-content:center;opacity:0;pointer-events:none;transition:opacity .25s;backdrop-filter:blur(4px)}
.modal-overlay.show{opacity:1;pointer-events:auto}
.modal-box{background:#fff;border:2px solid #2563eb;border-radius:12px;max-width:720px;max-height:80vh;width:92%;overflow-y:auto;padding:24px 28px;box-shadow:0 8px 40px rgba(37,99,235,.12)}
.modal-box h3{color:#2563eb;font-size:18px;margin-bottom:12px;display:flex;justify-content:space-between;align-items:center}
.modal-close{background:none;border:none;color:#64748b;font-size:22px;cursor:pointer;padding:0 6px}.modal-close:hover{color:#ef4444}
#report-modal{position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(15,23,42,.5);z-index:998;display:flex;align-items:center;justify-content:center;opacity:0;pointer-events:none;transition:opacity .3s;backdrop-filter:blur(3px)}
#report-modal.show{opacity:1;pointer-events:auto}
#report-modal>div{background:#fff;border:2px solid #2563eb;border-radius:16px;padding:32px 48px;text-align:center;box-shadow:0 12px 48px rgba(37,99,235,.15)}
#report-modal .rpt-spinner{width:48px;height:48px;border:3px solid #e2e8f0;border-top-color:#2563eb;border-radius:50%;animation:spin .8s linear infinite;margin:0 auto 16px}
#report-modal .rpt-title{font-size:20px;font-weight:700;color:#1e293b;margin-bottom:6px}
#report-modal .rpt-sub{font-size:14px;color:#64748b}

.chart-wrap{position:relative;width:100%;max-width:550px;margin:10px 0;background:#f8fafc;border-radius:8px;padding:14px;border:1px solid #e2e8f0}
.chart-wrap canvas{max-height:300px}
.loading{display:inline-block;width:14px;height:14px;border:2px solid #2563eb;border-top-color:transparent;border-radius:50%;animation:spin .7s linear infinite;vertical-align:middle}
@keyframes spin{to{transform:rotate(360deg)}}
#graph-loading{position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:#94a3b8;font-size:18px;z-index:3}
</style></head>
`

const bodyContent = `<div id="topbar">
  <h1>AI智能体ERP场景案例-数据ETL/模型推理/RAG知识库/知识图谱/自主研判一体化案例</h1>
  <div class="sub">实体关联 · 实时推理 · 全链路研判</div>
</div>
<div id="app">
<!-- LEFT PANEL -->
<div id="left-panel" style="width:350px;max-width:480px">
  <div class="crd"><div class="stat-row">
    <div class="stat-box"><span class="val" id="stat-entities">--</span><span class="lbl">实体总数</span></div>
    <div class="stat-box"><span class="val" id="stat-triples">--</span><span class="lbl">关系链路</span></div>
    <div class="stat-box"><span class="val" id="stat-docs">--</span><span class="lbl">知识文档</span></div>
    <div class="stat-box"><span class="val" id="stat-chats">--</span><span class="lbl">会话次数</span></div>
  </div></div>
  <div class="crd" id="chat-crd">
    <div class="crd-hd"><h2>智能体对话</h2><span style="font-size:13px;margin-left:auto" id="ws-status" class="off">离线</span></div>
    <div class="crd-bd" id="chat-area" style="display:flex;flex-direction:column">
      <div class="msg system">欢迎使用AI智能体ERP场景案例-数据ETL/模型推理/RAG知识库/知识图谱/自主研判一体化案例<br>已自动加载演示数据，可直接提问。</div>
    </div>
  </div>
  <div class="crd"><div class="crd-bd"><div class="inp-row">
    <textarea id="chat-input" placeholder="输入业务问题，例如：列出所有供应商..."></textarea>
    <div style="display:flex;gap:8px">
      <button class="btn btn-p" onclick="sendChat()" style="flex:1">提交研判</button>
      <button class="btn btn-g" onclick="clearChat()">清空对话</button>
    </div>
  </div></div></div>
</div>
<div id="divider-left" class="divider"></div>
<!-- CENTER: GRAPH -->
<div id="center-panel">
  <div class="crd" id="graph-card">
    <div id="phase-bar"><div id="phase-bar-inner"></div><div id="phase-box"><div id="phase-dot"></div><span id="phase-step">就绪</span><span id="phase-text">等待研判请求</span></div></div>
    <div id="phase-overlay"><div><div class="phase-title"></div><div class="phase-desc"></div></div></div>
    <div id="graph-loading">图谱加载中...</div>
    <div id="graph-legend">
      <div class="legend-item"><span class="legend-dot" style="background:#ef4444"></span>供应商</div>
      <div class="legend-item"><span class="legend-dot" style="background:#10b981"></span>物料</div>
      <div class="legend-item"><span class="legend-dot" style="background:#06b6d4"></span>采购合同</div>
      <div class="legend-item"><span class="legend-dot" style="background:#3b82f6"></span>采购订单</div>
      <div class="legend-item"><span class="legend-dot" style="background:#f97316"></span>入库单</div>
      <div class="legend-item"><span class="legend-dot" style="background:#ec4899"></span>生产工单</div>
      <div class="legend-item"><span class="legend-dot" style="background:#9ca3af"></span>质检</div>
      <div class="legend-item"><span class="legend-dot" style="background:#92400e"></span>设备</div>
      <div class="legend-item"><span class="legend-dot" style="background:#f59e0b"></span>物流</div>
      <div class="legend-item"><span class="legend-dot" style="background:#8b5cf6"></span>应付发票</div>
      <div class="legend-item"><span class="legend-dot" style="background:#047857"></span>销售订单</div>
      <div class="legend-item"><span class="legend-dot" style="background:#b91c1c"></span>应收账单</div>
    </div>
  </div>
</div>
<div id="divider-right" class="divider"></div>
<!-- RIGHT PANEL -->
<div id="right-panel" style="width:320px;max-width:460px">
  <div class="crd">
    <div class="crd-hd"><h2>连接器</h2></div>
    <div class="crd-bd">
      <div class="r-tabs">
        <button class="r-tab active" onclick="switchRTab('sync')">API同步</button>
        <button class="r-tab" onclick="switchRTab('manual')">手动录入</button>
        <button class="r-tab" onclick="switchRTab('kb')">知识库</button>
      </div>
      <div id="tp-sync" class="tab-pane active">
        <div class="form-g"><label>实体类型</label><select id="sync-etype"><option value="supplier">供应商</option><option value="material_stock">物料库存</option><option value="purchase_contract">采购合同</option><option value="purchase_order">采购订单</option><option value="warehouse_receipt">入库单</option><option value="production_order">生产工单</option><option value="quality_inspection">质检报告</option><option value="equipment">设备资产</option><option value="logistics">物流承运</option><option value="finance_invoice">应付发票</option><option value="sales_order">销售订单</option><option value="accounts_receivable">应收账单</option></select></div>
        <div class="fbtn-row"><button class="btn btn-p" onclick="syncEntity()">拉取</button><button class="btn btn-p" onclick="batchSyncAll()" style="background:#10b981">全部同步</button></div>
        <div id="sync-result" class="r-msg"></div>
      </div>
      <div id="tp-manual" class="tab-pane">
        <div class="form-g"><label>实体类型</label><select id="manual-etype" onchange="onManualTypeChange()"><option value="supplier">供应商</option><option value="material_stock">物料库存</option><option value="purchase_contract">采购合同</option><option value="purchase_order">采购订单</option><option value="warehouse_receipt">入库单</option><option value="production_order">生产工单</option><option value="quality_inspection">质检报告</option><option value="equipment">设备资产</option><option value="logistics">物流承运</option><option value="finance_invoice">应付发票</option><option value="sales_order">销售订单</option><option value="accounts_receivable">应收账单</option></select></div>
        <div class="form-g"><label>实体名称</label><input id="manual-name" placeholder="输入实体名称..."></div>
        <div class="form-g"><label>属性 <span style="color:#2563eb;cursor:pointer;font-size:12px" onclick="window.addAttrRow()">+ 添加</span></label>
          <div id="manual-attrs"><div class="attr-row" style="display:flex;gap:6px;margin-bottom:4px"><input placeholder="属性名" class="attr-key" style="flex:1"><input placeholder="属性值" class="attr-val" style="flex:1"><button class="btn btn-g" onclick="this.parentElement.remove()" style="padding:4px 8px;font-size:11px">×</button></div></div>
        </div>
        <div class="form-g"><label>关联关系 <span style="color:#2563eb;cursor:pointer;font-size:12px" onclick="window.addRelRow()">+ 添加</span></label>
          <div id="manual-rels"></div>
        </div>
        <button class="btn btn-p" onclick="createManualEntityV2()" style="width:100%;margin-top:6px">创建实体</button>
        <div id="manual-result" class="r-msg"></div>
      </div>
      <div id="tp-kb" class="tab-pane">
        <div class="form-g"><label>上传文件 (txt / md / json / csv)</label><input type="file" id="kb-file" accept=".txt,.md,.json,.csv"></div>
        <div class="fbtn-row"><button class="btn btn-p" onclick="uploadKB()" style="flex:2">上传并向量化</button><button class="btn btn-g" onclick="clearKB()" style="flex:1">清空</button></div>
        <div id="kb-result" class="r-msg"></div>
        <div id="kb-list" class="mini-lst"></div>
      </div>
    </div>
  </div>
  <div class="crd" style="flex:1;min-height:0">
    <div class="crd-hd"><h2>AI 推理链路</h2></div>
    <div class="crd-bd" id="trace-content"><span style="color:#94a3b8;font-size:13px">等待业务查询触发推理链路...</span></div>
  </div>
</div>
</div>
<div id="tooltip" class="graph-tooltip" style="display:none"></div>
<div id="modal-overlay" class="modal-overlay" onclick="if(event.target===this)closeModal()">
  <div class="modal-box" id="modal-box"><h3><span id="modal-title"></span><button class="modal-close" onclick="closeModal()">&times;</button></h3><div id="modal-body"></div></div>
</div>
<script src="/app.js"></script>
<div id="report-modal"><div><div class="rpt-spinner"></div><div class="rpt-title">正在生成研判报告...</div><div class="rpt-sub">AI正在分析数据并撰写报告，请稍候</div></div></div>
</body>
`
