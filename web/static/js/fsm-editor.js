// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// fsm-editor.js — Lit shell for T-14's FSM viewer/editor
// (internal/ui/fsmdef.go). A visual canvas, not a plain form: states
// are draggable circles, transitions are arrows between them, and
// dragging a state persists its position the same way fsm-toolkit's
// own fsmedit tool separates visual layout from the abstract machine
// (see fsmdef.go's own doc comment for the full reasoning) — this
// component is the client-side half of that same design.
//
// Validation happens twice, deliberately: locally (debounced, via
// validate-url, which runs fsm-toolkit's Validate()/Analyse() with no
// network call to xolu at all) as the person edits, and again
// server-side by xolu itself at actual save time (the authoritative
// check) — the local pass is a faster front for catching typos and
// structural mistakes, not a replacement for it.
//
// Simplification, disclosed here rather than silently: xolu allows a
// transition's "from" to be a single state or an array of states
// (TransitionDef.From). This editor always edits and saves a single
// from-state per transition row. Loading a machine with a multi-state
// transition expands it into one independent row per state — editing
// one afterward no longer affects the others. Full multi-from editing
// would need real diagram-interaction design (one arrow bundle
// representing several states) not attempted in this first pass.

import { LitElement, html, svg } from 'lit';

class XoluFsmEditor extends LitElement {
  static properties = {
    idValue: { attribute: 'id-value', type: Number },
    dataUrl: { attribute: 'data-url' },
    saveUrl: { attribute: 'save-url' },
    validateUrl: { attribute: 'validate-url' },
    layoutUrl: { attribute: 'layout-url' },
  };

  createRenderRoot() {
    return this;
  }

  constructor() {
    super();
    this._loading = true;
    this._saving = false;
    this._error = null;

    this._name = '';
    this._description = '';
    this._initial = '';
    this._determinism = 'strict';
    // states: array of {name, terminal} — order is the UI's own
    // display order, not meaningful to xolu (its own States is a map).
    this._states = [];
    this._transitions = []; // [{from, input, to, output, guard, set}]
    this._outputAlphabet = [];

    // Fields round-tripped but not editable in this first pass —
    // preserved from the loaded def so editing an existing machine
    // that uses them doesn't silently drop them on save.
    this._variables = undefined;
    this._linkedStates = undefined;
    this._gc = undefined;
    this._inputQueries = undefined;

    this._layout = { version: 1, canvasOffsetX: 0, canvasOffsetY: 0, states: {} };
    this._layoutDirty = false;

    this._selectedState = null;
    this._drag = null; // {name, offsetX, offsetY} while a pointer-drag is in progress

    this._showAddTransition = false;
    this._newTransition = { from: '', input: '', to: '', output: '', guard: '' };
    this._newStateName = '';
    this._showAddState = false;

    this._validation = null;
    this._validating = false;
    this._validateTimer = null;
    this._layoutSaveTimer = null;
  }

  connectedCallback() {
    super.connectedCallback();
    this._load();
  }

  async _load() {
    if (!this.idValue) {
      this._loading = false;
      this._states = [];
      this.requestUpdate();
      this._scheduleValidate();
      return;
    }
    try {
      const resp = await fetch(this.dataUrl);
      if (resp.ok) {
        const data = await resp.json();
        this._applyDef(data.def);
        if (data.layout) this._layout = data.layout;
      } else {
        this._error = await resp.text();
      }
    } catch (e) {
      this._error = String(e);
    }
    this._ensurePositions();
    this._loading = false;
    this.requestUpdate();
    this._scheduleValidate();
  }

  _applyDef(def) {
    if (!def || !def.spec) return;
    const spec = def.spec;
    this._name = spec.name || '';
    this._description = spec.description || '';
    this._initial = spec.initial || '';
    this._determinism = spec.determinism || 'strict';
    this._states = Object.entries(spec.states || {}).map(([name, s]) => ({ name, terminal: !!(s && s.terminal) }));
    this._outputAlphabet = spec.output_alphabet || [];
    this._variables = spec.variables;
    this._linkedStates = spec.linked_states;
    this._gc = spec.gc;
    this._inputQueries = spec.input_queries;

    const rows = [];
    for (const t of spec.transitions || []) {
      let froms = [];
      try {
        const parsed = typeof t.from === 'string' ? JSON.parse(t.from) : t.from;
        froms = Array.isArray(parsed) ? parsed : [parsed];
      } catch {
        froms = [String(t.from || '')];
      }
      for (const from of froms) {
        rows.push({ from, input: t.input || '', to: t.to || '', output: t.output || '', guard: t.guard || '', set: t.set });
      }
    }
    this._transitions = rows;
  }

  // _ensurePositions gives every state without a saved layout entry a
  // position — a simple circle arrangement, not fsm-toolkit's own
  // Sugiyama layered layout (that algorithm is real graph-layout work
  // this first pass doesn't port). A person can drag states anywhere
  // afterward; this just avoids every unplaced state stacking at the
  // same coordinate on first open.
  _ensurePositions() {
    const missing = this._states.filter((s) => !this._layout.states[s.name]);
    if (missing.length === 0) return;
    const cx = 320, cy = 220, r = Math.max(120, 40 * missing.length);
    missing.forEach((s, i) => {
      const angle = (2 * Math.PI * i) / missing.length - Math.PI / 2;
      this._layout.states[s.name] = {
        x: Math.round(cx + r * Math.cos(angle)),
        y: Math.round(cy + r * Math.sin(angle)),
      };
    });
    this._layoutDirty = true;
  }

  _buildSpec() {
    const states = {};
    for (const s of this._states) states[s.name] = { terminal: !!s.terminal };
    const transitions = this._transitions.map((t) => ({
      from: t.from,
      input: t.input,
      to: t.to,
      output: t.output || undefined,
      guard: t.guard || undefined,
      set: t.set || undefined,
    }));
    const spec = {
      name: this._name,
      description: this._description || undefined,
      initial: this._initial,
      determinism: this._determinism,
      states,
      transitions,
      output_alphabet: this._outputAlphabet.length ? this._outputAlphabet : undefined,
    };
    if (this._variables) spec.variables = this._variables;
    if (this._linkedStates) spec.linked_states = this._linkedStates;
    if (this._gc) spec.gc = this._gc;
    if (this._inputQueries) spec.input_queries = this._inputQueries;
    return spec;
  }

  _scheduleValidate() {
    if (this._validateTimer) clearTimeout(this._validateTimer);
    this._validateTimer = setTimeout(() => this._runValidation(), 400);
  }

  async _runValidation() {
    if (!this._name || !this._initial) {
      this._validation = null;
      this.requestUpdate();
      return;
    }
    this._validating = true;
    this.requestUpdate();
    try {
      const resp = await fetch(this.validateUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(this._buildSpec()),
      });
      if (resp.ok) this._validation = await resp.json();
    } catch (e) {
      // Best-effort — local validation failing to run shouldn't block
      // editing; the person still has xolu's own save-time check.
    }
    this._validating = false;
    this.requestUpdate();
  }

  _addState() {
    const name = this._newStateName.trim();
    if (!name || this._states.some((s) => s.name === name)) return;
    this._states = [...this._states, { name, terminal: false }];
    if (!this._initial) this._initial = name;
    const angle = Math.random() * 2 * Math.PI;
    this._layout.states[name] = { x: Math.round(320 + 100 * Math.cos(angle)), y: Math.round(220 + 100 * Math.sin(angle)) };
    this._layoutDirty = true;
    this._newStateName = '';
    this._showAddState = false;
    this.requestUpdate();
    this._scheduleValidate();
  }

  _deleteState(name) {
    this._states = this._states.filter((s) => s.name !== name);
    this._transitions = this._transitions.filter((t) => t.from !== name && t.to !== name);
    delete this._layout.states[name];
    if (this._initial === name) this._initial = this._states[0]?.name || '';
    if (this._selectedState === name) this._selectedState = null;
    this._layoutDirty = true;
    this.requestUpdate();
    this._scheduleValidate();
  }

  _toggleTerminal(name) {
    this._states = this._states.map((s) => (s.name === name ? { ...s, terminal: !s.terminal } : s));
    this.requestUpdate();
  }

  _addTransition() {
    const t = this._newTransition;
    if (!t.from || !t.input || !t.to) return;
    this._transitions = [...this._transitions, { ...t }];
    if (t.output && !this._outputAlphabet.includes(t.output)) this._outputAlphabet = [...this._outputAlphabet, t.output];
    this._newTransition = { from: '', input: '', to: '', output: '', guard: '' };
    this._showAddTransition = false;
    this.requestUpdate();
    this._scheduleValidate();
  }

  _deleteTransition(index) {
    this._transitions = this._transitions.filter((_, i) => i !== index);
    this.requestUpdate();
    this._scheduleValidate();
  }

  // ─── Dragging ───────────────────────────────────────────────────────

  _onStatePointerDown(name, e) {
    e.preventDefault();
    this._selectedState = name;
    const svg = this.querySelector('#fsm-canvas');
    const pt = this._svgPoint(svg, e);
    const pos = this._layout.states[name] || { x: 0, y: 0 };
    this._drag = { name, offsetX: pt.x - pos.x, offsetY: pt.y - pos.y };
    this.requestUpdate();
  }

  _onCanvasPointerMove(e) {
    if (!this._drag) return;
    const svg = this.querySelector('#fsm-canvas');
    const pt = this._svgPoint(svg, e);
    this._layout.states[this._drag.name] = {
      x: Math.round(pt.x - this._drag.offsetX),
      y: Math.round(pt.y - this._drag.offsetY),
    };
    this._layoutDirty = true;
    this.requestUpdate();
  }

  _onCanvasPointerUp() {
    if (this._drag) this._saveLayoutSoon();
    this._drag = null;
  }

  _svgPoint(svg, e) {
    const rect = svg.getBoundingClientRect();
    return { x: e.clientX - rect.left, y: e.clientY - rect.top };
  }

  _saveLayoutSoon() {
    if (!this.idValue) return; // a brand-new, unsaved machine has nowhere to persist a layout to yet
    if (this._layoutSaveTimer) clearTimeout(this._layoutSaveTimer);
    this._layoutSaveTimer = setTimeout(async () => {
      try {
        await fetch(this.layoutUrl, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(this._layout),
        });
        this._layoutDirty = false;
        this.requestUpdate();
      } catch (e) {
        // Best-effort — a failed layout save just means positions
        // reset to the circle arrangement next load, not a lost edit
        // to the machine itself.
      }
    }, 600);
  }

  // ─── Save ───────────────────────────────────────────────────────────

  async _save() {
    if (this._saving) return;
    this._saving = true;
    this._error = null;
    this.requestUpdate();

    const spec = this._buildSpec();
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
          // A brand-new machine now has a real ID — persist the
          // layout that was only ever held client-side until now, and
          // switch the component over to "editing an existing
          // machine" for anything saved after this point.
          this.idValue = result.id;
          const base = this.saveUrl.replace(/\/$/, '');
          this.layoutUrl = base + '/' + result.id + '/layout';
          await fetch(this.layoutUrl, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(this._layout),
          });
          this._layoutDirty = false;
          window.history.replaceState(null, '', base + '/' + result.id);
        }
      }
    } catch (e) {
      this._error = String(e);
    }
    this._saving = false;
    this.requestUpdate();
  }

  // ─── Rendering ──────────────────────────────────────────────────────

  _stateRadius() {
    return 26;
  }

  _renderArrow(t, i) {
    const from = this._layout.states[t.from];
    const to = this._layout.states[t.to];
    if (!from || !to) return null;
    const r = this._stateRadius();
    const label = t.output ? `${t.input} / ${t.output}` : t.input;

    if (t.from === t.to) {
      // Self-loop: a small arc above the state.
      const cx = from.x, cy = from.y - r;
      return svg`
        <g>
          <path d=${`M ${cx - 14} ${cy} C ${cx - 14} ${cy - 30}, ${cx + 14} ${cy - 30}, ${cx + 14} ${cy}`}
            fill="none" stroke="currentColor" class="text-gray-400 dark:text-gray-500" stroke-width="1.5" marker-end="url(#fsm-arrow)" />
          <text x=${cx} y=${cy - 34} text-anchor="middle" class="fill-gray-600 dark:fill-gray-300" font-size="11">${label}</text>
          <title>${`transition ${i}`}</title>
        </g>
      `;
    }

    const dx = to.x - from.x, dy = to.y - from.y;
    const dist = Math.sqrt(dx * dx + dy * dy) || 1;
    const ux = dx / dist, uy = dy / dist;
    const x1 = from.x + ux * r, y1 = from.y + uy * r;
    const x2 = to.x - ux * r, y2 = to.y - uy * r;
    const mx = (x1 + x2) / 2, my = (y1 + y2) / 2;
    return svg`
      <g>
        <line x1=${x1} y1=${y1} x2=${x2} y2=${y2} stroke="currentColor" class="text-gray-400 dark:text-gray-500" stroke-width="1.5" marker-end="url(#fsm-arrow)" />
        <text x=${mx} y=${my - 6} text-anchor="middle" class="fill-gray-600 dark:fill-gray-300" font-size="11">${label}</text>
      </g>
    `;
  }

  render() {
    if (this._loading) return html`<div class="text-gray-500 dark:text-gray-400">Loading…</div>`;

    return html`
      <style>
        .fsm-field { display: block; margin-bottom: 0.75rem; }
        .fsm-field label { display: block; font-size: 0.75rem; color: #6b7280; margin-bottom: 0.25rem; }
        .dark .fsm-field label { color: #9ca3af; }
        .fsm-field input, .fsm-field select {
          width: 100%; padding: 0.4rem 0.6rem; border: 1px solid #d1d5db; border-radius: 0.375rem;
          font-size: 0.875rem; background: #fff; color: #111827;
        }
        .dark .fsm-field input, .dark .fsm-field select { border-color: #374151; background: #1f2937; color: #f3f4f6; }
        .fsm-layout { display: grid; grid-template-columns: 16rem 1fr; gap: 1.25rem; }
        .fsm-canvas-wrap {
          border: 1px solid #e5e7eb; border-radius: 0.5rem; background: #fafafa; overflow: hidden;
        }
        .dark .fsm-canvas-wrap { border-color: #374151; background: #111827; }
        .fsm-state-circle { fill: #eef2ff; stroke: #6366f1; stroke-width: 2; cursor: grab; }
        .dark .fsm-state-circle { fill: #312e81; stroke: #818cf8; }
        .fsm-state-circle.selected { stroke: #4f46e5; stroke-width: 3; }
        .fsm-state-circle.initial { stroke-dasharray: none; filter: drop-shadow(0 0 0 #4f46e5); }
        .fsm-state-label { font-size: 12px; text-anchor: middle; dominant-baseline: middle; fill: #1f2937; pointer-events: none; }
        .dark .fsm-state-label { fill: #e5e7eb; }
        .fsm-btn {
          padding: 0.4rem 0.9rem; font-size: 0.8125rem; font-weight: 500; border-radius: 0.375rem;
          border: 1px solid #d1d5db; background: #fff; color: #374151; cursor: pointer;
        }
        .dark .fsm-btn { border-color: #4b5563; background: #1f2937; color: #e5e7eb; }
        .fsm-btn-primary { background: #4f46e5; color: #fff; border-color: #4f46e5; }
        .fsm-btn-primary:disabled { background: #a5b4fc; border-color: #a5b4fc; cursor: default; }
        .fsm-panel {
          margin-top: 1rem; padding: 0.75rem 1rem; border-radius: 0.5rem; font-size: 0.8125rem;
        }
        .fsm-panel-error { background: #fef2f2; border: 1px solid #fecaca; color: #991b1b; }
        .dark .fsm-panel-error { background: rgba(127,29,29,0.3); border-color: rgba(248,113,113,0.3); color: #fca5a5; }
        .fsm-panel-warn { background: #fffbeb; border: 1px solid #fde68a; color: #92400e; }
        .dark .fsm-panel-warn { background: rgba(120,53,15,0.3); border-color: rgba(252,211,77,0.3); color: #fcd34d; }
        .fsm-panel-ok { background: #f0fdf4; border: 1px solid #bbf7d0; color: #166534; }
        .dark .fsm-panel-ok { background: rgba(20,83,45,0.3); border-color: rgba(74,222,128,0.3); color: #86efac; }
        .fsm-transition-row { display: flex; align-items: center; gap: 0.5rem; padding: 0.35rem 0; font-size: 0.8125rem; font-family: monospace; }
        .fsm-transition-row button { color: #dc2626; background: none; border: none; cursor: pointer; }
      </style>

      <div class="flex items-center justify-between mb-4">
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
          ${this.idValue ? `Edit machine #${this.idValue}` : 'New machine'}
        </h1>
        <button class="fsm-btn fsm-btn-primary" ?disabled=${this._saving} @click=${this._save}>
          ${this._saving ? 'Saving…' : 'Save'}
        </button>
      </div>

      ${this._error ? html`<div class="fsm-panel fsm-panel-error">${this._error}</div>` : ''}

      <div class="fsm-layout">
        <div>
          <div class="fsm-field">
            <label>Name</label>
            <input type="text" .value=${this._name} @input=${(e) => { this._name = e.target.value; this._scheduleValidate(); }} />
          </div>
          <div class="fsm-field">
            <label>Description</label>
            <input type="text" .value=${this._description} @input=${(e) => (this._description = e.target.value)} />
          </div>
          <div class="fsm-field">
            <label>Initial state</label>
            <select .value=${this._initial} @change=${(e) => { this._initial = e.target.value; this._scheduleValidate(); }}>
              <option value="">—</option>
              ${this._states.map((s) => html`<option value=${s.name} ?selected=${s.name === this._initial}>${s.name}</option>`)}
            </select>
          </div>
          <div class="fsm-field">
            <label>Determinism</label>
            <select .value=${this._determinism} @change=${(e) => (this._determinism = e.target.value)}>
              <option value="strict">strict</option>
              <option value="loose">loose</option>
              <option value="firstmatch">firstmatch</option>
            </select>
          </div>

          <div class="fsm-field">
            <label>States (${this._states.length})</label>
            ${this._states.map(
              (s) => html`
                <div class="flex items-center gap-1.5 py-1 text-sm ${s.name === this._selectedState ? 'font-semibold' : ''}">
                  <span class="flex-1 truncate cursor-pointer" @click=${() => (this._selectedState = s.name)}>
                    ${s.name}${s.name === this._initial ? ' ★' : ''}${s.terminal ? ' ⏹' : ''}
                  </span>
                  <button class="text-xs text-gray-500 dark:text-gray-400" title="Toggle terminal" @click=${() => this._toggleTerminal(s.name)}>⏹</button>
                  <button class="text-xs text-red-600 dark:text-red-400" title="Delete state" @click=${() => this._deleteState(s.name)}>×</button>
                </div>
              `
            )}
            ${this._showAddState
              ? html`
                  <div class="flex gap-1 mt-1">
                    <input type="text" placeholder="state name" .value=${this._newStateName}
                      @input=${(e) => (this._newStateName = e.target.value)}
                      @keydown=${(e) => e.key === 'Enter' && this._addState()} />
                    <button class="fsm-btn" @click=${this._addState}>Add</button>
                  </div>
                `
              : html`<button class="fsm-btn mt-1" @click=${() => (this._showAddState = true)}>+ Add state</button>`}
          </div>

          <div class="fsm-field">
            <label>Transitions (${this._transitions.length})</label>
            ${this._transitions.map(
              (t, i) => html`
                <div class="fsm-transition-row">
                  <span class="flex-1 truncate">${t.from} —${t.input}${t.output ? `/${t.output}` : ''}→ ${t.to}</span>
                  <button @click=${() => this._deleteTransition(i)}>×</button>
                </div>
              `
            )}
            ${this._showAddTransition
              ? html`
                  <div class="mt-1 flex flex-col gap-1">
                    <select .value=${this._newTransition.from} @change=${(e) => (this._newTransition = { ...this._newTransition, from: e.target.value })}>
                      <option value="">from…</option>
                      ${this._states.map((s) => html`<option value=${s.name}>${s.name}</option>`)}
                    </select>
                    <input type="text" placeholder="input" .value=${this._newTransition.input}
                      @input=${(e) => (this._newTransition = { ...this._newTransition, input: e.target.value })} />
                    <select .value=${this._newTransition.to} @change=${(e) => (this._newTransition = { ...this._newTransition, to: e.target.value })}>
                      <option value="">to…</option>
                      ${this._states.map((s) => html`<option value=${s.name}>${s.name}</option>`)}
                    </select>
                    <input type="text" placeholder="output (optional)" .value=${this._newTransition.output}
                      @input=${(e) => (this._newTransition = { ...this._newTransition, output: e.target.value })} />
                    <input type="text" placeholder="guard (optional)" .value=${this._newTransition.guard}
                      @input=${(e) => (this._newTransition = { ...this._newTransition, guard: e.target.value })} />
                    <button class="fsm-btn" @click=${this._addTransition}>Add transition</button>
                  </div>
                `
              : html`<button class="fsm-btn mt-1" @click=${() => (this._showAddTransition = true)}>+ Add transition</button>`}
          </div>
        </div>

        <div>
          <div class="fsm-canvas-wrap">
            <svg id="fsm-canvas" viewBox="0 0 640 440" style="width:100%; height:440px;"
              @pointermove=${this._onCanvasPointerMove} @pointerup=${this._onCanvasPointerUp} @pointerleave=${this._onCanvasPointerUp}>
              <defs>
                <marker id="fsm-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                  <path d="M 0 0 L 10 5 L 0 10 z" class="fill-gray-400 dark:fill-gray-500" />
                </marker>
              </defs>
              ${this._transitions.map((t, i) => this._renderArrow(t, i))}
              ${this._states.map((s) => {
                const pos = this._layout.states[s.name] || { x: 320, y: 220 };
                return svg`
                  <g @pointerdown=${(e) => this._onStatePointerDown(s.name, e)}>
                    <circle cx=${pos.x} cy=${pos.y} r=${this._stateRadius()}
                      class="fsm-state-circle ${s.name === this._selectedState ? 'selected' : ''}" />
                    ${s.terminal ? svg`<circle cx=${pos.x} cy=${pos.y} r=${this._stateRadius() - 5} fill="none" stroke="currentColor" class="text-indigo-600 dark:text-indigo-400" stroke-width="1.5" />` : ''}
                    ${s.name === this._initial ? svg`<path d=${`M ${pos.x - 48} ${pos.y} L ${pos.x - this._stateRadius() - 2} ${pos.y}`} stroke="currentColor" class="text-indigo-600 dark:text-indigo-400" stroke-width="1.5" marker-end="url(#fsm-arrow)" />` : ''}
                    <text x=${pos.x} y=${pos.y} class="fsm-state-label">${s.name}</text>
                  </g>
                `;
              })}
            </svg>
          </div>

          ${this._validating ? html`<div class="text-xs text-gray-400 dark:text-gray-500 mt-1">Checking…</div>` : ''}
          ${this._validation
            ? html`
                <div class="fsm-panel ${this._validation.valid ? 'fsm-panel-ok' : 'fsm-panel-error'}">
                  <strong>${this._validation.valid ? 'Structurally valid' : 'Invalid'}</strong>
                  ${(this._validation.errors || []).map((e) => html`<div>${e}</div>`)}
                </div>
                ${(this._validation.warnings || []).length
                  ? html`<div class="fsm-panel fsm-panel-warn">${this._validation.warnings.map((w) => html`<div>${w}</div>`)}</div>`
                  : ''}
                ${(this._validation.skippedChecks || []).length
                  ? html`<div class="text-xs text-gray-400 dark:text-gray-500 mt-1">
                      Not checked locally: ${this._validation.skippedChecks.join('; ')}
                    </div>`
                  : ''}
              `
            : ''}
        </div>
      </div>
    `;
  }
}

customElements.define('xolu-fsm-editor', XoluFsmEditor);
