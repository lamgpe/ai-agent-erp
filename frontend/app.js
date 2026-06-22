
// ============ WebSocket ============
let ws;
const wsStatus = document.getElementById('ws-status');
function connectWS() {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(protocol+'//'+location.host+'/ws');
  ws.onopen = () => { wsStatus.textContent = '已连接'; wsStatus.className = 'on'; };
  ws.onclose = () => { wsStatus.textContent = '断开'; wsStatus.className = 'off'; setTimeout(connectWS, 3000); };
  ws.onerror = () => { wsStatus.textContent = '错误'; wsStatus.className = 'off'; };
  ws.onmessage = (e) => { handleWSMessage(JSON.parse(e.data)); };
}
connectWS();

function handleWSMessage(msg) {
  switch(msg.type) {
    case 'chat': addChatMessage('assistant', msg.content); break;
    case 'system':
      addChatMessage('system', msg.content);
      var m = (msg.content||'').match(/在线客户端:\s*(\d+)/);
      if (m) {} // client count handled elsewhere
      break;
    case 'report':
      addChatMessage('report', msg.content);
      // 报告返回，隐藏阶段进度条
      hidePhaseBar();
      hideReportModal();
      break;
    case 'trace': updateTrace(msg.content); break;
    case 'entity': refreshGraph(); break;
    case 'graph':
      if (msg.content && msg.content.phase) {
        console.log('[Graph] phase='+msg.content.phase+' nodes='+(msg.content.nodes||[]).length+' links='+(msg.content.links||[]).length);
        addToGraph(msg.content);
      }
      break;
  }
}

// ============ Phase Bar ============
function showPhaseBar(phase, desc) {
  var bar = document.getElementById('phase-bar');
  var inner = document.getElementById('phase-bar-inner');
  var step = document.getElementById('phase-step');
  var text = document.getElementById('phase-text');
  var phases = {retrieval:'语义检索',traversal:'图谱遍历',prediction:'AI推演预判'};
  var widths = {retrieval:'25%',traversal:'50%',scoring:'65%',prediction:'85%'};
  step.textContent = phases[phase] || phase;
  text.textContent = desc || '';
  inner.style.width = widths[phase] || '50%';
  bar.style.opacity = '1';
}
function hidePhaseBar() {
  var bar = document.getElementById('phase-bar');
  var inner = document.getElementById('phase-bar-inner');
  inner.style.width = '100%';
  setTimeout(function(){ bar.style.opacity = '0'; }, 600);
}

// ============ Chat ============
let isJudging = false;
function sendChat() {
  if (isJudging) return;
  const input = document.getElementById('chat-input');
  const query = input.value.trim();
  if (!query) return;
  addChatMessage('user', query);
  input.value = '';
  var cl=document.getElementById('chat-loading');if(cl)cl.style.display='inline';
  isJudging = true;
  // 显示阶段条初始状态
  showPhaseBar('retrieval', '正在分析问题意图...');
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({type: 'chat', content: {query: query}}));
  } else {
    fetch('/api/chat', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({query:query})})
      .then(r=>r.json()).then(data=>{
        var cl=document.getElementById('chat-loading');if(cl)cl.style.display='none';
        isJudging = false;
        hidePhaseBar();
      hideReportModal();
        if(data.report) addChatMessage('report', data.report);
        if(data.answer) addChatMessage('assistant', data.answer);
      }).catch(err=>{
        var cl=document.getElementById('chat-loading');if(cl)cl.style.display='none';
        isJudging = false;
        hidePhaseBar();
      hideReportModal();
        addChatMessage('system', '请求失败: '+err.message);
      });
  }
}

function addChatMessage(type, content) {
  if (type === 'report' || type === 'assistant') {
    var cl=document.getElementById('chat-loading');if(cl)cl.style.display='none';
    isJudging = false;
  }
  const area = document.getElementById('chat-area');
  const div = document.createElement('div');
  div.className = 'msg ' + type;
  if (type === 'report') {
    div.innerHTML = renderMarkdown(content) + '<span class=report-expand title=点击查看完整报告 onclick="showModal(\'研判报告\',this.parentElement.innerHTML.replace(/<span[^>]*>.*<\\/span>/,\'\'))">&#x26F6;</span>';
  }
  else { div.textContent = content; }
  area.appendChild(div);
  area.scrollTop = area.scrollHeight;
  if(type==='report'){ setTimeout(function(){ renderChartIfNeeded(content, div) }, 200); }
}

function clearChat() {
  document.getElementById('chat-area').innerHTML = '';
  document.getElementById('trace-content').innerHTML = '<span style="color:#3a4a5a;font-size:11px;">等待业务查询触发推理链路...</span>';
  allNodes = []; allLinks = [];
  renderGraph('full');
}

// ============ Markdown ============
function renderMarkdown(text) {
  if (!text) return '';
  text = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
  var lines = text.split('\n'), html = '', inList = false, listType = '', inTable = false, tableRows = [];
  for (var i = 0; i < lines.length; i++) {
    var line = lines[i], t = line.trim();
    if (t.startsWith('\x60\x60\x60')) { flushTable(); inList=false;listType=''; html += '<pre><code>'; continue; }
    var isTR = t.startsWith('|') && t.endsWith('|');
    var isAR = /^\|[\s\-:]+(\|[\s\-:]+)*\|$/.test(t);
    if (isTR && !isAR) { if(!inTable){closeList();inTable=true;tableRows=[];} tableRows.push(t); continue; }
    if (isAR && inTable) { tableRows.push(t); continue; }
    if (inTable) { flushTable(); }
    if (t === '') { closeList(); continue; }
    var h3 = t.match(/^###\s+(.+)/); if(h3){closeList();html+='<h3>'+inlineMd(h3[1])+'</h3>';continue;}
    var h2 = t.match(/^##\s+(.+)/); if(h2){closeList();html+='<h2>'+inlineMd(h2[1])+'</h2>';continue;}
    var h4 = t.match(/^####\s+(.+)/); if(h4){closeList();html+='<h4>'+inlineMd(h4[1])+'</h4>';continue;}
    if (t.match(/^(---|\*\*\*|___)$/)) { closeList(); html+='<hr>'; continue; }
    var ul = t.match(/^[\-\*]\s+(.+)/);
    if (ul) { if(!inList||listType!=='ul'){closeList();html+='<ul>';inList=true;listType='ul';} html+='<li>'+inlineMd(ul[1])+'</li>'; continue; }
    var ol = t.match(/^\d+\.\s+(.+)/);
    if (ol) { if(!inList||listType!=='ol'){closeList();html+='<ol>';inList=true;listType='ol';} html+='<li>'+inlineMd(ol[1])+'</li>'; continue; }
    closeList(); html+='<p>'+inlineMd(line)+'</p>';
  }
  flushTable(); closeList(); return html;
  function closeList(){if(inList){html+=(listType==='ul'?'</ul>':'</ol>');inList=false;listType='';}}
  function flushTable(){
    if(!inTable||tableRows.length===0){inTable=false;return;}
    var rows=[]; for(var r=0;r<tableRows.length;r++){if(!/^\|[\s\-:]+(\|[\s\-:]+)*\|$/.test(tableRows[r]))rows.push(tableRows[r]);}
    if(rows.length===0){inTable=false;tableRows=[];return;}
    html+='<table>';
    if(rows.length>=1){html+='<thead><tr>';var hc=rows[0].split('|').filter(function(_,i,s){return i>0&&i<s.length-1});for(var c=0;c<hc.length;c++)html+='<th>'+inlineMd(hc[c].trim())+'</th>';html+='</tr></thead>';}
    if(rows.length>1){html+='<tbody>';for(r=1;r<rows.length;r++){html+='<tr>';var dc=rows[r].split('|').filter(function(_,i,s){return i>0&&i<s.length-1});for(c=0;c<dc.length;c++)html+='<td>'+inlineMd(dc[c].trim())+'</td>';html+='</tr>';}html+='</tbody>';}
    html+='</table>';inTable=false;tableRows=[];
  }
}
function inlineMd(t){if(!t)return'';return t.replace(/\*\*(.+?)\*\*/g,'<strong>$1</strong>').replace(/\*(.+?)\*/g,'<em>$1</em>').replace(/\x60([^\x60]+)\x60/g,'<code>$1</code>');}

// ============ Right Tabs ============
function switchRTab(id) {
  document.querySelectorAll('.r-tab').forEach(function(t){t.classList.remove('active')});
  document.querySelectorAll('.tab-pane').forEach(function(p){p.classList.remove('active')});
  var tabEl = document.querySelector('.r-tab[onclick*=\"'+id+'\"]');
  if(tabEl) tabEl.classList.add('active');
  var pane = document.getElementById('tp-'+id);
  if(pane) pane.classList.add('active');
  if(id==='lowcode'){ loadEntityAttrs(); }
  if(id==='kb'){ loadKBList(); }
}

// ============ API Sync ============
function syncEntity() {
  var entityType = document.getElementById('sync-etype').value;
  var div = document.getElementById('sync-result');
  div.innerHTML = '<span class="loading"></span> 正在拉取...';
  fetch('/api/sync', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({entityType:entityType})})
    .then(r=>r.json()).then(function(data){
      if(data.success){ div.innerHTML='<span class=ok>[OK] '+data.message+'</span>'; allNodes=[];allLinks=[];refreshGraph();updateEntityCount(); }
      else{ div.innerHTML='<span class=err>[ERR] '+(data.errorMessage||data.message)+'</span>'; }
    }).catch(function(err){ div.innerHTML='<span class=err>[ERR] '+err.message+'</span>'; });

function showReportModal(){
  var m=document.getElementById("report-modal");
  if(m)m.classList.add("show");
}
function hideReportModal(){
  var m=document.getElementById("report-modal");
  if(m)m.classList.remove("show");
}
}
function batchSyncAll() {
  var div = document.getElementById('sync-result');
  div.innerHTML = '<span class="loading"></span> 批量同步中...';
  fetch('/api/sync/batch', {method:'POST'}).then(r=>r.json()).then(function(data){
    if(data.success){ div.innerHTML='<span class=ok>[OK] 已入库 '+data.totalEntities+' 条</span>'; allNodes=[];allLinks=[];refreshGraph();updateEntityCount(); }
    else{ div.innerHTML='<span class=err>[ERR]</span>'; }
  }).catch(function(err){ div.innerHTML='<span class=err>[ERR] '+err.message+'</span>'; });
}

// ============ Manual Entry ============
function addAttrRow(){
  var d=document.createElement('div');d.className='attr-row';
  d.style='display:flex;gap:4px;margin-bottom:4px';
  d.innerHTML='<input placeholder="属性名" class="attr-key" style="flex:1;font-size:10px;background:#0d141d;border:1px solid #1a2836;border-radius:3px;color:#b0b8c0;padding:4px 6px"><input placeholder="属性值" class="attr-val" style="flex:1;font-size:10px;background:#0d141d;border:1px solid #1a2836;border-radius:3px;color:#b0b8c0;padding:4px 6px"><button class="btn btn-g" onclick="this.parentElement.remove()" style="padding:3px 8px;font-size:9px;flex-shrink:0">x</button>';
  document.getElementById('manual-attrs').appendChild(d);
}
function addRelRow(){
  // Fetch entities for dropdown
  var opts='<option value="">-- 选择实体 --</option>';
  try{
    allNodes.forEach(function(n){ if(n.id&&n.name) opts+='<option value="'+n.id+'">'+n.name+' ('+n.id+')</option>'; });
  }catch(e){}
  var d=document.createElement('div');d.className='rel-row';
  d.style='display:flex;gap:3px;margin-bottom:4px;align-items:center';
  d.innerHTML='<select class="rel-type" style="flex:1;font-size:10px;background:#0d141d;border:1px solid #1a2836;border-radius:3px;color:#b0b8c0;padding:4px"><option>供应</option><option>执行采购</option><option>签约</option><option>物流承运</option><option>应付发票</option><option>应收账款</option><option>质量检验</option><option>生产执行</option><option>入库验收</option><option>销售出库</option><option>使用设备</option></select><select class="rel-target" style="flex:1;font-size:10px;background:#0d141d;border:1px solid #1a2836;border-radius:3px;color:#b0b8c0;padding:4px">'+opts+'</select><button class="btn btn-g" onclick="this.parentElement.remove()" style="padding:3px 8px;font-size:9px;flex-shrink:0">x</button>';
  document.getElementById('manual-rels').appendChild(d);
}
function onManualTypeChange(){}
function createManualEntityV2(){
  var etype=document.getElementById('manual-etype').value;
  var ename=document.getElementById('manual-name').value.trim();
  if(!ename){alert('请输入实体名称');return}
  var props={name:ename};
  document.querySelectorAll('#manual-attrs .attr-row').forEach(function(r){
    var k=r.querySelector('.attr-key').value.trim();
    var v=r.querySelector('.attr-val').value.trim();
    if(k)props[k]=v;
  });
  var rels=[];
  document.querySelectorAll('#manual-rels .rel-row').forEach(function(r){
    var rt=r.querySelector('.rel-type').value;
    var tid=r.querySelector('.rel-target').value;
    if(tid)rels.push({relation:rt,targetId:tid});
  });
  var div=document.getElementById('manual-result');
  div.innerHTML='<span class=loading></span>';
  fetch('/api/manual/v2',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({entityType:etype,name:ename,properties:props,relations:rels})})
    .then(function(r){return r.json()}).then(function(d){
      if(d.success){div.innerHTML='<span class=ok>[OK] '+d.message+' ('+d.entityId+')</span>';allNodes=[];allLinks=[];refreshGraph();updateStats()}
      else div.innerHTML='<span class=err>[ERR] '+d.message+'</span>';
    }).catch(function(){div.innerHTML='<span class=err>[ERR]</span>'});
}
const colors={1:'#ef4444',2:'#10b981',3:'#3b82f6',4:'#f59e0b',5:'#8b5cf6',6:'#06b6d4',7:'#f97316',8:'#ec4899',9:'#9ca3af',10:'#92400e',11:'#047857',12:'#b91c1c',0:'#6b7280'};
const colorMap={supplier:1,material_stock:2,purchase_order:3,logistics:4,finance_invoice:5,purchase_contract:6,warehouse_receipt:7,production_order:8,quality_inspection:9,equipment:10,sales_order:11,accounts_receivable:12};
const glowColors={1:'#ff6666',2:'#34d399',3:'#60a5fa',4:'#fbbf24',5:'#a78bfa',6:'#22d3ee',7:'#fb923c',8:'#f472b6',9:'#c4c9d0',10:'#b45309',11:'#059669',12:'#dc2626',0:'#9ca3af'};
var simulation,svg,linkGroup,nodeGroup,allNodes=[],allLinks=[];

function initGraph() {
  var container = document.getElementById('graph-card');
  svg = d3.select('#graph-card').append('svg');
  var W = container.clientWidth, H = container.clientHeight;
  svg.append('defs').append('marker').attr('id','arrow').attr('viewBox','0 -5 10 10').attr('refX',22).attr('refY',0).attr('markerWidth',6).attr('markerHeight',6).attr('orient','auto').append('path').attr('d','M0,-5L10,0L0,5').attr('fill','#3a4a5a');
  svg.select('defs').append('marker').attr('id','arrow-active').attr('viewBox','0 -5 10 10').attr('refX',22).attr('refY',0).attr('markerWidth',7).attr('markerHeight',7).attr('orient','auto').append('path').attr('d','M0,-5L10,0L0,5').attr('fill','#00d4ff');
  svg.select('defs').append('marker').attr('id','arrow-dashed').attr('viewBox','0 -5 10 10').attr('refX',22).attr('refY',0).attr('markerWidth',6).attr('markerHeight',6).attr('orient','auto').append('path').attr('d','M0,-5L10,0L0,5').attr('fill','#f59e0b');
  var f1 = svg.select('defs').append('filter').attr('id','glow'); f1.append('feGaussianBlur').attr('stdDeviation','4').attr('result','blur'); var m1 = f1.append('feMerge'); m1.append('feMergeNode').attr('in','blur'); m1.append('feMergeNode').attr('in','blur'); m1.append('feMergeNode').attr('in','SourceGraphic');
  var f2 = svg.select('defs').append('filter').attr('id','superglow'); f2.append('feGaussianBlur').attr('stdDeviation','6').attr('result','blur'); var m2 = f2.append('feMerge'); m2.append('feMergeNode').attr('in','blur'); m2.append('feMergeNode').attr('in','blur'); m2.append('feMergeNode').attr('in','SourceGraphic');
  linkGroup = svg.append('g').attr('class','links'); nodeGroup = svg.append('g').attr('class','nodes');
  svg.append('g').attr('class','rings');
  simulation = d3.forceSimulation().force('link',d3.forceLink().id(function(d){return d.id}).distance(100)).force('charge',d3.forceManyBody().strength(-400)).force('center',d3.forceCenter(W/2,H/2)).force('collision',d3.forceCollide(35)).alphaDecay(0.015);
  svg.call(d3.zoom().scaleExtent([0.2,4]).on('zoom',function(e){svg.selectAll('g').attr('transform',e.transform)}));
  window.addEventListener('resize',function(){var w2=container.clientWidth,h2=container.clientHeight;simulation.force('center',d3.forceCenter(w2/2,h2/2));simulation.alpha(0.3).restart()});
}

function addToGraph(graphData) {
  if (!graphData) return;
  var phase = graphData.phase || 'full';
  var incomingNodes = (graphData.nodes||[]).map(function(n){return Object.assign({},n,{group:n.group||(colorMap[n.type]||0),_phase:phase,_isNew:n.isNew!==false})});
  var incomingLinks = (graphData.links||[]).map(function(l){return{source:l.source,target:l.target,relation:l.relation,lineType:l.lineType||'solid',_phase:phase,_isNew:l.isNew!==false}});
  var nodeMap = {}; allNodes.forEach(function(n){nodeMap[n.id]=n});
  // 给新节点赋予初始位置：查找已存在邻居节点的中心点
  var newIds = []; incomingNodes.forEach(function(n){if(!nodeMap[n.id]){newIds.push(n.id)}});
  incomingNodes.forEach(function(n){
    if(!nodeMap[n.id]){
      // 尝试从已有连线中找到邻居
      var cx = null, cy = null, neighborCount = 0;
      incomingLinks.forEach(function(l){
        var neighborId = null;
        if(l.source===n.id) neighborId = l.target;
        if(l.target===n.id) neighborId = l.source;
        if(neighborId && nodeMap[neighborId] && nodeMap[neighborId].x!==undefined){
          if(cx===null){cx=nodeMap[neighborId].x;cy=nodeMap[neighborId].y;neighborCount=1}
          else{cx+=nodeMap[neighborId].x;cy+=nodeMap[neighborId].y;neighborCount++}
        }
        // 也检查allNodes中的邻居
        if(neighborId && !nodeMap[neighborId]){
          var nn = allNodes.find(function(x){return x.id===neighborId});
          if(nn && nn.x!==undefined){
            if(cx===null){cx=nn.x;cy=nn.y;neighborCount=1}
            else{cx+=nn.x;cy+=nn.y;neighborCount++}
          }
        }
      });
      if(cx!==null){n.x=cx/neighborCount; n.y=cy/neighborCount}
      else{var W=document.getElementById('graph-card').clientWidth; n.x=W/2+(Math.random()-0.5)*200; n.y=200+(Math.random()-0.5)*200}
      nodeMap[n.id]=n; allNodes.push(n);
    }else{Object.assign(nodeMap[n.id],n)}
  });
  var linkMap = {}; allLinks.forEach(function(l){linkMap[l.source+'-'+l.relation+'-'+l.target]=l});
  incomingLinks.forEach(function(l){var k=l.source+'-'+l.relation+'-'+l.target;if(!linkMap[k]){linkMap[k]=l;allLinks.push(l)}});
  showPhaseBar(phase, graphData.description||'');
  showPhaseOverlay(phase, graphData.description||'');
  renderGraph(phase);
  // 双重保障缩放
  setTimeout(function(){zoomToFit()}, 1500);
  // 非预判阶段才自动清除高亮（预判阶段的亮灯由spotlightPrediction自行管理）
  if(graphData.focusNodeId && phase !== 'prediction'){
    showRipple(graphData.focusNodeId);
    highlightNode(graphData.focusNodeId);
    setTimeout(function(){clearHighlight()},5000);
  }
}

function showPhaseOverlay(phase,desc){
  var ov=document.getElementById('phase-overlay');if(!ov)return;
  var config={retrieval:{t:'语义检索',d:'正在从向量库检索相关实体...'},traversal:{t:'图谱遍历',d:'正在遍历实体上下游关联链路...'},prediction:{t:'AI推演预判',d:'正在分析链路并预测待核查节点...'},full:{t:'加载完成',d:'全链路图谱已就绪'}};
  var cfg=config[phase]||{t:phase,d:desc};
  ov.querySelector('.phase-title').textContent=cfg.t;ov.querySelector('.phase-desc').textContent=desc||cfg.d;
  ov.style.opacity='1';ov.style.transform='translate(-50%,-50%) scale(1)';
  setTimeout(function(){ov.style.opacity='0';ov.style.transform='translate(-50%,-50%) scale(.8)'},2800)
}
function showRipple(nodeId){
  var node=allNodes.find(function(n){return n.id===nodeId});if(!node||!node.x)return;
  var rings=d3.select('.rings');if(rings.empty())return;
  for(var i=0;i<3;i++){rings.append('circle').attr('cx',node.x).attr('cy',node.y).attr('r',12).attr('fill','none').attr('stroke','#00d4ff').attr('stroke-width',3-i*.8).attr('opacity',.8).transition().delay(i*200).duration(1500).attr('r',80).attr('opacity',0).remove()}
}

function renderGraph(phase) {
  if (!linkGroup || !nodeGroup) return;
  var tooltip = d3.select('#tooltip');
  // 过滤有效links
  var validNodeIds = {}; allNodes.forEach(function(n){validNodeIds[n.id]=true});
  var validLinks = allLinks.filter(function(l){var s=typeof l.source==='object'?l.source.id:l.source;var t=typeof l.target==='object'?l.target.id:l.target;return validNodeIds[s]&&validNodeIds[t]});
  // links
  var link = linkGroup.selectAll('line').data(validLinks, function(d){return d.source+'-'+d.relation+'-'+d.target});
  link.exit().transition().duration(300).attr('opacity',0).remove();
  var linkEnter = link.enter().append('line').attr('stroke',function(d){return d.lineType==='dashed'?'#f59e0b':'#3a4a5a'}).attr('stroke-width',function(d){return d.lineType==='dashed'?3.5:1.2}).attr('stroke-dasharray',function(d){return d.lineType==='dashed'?'10,5':'none'}).attr('marker-end',function(d){return d.lineType==='dashed'?'url(#arrow-dashed)':'url(#arrow)'}).attr('opacity',0);
  linkEnter.transition().duration(600).attr('opacity',1);
  if(phase==='prediction'){
    // 预判链路首次渲染就加粗+流动
    linkEnter.filter(function(d){return d.lineType==='dashed'})
      .attr('stroke','#f59e0b').attr('stroke-width',4).attr('stroke-dasharray','12,6')
      .each(function(){var el=d3.select(this);var off=0;var t=setInterval(function(){off=(off+2)%36;el.attr('stroke-dashoffset',off)},40);setTimeout(function(){clearInterval(t)},8000)});
    // 预判链路亮灯效果：延迟等渲染完成后触发
    setTimeout(function(){ spotlightPrediction(); }, 1200);
  }
  var linkAll = linkEnter.merge(link);
  // nodes
  var node = nodeGroup.selectAll('circle').data(allNodes, function(d){return d.id});
  node.exit().transition().duration(300).attr('r',0).remove();
  var nodeEnter = node.enter().append('circle').attr('r',0).attr('fill',function(d){return colors[d.group]||'#6b7280'}).attr('stroke','rgba(255,255,255,.3)').attr('stroke-width',1.2).attr('cursor','pointer').attr('filter',function(d){return d.isFocus?'url(#glow)':null});
  nodeEnter.transition().duration(500).attr('r',function(d){return d.isFocus?12:7});
  nodeEnter.call(d3.drag().on('start',function(e,d){if(!e.active)simulation.alphaTarget(.3).restart();d.fx=d.x;d.fy=d.y}).on('drag',function(e,d){d.fx=e.x;d.fy=e.y}).on('end',function(e,d){if(!e.active)simulation.alphaTarget(0);d.fx=null;d.fy=null}));
  nodeEnter.on('mouseover',function(e,d){var cn=typeNameCN(d.type);tooltip.style('display','block').html('<strong>'+d.name+'</strong><br>类型: '+cn+'<br>ID: '+d.id).style('left',(e.pageX+10)+'px').style('top',(e.pageY-28)+'px')}).on('mouseout',function(){tooltip.style('display','none')}).on('click',function(e,d){highlightNode(d.id)});
  nodeEnter.filter(function(d){return d._isNew}).each(function(d){pulse(d3.select(this),glowColors[d.group]||'#fff')});
  var nodeAll = nodeEnter.merge(node);
  // labels
  var label = nodeGroup.selectAll('text').data(allNodes, function(d){return d.id});
  label.exit().transition().duration(200).attr('opacity',0).remove();
  var labelEnter = label.enter().append('text').text(function(d){return d.name&&d.name.length>10?d.name.substring(0,10)+'…':(d.name||d.id)}).attr('font-size',8).attr('dx',10).attr('dy',3).attr('fill','#334155').attr('pointer-events','none').attr('opacity',0);
  labelEnter.transition().duration(400).attr('opacity',1);
  var labelAll = labelEnter.merge(label);
  // simulation: 增量阶段轻推，全量刷新则重排
  simulation.nodes(allNodes); simulation.force('link').links(validLinks);
  if (phase === 'full') {
    simulation.alpha(0.5).restart();
  } else {
    simulation.alpha(Math.min(0.15, simulation.alpha() + 0.05)).restart();
  }
  simulation.on('tick',function(){linkAll.attr('x1',function(d){return d.source.x}).attr('y1',function(d){return d.source.y}).attr('x2',function(d){return d.target.x}).attr('y2',function(d){return d.target.y});nodeAll.attr('cx',function(d){return d.x}).attr('cy',function(d){return d.y});labelAll.attr('x',function(d){return d.x}).attr('y',function(d){return d.y})});
  // 等模拟冷却后再自动缩放
  simulation.on('end', function(){ zoomToFit(); });
}

function pulse(el,color){el.transition().duration(300).attr('r',15).attr('stroke',color).attr('stroke-width',3).attr('filter','url(#superglow)').transition().duration(300).attr('r',7).attr('stroke','rgba(255,255,255,.3)').attr('stroke-width',1.2).attr('filter',null).on('end',function(){pulse(d3.select(this),color)})}

function highlightNode(nodeId){
  var rs={};rs[nodeId]=true;allLinks.forEach(function(l){var s=typeof l.source==='object'?l.source.id:l.source;var t=typeof l.target==='object'?l.target.id:l.target;if(s===nodeId||t===nodeId){rs[s]=true;rs[t]=true}});
  linkGroup.selectAll('line').transition().duration(400).attr('stroke',function(l){var s=typeof l.source==='object'?l.source.id:l.source;var t=typeof l.target==='object'?l.target.id:l.target;if(s===nodeId||t===nodeId)return l.lineType==='dashed'?'#fbbf24':'#00d4ff';if(rs[s]&&rs[t])return'#2a5a6a';return'#1a2a3a'}).attr('stroke-width',function(l){var s=typeof l.source==='object'?l.source.id:l.source;var t=typeof l.target==='object'?l.target.id:l.target;if(s===nodeId||t===nodeId)return 4;if(rs[s]&&rs[t])return 2;return .5}).attr('opacity',function(l){var s=typeof l.source==='object'?l.source.id:l.source;var t=typeof l.target==='object'?l.target.id:l.target;if(s===nodeId||t===nodeId)return 1;if(rs[s]&&rs[t])return .7;return .08}).attr('marker-end',function(l){var s=typeof l.source==='object'?l.source.id:l.source;var t=typeof l.target==='object'?l.target.id:l.target;if(s===nodeId||t===nodeId)return'url(#arrow-active)';return l.lineType==='dashed'?'url(#arrow-dashed)':'url(#arrow)'});
  nodeGroup.selectAll('circle').transition().duration(400).attr('opacity',function(n){if(n.id===nodeId)return 1;if(rs[n.id])return .8;return .12}).attr('r',function(n){if(n.id===nodeId)return 15;if(rs[n.id])return 9;return 3}).attr('filter',function(n){return n.id===nodeId?'url(#superglow)':null});
  nodeGroup.selectAll('text').transition().duration(400).attr('opacity',function(n){if(n.id===nodeId)return 1;if(rs[n.id])return .8;return .08}).attr('font-size',function(n){return n.id===nodeId?12:8}).attr('fill',function(n){return n.id===nodeId?'#fff':'#334155'});
}
function typeNameCN(t){
  var m={supplier:'供应商',material_stock:'物料库存',purchase_contract:'采购合同',purchase_order:'采购订单',warehouse_receipt:'入库单',production_order:'生产工单',quality_inspection:'质检报告',equipment:'设备资产',logistics:'物流承运',finance_invoice:'应付发票',sales_order:'销售订单',accounts_receivable:'应收账单'};
  return m[t]||t;
}
function spotlightPrediction(){
  var predNodeIds = {}, predLinkKeys = {}, predCount = 0;
  allLinks.forEach(function(l){
    if(l.lineType==='dashed'){
      var s = typeof l.source==='object'?l.source.id:l.source;
      var t = typeof l.target==='object'?l.target.id:l.target;
      predNodeIds[s]=true; predNodeIds[t]=true;
      predLinkKeys[s+'-'+l.relation+'-'+t]=true;
      predCount++;
    }
  });
  if(predCount === 0) return;

  function linkKey(d){ return (typeof d.source==='object'?d.source.id:d.source)+'-'+d.relation+'-'+(typeof d.target==='object'?d.target.id:d.target); }

  // 第一步：所有元素瞬间置灰
  linkGroup.selectAll('line').transition().duration(300)
    .attr('stroke','#0f151e').attr('stroke-width',0.3).attr('opacity',0.05);
  nodeGroup.selectAll('circle').transition().duration(300)
    .attr('opacity',0.06).attr('r',3).attr('stroke','none');
  nodeGroup.selectAll('text').transition().duration(300).attr('opacity',0.03);

  // 第二步：300ms后预判链路炸亮
  setTimeout(function(){
    // 预判连线：超粗金黄+流动虚线
    linkGroup.selectAll('line').filter(function(d){return predLinkKeys[linkKey(d)];})
      .interrupt().transition().duration(200)
      .attr('stroke','#ffb800').attr('stroke-width',8).attr('stroke-dasharray','12,6').attr('opacity',1)
      .attr('marker-end','url(#arrow-dashed)');

    // 预判节点：放大+白边+superglow
    nodeGroup.selectAll('circle').filter(function(n){return predNodeIds[n.id];})
      .interrupt().transition().duration(200)
      .attr('opacity',1).attr('r',15).attr('stroke','#fff').attr('stroke-width',3)
      .attr('filter','url(#superglow)');

    // 预判标签
    nodeGroup.selectAll('text').filter(function(n){return predNodeIds[n.id];})
      .interrupt().transition().duration(200)
      .attr('opacity',1).attr('font-size',13).attr('fill','#fff');

    // 脉冲呼吸：白金色 ↔ 亮金色
    var pulseOn = true;
    var pulseTimer = setInterval(function(){
      var s = pulseOn ? '#fff3b0' : '#ff8c00';
      var w = pulseOn ? 10 : 7;
      linkGroup.selectAll('line').filter(function(d){return predLinkKeys[linkKey(d)];})
        .transition().duration(350).attr('stroke',s).attr('stroke-width',w);
      nodeGroup.selectAll('circle').filter(function(n){return predNodeIds[n.id];})
        .transition().duration(350).attr('r',pulseOn?17:14);
      pulseOn = !pulseOn;
    }, 350);

    // 仅双击恢复，不自动恢复
    var recovery = function(){
      clearInterval(pulseTimer);
      clearHighlight();
      svg.on('dblclick.recovery',null);
    };
    svg.on('dblclick.recovery', recovery);
    // 弹出预判链路详情窗
    // showPredictionPopup disabled for performance
  }, 300);
}

function showPredictionPopup(nodeIds, linkKeys){
  var steps = [];
  allLinks.forEach(function(l){
    var k = (typeof l.source==='object'?l.source.id:l.source)+'-'+l.relation+'-'+(typeof l.target==='object'?l.target.id:l.target);
    if(linkKeys[k]){
      var s = typeof l.source==='object'?l.source.id:l.source;
      var t = typeof l.target==='object'?l.target.id:l.target;
      var sn=s, tn=t;
      allNodes.forEach(function(n){if(n.id===s)sn=n.name; if(n.id===t)tn=n.name;});
      steps.push({source:s, sname:sn, target:t, tname:tn, relation:l.relation, meta:l._meta||''});
    }
  });
  if(steps.length===0) return;

  // 展示逐步核查弹窗
  showModal('AI自动核查中...', '<div id=invest-steps></div>');
  var container = document.getElementById('invest-steps');
  var summary = [];
  var currentIdx = 0;

  function runNext(){
    if(currentIdx >= steps.length){
      // 全部完成，追加总结到聊天
      var summaryText = '## 自动核查结果\n\n';
      summary.forEach(function(s,i){
        summaryText += (i+1)+'. **'+s.sname+'** --['+s.relation+']--> **'+s.tname+'**';
        if(s.finding) summaryText += '  \n   核查结论: '+s.finding;
        summaryText += '\n';
      });
      summaryText += '\n> 以上为AI自动核查完成，共'+steps.length+'条预判链路。双击图谱可恢复完整视图。';
      addChatMessage('report', summaryText);
      // 3秒后关闭弹窗
      setTimeout(function(){ closeModal(); }, 3000);
      return;
    }
    var step = steps[currentIdx];
    // 解析relation中的元数据（relation|key:val|key:val）
    var relParts = step.relation.split('|');
    var relName = relParts[0].replace('?','');
    var metaStr = relParts.slice(1).join(' | ');
    // 渲染当前步骤
    var html = '<div style="padding:12px;background:#0f1820;border-radius:6px;margin-bottom:8px">';
    html += '<div style="color:#f59e0b;font-size:13px;margin-bottom:6px">正在核查第'+(currentIdx+1)+'/'+steps.length+'条</div>';
    html += '<div style="display:flex;align-items:center;gap:8px;margin-bottom:4px">';
    html += '<span style="color:#f59e0b;font-weight:600;font-size:14px">'+step.sname+'</span>';
    html += '<span style="color:#00d4ff;font-size:12px">--['+relName+']--></span>';
    html += '<span style="color:#f59e0b;font-weight:600;font-size:14px">'+step.tname+'</span>';
    html += '</div>';
    if(metaStr) html += '<div style="color:#8899aa;font-size:11px;margin-bottom:6px">数据: '+metaStr+'</div>';
    html += '<div id=invest-step-'+currentIdx+'><span class=loading></span> 正在自动核查目标实体...</div>';
    html += '</div>';
    // 已完成的历史步骤
    for(var j=0; j<currentIdx; j++){
      var prev = steps[j];
      var prevHtml = '<div style="padding:8px 12px;margin-bottom:4px;background:#0f1a14;border-left:3px solid #10b981;border-radius:4px;font-size:12px;color:#6b9e7a;opacity:.7">';
      prevHtml += '第'+(j+1)+'条: <strong>'+prev.sname+'</strong> --['+prev.relation+']--> <strong>'+prev.tname+'</strong>';
      if(prev.finding) prevHtml += ' → '+prev.finding;
      prevHtml += '</div>';
      html += prevHtml;
    }
    container.innerHTML = html;

    // 模拟自动核查（查图谱数据）
    setTimeout(function(){
      var finding = '';
      var targetNode = allNodes.find(function(n){return n.id===step.target;});
      if(targetNode){
        finding = '已定位目标实体「'+step.tname+'」(类型:'+typeNameCN(targetNode.type||'')+')，请优先核查此节点。';
        // 高亮该目标节点
        highlightNode(step.target);
      } else {
        finding = '目标实体未在当前图谱中，建议扩大检索范围或手动查询。';
      }
      step.finding = finding;
      summary.push(step);
      // 更新当前步骤状态
      var el = document.getElementById('invest-step-'+currentIdx);
      if(el) el.innerHTML = '<span style=color:#10b981>&#x2713;</span> '+finding;
      currentIdx++;
      // 1.2秒后进入下一步
      setTimeout(function(){ runNext(); }, 1200);
    }, 800);
  }

  runNext();
}

function showModal(title, bodyHtml){
  document.getElementById('modal-title').textContent = title;
  document.getElementById('modal-body').innerHTML = bodyHtml;
  document.getElementById('modal-overlay').classList.add('show');
}
function closeModal(){
  document.getElementById('modal-overlay').classList.remove('show');
}
// ESC关闭弹窗
document.addEventListener('keydown',function(e){if(e.key==='Escape')closeModal();});
function clearHighlight(){
  linkGroup.selectAll('line').interrupt();
  nodeGroup.selectAll('circle').interrupt();
  linkGroup.selectAll('line').transition().duration(600).attr('stroke',function(d){return d.lineType==='dashed'?'#f59e0b':'#3a4a5a'}).attr('stroke-width',function(d){return d.lineType==='dashed'?2:1.2}).attr('opacity',1).attr('marker-end',function(d){return d.lineType==='dashed'?'url(#arrow-dashed)':'url(#arrow)'});
  nodeGroup.selectAll('circle').transition().duration(600).attr('opacity',1).attr('r',7).attr('stroke','rgba(255,255,255,.3)').attr('stroke-width',1.2).attr('filter',null);
  nodeGroup.selectAll('text').transition().duration(600).attr('opacity',1).attr('font-size',8).attr('fill','#334155');
}
function zoomToFit(){
  var container=document.getElementById('graph-card');if(!container)return;
  var W=container.clientWidth,H=container.clientHeight;
  if(W<=0||H<=0)return;
  var minX=Infinity,minY=Infinity,maxX=-Infinity,maxY=-Infinity;
  allNodes.forEach(function(n){if(n.x!==undefined&&!isNaN(n.x)){if(n.x<minX)minX=n.x;if(n.x>maxX)maxX=n.x;if(n.y<minY)minY=n.y;if(n.y>maxY)maxY=n.y}});
  if(!isFinite(minX))return;
  var pad=80, gw=maxX-minX+pad*2, gh=maxY-minY+pad*2;
  if(gw<200)gw=200;if(gh<200)gh=200;
  var scale=Math.min(W/gw,H/gh,2.0);
  var cx=(minX+maxX)/2,cy=(minY+maxY)/2;
  svg.transition().duration(800).ease(d3.easeCubicOut).call(
    d3.zoom().transform,
    d3.zoomIdentity.translate(W/2-cx*scale,H/2-cy*scale).scale(scale)
  );
}
function refreshGraph(){
  fetch('/api/graph').then(function(r){return r.json()}).then(function(data){
    allNodes=(data.nodes||[]).map(function(n){return Object.assign({},n,{group:n.group||(colorMap[n.type]||0)})});
    allLinks=(data.links||[]).map(function(l){return Object.assign({},l,{lineType:l.type||'solid'})});
    renderGraph('full');
    var loading=document.getElementById('graph-loading');if(loading)loading.style.display='none';
  }).catch(function(err){console.error('获取图谱失败:',err)});
}
function updateEntityCount(){
  fetch('/api/entities').then(function(r){return r.json()}).then(function(data){document.getElementById('stat-entities').textContent='实体: '+(data.count||0)}).catch(function(){});
  updateStats();
}
function updateStats(){
  fetch('/api/stats').then(function(r){return r.json()}).then(function(d){
    document.getElementById('stat-entities').textContent = d.totalEntities||'--';
    document.getElementById('stat-triples').textContent = d.totalTriples||'--';
    document.getElementById('stat-docs').textContent = d.totalDocs||'--';
    document.getElementById('stat-chats').textContent = d.chatSessions||'0';
  }).catch(function(){});
}

// ============ Trace ============
function updateTrace(step){
  var content=document.getElementById('trace-content');
  var el=document.getElementById('step-'+step.stepId);
  var isNew = !el;
  if(!el){el=document.createElement('div');el.id='step-'+step.stepId;content.appendChild(el)}
  el.className='trace-step '+(step.status||'running');
  var html = '<div class=trace-row><span class=sn>'+step.stepId+'</span>';
  html += '<div class=trace-body><div class=trace-name>'+(step.status==='running'?'<span class=active-dot></span> ':'')+'<strong>'+step.name+'</strong></div>';
  html += '<div class=trace-desc>'+step.description+'</div>';
  if(step.result) html += '<div class=trace-result>→ '+step.result+'</div>';
  // 下一步计划
  if(step.nextPlan && step.nextPlan.length>0){
    html += '<div class=trace-plan><span class=plan-label>AI计划下一步:</span>';
    step.nextPlan.forEach(function(p){html += '<span class=plan-item>'+p+'</span>';});
    html += '</div>';
  }
  // 多分支选择
  if(step.branchOpts && step.branchOpts.length>0){
    html += '<div class=trace-branch><span class=branch-label>多分支研判路径:</span>';
    step.branchOpts.forEach(function(b,i){html += '<span class=branch-item>'+(i+1)+'. '+b+'</span>';});
    html += '</div>';
  }
  html += '</div></div>';
  el.innerHTML = html;
  if(isNew && step.status==='running'){
	  if(step.name.indexOf("生成研判报告")!==-1) showReportModal();

    el.style.animation = 'slideIn .3s ease';
  }
  content.scrollTop=content.scrollHeight;
  // 同步更新阶段进度条
  if(step.status==='running'){var phaseMap={1:'retrieval',2:'retrieval',3:'traversal',4:'scoring',5:'prediction',6:'prediction'};showPhaseBar(phaseMap[step.stepId]||'',step.name+': '+step.description)}
}

// ============ 可拖拽面板分隔条 ============
(function(){
  var leftPanel=document.getElementById('left-panel');
  var rightPanel=document.getElementById('right-panel');
  var dl=document.getElementById('divider-left');
  var dr=document.getElementById('divider-right');
  var dragging=null, startX=0, startW=0;

  function onDown(e, panel, divider){
    dragging=panel; divider.classList.add('active');
    startX=e.clientX; startW=panel.offsetWidth;
    document.body.style.cursor='col-resize'; document.body.style.userSelect='none';
    e.preventDefault();
  }
  function onMove(e){
    if(!dragging)return;
    var diff=e.clientX-startX, newW=startW+diff;
    var mn=parseInt(dragging.style.minWidth)||260, mx=parseInt(dragging.style.maxWidth)||500;
    if(newW<mn)newW=mn; if(newW>mx)newW=mx;
    dragging.style.width=newW+'px';
    // 重绘图谱
    var c=document.getElementById('graph-card'); if(c){
      simulation.force('center',d3.forceCenter(c.clientWidth/2,c.clientHeight/2)); simulation.alpha(0.1).restart();
    }
  }
  function onUp(){
    if(dragging){ dl.classList.remove('active'); dr.classList.remove('active'); }
    dragging=null; document.body.style.cursor=''; document.body.style.userSelect='';
  }
  if(dl) dl.addEventListener('mousedown',function(e){onDown(e,leftPanel,dl)});
  if(dr) dr.addEventListener('mousedown',function(e){onDown(e,rightPanel,dr)});
  document.addEventListener('mousemove',onMove);
  document.addEventListener('mouseup',onUp);
})();

// ============ 低代码平台 ============
function loadEntityAttrs(){
  var sel=document.getElementById('lc-etype');
  if(!sel.value)return;
  fetch('/api/lowcode/entities').then(function(r){return r.json()}).then(function(d){
    var types=d.entityTypes||{};
    var info=types[sel.value];
    if(!info)return;
    var attrs=info.attributes||[];
    var html='<div style="font-size:11px;color:#5a6a7a;margin-bottom:4px">属性字段(勾选需要的):</div>';
    attrs.forEach(function(a){
      html+='<label style="display:flex;align-items:center;gap:6px;padding:3px 0;font-size:11px;cursor:pointer">';
      html+='<input type=checkbox class=lc-attr-check value='+a.name+' checked> <span>'+a.name+'</span>';
      html+='<span style="color:#3a4a5a;margin-left:auto">'+ (a.sampleValue||'').substring(0,20)+'</span></label>';
    });
    document.getElementById('lc-attrs').innerHTML=html;
  });
}
// 初始加载实体类型列表
(function initLCTypes(){
  fetch('/api/lowcode/entities').then(function(r){return r.json()}).then(function(d){
    var types=d.entityTypes||{};
    var sel=document.getElementById('lc-etype');if(!sel)return;
    Object.keys(types).forEach(function(t){
      sel.innerHTML+='<option value='+t+'>'+t+'</option>';
    });
  });
})();

function generateForm(){
  var lcSel=document.getElementById('lc-etype');if(!lcSel)return;var entityType=lcSel.value;
  var formName=document.getElementById('lc-fname').value||('表单_'+entityType);
  var approvers=document.getElementById('lc-approvers').value.split(',').filter(Boolean);
  var attrs=[]; document.querySelectorAll('.lc-attr-check:checked').forEach(function(c){attrs.push(c.value)});
  if(!entityType||attrs.length===0){alert('请选择实体类型和至少一个属性');return}
  var div=document.getElementById('lc-result');
  div.innerHTML='<span class=loading></span> 正在生成...';
  fetch('/api/lowcode/generate-form',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({entityType:entityType,attributes:attrs,formName:formName,approvers:approvers})})
    .then(function(r){return r.json()}).then(function(d){
      div.innerHTML='<div class=ok>[OK] 表单已保存到BoltDB (ID:'+(d.formId||'')+')</div>';
      showModal('低代码表单: '+d.formName,
        '<div style="font-size:12px"><strong>表单ID:</strong> '+d.formId+'</div>'+
        '<div style="font-size:12px"><strong>实体类型:</strong> '+d.entityType+'</div>'+
        '<div style="font-size:12px;margin:8px 0"><strong>表单字段:</strong></div>'+
        (d.fields||[]).map(function(f,i){return '<div style="padding:4px 8px;margin:2px 0;background:#0a1218;border-radius:4px;font-size:11px">'+(i+1)+'. <strong>'+f.label+'</strong> ('+f.type+')</div>'}).join('')+
        '<div style="font-size:12px;margin:12px 0 4px"><strong>审批流程:</strong></div>'+
        ((d.workflow||{}).nodes||[]).map(function(n){return '<div style="padding:4px 8px;margin:2px 0;background:#0a1218;border-radius:4px;font-size:11px">步骤'+n.step+': '+n.role+' - '+n.approver+'</div>'}).join('')+
        '<div style="margin-top:8px;font-size:10px;color:#94a3b8">生成时间: '+d.generatedAt+'</div>'
      );
      loadFormsList();
    }).catch(function(err){div.innerHTML='<span class=err>[ERR] '+err.message+'</span>'});
}
function loadFormsList(){
  fetch('/api/lowcode/forms').then(function(r){return r.json()}).then(function(d){
    var forms=(d.forms||[]).slice(-5);
    var html='';
    forms.forEach(function(f){
      var data=f.data||{};
      html+='<div style="padding:2px 0;font-size:10px;color:#94a3b8;cursor:pointer"> '+ (data.formName||f.message) +' ('+(data.entityType||'')+')</div>';
    });
    var el=document.getElementById('lc-forms-list');if(el)el.innerHTML='<div style="font-size:10px;color:#94a3b8;margin-top:6px">最近表单('+forms.length+'):</div>'+html;
  });
}
loadFormsList();


// ============ 知识库 ============
function uploadKB(){
  var file=document.getElementById('kb-file').files[0];
  if(!file){alert('请选择文件');return}
  var div=document.getElementById('kb-result');
  div.innerHTML='<span class=loading></span> 正在上传并向量化...';
  var fd=new FormData(); fd.append('file',file);
  fetch('/api/knowledge/upload',{method:'POST',body:fd})
    .then(function(r){return r.json()}).then(function(d){
      if(d.success){div.innerHTML='<span class=ok>[OK] 已上传: '+d.filename+' ('+d.size+'字节)</span>'; document.getElementById('kb-file').value=''; loadKBList();}
      else{div.innerHTML='<span class=err>[ERR] '+(d.error||'failed')+'</span>';}
    }).catch(function(err){div.innerHTML='<span class=err>[ERR] '+err.message+'</span>'});
}
function clearKB(){
  if(!confirm('确定清空所有知识库文档？此操作不可恢复。')) return;
  var div = document.getElementById('kb-result');
  div.innerHTML = '<span class=loading></span> 正在清空...';
  fetch('/api/knowledge/clear', {method:'POST'})
    .then(function(r){return r.json()}).then(function(d){
      if(d.success){div.innerHTML='<span class=ok>[OK] '+d.message+'</span>'; loadKBList();}
      else{div.innerHTML='<span class=err>[ERR] '+(d.error||'')+'</span>';}
    }).catch(function(err){div.innerHTML='<span class=err>[ERR] '+err.message+'</span>'});
}
function loadKBList(){
  fetch('/api/knowledge/list').then(function(r){return r.json()}).then(function(d){
    var docs=d.documents||[];
    var html='<div style="font-size:10px;color:#94a3b8;margin-bottom:4px">知识库文档('+docs.length+'篇):</div>';
    docs.forEach(function(doc){html+='<div style="padding:2px 0;font-size:10px">'+ (doc.data?doc.data.filename||doc.message:'')+'</div>'});
    document.getElementById('kb-list').innerHTML=html;
  });
}
loadKBList();


function renderChartIfNeeded(query, reportEl) {
  if (!reportEl) return;
  var q = (query||'').toLowerCase();
  var hasChart = /(图表|柱状图|饼图|折线图|趋势图|占比|分布|排名|对比|统计图|可视化)/.test(q);
  if (!hasChart) return;
  
  var tables = reportEl.querySelectorAll('table');
  if (tables.length === 0) return;
  
  var table = tables[0];
  var headers = [], rows = [];
  table.querySelectorAll('thead th').forEach(function(th){ headers.push(th.textContent.trim()) });
  table.querySelectorAll('tbody tr, tr').forEach(function(tr){
    var row = [];
    tr.querySelectorAll('td, th').forEach(function(td){ row.push(td.textContent.trim()) });
    if(row.length >= 2) rows.push(row);
  });
  if(rows.length < 2) return;
  
  var chartType = 'bar';
  if(/(饼图|占比|分布|比例)/.test(q)) chartType = 'pie';
  else if(/(折线|趋势|走势)/.test(q)) chartType = 'line';
  else if(/(数量|多少)/.test(q) && rows.length <= 8) chartType = 'pie';
  
  // 智能选择标签列和数值列（跳过序号列）
  var labelIdx = 0, valueIdx = 1;
  if(headers.length > 1 && /序号|编号|#/.test(headers[0])){ labelIdx = 1; valueIdx = 2; }
  var labels = rows.map(function(r){ return (r[labelIdx]||'').substring(0,15) });
  var values = rows.map(function(r){
    var v = parseFloat((r[valueIdx]||'').replace(/[^0-9.-]/g,''));
    return isNaN(v) ? 0 : v;
  });
  
  var canvasId = 'chart-' + Date.now();
  var wrap = document.createElement('div');
  wrap.className = 'chart-wrap';
  wrap.innerHTML = '<canvas id="'+canvasId+'"></canvas>';
  reportEl.appendChild(wrap);
  
  var ctx = document.getElementById(canvasId);
  if(!ctx) return;
  ctx = ctx.getContext('2d');
  var colors = ['#00d4ff','#10b981','#f59e0b','#ef4444','#8b5cf6','#ec4899','#3b82f6','#f97316','#06b6d4','#84cc16','#eab308','#14b8a6'];
  
  new Chart(ctx, {
    type: chartType,
    data: {
      labels: labels,
      datasets: [{
        label: headers[1] || '数值',
        data: values,
        backgroundColor: chartType==='pie' ? colors.slice(0, rows.length) : colors[0]+'88',
        borderColor: chartType==='pie' ? '#111820' : colors[0],
        borderWidth: 1
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: true,
      plugins: { legend: { labels: { color: '#8899aa', font: { size: 10 } } } },
      scales: chartType==='pie' ? {} : {
        x: { ticks: { color: '#6a7a8a', font: { size: 9 }, maxRotation: 45 }, grid: { color: '#1a2836' } },
        y: { ticks: { color: '#6a7a8a', font: { size: 9 } }, grid: { color: '#1a2836' } }
      }
    }
  });
}

// ============ Init ============
var _initRetries=0;
(function initApp(){
  if(typeof d3==='undefined'){if(_initRetries<15){_initRetries++;setTimeout(initApp,300)}else{console.error('D3.js加载失败')}return}
  var container=document.getElementById('graph-card');
  if(!container||container.clientWidth<=0){if(_initRetries<20){_initRetries++;setTimeout(initApp,250)}return}
  try{initGraph();refreshGraph();updateEntityCount();updateStats();console.log('AI智能体ERP场景案例-数据ETL/模型推理/RAG知识库/知识图谱/自主研判一体化案例已就绪')}catch(e){console.error('图谱初始化失败:',e)}
})();
setInterval(function(){
  var svgEl=document.querySelector('#graph-card svg');
  var container=document.getElementById('graph-card');
  if(container&&container.clientWidth>0&&typeof d3!=='undefined'){if(!svgEl||svgEl.clientWidth<=0||svgEl.childElementCount<2){console.log('图谱修复');var old=container.querySelector('svg');if(old)old.remove();initGraph();refreshGraph()}}
},8000);

document.addEventListener('keydown',function(e){if(e.ctrlKey&&e.key==='Enter')sendChat()});
