// ── Config ───────────────────────────────────────────
let API = () => document.getElementById('api-url-input').value.replace(/\/$/, '');

// ── State ────────────────────────────────────────────
let currentGameId = null;
let sseSource = null;
let elapsedInterval = null;
let gameStartTime = null;
let lobbyInterval = null;
let lastBoard = null;    // last received board for diff highlighting
let lastFromACF = null;
let lastToACF = null;
let openManagerSessionId = null;
let managerDurationInterval = null;

// ── Routing ──────────────────────────────────────────
function showLobby() {
  closeBoardConnection();
  show('lobby');
  startLobbyPolling();
}

function showBoard(gameId) {
  stopLobbyPolling();
  currentGameId = gameId;
  show('board-view');
  connectSSE(gameId);
}

function show(id) {
  document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
  document.getElementById(id).classList.add('active');
}

// ── Lobby polling ────────────────────────────────────
function startLobbyPolling() {
  fetchGames();
  lobbyInterval = setInterval(fetchGames, 3000);
}

function stopLobbyPolling() {
  clearInterval(lobbyInterval);
  lobbyInterval = null;
}

async function fetchGames() {
  try {
    // Fetch both simultaneously to render in the same lobby
    const [resG, resM] = await Promise.all([
      fetch(`${API()}/api/v1/games`),
      fetch(`${API()}/api/v1/manager`)
    ]);
    if (!resG.ok || !resM.ok) throw new Error("API error");
    const [dataG, dataM] = await Promise.all([resG.json(), resM.json()]);
    
    // Tag them to differentiate in the rendering logic
    const games = (dataG.games || []).map(g => ({ ...g, _type: 'game' }));
    const sessions = (dataM.sessions || []).map(s => ({ ...s, _type: 'session' }));
    
    // Filter out games that are managed by active sessions
    const managedGameIds = new Set(sessions.map(s => s.current_game_id).filter(id => id));
    const filteredGames = games.filter(g => !managedGameIds.has(g.id));
    
    // Combine and sort (games have start_time, sessions don't have explicit time yet but we can put them at top)
    const combined = [...sessions.reverse(), ...filteredGames.sort((a,b) => new Date(b.start_time) - new Date(a.start_time))];
    
    // Update active manager modal if open
    if (openManagerSessionId) {
      const activeSession = sessions.find(s => s.id === openManagerSessionId);
      if (activeSession) {
        updateManagerDetailsUI(activeSession);
      }
    }
    
    renderLobby(combined);
    setOnline(true);
  } catch(e) {
    setOnline(false);
  }
}

// Per-card elapsed timers
let cardTimers = {};
function startCardTimers(games) {
  Object.values(cardTimers).forEach(clearInterval);
  cardTimers = {};
  games.forEach(g => {
    const el = document.getElementById(`ct-${g.id}`);
    if (!el) return;
    const serverElapsed = g.elapsed_sec || 0;
    const sysTime = Date.now();
    const update = () => {
      const diff = g.status === 'in_progress' ? (serverElapsed + ((Date.now() - sysTime) / 1000)) : serverElapsed;
      el.textContent = formatTime(diff);
    };
    update();
    if (g.status === 'in_progress') {
      cardTimers[g.id] = setInterval(update, 1000);
    }
  });
}

function renderLobby(items) {
  const badge = document.getElementById('game-count-badge');
  const grid = document.getElementById('games-grid');
  const empty = document.getElementById('lobby-empty');

  badge.textContent = items.length;

  if (items.length === 0) {
    grid.innerHTML = '';
    empty.style.display = 'flex';
    return;
  }
  empty.style.display = 'none';

  // Preserve existing cards
  const existing = new Set([...grid.querySelectorAll('.game-card')].map(c => c.dataset.id));
  const incoming = new Set(items.map(i => i.id));

  // Remove stale
  existing.forEach(id => {
    if (!incoming.has(id)) grid.querySelector(`[data-id="${id}"]`)?.remove();
  });

  // Keep a map of latest session data for click handlers
  const latestSessions = {};

  items.forEach(item => {
    let card = grid.querySelector(`[data-id="${item.id}"]`);
    const isSession = item._type === 'session';
    const html = isSession ? sessionCardHTML(item) : cardHTML(item);
    
    if (!card) {
      card = document.createElement('div');
      card.className = 'game-card';
      if (isSession) card.classList.add('session-card');
      card.dataset.id = item.id;
      if (!isSession) {
        card.onclick = () => watchGame(item.id);
      } else {
        // Use an indirect reference so the handler always picks up the latest data
        card.onclick = () => openManagerDetails(latestSessions[item.id]);
      }
      grid.appendChild(card);
    }

    if (isSession) {
      // Always keep the latest snapshot so the closure above stays current
      latestSessions[item.id] = item;
    }

    card.innerHTML = html;
  });

  startCardTimers(items.filter(i => i._type === 'game'));
}

function cardHTML(g) {
  const capRed = Math.max(0, 12 - g.red_men - g.red_kings);
  const capBlack = Math.max(0, 12 - g.black_men - g.black_kings);
  return `
    <div class="card-top">
      <div class="vs-block">
        <div class="player-row"><div class="piece-dot black"></div><div class="player-name">${esc(g.black_player)}</div></div>
        <div class="vs-sep">vs</div>
        <div class="player-row"><div class="piece-dot red"></div><div class="player-name">${esc(g.red_player)}</div></div>
      </div>
      <div class="status-badge ${statusBadgeClass(g.status)}">${statusText(g.status)}</div>
    </div>
    <div class="card-stats">
      <div class="stat-box"><div class="stat-label">Time</div><div class="stat-value card-timer" id="ct-${g.id}">…</div></div>
      <div class="stat-box"><div class="stat-label">Moves</div><div class="stat-value">${g.move_count}</div></div>
      <div class="stat-box"><div class="stat-label">Turn</div><div class="stat-value" style="font-size:.8rem;color:${g.turn==='red'?'#f87171':'#9ca3af'}">${g.status==='in_progress'?cap(g.turn):'—'}</div></div>
    </div>
    <div class="card-footer">
      <div class="piece-counts">
        <div class="pc-group"><div class="piece-dot red" style="width:10px;height:10px"></div> ${g.red_men}m ${g.red_kings}k <span style="color:var(--text3)">-${capRed}</span></div>
        <div class="pc-group"><div class="piece-dot black" style="width:10px;height:10px"></div> ${g.black_men}m ${g.black_kings}k <span style="color:var(--text3)">-${capBlack}</span></div>
      </div>
      <button class="watch-btn">Watch →</button>
    </div>`;
}








function openManagerDetails(s) {
  openManagerSessionId = s.id;
  updateManagerDetailsUI(s);
  
  // Start duration tick (total session) + per-game elapsed tick
  clearInterval(managerDurationInterval);
  managerDurationInterval = setInterval(() => {
    if (!openManagerSessionId) return;

    // Total session duration
    const el = document.getElementById('mgr-detail-dur');
    if (el.dataset.startTime) {
      const start = new Date(el.dataset.startTime);
      const isFinished = el.dataset.isFinished === 'true';
      const end = isFinished ? new Date(el.dataset.endTime) : new Date();
      el.textContent = formatTime((end - start)/1000);
    }

    // Current game elapsed (ticked locally, reset on game change via updateManagerDetailsUI)
    const gameEl = document.getElementById('mgr-detail-game-elapsed');
    if (gameEl && gameEl.dataset.gameId && el.dataset.isFinished !== 'true') {
      const baseElapsed = parseFloat(gameEl.dataset.gameElapsed || 0);
      const baseSysTime = parseFloat(gameEl.dataset.gameSysTime || Date.now());
      gameEl.textContent = formatTime(baseElapsed + (Date.now() - baseSysTime) / 1000);
    }
  }, 1000);

  document.getElementById('manager-detail-modal').classList.add('open');
}

function updateManagerDetailsUI(s) {
  // Inject the dynamically generated Absolute HTTP format URI mapped straight to the results
  document.getElementById('mgr-detail-dir').innerHTML = `<a href="file://${s.base_dir}" target="_blank" style="color:#60a5fa; text-decoration:none">${s.base_dir}</a>`;
  
  // Create beautiful configuration header
  let configText = `${s.config.red_bot ? s.config.red_bot.toUpperCase() + ' Bot' : 'Human'} (Red) vs ${s.config.black_bot ? s.config.black_bot.toUpperCase() + ' Bot' : 'Human'} (Black)`;
  if (s.config.epochs > 1) {
    configText += ` | ${s.config.epochs} Epochs of ${s.config.matches_per_epoch}`;
  } else {
    configText += ` | Best of ${s.config.matches_per_epoch}`;
  }
  document.getElementById('mgr-detail-config').textContent = configText;
  
  const durEl = document.getElementById('mgr-detail-dur');
  durEl.dataset.startTime = s.start_time || '';
  durEl.dataset.endTime = s.end_time || '';
  durEl.dataset.isFinished = s.is_finished;

  let dur = "Calculating...";
  if (s.start_time) {
    const start = new Date(s.start_time);
    const end = s.is_finished ? new Date(s.end_time) : new Date();
    dur = formatTime((end - start)/1000);
  }
  durEl.textContent = dur;
  
  document.getElementById('mgr-detail-score').innerHTML = `
    <span style="color:#ef4444">${s.red_wins} Red</span> | 
    <span style="color:#9ca3af">${s.black_wins} Black</span> | 
    <span style="color:#fcd34d">${s.draws} Draws</span>
  `;
  
  // ── Current game in-progress timer ────────────────────
  // Reset the per-game elapsed counter whenever current_game_id changes
  const gameEl = document.getElementById('mgr-detail-game-elapsed');
  if (gameEl) {
    const prevGameId = gameEl.dataset.gameId || '';
    const newGameId  = s.current_game_id || '';

    if (newGameId !== prevGameId) {
      // Game changed: seed the counter fresh from the server-supplied elapsed
      gameEl.dataset.gameId      = newGameId;
      gameEl.dataset.gameElapsed = s.current_game_elapsed_sec || 0;
      gameEl.dataset.gameSysTime = Date.now();
    }

    if (newGameId && !s.is_finished) {
      const baseElapsed = parseFloat(gameEl.dataset.gameElapsed || 0);
      const baseSysTime = parseFloat(gameEl.dataset.gameSysTime || Date.now());
      const elapsed = baseElapsed + (Date.now() - baseSysTime) / 1000;
      gameEl.textContent = formatTime(elapsed);
    } else if (!newGameId || s.is_finished) {
      gameEl.textContent = '—';
    }
  }

  const wBtn = document.getElementById('mgr-detail-watch-btn');
  if (s.current_game_id && !s.is_finished) {
    // Always refresh the click target to the latest game id
    wBtn.onclick = () => { closeManagerDetails(); watchGame(s.current_game_id); };
    wBtn.style.display = 'block';
  } else {
    wBtn.style.display = 'none';
  }
}

function closeManagerDetails() {
  openManagerSessionId = null;
  clearInterval(managerDurationInterval);
  document.getElementById('manager-detail-modal').classList.remove('open');
}

function sessionCardHTML(s) {
  let headerTxt = '';
  if (s.config.epochs > 1) {
     headerTxt = `Epoch ${s.current_epoch}/${s.config.epochs} (Match ${s.current_match}/${s.config.matches_per_epoch})`;
  } else {
     headerTxt = `Best of ${s.config.matches_per_epoch} (Match ${s.current_match}/${s.config.matches_per_epoch})`;
  }
  
  if (s.is_finished) {
    if (s.red_wins > s.black_wins) headerTxt = `RED WINS SERIES`;
    else if (s.black_wins > s.red_wins) headerTxt = `BLACK WINS SERIES`;
    else headerTxt = `SERIES DRAW`;
  }

  return `
    <div class="card-top">
      <div class="vs-block">
        <div class="player-row"><div class="piece-dot black"></div><div class="player-name">${esc(s.config.black_player)}</div></div>
        <div class="vs-sep">vs</div>
        <div class="player-row"><div class="piece-dot red"></div><div class="player-name">${esc(s.config.red_player)}</div></div>
      </div>
      <div class="status-badge ${s.is_finished ? 'badge-draw' : 'badge-progress'}">${headerTxt}</div>
    </div>
    <div class="card-stats">
      <div class="stat-box" style="grid-column: span 3">
        <div class="stat-label">Aggregated Score</div>
        <div class="stat-value" style="font-size:14px;letter-spacing:-0.03em;color:#e5e7eb">
          <span style="color:#ef4444">${s.red_wins} Red</span> - 
          <span style="color:#9ca3af">${s.black_wins} Black</span> - 
          <span style="color:#fcd34d">${s.draws} Drw</span>
        </div>
      </div>
    </div>
    <div class="card-footer" style="justify-content:flex-end">
      <button class="watch-btn" style="pointer-events:none">View Details →</button>
    </div>`;
}

function watchGame(id) {
  showBoard(id);
}

// ── Board SSE connection ──────────────────────────────
function connectSSE(gameId) {
  document.getElementById('board-title').textContent = 'Loading…';
  document.getElementById('game-id-label').textContent = `#${gameId}`;
  setConnStatus('connecting', 'Connecting…');

  if (sseSource) { sseSource.close(); sseSource = null; }

  const url = `${API()}/api/v1/games/${gameId}/watch`;
  sseSource = new EventSource(url);

  sseSource.addEventListener('board', e => {
    const board = JSON.parse(e.data);
    updateBoard(board);
    setConnStatus('ok', 'Live');
  });

  sseSource.onerror = () => {
    setConnStatus('err', 'Reconnecting…');
  };
}

function closeBoardConnection() {
  if (sseSource) { sseSource.close(); sseSource = null; }
  clearInterval(elapsedInterval); elapsedInterval = null;
  lastBoard = null; lastFromACF = null; lastToACF = null;
}

// ── Board rendering ───────────────────────────────────
function updateBoard(board) {
  // Player names
  document.getElementById('black-name').textContent = board.black_player;
  document.getElementById('red-name').textContent = board.red_player;
  document.getElementById('board-title').textContent = `${board.red_player} vs ${board.black_player}`;
  document.getElementById('top-label').textContent = `${board.black_player}  (Black)`;
  document.getElementById('bottom-label').textContent = `${board.red_player}  (Red)`;

  // Piece panels
  renderPiecePanels(board);

  // Turn indicator
  const turn = board.turn;
  const dot = document.getElementById('turn-dot');
  dot.style.background = turn === 'red' ? '#ef4444' : '#6b7280';
  dot.style.boxShadow = `0 0 6px ${turn === 'red' ? '#ef4444' : '#6b7280'}`;
  document.getElementById('turn-label').textContent =
    board.status === 'in_progress' ? `${turn === 'red' ? board.red_player : board.black_player}'s turn` : '—';

  // Status
  document.getElementById('game-status-text').textContent = humanStatus(board.status);

  // Board
  const moves = board.pieces ? [] : [];
  // Detect last move for highlighting from pieces diff
  if (lastBoard && board.status === 'in_progress') {
    // We track last from/to via the move history
  }

  renderBoardGrid(board);

  // Elapsed timer
  if (board.status === 'in_progress') {
    const sysTime = Date.now();
    const srvElapsed = board.elapsed_seconds || 0;
    clearInterval(elapsedInterval);
    elapsedInterval = setInterval(() => {
      const s = srvElapsed + ((Date.now() - sysTime) / 1000);
      document.getElementById('game-elapsed').textContent = formatTime(s);
    }, 1000);
  } else {
    clearInterval(elapsedInterval);
    document.getElementById('game-elapsed').textContent = formatTime(board.elapsed_seconds || 0);
  }

  // Move log (right sidebar)
  if (board.pieces) updateMoveLog(board);

  lastBoard = board;
}

function renderBoardGrid(board) {
  const boardEl = document.getElementById('board');
  // Build piece lookup: key = "row-col" → piece value
  const pieceMap = {};
  if (board.pieces) {
    board.pieces.forEach(p => { pieceMap[`${p.row}-${p.col}`] = p; });
  }

  // Detect last from/to from status
  let lf = -1, lt = -1;
  if (board.matrix) {
    // Use matrix directly; last move would need the events — we'll use a simpler approach:
    // track lastFrom/lastTo via the first event that differs from last render
    if (lastBoard && lastBoard.pieces) {
      // find squares that changed
      const prev = {};
      lastBoard.pieces.forEach(p => { prev[`${p.row}-${p.col}`] = p; });
      let newSq = null;
      Object.entries(pieceMap).forEach(([k, p]) => {
        if (!prev[k]) newSq = k;
      });
      if (newSq) { const [r,c] = newSq.split('-'); lt = parseInt(r)*8+parseInt(c); }
    }

    const cells = boardEl.children;
    const totalCells = 64;

    // If board is empty, build it
    if (boardEl.children.length !== 64) {
      boardEl.innerHTML = '';
      for (let i = 0; i < 64; i++) {
        const cell = document.createElement('div');
        cell.className = 'cell';
        boardEl.appendChild(cell);
      }
    }

    // Update each cell
    for (let row = 0; row < 8; row++) {
      for (let col = 0; col < 8; col++) {
        const idx = row * 8 + col;
        const cell = boardEl.children[idx];
        const dark = (row + col) % 2 === 1;
        cell.className = `cell ${dark ? 'dark' : 'light'}`;
        cell.innerHTML = '';

        if (!dark) continue;

        const piece = pieceMap[`${row}-${col}`];
        if (!piece) continue;

        const el = document.createElement('div');
        el.className = `piece-el ${piece.color}${piece.king ? ' king' : ''}`;
        cell.appendChild(el);
      }
    }
  }
}

function renderPiecePanels(board) {
  const capRed   = Math.max(0, 12 - board.red_men   - board.red_kings);
  const capBlack = Math.max(0, 12 - board.black_men - board.black_kings);

  renderPieceDots('red-pieces',   board.red_men,   board.red_kings,   'red');
  renderPieceDots('black-pieces', board.black_men, board.black_kings, 'black');
  renderCaptured('red-captured',   capRed,   'red');
  renderCaptured('black-captured', capBlack, 'black');
}

function renderPieceDots(elId, men, kings, color) {
  const el = document.getElementById(elId);
  el.innerHTML = '';
  for (let i = 0; i < men; i++) {
    const d = document.createElement('div');
    d.className = `panel-piece ${color}`;
    el.appendChild(d);
  }
  for (let i = 0; i < kings; i++) {
    const d = document.createElement('div');
    d.className = `panel-piece ${color} king`;
    el.appendChild(d);
  }
}

function renderCaptured(elId, count, color) {
  const el = document.getElementById(elId);
  el.innerHTML = '';
  for (let i = 0; i < count; i++) {
    const d = document.createElement('div');
    d.className = `cap-dot ${color}`;
    el.appendChild(d);
  }
}

let lastMoveCount = -1;
async function updateMoveLog(board) {
  try {
    const res = await fetch(`${API()}/api/v1/games/${board.game_id}/moves`);
    if (!res.ok) return;
    const data = await res.json();
    
    // Quick optimisation: only re-render if count changed
    if (data.total === lastMoveCount && lastMoveCount > 0) return;
    lastMoveCount = data.total;

    const logEl = document.getElementById('move-log');
    logEl.innerHTML = '';
    
    // Sort moves perfectly (newest top if we want, or oldest top. Based on overflow-y:auto we usually want newest at bottom or top)
    const moves = data.moves || [];
    moves.sort((a,b) => b.move_number - a.move_number); // newest first (top)
    
    moves.forEach(m => {
      const div = document.createElement('div');
      div.className = `move-entry ${m.player}-move`;
      let txt = `<span class="mn">${m.move_number}.</span> ${m.from_acf}→${m.to_acf}`;
      if (m.captures_acf && m.captures_acf.length > 0) {
        txt += ` <span class="cap-icon">⚔</span>`;
      }
      if (m.promoted) txt += ` ♛`;
      div.innerHTML = txt;
      logEl.appendChild(div);
    });
  } catch (e) {
    console.warn("Failed to update move log:", e);
  }
}


// ── Utilities ─────────────────────────────────────────
function formatTime(s) {
  if (s < 0) s = 0;
  const m = Math.floor(s / 60), sec = Math.floor(s % 60);
  return `${String(m).padStart(2,'0')}:${String(sec).padStart(2,'0')}`;
}

function statusBadgeClass(s) {
  return {in_progress:'badge-progress',red_wins:'badge-red-wins',black_wins:'badge-black-wins',draw:'badge-draw'}[s]||'badge-progress';
}
function statusText(s) {
  return {in_progress:'IN PROGRESS',red_wins:'RED WINS',black_wins:'BLACK WINS',draw:'DRAW'}[s]||s.toUpperCase();
}
function humanStatus(s) {
  return {in_progress:'Game in progress',red_wins:'Red player wins!',black_wins:'Black player wins!',draw:'Game drawn (40-move rule)'}[s]||s;
}
function cap(s) { return s ? s[0].toUpperCase()+s.slice(1) : s; }
function esc(s) { const d=document.createElement('div');d.textContent=s;return d.innerHTML; }
function setOnline(ok) {
  const d = document.getElementById('status-dot');
  d.style.background = ok ? '#22c55e' : '#ef4444';
  d.style.boxShadow = ok ? '0 0 6px #22c55e' : '0 0 6px #ef4444';
}
function setConnStatus(state, label) {
  const dot = document.getElementById('conn-dot');
  dot.className = `conn-dot ${state}`;
  document.getElementById('conn-label').textContent = label;
}

// ── Boot ──────────────────────────────────────────────
showLobby();

// ── New-Game modal ────────────────────────────────────
const playerTypes = { red: 'human', black: 'human' };

function openNewGame() {
  document.getElementById('new-game-modal').classList.add('open');
}
function closeNewGame() {
  document.getElementById('new-game-modal').classList.remove('open');
}

function setType(side, type, btn) {
  playerTypes[side] = type;
  document.querySelectorAll(`#${side}-tabs .type-tab`).forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  const input = document.getElementById(`${side}-name-input`);
  if (type === 'human') {
    input.readOnly = false;
    if (['BFS Bot','DFS Bot'].includes(input.value)) input.value = '';
    input.placeholder = 'Player name';
  } else {
    input.value = type === 'bfs' ? 'BFS Bot' : 'DFS Bot';
    input.readOnly = true;
  }
}

function toggleMultiInfo() {
  const val = document.getElementById('match-type-select').value;
  const tDiv = document.getElementById('manager-inputs');
  const eCont = document.getElementById('epochs-container');
  
  if (val === 'single') {
    tDiv.style.display = 'none';
  } else if (val === 'best_of_n') {
    tDiv.style.display = 'flex';
    eCont.style.display = 'none';
  } else if (val === 'epochs') {
    tDiv.style.display = 'flex';
    eCont.style.display = 'flex';
  }
}

async function submitNewGame() {
  const redName  = document.getElementById('red-name-input').value.trim()  || (playerTypes.red  !== 'human' ? (playerTypes.red  === 'bfs' ? 'BFS Bot' : 'DFS Bot') : 'Red Player');
  const blackName= document.getElementById('black-name-input').value.trim() || (playerTypes.black !== 'human' ? (playerTypes.black === 'bfs' ? 'BFS Bot' : 'DFS Bot') : 'Black Player');

  const matchType = document.getElementById('match-type-select').value;
  const isManager = matchType !== 'single';
  const isHumanSpeed = document.getElementById('human-speed-checkbox').checked;
  
  if (isManager) {
    const body = {
      red_player:   redName,
      black_player: blackName,
      red_bot:      playerTypes.red   !== 'human' ? playerTypes.red   : undefined,
      black_bot:    playerTypes.black !== 'human' ? playerTypes.black : undefined,
      matches_per_epoch: parseInt(document.getElementById('matches-input').value) || 5,
      epochs: matchType === 'epochs' ? (parseInt(document.getElementById('epochs-input').value) || 3) : 1,
      human_speed:  isHumanSpeed
    };
    Object.keys(body).forEach(k => body[k] === undefined && delete body[k]);
    try {
      const res = await fetch(`${API()}/api/v1/manager`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error(await res.text());
      closeNewGame();
      // Keep user in lobby to watch the manager overall progress
      fetchGames();
    } catch(e) {
      alert('Failed to start manager: ' + e.message);
    }
  } else {
    const body = {
      red_player:   redName,
      black_player: blackName,
      red_bot:      playerTypes.red   !== 'human' ? playerTypes.red   : undefined,
      black_bot:    playerTypes.black !== 'human' ? playerTypes.black : undefined,
      human_speed:  isHumanSpeed
    };
    Object.keys(body).forEach(k => body[k] === undefined && delete body[k]);
  
    try {
      const res = await fetch(`${API()}/api/v1/games`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      closeNewGame();
      showBoard(data.game_id);
    } catch(e) {
      alert('Failed to create game: ' + e.message);
    }
  }
}