# -*- coding: utf-8 -*-
# race_studio.py — ЛОКАЛЬНАЯ студия отбора АВАТАРОВ РАС (по семействам).
# Показывает пачку из races_pool/, кнопки ПРИНЯТЬ / УДАЛИТЬ + обрезка + генерация.
# Запуск: python race_studio.py  (http://127.0.0.1:8798)
import os, sys, json, shutil, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse, parse_qs

POOL = r"C:\Zorion2\ai_drafts\races_pool"
ACCEPT = r"C:\Zorion2\ai_drafts\final_accepted"
REJECT = r"C:\Zorion2\ai_drafts\races_rejected"
COCKPIT = os.path.join(POOL, "cockpit_1024.png")
PORT = 8798
os.makedirs(REJECT, exist_ok=True)


def composite_on_cockpit(fp, out_size=1024):
    """Наложить прозрачный PNG на кабину того же размера."""
    from PIL import Image
    person = Image.open(fp).convert("RGBA")
    if os.path.exists(COCKPIT):
        bg = Image.open(COCKPIT).convert("RGBA")
        if bg.size != person.size:
            bg = bg.resize(person.size, Image.LANCZOS)
        bg.paste(person, (0, 0), person)
        return bg
    return person

HTML = """<!DOCTYPE html>
<html lang="ru"><head><meta charset="utf-8">
<title>Студия РАС</title>
<style>
 body{background:#111;color:#ddd;font-family:Arial;margin:0;padding:20px}
 h1{font-size:18px}
 .note{color:#999;font-size:13px;margin-bottom:16px}
 .tabs{display:flex;gap:8px;margin-bottom:12px}
 .tab{background:#222;border:1px solid #444;color:#ddd;border-radius:6px;padding:8px 16px;cursor:pointer;font-size:14px}
 .tab.active{background:#26a;border-color:#26a}
 .toolbar{display:flex;gap:10px;align-items:center;margin-bottom:16px;flex-wrap:wrap}
 .toolbar select,.toolbar input{background:#222;border:1px solid #444;color:#ddd;padding:6px;border-radius:6px}
 .toolbar input{width:70px}
 .toolbar button{border:none;border-radius:6px;padding:8px 14px;font-size:14px;cursor:pointer}
 .gen{background:#26a;color:#fff}
 .grid{display:flex;flex-wrap:wrap;gap:16px}
 .cell{background:#1c1c1c;border:1px solid #333;border-radius:8px;padding:10px;width:300px;text-align:center}
 .cell img{width:300px;height:300px;object-fit:cover;background:#222;border-radius:6px;cursor:pointer}
 .cell .name{font-size:12px;color:#8cf;margin:4px 0}
 .btns{display:flex;gap:6px;justify-content:center;flex-wrap:wrap}
 .btns button{border:none;border-radius:6px;padding:6px 10px;font-size:13px;cursor:pointer}
 .ok{background:#2a7;color:#fff}.del{background:#a33;color:#fff}
 .done{color:#2a7;font-size:14px;margin-top:12px}
 .modal{position:fixed;inset:0;background:#000c;display:none;align-items:center;justify-content:center;z-index:9}
 .modal.show{display:flex}
 .modal img{max-width:85vw;max-height:75vh;background:#222}
 .cropbar{margin-top:8px;display:flex;gap:8px;align-items:center}
 .cropbar input{width:300px}
 .cropbar button{background:#2a7;border:none;border-radius:6px;padding:6px 12px;color:#fff;cursor:pointer}
</style></head><body>
<h1>Студия генерации РАС (неантропоморфные)</h1>
<div class="note">ОТБОР: генерация + принять/удалить. ЭТАЛОН: кандидаты (широкий поиск формы) -> выбрать эталон -> вариации (узко вокруг эталона).</div>
<div class="tabs">
  <button class="tab active" id="tabbtn-pool" onclick="showTab('pool')">Отбор</button>
  <button class="tab" id="tabbtn-ref" onclick="showTab('ref')">Эталон</button>
</div>
<div id="tab-pool">
<div class="toolbar">
  <span>Семейство:</span>
  <select id="fam">
    <option value="F2">F2 Крио-аммиачные</option>
    <option value="F3">F3 Метановые</option>
    <option value="F4">F4 Серные</option>
    <option value="F5">F5 Терморедокс</option>
    <option value="F6">F6 Кремниевые</option>
    <option value="F7">F7 Небесные</option>
    <option value="F8">F8 Углекислые</option>
    <option value="F9">F9 Экзотика</option>
  </select>
  <span>Раса:</span>
  <select id="racePool">
    <option value="">все</option>
    <option value="5">5 Аммиачники</option>
    <option value="6">6 Крио-лесные</option>
    <option value="7">7 Ледяные пастухи</option>
    <option value="8">8 Туманники</option>
    <option value="9">9 Крио-рои</option>
    <option value="48">48 Крио-небесные</option>
  </select>
  <span>Штук:</span>
  <input id="cnt" type="number" value="8" min="1" max="24">
  <button class="gen" id="btnCands" onclick="genCandsPool()">СГЕНЕРИРОВАТЬ ЭТАЛОНЫ (8)</button>
  <button class="gen" style="background:#a62" id="btnVar" onclick="genVarPool()" disabled>СГЕНЕРИРОВАТЬ РАСУ (8)</button>
  <span>denoise:</span>
  <input id="denoise" type="range" min="0.25" max="0.6" step="0.05" value="0.35" style="width:120px">
  <span id="denoiseLbl" style="font-size:13px;color:#8cf">0.35</span>
</div>
<div id="done" class="done"></div>
<div id="genStatePool" style="margin-bottom:8px;color:#8cf;font-size:14px">Статус генератора: ...</div>
<div id="prog" class="prog" style="display:none;margin-bottom:12px;color:#8cf;font-size:14px"></div>
<div id="refsRow" style="margin-bottom:12px"></div>
<div class="grid" id="grid"></div>
</div>

<div id="tab-ref" style="display:none">
  <div class="toolbar">
    <span>Семейство:</span>
    <select id="famRef">
      <option value="F2">F2 Крио-аммиачные</option>
      <option value="F3">F3 Метановые</option>
      <option value="F4">F4 Серные</option>
      <option value="F5">F5 Терморедокс</option>
      <option value="F6">F6 Кремниевые</option>
      <option value="F7">F7 Небесные</option>
      <option value="F8">F8 Углекислые</option>
      <option value="F9">F9 Экзотика</option>
    </select>
    <label style="color:#ddd;font-size:13px"><input id="anthroRef" type="checkbox"> Антропоморфный</label>
    <span style="margin-left:12px">Сброс эталона расы:</span>
    <select id="raceRefClear" style="background:#222;border:1px solid #444;color:#ddd;padding:4px;border-radius:4px"></select>
    <button class="gen" style="background:#a62" onclick="clearRef()">СБРОС ЭТАЛОНА</button>
    <button class="gen" style="background:#a33;display:none" onclick="stopGen()" id="btnStop">СТОП</button>
  </div>
  <div id="doneRef" class="done"></div>
  <div id="genState" style="margin-bottom:8px;color:#8cf;font-size:14px">Статус генератора: ...</div>
  <div id="progRef" class="prog" style="display:none;margin-bottom:12px;color:#fa0;font-size:14px"></div>
  <div id="refStatus" style="margin-bottom:12px;color:#8cf;font-size:13px"></div>
  <div class="grid" id="gridRef"></div>
</div>

<div class="modal" id="modal">
  <div>
    <img id="mimg">
    <div class="cropbar">
      <input id="cropVal" type="range" min="0" max="100" value="100">
      <span id="cropLbl">100%</span>
      <button onclick="doCrop()">Обрезать снизу</button>
      <button style="background:#555" onclick="closeModal()">Закрыть</button>
    </div>
  </div>
</div>

<script>
// показ JS-ошибок прямо на странице (для диагностики)
window.onerror = function(msg, src, line){
  const el=document.getElementById('doneRef')||document.body;
  el.textContent='JS-ошибка: '+msg+' (строка '+line+')';
};
window.addEventListener('unhandledrejection', function(e){
  const el=document.getElementById('doneRef')||document.body;
  el.textContent='Promise-ошибка: '+e.reason;
});
let curFile=null;
async function load(){
  const raceSel=document.getElementById('racePool');
  const raceFilter=raceSel? raceSel.value : '';
  const fam=document.getElementById('fam').value;
  const r=await fetch('/list?fam='+fam+'&race='+raceFilter); const items=await r.json();
  const g=document.getElementById('grid');
  const existing={};
  g.querySelectorAll('.cell[data-file]').forEach(c=>{ existing[c.dataset.file]=c; });
  const seen={};
  items.forEach(it=>{
    seen[it.file]=true;
    let d=existing[it.file];
    if(!d){
      d=document.createElement('div'); d.className='cell'; d.dataset.file=it.file;
      d.innerHTML=`<img src="/img/${it.file}" onclick="openCrop('${it.file}')">
      <div class="name">${it.file}${it.fam? ' ['+it.fam+']':''}${it.race? ' — '+it.race:''}</div>
      <div class="btns">
        <button class="ok" onclick="act('${it.file}','accept')">Принять</button>
        <button class="del" onclick="act('${it.file}','reject')">Удалить</button>
      </div>`;
      g.appendChild(d);
    }
  });
  Object.keys(existing).forEach(f=>{ if(!seen[f]) existing[f].remove(); });
  const oldHint=document.getElementById('poolHint');
  if(oldHint) oldHint.remove();
  if(!items.length && raceFilter){
    const hint=document.createElement('div');
    hint.id='poolHint';
    hint.style.cssText='color:#fa0;font-size:14px;padding:8px 0';
    hint.textContent='Нет файлов этой расы в пуле — нажми «СГЕНЕРИРОВАТЬ РАСУ» (от эталона) или выбери другую расу.';
    g.appendChild(hint);
  }
  loadRefs();
}
async function loadRefs(){
  const fam=document.getElementById('fam').value;
  const r=await fetch('/refs?fam='+fam); const j=await r.json();
  const el=document.getElementById('refsRow');
  if(!j.refs || !j.refs.length){ el.innerHTML=''; return; }
  let h='<div style="color:#8cf;font-size:13px;margin-bottom:6px">Эталоны семейства '+fam+' (для сравнения):</div><div style="display:flex;gap:8px;flex-wrap:wrap">';
  j.refs.forEach(x=>{
    h+=`<div style="text-align:center"><img src="/refimg?fam=${fam}&race=${x.race}" style="width:70px;height:70px;object-fit:cover;background:#222;border-radius:6px"><div style="font-size:10px;color:#999">${x.race_name}</div></div>`;
  });
  h+='</div>';
  el.innerHTML=h;
}
async function loadRaces(){
  const fam=document.getElementById('fam').value;
  const sel=document.getElementById('racePool');
  const r=await fetch('/races?fam='+fam); const j=await r.json();
  let h='<option value="">все</option>';
  j.races.forEach(x=>{ h+=`<option value="${x.id}">${x.name}</option>`; });
  sel.innerHTML=h;
  const saved=localStorage.getItem('racePool');
  if(saved && [...sel.options].some(o=>o.value===saved)) sel.value=saved;
  updateVarBtn();
}
async function updateVarBtn(){
  const btn=document.getElementById('btnVar');
  const race=document.getElementById('racePool').value;
  const fam=document.getElementById('fam').value;
  if(!race){ btn.disabled=true; btn.title='выбери расу'; return; }
  const r=await fetch('/refs?fam='+fam); const j=await r.json();
  const has=(j.refs||[]).some(x=>x.race===race);
  btn.disabled=!has;
  btn.title=has? '' : 'сначала сгенерируй эталон';
}
async function genVarPool(){
  const fam=document.getElementById('fam').value;
  const race=document.getElementById('racePool').value;
  const n=document.getElementById('cnt').value;
  const denoise=document.getElementById('denoise').value;
  document.getElementById('done').textContent='Генерация вариаций...';
  const r=await fetch('/genvar?fam='+fam+'&race='+race+'&n='+n+'&denoise='+denoise); const j=await r.json();
  document.getElementById('done').textContent=j.msg;
  load();
}
async function genCandsPool(){
  const fam=document.getElementById('fam').value;
  const n=document.getElementById('cnt').value;
  document.getElementById('done').textContent='Генерация кандидатов эталона...';
  const r=await fetch('/genref?fam='+fam+'&n='+n); const j=await r.json();
  document.getElementById('done').textContent=j.msg+' Кандидаты — на вкладке Эталон.';
  load();
}
function showTab(name){
  document.getElementById('tab-pool').style.display = name==='pool' ? '' : 'none';
  document.getElementById('tab-ref').style.display = name==='ref' ? '' : 'none';
  document.getElementById('tabbtn-pool').classList.toggle('active', name==='pool');
  document.getElementById('tabbtn-ref').classList.toggle('active', name==='ref');
  localStorage.setItem('raceTab', name);
  if(name==='ref') loadRef();
  if(name==='pool') updateVarBtn();
}
// восстановить вкладку/выбор после Ctrl+F5
function restoreState(){
  const tab = localStorage.getItem('raceTab') || 'pool';
  if(tab==='ref'){
    const fam = localStorage.getItem('raceFamRef');
    if(fam) document.getElementById('famRef').value = fam;
  } else {
    const fam = localStorage.getItem('raceFam');
    if(fam) document.getElementById('fam').value = fam;
  }
  showTab(tab);
  loadRaces();
}
document.getElementById('famRef').onchange = e=>localStorage.setItem('raceFamRef', e.target.value);
document.getElementById('fam').onchange = e=>{ localStorage.setItem('raceFam', e.target.value); loadRaces(); loadRefs(); };
document.getElementById('racePool').onchange = e=>{ localStorage.setItem('racePool', e.target.value); updateVarBtn(); load(); };
document.getElementById('cnt').oninput = e=>{
  document.getElementById('btnVar').textContent='СГЕНЕРИРОВАТЬ РАСУ ('+e.target.value+')';
  document.getElementById('btnCands').textContent='СГЕНЕРИРОВАТЬ ЭТАЛОНЫ ('+e.target.value+')';
};
document.getElementById('denoise').oninput = e=>{ document.getElementById('denoiseLbl').textContent=e.target.value; };
async function setRef(file){
  const fam=document.getElementById('famRef').value;
  const sel=document.getElementById('raceSel-'+file);
  const race=sel? sel.value : '5';
  const r=await fetch('/refset?file='+file+'&fam='+fam+'&race='+race); const j=await r.json();
  document.getElementById('doneRef').textContent=j.msg;
  loadRef();
}
async function clearRef(){
  const fam=document.getElementById('famRef').value;
  const sel=document.getElementById('raceRefClear');
  const race=sel? sel.value : '';
  if(!race){ document.getElementById('doneRef').textContent='выбери расу'; return; }
  const r=await fetch('/refclear?fam='+fam+'&race='+race); const j=await r.json();
  document.getElementById('doneRef').textContent=j.msg;
  loadRef();
}
async function loadRef(){
  const fam=document.getElementById('famRef').value;
  const r=await fetch('/ref?fam='+fam); const j=await r.json();
  // статус эталонов по расам семейства
  const st=document.getElementById('refStatus');
  let s='';
  if(j.raceStatus && j.raceStatus.length){
    s='Эталоны: ';
    s+=j.raceStatus.map(x=>x.has? `<b style="color:#2a7">✓ ${x.name}</b>` : `<span style="color:#666">· ${x.name}</span>`).join(' ');
  }
  st.innerHTML=s;
  // селект «сброс эталона расы» — опции рас семейства
  const rc=document.getElementById('raceRefClear');
  if(rc){
    const prev=rc.value;
    rc.innerHTML=(j.raceStatus||[]).map(x=>`<option value="${x.id}">${x.name}</option>`).join('');
    if(prev) rc.value=prev;
  }
  // ряд миниатюр принятых эталонов семейства
  const hasRefs = (j.raceStatus||[]).filter(x=>x.has);
  const rw=document.createElement('div');
  rw.style.cssText='display:flex;gap:10px;flex-wrap:wrap;margin-bottom:14px';
  hasRefs.forEach(x=>{
    const box=document.createElement('div');
    box.style.cssText='text-align:center';
    box.innerHTML=`<img src="/refimg?fam=${fam}&race=${x.id}" style="width:80px;height:80px;object-fit:cover;background:#222;border-radius:6px;border:2px solid #2a7;cursor:pointer" onclick="window.open('/refimg?fam=${fam}&race=${x.id}')">
      <div style="font-size:10px;color:#8cf;margin-top:2px">${x.name.replace(/^\d+\s*/,'')}</div>`;
    rw.appendChild(box);
  });
  if(hasRefs.length){
    const prev=document.getElementById('refRow');
    if(prev) prev.remove();
    rw.id='refRow';
    st.after(rw);
  } else {
    const prev=document.getElementById('refRow');
    if(prev) prev.remove();
  }
  const g=document.getElementById('gridRef'); 
  // дифф-рендер: не перерисовывать загруженные карточки (иначе таймер 3с сбрасывает картинки)
  const prevSel={};
  g.querySelectorAll('select[id^="raceSel-"]').forEach(s=>{ prevSel[s.id]=s.value; });
  const existing={};
  g.querySelectorAll('.cell[data-file]').forEach(c=>{ existing[c.dataset.file]=c; });
  const seen={};
  if(j.cands) j.cands.forEach(it=>{
    seen[it.file]=true;
    let d=existing[it.file];
    if(!d){
      d=document.createElement('div'); d.className='cell'; d.dataset.file=it.file;
      d.innerHTML=`<img src="/refcand/${it.file}">
      <div class="name" style="font-size:11px;line-height:1.3">${it.info || it.file}</div>
      <div class="btns">
        <select id="raceSel-${it.file}" style="background:#222;border:1px solid #444;color:#ddd;padding:4px;border-radius:4px">${j.raceOptions}</select>
        <button class="ok" onclick="setRef('${it.file}')">Эталон</button>
      </div>`;
      g.appendChild(d);
    } else {
      // обновить опции расы (могли появиться новые ✓) и подсветку
      const sel=d.querySelector('select');
      if(sel && sel.innerHTML!==j.raceOptions){
        const v=sel.value; sel.innerHTML=j.raceOptions; if(v) sel.value=v;
      }
    }
    const sel=d.querySelector('select');
    if(prevSel['raceSel-'+it.file]!==undefined) sel.value=prevSel['raceSel-'+it.file];
    // подсветка: если у расы кандидата эталон уже есть
    if(it.raceId && j.raceStatus){
      const st2=j.raceStatus.find(x=>x.id===it.raceId);
      if(st2 && st2.has){
        sel.style.border='2px solid #2a7';
        sel.title='У этой расы уже есть эталон';
      }
    }
  });
  Object.keys(existing).forEach(f=>{ if(!seen[f]) existing[f].remove(); });
}
async function pollStatus(){
  const r=await fetch('/status'); const s=await r.json();
  const el=document.getElementById('prog');
  const elRef=document.getElementById('progRef');
  const btn=document.getElementById('btnStop');
  const st=document.getElementById('genState');
  const stp=document.getElementById('genStatePool');
  if(s.running){
    const txt='Генерация: '+s.done+'/'+s.total+' — '+s.current;
    el.style.display='block'; el.textContent=txt;
    elRef.style.display='block'; elRef.textContent='Эталон: '+txt;
    if(btn) btn.style.display='inline-block';
    if(st) st.innerHTML='<span style="color:#fa0">⏳ Генерируется: '+s.done+'/'+s.total+' — '+s.current+'</span>';
    if(stp) stp.innerHTML='<span style="color:#fa0">⏳ Генерируется: '+s.done+'/'+s.total+' — '+s.current+'</span>';
  } else {
    el.style.display='none';
    elRef.style.display='none';
    if(btn) btn.style.display='none';
    const cur=s.current||'';
    const idle = cur==='остановлено' ? '<span style="color:#a33">⏹ Остановлено</span>'
      : (cur==='готово' ? '<span style="color:#2a7">✓ Простаивает (готово)</span>'
        : '<span style="color:#2a7">✓ Простаивает</span>');
    if(st) st.innerHTML=idle;
    if(stp) stp.innerHTML=idle;
    // если только что остановили — убрать «Останавливаю генерацию...»
    if(cur==='остановлено' || cur==='готово'){
      const dr=document.getElementById('doneRef');
      if(dr && dr.textContent.indexOf('Останавливаю')!==-1) dr.textContent='Остановлено.';
    }
  }
}
async function stopGen(){
  const r=await fetch('/stop'); const j=await r.json();
  document.getElementById('doneRef').textContent=j.msg;
  const btn=document.getElementById('btnStop');
  if(btn) btn.style.display='none';
}
function openCrop(file){
  curFile=file;
  document.getElementById('cropVal').value=100;
  document.getElementById('cropLbl').textContent='100%';
  document.getElementById('modal').classList.add('show');
  updatePreview();
}
function updatePreview(){
  const pct=parseInt(document.getElementById('cropVal').value);
  document.getElementById('cropLbl').textContent=pct+'%';
  document.getElementById('mimg').src='/preview?'+new URLSearchParams({file:curFile,pct})+'&t='+Date.now();
}
document.getElementById('cropVal').oninput=e=>{ updatePreview(); };
function closeModal(){document.getElementById('modal').classList.remove('show');}
async function doCrop(){
  const pct=parseInt(document.getElementById('cropVal').value);
  const r=await fetch('/crop?'+new URLSearchParams({file:curFile,pct}));
  const j=await r.json();
  document.getElementById('done').textContent=j.msg;
  closeModal(); load();
}
async function act(file, what){
  const r=await fetch('/act?'+new URLSearchParams({file,what}));
  const j=await r.json();
  document.getElementById('done').textContent=j.msg;
  load();
}
// авто-обновление: НЕ трогает текущую вкладку
setInterval(()=>{
  const poolVisible = document.getElementById('tab-pool').style.display !== 'none';
  if(poolVisible) load();
  loadRef();
  pollStatus();
}, 3000);
restoreState(); pollStatus();
// принудительная загрузка кандидатов при старте (диагностика)
(async()=>{
  try{
    const r=await fetch('/ref?fam=F2'); const j=await r.json();
    const dbg=document.getElementById('doneRef');
    // принудительный рендер + проверка
    document.getElementById('famRef').value='F2';
    await loadRef();
    const gr=document.getElementById('gridRef');
    dbg.textContent='Диагностика: кандидатов='+(j.cands?j.cands.length:'нет')+', рас='+(j.raceStatus?j.raceStatus.length:'нет')+', в gridRef='+gr.children.length;
  }catch(e){ document.getElementById('doneRef').textContent='Диагностика ошибка: '+e; }
})();
</script></body></html>"""



def list_pool(fam="", race=""):
    files = sorted(f for f in os.listdir(POOL) if f.endswith(".png") and f != os.path.basename(COCKPIT))
    meta = {}
    if os.path.exists(os.path.join(POOL, "meta.json")):
        try:
            meta = {m["file"]: (m.get("family", ""), m.get("race", ""), m.get("race_id", "")) for m in json.load(open(os.path.join(POOL, "meta.json"), encoding="utf-8"))}
        except Exception:
            pass
    out = []
    for f in files:
        mf, mr, mrid = meta.get(f, ("", "", ""))
        if fam and mf != fam:
            continue
        if race and mrid != race:
            continue
        out.append({"file": f, "fam": mf, "race": mr})
    return out


def next_accept_name():
    n = 1
    while os.path.exists(os.path.join(ACCEPT, f"race_{n:02d}.png")):
        n += 1
    return f"race_{n:02d}.png"


def accept_file(file):
    """Принять файл из пула. Если в meta.json есть (family, race_id) — в подпапку
    final_accepted/<FAM>/r<race_id>/NN.png с ресайзом 200x200 (центр/низ);
    иначе — старое поведение: race_NN.png плоско (без ресайза)."""
    from PIL import Image, ImageOps
    src = os.path.join(POOL, file)
    fam, race_id = "", ""
    mp = os.path.join(POOL, "meta.json")
    if os.path.exists(mp):
        try:
            for m in json.load(open(mp, encoding="utf-8")):
                if m.get("file") == file:
                    fam = m.get("family", "")
                    race_id = m.get("race_id", "")
                    break
        except Exception:
            pass
    if fam and race_id:
        d = os.path.join(ACCEPT, fam, f"r{race_id}")
        os.makedirs(d, exist_ok=True)
        n = 1
        while os.path.exists(os.path.join(d, f"{n:02d}.png")):
            n += 1
        dst = os.path.join(d, f"{n:02d}.png")
        img = Image.open(src).convert("RGBA")
        img = ImageOps.pad(img, (200, 200), color=(0, 0, 0, 0), centering=(0.5, 1.0))
        img.save(dst)
        os.remove(src)
        return dst, f"Принято: {os.path.relpath(dst, ACCEPT)}"
    dst = os.path.join(ACCEPT, next_accept_name())
    shutil.copy2(src, dst)
    os.remove(src)
    return dst, f"Принято: {os.path.basename(dst)}"


STATUS_FILE = os.path.join(POOL, "status.json")


def read_status():
    if os.path.exists(STATUS_FILE):
        try:
            with open(STATUS_FILE, encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            pass
    return {"running": False, "done": 0, "total": 0, "current": ""}


def crop_bottom(file, pct):
    """Обрезать снизу: оставить pct% высоты. Сохраняет поверх (в пуле)."""
    import numpy as np
    from PIL import Image
    fp = os.path.join(POOL, file)
    if not os.path.exists(fp):
        return "нет файла"
    img = Image.open(fp).convert("RGBA")
    h = img.height
    keep = max(50, int(h * pct / 100))
    img = img.crop((0, 0, img.width, keep))
    # крупнейший компонент (убрать фон-хвосты после среза)
    from scipy import ndimage
    a = np.array(img); m = a[:, :, 3] > 40
    labels, num = ndimage.label(m)
    if num > 1:
        sizes = ndimage.sum(m, labels, range(1, num + 1))
        keep_i = np.argmax(sizes) + 1
        a[labels != keep_i] = (0, 0, 0, 0)
        img = Image.fromarray(a)
    img.save(fp)
    return f"Обрезано {file}: осталось {pct}%"


REFCAND = os.path.join(POOL, "ref_cands")


def ref_path(fam, race):
    return os.path.join(POOL, f"ref_{fam}_r{race}.png"), os.path.join(POOL, f"ref_{fam}_r{race}.json")


def _cand_info(prompt):
    """Выдержка из промпта для карточки: материал + форма (без служебных слов)."""
    if not prompt:
        return ""
    # материал: после 'made of' до следующей запятой
    mat = ""
    m = prompt.find("made of ")
    if m != -1:
        rest = prompt[m + 8:]
        mat = rest.split(",")[0].strip()
    # форма: кусок между 'viewer,'/'FRONT VIEW,' и 'made of' (антропо),
    # или 'a ... of material, formed of ...' (неантропо)
    form = ""
    i = prompt.find("viewer, ")
    if i != -1:
        seg = prompt[i + 8:]
        j = seg.find("made of")
        if j != -1:
            form = seg[:j].strip()
    if not form:
        k = prompt.find("a ")
        l = prompt.find(" of material")
        if k != -1 and l != -1 and l > k:
            form = prompt[k + 2:l].strip()
    parts = [x.strip() for x in (mat, form) if x]
    return " · ".join(parts) if parts else ""


def read_ref_status(fam):
    """Расы семейства + статус эталонов + кандидаты (с инфо о генерации)."""
    from race_gen import FAMILIES
    fam_cfg = FAMILIES.get(fam)
    race_status = []
    race_options = ""
    if fam_cfg:
        for rid, rname, *_ in fam_cfg["races"]:
            has = os.path.exists(ref_path(fam, rid)[0])
            race_status.append({"id": rid, "name": rname, "has": has})
            race_options += f'<option value="{rid}">{"✓ " if has else ""}{rname}</option>'
    cands = []
    if os.path.isdir(REFCAND):
        cand_meta = {}
        if os.path.exists(os.path.join(REFCAND, "meta.json")):
            try:
                cand_meta = json.load(open(os.path.join(REFCAND, "meta.json"), encoding="utf-8"))
            except Exception:
                pass
        for f in sorted(os.listdir(REFCAND)):
            if f.endswith(".png"):
                rn, rid, info = "", "", ""
                if f in cand_meta:
                    rn = cand_meta[f].get("race", "")
                    rid = cand_meta[f].get("race_id", "")
                    info = _cand_info(cand_meta[f].get("prompt", ""))
                cands.append({"file": f, "race": rn, "raceId": rid, "info": info})
    return {"raceStatus": race_status, "raceOptions": race_options, "cands": cands}


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass


    def _json(self, obj):
        data = json.dumps(obj).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Cache-Control", "no-store, no-cache, must-revalidate")
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        p = urlparse(self.path)
        if p.path == "/":
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Cache-Control", "no-store, no-cache, must-revalidate")
            self.send_header("Pragma", "no-cache")
            self.end_headers()
            self.wfile.write(HTML.encode("utf-8"))
        elif p.path == "/list":
            q = parse_qs(p.query)
            fam = q.get("fam", [""])[0]
            race = q.get("race", [""])[0]
            self._json(list_pool(fam, race))
        elif p.path == "/races":
            q = parse_qs(p.query)
            fam = q.get("fam", ["F2"])[0]
            from race_gen import FAMILIES
            fam_cfg = FAMILIES.get(fam)
            races = []
            if fam_cfg:
                races = [{"id": rid, "name": rname} for rid, rname, *_ in fam_cfg["races"]]
            self._json({"races": races})
        elif p.path.startswith("/img/"):
            fname = os.path.basename(p.path[5:])
            fp = os.path.join(POOL, fname)
            if os.path.exists(fp) and fname != os.path.basename(COCKPIT):
                import io
                buf = io.BytesIO()
                composite_on_cockpit(fp).save(buf, "PNG")
                data = buf.getvalue()
                self.send_response(200)
                self.send_header("Content-Type", "image/png")
                self.end_headers()
                self.wfile.write(data)
            elif os.path.exists(fp):
                self.send_response(200)
                self.send_header("Content-Type", "image/png")
                self.end_headers()
                with open(fp, "rb") as f:
                    self.wfile.write(f.read())
            else:
                self.send_response(404); self.end_headers()
        elif p.path == "/gen":
            q = parse_qs(p.query)
            n = int(q.get("n", ["8"])[0])
            # проверяем, не генерит ли уже кто-то
            st = read_status()
            if st.get("running"):
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({"msg": "Уже идёт генерация: %d/%d" % (st["done"], st["total"])}).encode())
                return
            # запуск в фоне (не блокируем HTTP-запрос)
            import subprocess
            gen_script = os.path.join(os.path.dirname(os.path.abspath(__file__)), "race_gen.py")
            fam = q.get("fam", ["F2"])[0]
            subprocess.Popen(
                [sys.executable, gen_script, fam, str(n)],
                cwd=os.path.dirname(os.path.abspath(__file__)),
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": f"Запущена генерация {n} шт (прогресс на панели)"}).encode())
        elif p.path == "/stop":
            open(os.path.join(POOL, "stop.flag"), "w").close()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": "Останавливаю генерацию..."}).encode())
        elif p.path == "/status":
            self._json(read_status())
        elif p.path == "/refs":
            q = parse_qs(p.query)
            fam = q.get("fam", ["F2"])[0]
            from race_gen import FAMILIES
            fam_cfg = FAMILIES.get(fam)
            race_names = {rid: rname for rid, rname, *_ in fam_cfg["races"]} if fam_cfg else {}
            out = []
            for f in sorted(os.listdir(POOL)):
                if f.startswith(f"ref_{fam}_r") and f.endswith(".png"):
                    race = f[len(f"ref_{fam}_r"):-4]
                    out.append({"race": race, "race_name": race_names.get(race, race)})
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"refs": out}).encode())
        elif p.path == "/refclear":
            q = parse_qs(p.query)
            fam = q.get("fam", ["F2"])[0]
            race = q.get("race", ["5"])[0]
            rp, rm = ref_path(fam, race)
            removed = False
            for fp in (rp, rm):
                if os.path.exists(fp):
                    os.remove(fp)
                    removed = True
            msg = f"Эталон расы {race} сброшен" if removed else "эталона и не было"
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": msg}).encode())
        elif p.path == "/ref":
            q = parse_qs(p.query)
            fam = q.get("fam", ["F2"])[0]
            self._json(read_ref_status(fam))
        elif p.path == "/refimg":
            q = parse_qs(p.query)
            fam = q.get("fam", ["F2"])[0]
            race = q.get("race", ["5"])[0]
            rp, _ = ref_path(fam, race)
            if os.path.exists(rp):
                self.send_response(200)
                self.send_header("Content-Type", "image/png")
                self.end_headers()
                with open(rp, "rb") as f:
                    self.wfile.write(f.read())
            else:
                self.send_response(404); self.end_headers()
        elif p.path.startswith("/refcand/"):
            fname = os.path.basename(p.path[9:])
            fp = os.path.join(REFCAND, fname)
            if os.path.exists(fp):
                import io
                buf = io.BytesIO()
                composite_on_cockpit(fp).save(buf, "PNG")
                self.send_response(200)
                self.send_header("Content-Type", "image/png")
                self.end_headers()
                self.wfile.write(buf.getvalue())
            else:
                self.send_response(404); self.end_headers()
        elif p.path == "/genref":
            q = parse_qs(p.query)
            fam = q.get("fam", ["F2"])[0]
            n = int(q.get("n", ["12"])[0])
            anthro = q.get("anthro", [""])[0]
            import subprocess
            gen_script = os.path.join(os.path.dirname(os.path.abspath(__file__)), "race_gen.py")
            os.makedirs(REFCAND, exist_ok=True)
            for f in os.listdir(REFCAND):
                os.remove(os.path.join(REFCAND, f))
            args = [sys.executable, gen_script, fam, str(n), "refcand"]
            if anthro == "anthro":
                args.append("anthro")
            subprocess.Popen(args, cwd=os.path.dirname(os.path.abspath(__file__)),
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            kind = "антропоморфных " if anthro == "anthro" else ""
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": f"Генерация {n} {kind}кандидатов (все расы {fam})..."}).encode())
        elif p.path == "/genvar":
            q = parse_qs(p.query)
            fam = q.get("fam", ["F2"])[0]
            race = q.get("race", ["5"])[0]
            n = q.get("n", ["8"])[0]
            denoise = q.get("denoise", ["0.35"])[0]
            import subprocess
            gen_script = os.path.join(os.path.dirname(os.path.abspath(__file__)), "race_gen.py")
            subprocess.Popen(
                [sys.executable, gen_script, fam, "ref", race, str(n), str(denoise)],
                cwd=os.path.dirname(os.path.abspath(__file__)),
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": f"Вариации от эталона расы: {n} шт (denoise {denoise})"}).encode())
        elif p.path == "/refset":
            q = parse_qs(p.query)
            file = q.get("file", [""])[0]
            fam = q.get("fam", ["F2"])[0]
            race = q.get("race", ["5"])[0]
            src = os.path.join(REFCAND, file)
            if os.path.exists(src):
                rp, rm = ref_path(fam, race)
                shutil.copy2(src, rp)
                # имя расы — из конфига FAMILIES (не из meta.json кандидата: там имя
                # расы, по которой кандидат сгенерирован, а эталон назначается другой)
                from race_gen import FAMILIES
                fam_cfg = FAMILIES.get(fam)
                race_name = race
                if fam_cfg:
                    for rid, rname, *_ in fam_cfg["races"]:
                        if rid == race:
                            race_name = rname
                            break
                with open(rm, "w", encoding="utf-8") as f:
                    json.dump({"fam": fam, "race": race_name}, f)
                msg = f"Эталон расы {race_name} установлен"
            else:
                msg = "нет файла кандидата"
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": msg}).encode())
        elif p.path == "/preview":
            q = parse_qs(p.query)
            file, pct = q.get("file", [""])[0], int(q.get("pct", ["100"])[0])
            import io
            from PIL import Image
            fp = os.path.join(POOL, file)
            if os.path.exists(fp):
                img = Image.open(fp).convert("RGBA")
                keep = max(50, int(img.height * pct / 100))
                img = img.crop((0, 0, img.width, keep))
                if os.path.exists(COCKPIT):
                    bg = Image.open(COCKPIT).convert("RGBA")
                    if bg.size != img.size:
                        bg = bg.resize(img.size, Image.LANCZOS)
                    bg.paste(img, (0, 0), img)
                    img = bg
                buf = io.BytesIO()
                img.save(buf, "PNG")
                self.send_response(200)
                self.send_header("Content-Type", "image/png")
                self.end_headers()
                self.wfile.write(buf.getvalue())
            else:
                self.send_response(404); self.end_headers()
        elif p.path == "/crop":
            q = parse_qs(p.query)
            file, pct = q.get("file", [""])[0], int(q.get("pct", ["100"])[0])
            msg = crop_bottom(file, pct)
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": msg}).encode())
        elif p.path == "/act":
            q = parse_qs(p.query)
            file, what = q.get("file", [""])[0], q.get("what", [""])[0]
            src = os.path.join(POOL, file)
            msg = "?"
            if what == "accept" and os.path.exists(src):
                dst, msg = accept_file(file)
            elif what == "reject" and os.path.exists(src):
                shutil.move(src, os.path.join(REJECT, file))
                msg = f"Удалено: {file}"
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": msg}).encode())
        else:
            self.send_response(404); self.end_headers()


def main():
    print(f"Студия рас: http://127.0.0.1:{PORT}")
    print(f"Пачка: {POOL}")
    print(f"Принятые -> {ACCEPT}")
    # защита от дублей: если порт занят — это уже запущенная копия
    import socket
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.bind(("127.0.0.1", PORT))
    except OSError:
        print(f"Порт {PORT} уже занят — студия уже запущена. Выход.")
        sys.exit(1)
    s.close()
    ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()


if __name__ == "__main__":
    main()