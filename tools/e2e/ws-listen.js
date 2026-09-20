// tools/e2e/ws-listen.js — слушает /ws и печатает все pacman_* фреймы
// (диагностика: доходят ли события позднему клиенту). Node 22+ (global WebSocket).
const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const LISTEN_MS = parseInt(process.env.LISTEN_MS || '25000', 10);

async function main() {
  // Register a fresh player (late client).
  const username = 'e2e_' + Date.now();
  const res = await fetch(BASE_URL + '/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password: 'e2e-pass-' + Date.now() }),
  });
  if (res.status !== 201) { console.log('register FAIL ' + res.status); process.exit(1); }
  const { token } = await res.json();
  const proto = BASE_URL.startsWith('https') ? 'wss://' : 'ws://';
  const url = proto + BASE_URL.replace(/^https?:\/\//, '') + '/ws?token=' + encodeURIComponent(token);
  console.log('connecting: ' + url.replace(/token=.*/, 'token=***'));

  const ws = new WebSocket(url);
  let frames = 0;
  const pacman = [];
  ws.onopen = () => console.log('WS OPEN');
  ws.onerror = (e) => console.log('WS ERROR: ' + (e && e.message ? e.message : 'err'));
  ws.onclose = (e) => console.log('WS CLOSE code=' + e.code + ' reason=' + e.reason);
  ws.onmessage = (e) => {
    frames++;
    let data;
    try { data = JSON.parse(String(e.data)); } catch (err) { data = { raw: String(e.data).slice(0, 80) }; }
    if (data.type && String(data.type).indexOf('pacman_') === 0) {
      pacman.push(String(e.data).slice(0, 140));
      console.log('PACMAN: ' + String(e.data).slice(0, 140));
    }
  };

  await new Promise(r => setTimeout(r, LISTEN_MS));
  console.log('totalFrames=' + frames + ' pacmanFrames=' + pacman.length);
  try { ws.close(); } catch (e) {}
  process.exit(0);
}
main();