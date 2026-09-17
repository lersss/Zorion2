# -*- coding: utf-8 -*-
# human_studio.py — ЛОКАЛЬНАЯ студия отбора людей.
# Показывает пачку из humans_pool/, кнопки ПРИНЯТЬ / УДАЛИТЬ.
# Принятые -> final_accepted/race_f1_humans_*.png (нумеруются автоматически).
# Удалённые -> humans_rejected/.
# Запуск: python human_studio.py  (потом открыть http://127.0.0.1:8799)
import os, sys, json, shutil, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse, parse_qs

POOL = r"C:\Zorion2\ai_drafts\humans_pool"
ACCEPT = r"C:\Zorion2\ai_drafts\final_accepted"
REJECT = r"C:\Zorion2\ai_drafts\humans_rejected"
COCKPIT = os.path.join(POOL, "cockpit_1024.png")
PORT = 8799
os.makedirs(REJECT, exist_ok=True)


def composite_on_cockpit(fp, out_size=1024):
    """Наложить прозрачный PNG человека на кабину того же размера."""
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
<title>Студия людей</title>
<style>
 body{background:#111;color:#ddd;font-family:Arial;margin:0;padding:20px}
 h1{font-size:18px}
 .note{color:#999;font-size:13px;margin-bottom:16px}
 .toolbar{display:flex;gap:10px;align-items:center;margin-bottom:16px}
 .toolbar input{background:#222;border:1px solid #444;color:#ddd;padding:6px;border-radius:6px;width:70px}
 .toolbar button{border:none;border-radius:6px;padding:8px 14px;font-size:14px;cursor:pointer}
 .gen{background:#26a;color:#fff}.crop{background:#a62;color:#fff}
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
<h1>Студия генерации людей</h1>
<div class="note">Кнопки: СГЕНЕРИРОВАТЬ (пачка в humans_pool), ПРИНЯТЬ (-> final_accepted), УДАЛИТЬ. Клик по картинке — инструмент обрезки (выдели, докуда обрезать).</div>
<div class="toolbar">
  <span>Сгенерировать:</span>
  <input id="cnt" type="number" value="8" min="1" max="24">
  <button class="gen" onclick="gen()">СГЕНЕРИРОВАТЬ</button>
</div>
<div id="done" class="done"></div>
<div id="prog" class="prog" style="display:none;margin-bottom:12px;color:#8cf;font-size:14px"></div>
<div class="grid" id="grid"></div>

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
let curFile=null;
async function load(){
  const r=await fetch('/list'); const items=await r.json();
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
      <div class="name">${it.file}${it.sex? ' ('+it.sex+')':''}</div>
      <div class="btns">
        <button class="ok" onclick="act('${it.file}','accept')">Принять</button>
        <button class="del" onclick="act('${it.file}','reject')">Удалить</button>
      </div>`;
      g.appendChild(d);
    }
  });
  Object.keys(existing).forEach(f=>{ if(!seen[f]) existing[f].remove(); });
}
async function gen(){
  const n=document.getElementById('cnt').value;
  const r=await fetch('/gen?n='+n); const j=await r.json();
  document.getElementById('done').textContent=j.msg;
  load();
}
async function pollStatus(){
  const r=await fetch('/status'); const s=await r.json();
  const el=document.getElementById('prog');
  if(s.running){
    el.style.display='block';
    el.textContent='Генерация: '+s.done+'/'+s.total+' — '+s.current;
  } else {
    el.style.display='none';
  }
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
// авто-обновление: новые картинки + прогресс (каждые 3с)
setInterval(()=>{ load(); pollStatus(); }, 3000);
load(); pollStatus();
</script></body></html>"""


def list_pool():
    files = sorted(f for f in os.listdir(POOL) if f.endswith(".png") and f != os.path.basename(COCKPIT))
    out = []
    for f in files:
        sex = "ж" if "_f" in f else ("м" if "_m" in f else "")
        out.append({"file": f, "sex": sex})
    return out


def next_accept_name():
    n = 1
    while os.path.exists(os.path.join(ACCEPT, f"race_f1_humans_{n:02d}.png")):
        n += 1
    return f"race_f1_humans_{n:02d}.png"


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


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_GET(self):
        p = urlparse(self.path)
        if p.path == "/":
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.end_headers()
            self.wfile.write(HTML.encode("utf-8"))
        elif p.path == "/list":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(list_pool()).encode())
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
            gen_script = os.path.join(os.path.dirname(os.path.abspath(__file__)), "human_gen.py")
            subprocess.Popen(
                [sys.executable, gen_script, str(n)],
                cwd=os.path.dirname(os.path.abspath(__file__)),
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"msg": f"Запущена генерация {n} шт (прогресс на панели)"}).encode())
        elif p.path == "/status":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(read_status()).encode())
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
                dst = os.path.join(ACCEPT, next_accept_name())
                shutil.copy2(src, dst)
                os.remove(src)
                msg = f"Принято: {os.path.basename(dst)}"
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
    print(f"Студия людей: http://127.0.0.1:{PORT}")
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