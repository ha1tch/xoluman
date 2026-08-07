// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// fsm-editor.js — Lit shell for T-14's FSM editor.
//
// Reuses fsm-canvas-engine.js directly — Seam's own extended fork of
// Evan Wallace's Finite State Machine Designer — rather than the
// from-scratch SVG canvas this file originally contained. That first
// version worked, but a direct comparison against the actual engine
// (not a stale summary of it) showed real, deliberate interaction
// design this shell doesn't attempt to reproduce: a pulsing
// in-progress link while dragging, an eased snap-back animation when
// a drag doesn't connect, hover/selection halos, rubber-band
// multi-select with its own pulsing group halo, momentum-based zoom,
// smooth label repositioning, per-link custom colours, and JSON/SVG/
// LaTeX export. None of that is rebuilt here; the engine does it, and
// this shell is deliberately thin — modeled closely on Seam's own
// integration pattern (theming via a canvas-context Proxy, the same
// documented Engine API), not a redesign of it.
//
// This shell owns only what's specific to xoluman: converting between
// the engine's own backup format (nodes[]/links[], each node an
// {x,y,text,isAcceptState} pair, not xolu's own shape at all) and
// xolu's real MachineSpec; local validation via fsm-toolkit (see
// internal/ui/fsmdef.go's own doc comment); and layout persistence as
// xoluman's own xoluman_fsm_layout entity, split out from the engine's
// combined backup object on save and merged back in on load — the
// same "layout kept structurally separate from the abstract machine"
// principle fsm-toolkit's own fsmedit tool uses, still followed here
// even though the engine's own native format keeps them together.
//
// Data-model gap, disclosed: xolu's transitions carry Set, a map of
// potentially several variable assignments. The engine's own
// linkProperties only ever had a single free-text `action` field
// (guard and, as of this session, output are real, separate fields —
// see fsm-canvas-engine.js's own top comment). Set is represented here
// as `action` holding a semicolon-separated list of `key=value` pairs,
// parsed and re-serialized on each round trip — a disclosed
// convention, not a full structured editor for multiple assignments.

import { LitElement, html } from 'lit';

function fsmPalette() {
  const dark = document.documentElement.classList.contains('dark');
  return {
    fg:       dark ? '#e2e8f0' : '#0f172a',
    selected: dark ? '#818cf8' : '#4f46e5',
    canvas:   dark ? '#1e293b' : '#f8fafc',
    gridLine: dark ? 'rgba(99,102,241,0.10)' : 'rgba(99,102,241,0.12)',
  };
}

// Colour-remapping proxy around the canvas 2D context — the same
// mechanism Seam's own shell uses to theme the engine without
// touching its hardcoded 'black'/'blue' default colours.
function themedContext(rawCtx) {
  return new Proxy(rawCtx, {
    set(target, prop, value) {
      if ((prop === 'strokeStyle' || prop === 'fillStyle') && typeof value === 'string') {
        const p = fsmPalette();
        if (value === 'black') value = p.fg;
        else if (value === 'blue') value = p.selected;
      }
      target[prop] = value;
      return true;
    },
    get(target, prop) {
      const v = target[prop];
      return typeof v === 'function' ? v.bind(target) : v;
    },
  });
}

// ─── Conversion: engine backup format <-> xolu MachineSpec ─────────────────

// setToAction/actionToSet: the disclosed Set<->action convention (see
// this file's own top comment) — semicolon-separated key=value pairs.
function setToAction(set) {
  if (!set) return '';
  return Object.entries(set).map(([k, v]) => `${k}=${v}`).join(';');
}
function actionToSet(action) {
  const trimmed = (action || '').trim();
  if (!trimmed) return undefined;
  const set = {};
  for (const part of trimmed.split(';')) {
    const eq = part.indexOf('=');
    if (eq === -1) continue; // not a key=value pair — skipped, not an error; this is a lightweight convention, not a strict grammar
    const key = part.slice(0, eq).trim();
    const val = part.slice(eq + 1).trim();
    if (key) set[key] = val;
  }
  return Object.keys(set).length ? set : undefined;
}

// backupToSpec builds a xolu MachineSpec from the engine's own
// getBackupData() output, plus the name/description/determinism the
// engine itself has no concept of (those live in this shell's own
// form fields, not the canvas).
function backupToSpec(backup, name, description, determinism) {
  const nodeNames = (backup.nodes || []).map((n, i) => (n.text || '').trim() || `state_${i}`);
  const states = {};
  nodeNames.forEach((n, i) => { states[n] = { terminal: !!backup.nodes[i].isAcceptState }; });

  let initial = '';
  const transitions = [];
  const outputAlphabet = [];
  for (const link of backup.links || []) {
    if (link.type === 'StartLink') {
      initial = nodeNames[link.node];
      continue; // marks the initial state only, not a real transition
    }
    let from, to;
    if (link.type === 'SelfLink') {
      from = to = nodeNames[link.node];
    } else if (link.type === 'Link') {
      from = nodeNames[link.nodeA];
      to = nodeNames[link.nodeB];
    } else {
      continue;
    }
    const t = { from, input: link.text || '', to };
    if (link.guard) t.guard = link.guard;
    if (link.output) {
      t.output = link.output;
      if (!outputAlphabet.includes(link.output)) outputAlphabet.push(link.output);
    }
    const set = actionToSet(link.action);
    if (set) t.set = set;
    transitions.push(t);
  }

  const spec = { name, description: description || undefined, initial, determinism, states, transitions };
  if (outputAlphabet.length) spec.output_alphabet = outputAlphabet;
  return spec;
}

// specToBackup builds the engine's own backup format from a xolu
// MachineSpec plus a saved xoluman_fsm_layout (or none, for a
// brand-new machine — nodes then get a simple circle arrangement, not
// the engine's own layout algorithm, since the engine has no built-in
// auto-layout of its own to call).
function specToBackup(spec, layout) {
  const stateNames = Object.keys(spec.states || {});
  const positions = (layout && layout.states) || {};
  const cx = 400, cy = 250, r = Math.max(120, 40 * stateNames.length);

  const nodes = stateNames.map((name, i) => {
    const pos = positions[name];
    if (pos) return { x: pos.x, y: pos.y, text: name, isAcceptState: !!spec.states[name].terminal, textOnly: false };
    const angle = (2 * Math.PI * i) / Math.max(stateNames.length, 1) - Math.PI / 2;
    return {
      x: Math.round(cx + r * Math.cos(angle)), y: Math.round(cy + r * Math.sin(angle)),
      text: name, isAcceptState: !!spec.states[name].terminal, textOnly: false,
    };
  });
  const indexOf = {};
  stateNames.forEach((n, i) => { indexOf[n] = i; });

  const links = [];
  if (spec.initial && indexOf[spec.initial] !== undefined) {
    links.push({ type: 'StartLink', node: indexOf[spec.initial], deltaX: -60, deltaY: 0, text: '' });
  }
  for (const t of spec.transitions || []) {
    if (indexOf[t.from] === undefined || indexOf[t.to] === undefined) continue; // dangling reference — skipped here, fsm-toolkit's own validation is what should surface this to the person, not a silent canvas crash
    const base = { text: t.input || '', guard: t.guard || '', action: setToAction(t.set), output: t.output || '' };
    if (t.from === t.to) {
      links.push({ type: 'SelfLink', node: indexOf[t.from], anchorAngle: -Math.PI / 2, ...base });
    } else {
      links.push({ type: 'Link', nodeA: indexOf[t.from], nodeB: indexOf[t.to], parallelPart: 0.5, perpendicularPart: 0, lineAngleAdjust: 0, ...base });
    }
  }

  return { nodeRadius: 30, nodes, links };
}

class XoluFsmEditor extends LitElement {
  static properties = {
    idValue: { attribute: 'id-value', type: Number },
    dataUrl: { attribute: 'data-url' },
    saveUrl: { attribute: 'save-url' },
    validateUrl: { attribute: 'validate-url' },
    layoutUrl: { attribute: 'layout-url' },
    // Graph-mode configuration — see this file's own top comment on
    // the whole mode split. graphUrl is the same connection's
    // /query/graph endpoint; mode switches the whole shell between
    // "fsm" (default: editable machine definition) and "graph"
    // (read-only Sulpher-query visualization, click-to-inspect).
    mode: { type: String },
    graphUrl: { attribute: 'graph-url' },
    _dark: { state: true },
    _saving: { state: true },
    _error: { state: true },
    _validation: { state: true },
    _inspecting: { state: true },
    _graphLoading: { state: true },
  };

  createRenderRoot() {
    return this;
  }

  constructor() {
    super();
    this._name = '';
    this._description = '';
    this._determinism = 'strict';
    this._saving = false;
    this._error = null;
    this._validation = null;
    this._validateTimer = null;
    this._layoutSaveTimer = null;
    this._dark = document.documentElement.classList.contains('dark');
    this.mode = 'fsm';
    this._graphQuery = '';
    this._graphLoading = false;
    this._inspecting = null; // {kind: 'node'|'edge', label, data}
    this._lastSelectedObject = null;
  }

  async firstUpdated() {
    const canvas = this.querySelector('#fsm-canvas');

    const origGetContext = canvas.getContext.bind(canvas);
    canvas.getContext = (type, ...args) => {
      const rawCtx = origGetContext(type, ...args);
      if (type === '2d') return themedContext(rawCtx);
      return rawCtx;
    };

    initFSM(canvas);

    // Wrap draw() — in fsm mode this also triggers debounced local
    // validation and a debounced layout save (the engine has no
    // "something changed" event of its own, but every meaningful edit
    // goes through draw() to actually show up, so hooking it here
    // catches all of them). In both modes, this also checks for a
    // selection change, driving graph mode's click-to-inspect panel —
    // the engine has no "selection changed" event either, but
    // selecting something always redraws.
    const origDraw = window.draw;
    window.draw = () => {
      origDraw();
      if (this.mode === 'fsm') {
        this._scheduleValidate();
        this._scheduleLayoutSave();
      }
      this._checkSelectionChange();
    };

    this._resize(canvas);
    this._resizeObserver = new ResizeObserver(() => this._resize(canvas));
    this._resizeObserver.observe(this.querySelector('.fsm-canvas-wrap'));

    if (this.mode === 'graph') {
      zoomToFit();
    } else {
      await this._load(canvas);
    }

    this._applyTheme(canvas);
    this._themeObserver = new MutationObserver(() => {
      this._dark = document.documentElement.classList.contains('dark');
      this._applyTheme(canvas);
      if (typeof draw === 'function') draw();
    });
    this._themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._themeObserver?.disconnect();
    this._resizeObserver?.disconnect();
  }

  async _load(canvas) {
    if (!this.idValue) {
      zoomToFit();
      return;
    }
    try {
      const resp = await fetch(this.dataUrl);
      if (!resp.ok) {
        this._error = await resp.text();
        return;
      }
      const data = await resp.json();
      if (data.def && data.def.spec) {
        this._name = data.def.spec.name || '';
        this._description = data.def.spec.description || '';
        this._determinism = data.def.spec.determinism || 'strict';
        const backup = specToBackup(data.def.spec, data.layout);
        restoreFromBackupData(backup);
      }
      this._resize(canvas);
      zoomToFit();
    } catch (e) {
      this._error = String(e);
    }
  }

  _applyTheme(canvas) {
    const dark = this._dark;
    const p = fsmPalette();
    canvas.style.backgroundColor = p.canvas;
    if (typeof seamFont            !== 'undefined') window.seamFont            = '12px system-ui, -apple-system, "Segoe UI", sans-serif';
    if (typeof seamNodeFill        !== 'undefined') window.seamNodeFill        = p.canvas;
    if (typeof seamAccentColor     !== 'undefined') window.seamAccentColor     = '#4f46e5';
    if (typeof seamNodeStrokeColor !== 'undefined') window.seamNodeStrokeColor = dark ? '#818cf8' : '#6366f1';
    if (typeof seamNodeLabelColor  !== 'undefined') window.seamNodeLabelColor  = dark ? '#e2e8f0' : '#1e293b';
    if (typeof seamLinkStrokeColor !== 'undefined') window.seamLinkStrokeColor = dark ? '#94a3b8' : '#64748b';
    if (typeof seamLinkLabelColor  !== 'undefined') window.seamLinkLabelColor  = dark ? '#94a3b8' : '#64748b';
    if (typeof seamEditBg          !== 'undefined') window.seamEditBg          = dark ? 'rgba(30,41,59,0.97)'   : 'rgba(248,250,252,0.97)';
    if (typeof seamEditBorder      !== 'undefined') window.seamEditBorder      = dark ? 'rgba(129,140,248,0.7)' : 'rgba(99,102,241,0.5)';
  }

  _resize(canvas) {
    const wrap = this.querySelector('.fsm-canvas-wrap');
    if (!wrap) return;
    const { width, height } = wrap.getBoundingClientRect();
    if (width > 0 && height > 0) {
      canvas.width = width;
      canvas.height = height;
      if (typeof draw === 'function') draw();
    }
  }

  // _checkSelectionChange drives graph mode's click-to-inspect panel.
  // The engine has no "selection changed" event — this runs on every
  // draw() (see firstUpdated's own wrapping), comparing against the
  // last-seen selectedObject, so it only actually updates state (and
  // triggers a Lit re-render) when the selection genuinely changed,
  // not on every single redraw.
  _checkSelectionChange() {
    if (this.mode !== 'graph') return;
    const sel = typeof selectedObject !== 'undefined' ? selectedObject : null;
    if (sel === this._lastSelectedObject) return;
    this._lastSelectedObject = sel;
    if (!sel) {
      this._inspecting = null;
    } else if (sel._graphNode) {
      this._inspecting = { kind: 'node', label: sel.text, data: sel._graphNode.data };
    } else if (sel._graphEdge) {
      this._inspecting = { kind: 'edge', label: sel._graphEdge.rel, data: sel._graphEdge };
    } else {
      this._inspecting = null;
    }
  }

  // _labelForNode picks a reasonable display field from a node's own
  // data rather than showing "companies:6" on the canvas — the first
  // of a short list of common name-shaped fields, falling back to
  // "type#id" when none apply. Deliberately simple: this is a label
  // for a small canvas circle, not a summary.
  _labelForNode(node) {
    const d = node.data || {};
    if (d.name) return String(d.name);
    if (d.title) return String(d.title);
    if (d.first_name || d.last_name) return `${d.first_name || ''} ${d.last_name || ''}`.trim();
    if (d.subject) return String(d.subject);
    return `${node.type}#${d.id ?? ''}`;
  }

  async _runGraphQuery() {
    if (!this._graphQuery.trim() || typeof clearCanvas !== 'function') return;
    this._graphLoading = true;
    this._error = null;
    this._inspecting = null;

    try {
      const resp = await fetch(this.graphUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: this._graphQuery }),
      });
      const text = await resp.text();
      if (!resp.ok) {
        this._error = text || `HTTP ${resp.status}`;
        this._graphLoading = false;
        return;
      }
      const data = JSON.parse(text);
      this._applyGraphData(data);
    } catch (e) {
      this._error = String(e);
    }
    this._graphLoading = false;
  }

  // _applyGraphData converts {nodes, edges} (internal/ui/query.go's
  // Graph handler — see that file's own doc comment on the shape) into
  // the engine's own backup format and loads it, then attaches each
  // node/edge's real data directly onto the live engine objects
  // (_graphNode/_graphEdge — names deliberately not colliding with
  // any field the engine's own Node/Link classes use) for
  // _checkSelectionChange to read back out on click. A simple circle
  // layout, same reasoning as fsm mode's own fallback for a machine
  // with no saved layout — this is a fresh visualization each query
  // run, not something with a persisted layout to load.
  _applyGraphData(data) {
    const nodes = data.nodes || [];
    const edges = data.edges || [];
    const indexOf = {};
    nodes.forEach((n, i) => { indexOf[n.id] = i; });

    const cx = 400, cy = 250, r = Math.max(120, 45 * nodes.length);
    const backup = {
      nodeRadius: 30,
      nodes: nodes.map((n, i) => {
        const angle = (2 * Math.PI * i) / Math.max(nodes.length, 1) - Math.PI / 2;
        return {
          x: Math.round(cx + r * Math.cos(angle)), y: Math.round(cy + r * Math.sin(angle)),
          text: this._labelForNode(n), isAcceptState: false, textOnly: false,
        };
      }),
      links: edges
        .filter((e) => indexOf[e.from] !== undefined && indexOf[e.to] !== undefined)
        .map((e) =>
          e.from === e.to
            ? { type: 'SelfLink', node: indexOf[e.from], anchorAngle: -Math.PI / 2, text: e.rel }
            : { type: 'Link', nodeA: indexOf[e.from], nodeB: indexOf[e.to], text: e.rel, parallelPart: 0.5, perpendicularPart: 0, lineAngleAdjust: 0 }
        ),
    };

    clearCanvas();
    restoreFromBackupData(backup);
    // Attach original data by position — restoreFromBackupData pushes
    // to the engine's own global nodes[]/links[] in the same order
    // the backup object listed them, confirmed directly against
    // fsm-canvas-engine.js's own restoreFromBackupData, not assumed.
    nodes.forEach((n, i) => { if (window.nodes[i]) window.nodes[i]._graphNode = n; });
    let linkIdx = 0;
    edges.forEach((e) => {
      if (indexOf[e.from] === undefined || indexOf[e.to] === undefined) return;
      if (window.links[linkIdx]) window.links[linkIdx]._graphEdge = e;
      linkIdx++;
    });
    zoomToFit();
  }

  _scheduleValidate() {
    if (this._validateTimer) clearTimeout(this._validateTimer);
    this._validateTimer = setTimeout(() => this._runValidation(), 500);
  }

  async _runValidation() {
    if (!this._name || typeof getBackupData !== 'function') {
      this._validation = null;
      return;
    }
    const spec = backupToSpec(getBackupData(), this._name, this._description, this._determinism);
    if (!spec.initial) {
      this._validation = null;
      return;
    }
    try {
      const resp = await fetch(this.validateUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(spec),
      });
      if (resp.ok) this._validation = await resp.json();
    } catch (e) {
      // Best-effort — local validation failing to run shouldn't block
      // editing; xolu's own save-time check still applies.
    }
  }

  _scheduleLayoutSave() {
    if (!this.idValue) return; // nowhere to persist a layout for a not-yet-saved machine
    if (this._layoutSaveTimer) clearTimeout(this._layoutSaveTimer);
    this._layoutSaveTimer = setTimeout(async () => {
      if (typeof getBackupData !== 'function') return;
      const backup = getBackupData();
      const states = {};
      for (const node of backup.nodes || []) {
        const name = (node.text || '').trim();
        if (name) states[name] = { x: Math.round(node.x), y: Math.round(node.y) };
      }
      try {
        await fetch(this.layoutUrl, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ version: 1, canvasOffsetX: 0, canvasOffsetY: 0, states }),
        });
      } catch (e) {
        // Best-effort — a failed layout save just means positions
        // reset to the circle arrangement next load, not a lost edit
        // to the machine itself.
      }
    }, 700);
  }

  async _save() {
    if (this._saving || typeof getBackupData !== 'function') return;
    this._saving = true;
    this._error = null;

    const spec = backupToSpec(getBackupData(), this._name, this._description, this._determinism);
    try {
      const method = this.idValue ? 'PUT' : 'POST';
      const resp = await fetch(this.saveUrl, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(spec),
      });
      const text = await resp.text();
      if (!resp.ok) {
        this._error = text || `HTTP ${resp.status}`;
      } else {
        const result = JSON.parse(text);
        if (!this.idValue && result.id) {
          this.idValue = result.id;
          const base = this.saveUrl.replace(/\/$/, '');
          this.layoutUrl = base + '/' + result.id + '/layout';
          window.history.replaceState(null, '', base + '/' + result.id);
          this._scheduleLayoutSave();
        }
      }
    } catch (e) {
      this._error = String(e);
    }
    this._saving = false;
  }

  render() {
    const dark = this._dark;
    const border = dark ? '#334155' : '#e2e8f0';
    const toolbarBg = dark ? '#1e293b' : '#ffffff';
    const textCol = dark ? '#e2e8f0' : '#0f172a';
    const inputBorder = dark ? '#475569' : '#cbd5e1';
    const btnStyle = `display:inline-flex;align-items:center;gap:4px;padding:5px 12px;font-size:13px;font-weight:500;border-radius:6px;border:1px solid ${inputBorder};background:${toolbarBg};color:${textCol};cursor:pointer`;
    const isGraph = this.mode === 'graph';

    return html`
      <style>
        xolu-fsm-editor { display:flex; flex-direction:column; height:640px; width:100%; }
        .fsm-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:0.75rem; }
        .fsm-fields { display:flex; gap:0.75rem; flex-wrap:wrap; padding:0.5rem 0 0.75rem; }
        .fsm-fields input, .fsm-fields select {
          padding:0.4rem 0.6rem; border:1px solid #d1d5db; border-radius:0.375rem; font-size:0.875rem;
          background:#fff; color:#111827;
        }
        .dark .fsm-fields input, .dark .fsm-fields select { border-color:#374151; background:#1f2937; color:#f3f4f6; }
        .fsm-graph-query { flex:1; min-width:20rem; font-family:monospace; }
        .fsm-toolbar { display:flex; align-items:center; gap:8px; padding:8px 12px; border-bottom:1px solid ${border}; background:${toolbarBg}; }
        .fsm-hint { flex:1; font-size:12px; color:${dark ? '#64748b' : '#94a3b8'}; }
        .fsm-canvas-wrap { flex:1; overflow:hidden; position:relative; border:1px solid ${border}; border-radius:0 0 0.5rem 0.5rem; }
        #fsm-canvas { display:block; width:100%; height:100%; cursor:crosshair; }
        .fsm-panel { margin-top:0.75rem; padding:0.6rem 0.9rem; border-radius:0.5rem; font-size:0.8125rem; }
        .fsm-panel-error { background:#fef2f2; border:1px solid #fecaca; color:#991b1b; }
        .dark .fsm-panel-error { background:rgba(127,29,29,0.3); border-color:rgba(248,113,113,0.3); color:#fca5a5; }
        .fsm-panel-warn { background:#fffbeb; border:1px solid #fde68a; color:#92400e; margin-top:0.4rem; }
        .dark .fsm-panel-warn { background:rgba(120,53,15,0.3); border-color:rgba(252,211,77,0.3); color:#fcd34d; }
        .fsm-panel-ok { background:#f0fdf4; border:1px solid #bbf7d0; color:#166534; }
        .dark .fsm-panel-ok { background:rgba(20,83,45,0.3); border-color:rgba(74,222,128,0.3); color:#86efac; }
        .fsm-inspect { background:#f9fafb; border:1px solid #e5e7eb; color:#374151; }
        .dark .fsm-inspect { background:#1f2937; border-color:#374151; color:#d1d5db; }
        .fsm-inspect-title { font-weight:600; margin-bottom:0.35rem; text-transform:uppercase; font-size:0.7rem; letter-spacing:0.05em; color:${dark ? '#818cf8' : '#4f46e5'}; }
        .fsm-inspect-row { display:flex; gap:0.5rem; padding:0.15rem 0; font-family:monospace; font-size:0.75rem; }
        .fsm-inspect-key { color:${dark ? '#9ca3af' : '#6b7280'}; min-width:8rem; flex-shrink:0; }
        .fsm-inspect-val { word-break:break-all; }
      </style>

      <div class="fsm-header">
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
          ${isGraph ? 'Graph viewer' : this.idValue ? `Edit machine #${this.idValue}` : 'New machine'}
        </h1>
        ${isGraph
          ? ''
          : html`
              <button class="${btnStyle}" style="background:#4f46e5;color:#fff;border-color:#4f46e5" ?disabled=${this._saving} @click=${this._save}>
                ${this._saving ? 'Saving…' : 'Save'}
              </button>
            `}
      </div>

      ${this._error ? html`<div class="fsm-panel fsm-panel-error">${this._error}</div>` : ''}

      ${isGraph
        ? html`
            <div class="fsm-fields">
              <input type="text" class="fsm-graph-query" placeholder="MATCH (a)-[r]->(b) RETURN a, r, b"
                .value=${this._graphQuery}
                @input=${(e) => (this._graphQuery = e.target.value)}
                @keydown=${(e) => e.key === 'Enter' && this._runGraphQuery()} />
              <button class="${btnStyle}" style="background:#4f46e5;color:#fff;border-color:#4f46e5" ?disabled=${this._graphLoading} @click=${this._runGraphQuery}>
                ${this._graphLoading ? 'Running…' : 'Run'}
              </button>
            </div>
          `
        : html`
            <div class="fsm-fields">
              <input type="text" placeholder="Name" .value=${this._name}
                @input=${(e) => { this._name = e.target.value; this._scheduleValidate(); }} />
              <input type="text" placeholder="Description (optional)" .value=${this._description}
                @input=${(e) => (this._description = e.target.value)} />
              <select .value=${this._determinism} @change=${(e) => (this._determinism = e.target.value)}>
                <option value="strict">strict</option>
                <option value="loose">loose</option>
                <option value="firstmatch">firstmatch</option>
              </select>
            </div>
          `}

      <div class="fsm-toolbar">
        <span class="fsm-hint">
          ${isGraph
            ? html`Click a node or edge to inspect its data &nbsp;·&nbsp; Drag background: pan &nbsp;·&nbsp; Scroll: zoom`
            : html`
                Double-click: add state &nbsp;·&nbsp;
                Shift-click: toggle terminal state &nbsp;·&nbsp;
                Drag from a state's edge: add transition &nbsp;·&nbsp;
                Right-click a transition: guard / output / set &nbsp;·&nbsp;
                Drag background: pan &nbsp;·&nbsp; Scroll: zoom &nbsp;·&nbsp; Delete: remove selected
              `}
        </span>
        <button class="${btnStyle}" @click=${() => typeof zoomBy === 'function' && zoomBy(1 / 1.25)}>−</button>
        <button class="${btnStyle}" @click=${() => typeof zoomToFit === 'function' && zoomToFit()} title="Fit to screen">⤢</button>
        <button class="${btnStyle}" @click=${() => typeof zoomBy === 'function' && zoomBy(1.25)}>+</button>
      </div>

      <div class="fsm-canvas-wrap">
        <canvas id="fsm-canvas"></canvas>
      </div>

      ${isGraph ? this._renderInspectPanel() : this._renderValidationPanel()}
    `;
  }

  _renderValidationPanel() {
    if (!this._validation) return '';
    return html`
      <div class="fsm-panel ${this._validation.valid ? 'fsm-panel-ok' : 'fsm-panel-error'}">
        <strong>${this._validation.valid ? 'Structurally valid' : 'Invalid'}</strong>
        ${(this._validation.errors || []).map((e) => html`<div>${e}</div>`)}
      </div>
      ${(this._validation.warnings || []).length
        ? html`<div class="fsm-panel fsm-panel-warn">${this._validation.warnings.map((w) => html`<div>${w}</div>`)}</div>`
        : ''}
      ${(this._validation.skippedChecks || []).length
        ? html`<div class="text-xs text-gray-400 dark:text-gray-500 mt-1">Not checked locally: ${this._validation.skippedChecks.join('; ')}</div>`
        : ''}
    `;
  }

  // _renderInspectPanel is the direct answer to "click on nodes and
  // see a form with the data contained in the nodes, click on the
  // edges and see the data contained in the edges" — every field from
  // the clicked node's real entity document, or the clicked edge's
  // from/rel/to, listed plainly. Read-only: this is a viewer, not an
  // editor for graph data, which would be a genuinely separate,
  // larger feature (write-back semantics for arbitrary entity types
  // through a generic panel) not attempted here.
  _renderInspectPanel() {
    if (!this._inspecting) {
      return html`<div class="text-xs text-gray-400 dark:text-gray-500 mt-2">Run a query, then click a node or edge to inspect it.</div>`;
    }
    const { kind, label, data } = this._inspecting;
    const entries = Object.entries(data || {});
    return html`
      <div class="fsm-panel fsm-inspect">
        <div class="fsm-inspect-title">${kind === 'node' ? 'Node' : 'Edge'}: ${label}</div>
        ${entries.map(
          ([k, v]) => html`
            <div class="fsm-inspect-row">
              <span class="fsm-inspect-key">${k}</span>
              <span class="fsm-inspect-val">${typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v)}</span>
            </div>
          `
        )}
      </div>
    `;
  }
}

customElements.define('xolu-fsm-editor', XoluFsmEditor);
