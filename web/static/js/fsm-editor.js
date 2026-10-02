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
// isNodeSystemField and isRefValue together decide which of a
// clicked node's fields the inspect panel offers for editing. System
// fields (identity, not data) are always read-only. REF-typed fields
// are also excluded here deliberately — retargeting a relationship
// is a structural change to the graph itself, handled by the edge
// form instead, not a side effect of editing a form field that
// happens to hold a REF object.
function isNodeSystemField(key) {
  return key === '_id' || key === '_version' || key === 'id' || key === 'type';
}
function isRefValue(v) {
  return v !== null && typeof v === 'object' && v.type === 'REF';
}

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
    graphEntityTypesUrl: { attribute: 'graph-entity-types-url' },
    entitiesBaseUrl: { attribute: 'entities-base-url' },
    // The FSM's own name/description fields — found missing while
    // auditing every component in this file against the same
    // undeclared-reactive-property bug found in query-editor.js and
    // dxp-editor.js. Being controlled inputs (.value=${this._name}),
    // a render triggered by an unrelated reactive property (e.g.
    // _validation updating mid-typing) could otherwise reset the
    // visible field to a stale value even though the underlying
    // assignment itself was never broken.
    _name: { state: true },
    _description: { state: true },
    _determinism: { state: true },
    _graphEntityType: { state: true },
    _graphDepth: { state: true },
    _graphEntityTypeOptions: { state: true },
    _dark: { state: true },
    _maximized: { state: true },
    _saving: { state: true },
    _error: { state: true },
    _validation: { state: true },
    _inspecting: { state: true },
    _inspectEdits: { state: true },
    _inspectSaving: { state: true },
    _inspectError: { state: true },
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
    this._maximized = false;
    this.mode = 'fsm';
    this._graphEntityType = '';
    this._graphDepth = 1;
    this._graphEntityTypeOptions = [];
    this._graphLoading = false;
    this._inspecting = null; // {kind: 'node'|'edge', label, data}
    this._inspectEdits = {};
    this._inspectSaving = false;
    this._inspectError = null;
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

    // Intercept the halo gesture specifically for graph mode. The
    // engine's own halo click (canvas.onmousedown, just set up by
    // initFSM above) has no mode awareness at all, and its default
    // behavior is actively broken here, confirmed directly: left-
    // click-halo silently created a fake, unpersisted Link between
    // two real graph entities; right-click-halo (add-linked-node)
    // created a brand-new Node with no _graphNode at all. Right-click-
    // halo is repurposed as the fast path for the same expand/
    // collapse the inspect panel's own "Expand" button already
    // offers — a toggle: collapsed -> expand, anything else ->
    // collapse. Left-click-halo is suppressed entirely in graph mode:
    // there's no xoluman feature yet for "create a new, real REF
    // relationship between two entities" for it to mean.
    const origMouseDown = canvas.onmousedown;
    canvas.onmousedown = (e) => {
      if (this.mode === 'graph' && typeof _haloNode !== 'undefined' && _haloNode != null && _haloVisible && !e.shiftKey) {
        const target = _haloNode;
        if (e.button === 2) {
          _haloNode = null;
          _haloVisible = false;
          _haloOpacity = 0;
          _haloTarget = 0;
          if (target._graphNode && target._graphNode.collapsed) {
            this._expandNode(target);
          } else {
            this._collapseNode(target);
          }
          if (typeof draw === 'function') draw();
          return false;
        }
        // Left-click-halo: starts a real edge-creation drag, but only
        // from a node whose own REF fields are actually known — a
        // collapsed node's data is just {id}, nothing to offer as a
        // drag candidate, and a draft node has no REF fields at all
        // (no schema assigned yet). Otherwise let the engine's own
        // halo-click logic run as normal (starts the same
        // TemporaryLink drag visual FSM mode already uses) — only the
        // completion, in the onmouseup wrap below, differs for graph
        // mode: no fake local Link ever gets pushed to links[].
        if (!target._graphNode || target._graphNode.collapsed) {
          _haloNode = null;
          _haloVisible = false;
          _haloOpacity = 0;
          _haloTarget = 0;
          if (typeof draw === 'function') draw();
          return false;
        }
        this._graphEdgeDragOrigin = target;
        return origMouseDown.call(canvas, e);
      }
      return origMouseDown.call(canvas, e);
    };

    // Double-click on empty canvas already creates a brand-new,
    // blank Node — genuinely useful behavior as-is (see
    // ondblclick's own logic), just needs graph mode's own follow-up:
    // mark it as a local-only draft (not yet a real xolu entity,
    // nothing to persist yet — see the "vode" invariant this whole
    // design is built around: a REF can never point at something
    // that doesn't exist yet, so this node can't participate in any
    // edge until it's actually created) and colour it invalid, since
    // a node with no entity type assigned cannot be persisted at all
    // yet either.
    const origDblClick = canvas.ondblclick;
    canvas.ondblclick = (e) => {
      const nodesBefore = window.nodes.length;
      origDblClick.call(canvas, e);
      if (this.mode === 'graph' && window.nodes.length > nodesBefore) {
        const newNode = window.nodes[window.nodes.length - 1];
        newNode._graphDraft = true;
        const id = ensureNodeId(newNode);
        nodeColors.set(id, seamStateColors.invalid);
        if (typeof draw === 'function') draw();
      }
    };

    // Right-click, away from a halo (the halo's own right-click is
    // already handled above): in FSM mode, the engine's own
    // "Transition Properties" dialog (guard/action/output) — genuinely
    // fine there, that's what it's for. In graph mode the same
    // gesture instead does whatever's appropriate for xolu's own
    // relationship model: on a draft node, starts entity-type
    // assignment; on an existing edge, offers to clear the
    // relationship (the edge's own REF field set to null — see
    // _clearGraphEdge's own doc comment). On an existing, real node
    // there's not yet a graph-mode-appropriate menu to show (a real
    // next step, not this turn's own work) — for now, do nothing
    // rather than show the FSM-specific dialog, which was confirmed
    // directly this session to have no meaning here at all: right-
    // clicking a graph edge opened a form asking for values that
    // don't correspond to anything in xolu's own relationship model.
    const origContextMenu = canvas.oncontextmenu;
    canvas.oncontextmenu = (e) => {
      e.preventDefault();
      if (this.mode !== 'graph') {
        return origContextMenu.call(canvas, e);
      }
      const mouse = crossBrowserRelativeMousePos(e);
      const target = selectObject(mouse.x, mouse.y);
      if (target instanceof Node && target._graphDraft) {
        this._assignEntityType(target);
      } else if (target != null && !(target instanceof Node) && target._graphEdge) {
        this._clearGraphEdge(target);
      }
      return false;
    };

    // Completes a graph-mode edge-creation drag started at the
    // onmousedown wrap above. Graph mode never lets the engine's own
    // default completion run (push a fake, local-only Link to
    // links[]) — see _completeGraphEdgeDrag's own doc comment for the
    // real, server-persisted alternative. A drag that ends as a
    // TemporaryLink (released on empty space, not on any node) still
    // falls through to the original handler unchanged, since its own
    // snap-back animation already does exactly the right thing.
    const origMouseUp = canvas.onmouseup;
    canvas.onmouseup = (e) => {
      const dragOrigin = this._graphEdgeDragOrigin;
      this._graphEdgeDragOrigin = null;
      if (this.mode === 'graph' && dragOrigin && currentLink instanceof Link) {
        const dropTarget = currentLink.nodeB;
        currentLink = null;
        if (typeof draw === 'function') draw();
        this._completeGraphEdgeDrag(dragOrigin, dropTarget);
        return false;
      }
      return origMouseUp.call(canvas, e);
    };

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
      await this._loadGraphEntityTypes();
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

  // Entering or exiting full screen changes the canvas-wrap's own
  // size significantly (the existing ResizeObserver already resizes
  // the canvas buffer to match — see _resize above), but never re-fits
  // the view on its own, which is the right default for an ordinary
  // window resize (preserve whatever the person was looking at) but
  // the wrong one here: the whole point of full screen is to see more
  // of the graph, not more empty canvas at the same zoom level.
  // updateComplete + a frame wait ensures the CSS change (and the
  // ResizeObserver firing from it) has actually happened before
  // zoomToFit runs, not just that _maximized itself has flipped.
  async _toggleMaximized() {
    this._maximized = !this._maximized;
    await this.updateComplete;
    requestAnimationFrame(() => {
      if (typeof zoomToFit === 'function') zoomToFit();
    });
  }

  // _exportBaseUrl derives the export routes' own base
  // ("/connections/{name}/fsm/{id}/export") from layoutUrl
  // ("/connections/{name}/fsm/{id}/layout") — the same
  // string-derivation convention this codebase already uses
  // elsewhere (query-editor.js's own entitiesUrlFor, derived from
  // runUrl) rather than a fourth server-passed attribute for what's
  // already fully determined by the one that exists.
  get _exportBaseUrl() {
    return this.layoutUrl.replace(/\/layout$/, '/export');
  }

  // _checkSelectionChange drives graph mode's click-to-inspect panel.
  // The engine has no "selection changed" event — this runs on every
  // draw() (see firstUpdated's own wrapping), comparing against the
  // last-seen selectedObject, so it only actually updates state (and
  // triggers a Lit re-render) when the selection genuinely changed,
  // not on every single redraw.
  _checkSelectionChange() {
    if (this.mode !== 'graph') return;
    // The halo-click drag (onmousedown wrap) sets selectedObject to
    // the drag's own origin node as a side effect of how the engine's
    // own halo-click logic works, not because anything was actually
    // clicked-to-select. Reacting to that here would open the inspect
    // panel mid-drag, resizing the canvas (confirmed directly: ~130px
    // shorter once the panel appears) and pulling the still-held
    // mouse position outside the new, smaller bounds — which fires a
    // spurious mouseleave that snaps the drag back before it can ever
    // complete. Suppressed for the drag's own duration; the panel
    // reacts normally to a real click again once it ends.
    if (this._graphEdgeDragOrigin) return;
    const sel = typeof selectedObject !== 'undefined' ? selectedObject : null;
    if (sel === this._lastSelectedObject) return;
    this._lastSelectedObject = sel;
    this._inspectError = null;
    this._inspectSaving = false;
    if (!sel) {
      this._inspecting = null;
    } else if (sel._graphNode && sel._graphNode.collapsed) {
      // A collapsed node's own data is just {id} -- nothing worth
      // showing as editable fields. A distinct kind so render() shows
      // an explicit "Expand" action instead of the normal edit form.
      this._inspecting = { kind: 'collapsed-node', type: sel._graphNode.type, label: sel.text, data: sel._graphNode.data };
      this._inspectEdits = {};
    } else if (sel._graphNode) {
      // type stored separately from data — data is the entity's own
      // domain document (no "type" field of its own in general; the
      // real, confirmed bug this replaced sent {id, changes} with no
      // type at all, which the server correctly rejects with "type
      // and a positive id are required", found by testing the actual
      // save flow end to end, not assumed from reading the code).
      this._inspecting = { kind: 'node', type: sel._graphNode.type, label: sel.text, data: sel._graphNode.data };
      // A fresh copy of the editable fields' current values -- kept
      // separate from the node's own real data so typing into the
      // form doesn't mutate anything until Save actually succeeds.
      this._inspectEdits = {};
      for (const [k, v] of Object.entries(this._inspecting.data)) {
        if (!isNodeSystemField(k) && !isRefValue(v)) this._inspectEdits[k] = v;
      }
    } else if (sel._graphEdge) {
      this._inspecting = { kind: 'edge', label: sel._graphEdge.rel, data: sel._graphEdge };
      const [, targetID] = sel._graphEdge.to.split(':');
      this._inspectEdits = { targetID };
    } else {
      this._inspecting = null;
    }
  }

  async _saveInspectedNode() {
    if (!this._inspecting || this._inspecting.kind !== 'node') return;
    const node = this._lastSelectedObject;
    const original = this._inspecting.data;
    const changes = {};
    for (const [k, v] of Object.entries(this._inspectEdits)) {
      if (String(original[k] ?? '') !== String(v)) changes[k] = v;
    }
    if (Object.keys(changes).length === 0) return;

    this._inspectSaving = true;
    this._inspectError = null;
    try {
      const resp = await fetch(this.graphUrl + '/node', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: this._inspecting.type, id: original.id, changes }),
      });
      if (!resp.ok) {
        this._inspectError = await resp.text();
        this._inspectSaving = false;
        return;
      }
      // The graph structure itself didn't change -- update the node's
      // own data and label locally rather than re-running the whole
      // query, which would also reset pan/zoom/layout for no reason.
      Object.assign(original, changes);
      node.text = this._labelForNode({ type: this._inspecting.type, data: original });
      this._inspecting = { ...this._inspecting, label: node.text, data: original };
      if (typeof draw === 'function') draw();
    } catch (e) {
      this._inspectError = String(e);
    }
    this._inspectSaving = false;
  }

  async _saveInspectedEdge() {
    if (!this._inspecting || this._inspecting.kind !== 'edge') return;
    const edge = this._inspecting.data;
    const [targetType] = edge.to.split(':');
    const newTo = `${targetType}:${this._inspectEdits.targetID}`;
    if (newTo === edge.to) return;

    this._inspectSaving = true;
    this._inspectError = null;
    try {
      const resp = await fetch(this.graphUrl + '/edge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ from: edge.from, relField: edge.rel, newTo }),
      });
      if (!resp.ok) {
        this._inspectError = await resp.text();
        this._inspectSaving = false;
        return;
      }
      // Retargeting genuinely changes the graph's own structure (the
      // edge may now point at a node not even in the currently-loaded
      // set) -- re-running the query is the honest way to reflect
      // that, not an attempt to surgically patch the canvas in place.
      this._inspecting = null;
      await this._runGraphQuery();
    } catch (e) {
      this._inspectError = String(e);
      this._inspectSaving = false;
    }
  }

  // _clearGraphEdge is the right-click-on-an-edge answer for graph
  // mode — replaces the FSM-specific "Transition Properties" dialog
  // (guard/action/output — confirmed directly this session to have no
  // meaning against xolu's own relationship model, since a REF is a
  // plain field value, not a first-class object with its own
  // properties). The one meaningful edge-level operation this model
  // actually supports beyond retargeting (already in the left-click
  // inspect panel) is clearing the relationship entirely — setting
  // the REF field back to empty, confirmed directly against a real
  // xolu instance to genuinely remove the field, not just store a
  // literal null (SaveGraphEdge's own doc comment has the details).
  // A plain confirm() rather than a full dialog: this is a single,
  // named, destructive yes/no action, not a form.
  async _clearGraphEdge(target) {
    const edge = target._graphEdge;
    if (!edge) return;
    const confirmed = await showFSMDialog({
      title: `Clear "${edge.rel}"?`,
      fields: [],
      saveLabel: 'Clear relationship',
    });
    if (confirmed === null) return; // cancelled

    try {
      const resp = await fetch(this.graphUrl + '/edge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ from: edge.from, relField: edge.rel, newTo: '' }),
      });
      if (!resp.ok) return; // best-effort for a first version -- the edge stays as-is, no error surfaced yet
      // Same reasoning as _saveInspectedEdge's own retarget case:
      // clearing genuinely changes the graph's own structure, so a
      // full re-query is the honest way to reflect it.
      await this._runGraphQuery();
    } catch (e) {
      // best-effort
    }
  }

  // _completeGraphEdgeDrag is the real, server-persisted alternative
  // to the engine's own default drag-completion (push a fake, local-
  // only Link to links[] — meaningless here, since a graph edge is a
  // REF field on a real xolu entity, not an arbitrary drawable line).
  // Applies this session's own agreed rule: for a schema-ful origin
  // (RefFieldsForType's own hasSchema), only an empty field whose
  // known target matches the drop's own entity type is valid — the
  // first such field, if more than one qualifies (this session's own
  // agreed disambiguation). For a genuinely schema-less origin, any
  // on-canvas target is valid regardless of type, but the field name
  // isn't predetermined by anything, so it's asked for directly.
  // Silently rejects (no dialog, no error) when the origin has no
  // valid field for this particular drop — the drag simply produces
  // nothing, which reads as "that connection isn't possible" without
  // needing to explain the schema's own constraints in a popup.
  async _completeGraphEdgeDrag(origin, target) {
    if (!origin._graphNode || !target._graphNode || target._graphNode.collapsed) return;
    const originType = origin._graphNode.type;
    const originData = origin._graphNode.data;
    const targetType = target._graphNode.type;

    let refFieldsResult;
    try {
      const resp = await fetch(`${this.graphUrl}/ref-fields/${encodeURIComponent(originType)}`);
      refFieldsResult = await resp.json();
    } catch (e) {
      return;
    }

    let relField;
    if (refFieldsResult.hasSchema) {
      const candidate = (refFieldsResult.fields || []).find((f) =>
        f.target === targetType && (originData[f.name] === undefined || originData[f.name] === null)
      );
      if (!candidate) return; // no empty, type-matching field on this origin -- nothing valid to drop here
      relField = candidate.name;
    } else {
      const result = await showFSMDialog({
        title: `New relationship on ${originType}#${originData.id}`,
        fields: [{ key: 'fieldName', label: 'Field name', value: '', placeholder: 'e.g. related_to' }],
        saveLabel: 'Create relationship',
      });
      if (!result || !result.fieldName) return; // cancelled
      relField = result.fieldName;
    }

    try {
      const resp = await fetch(this.graphUrl + '/edge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ from: origin._graphNode.id, relField, newTo: target._graphNode.id }),
      });
      if (!resp.ok) return;
      await this._runGraphQuery();
    } catch (e) {
      // best-effort
    }
  }

  // _expandCollapsedNode is the inspect panel's own "Expand" button
  // handler -- thin wrapper over _expandNode using whatever's
  // currently selected, since the button only ever appears once a
  // collapsed node has already been clicked once. See _expandNode's
  // own doc comment for the actual logic.
  async _expandCollapsedNode() {
    if (!this._inspecting || this._inspecting.kind !== 'collapsed-node') return;
    await this._expandNode(this._lastSelectedObject);
  }

  // _expandNode fetches a node's real data plus one hop of its own
  // REFs (see graphrest.go's Expand and its own doc comment), then
  // merges the result into the current canvas -- deliberately not a
  // full re-query, which would reset pan/zoom/layout and everyone's
  // existing position for no reason. Every node already present (by
  // its own "type:id" identity, not object identity) is left alone
  // unless it was itself only a collapsed stub, in which case it's
  // upgraded in place rather than duplicated; every edge already
  // present (by from/rel/to) is left alone too. New nodes are placed
  // in a small ring around the node that was expanded -- a reasonable
  // default position for something that had no layout of its own
  // before this moment, not an attempt at a globally consistent
  // layout. Called both from the inspect panel's own "Expand" button
  // (via _expandCollapsedNode) and directly from a right-click-halo
  // gesture (_checkHaloGraphAction), which doesn't require a node to
  // already be selected/inspecting first.
  async _expandNode(origin) {
    if (!origin || !origin._graphNode) return;
    const { type, id } = origin._graphNode;

    this._inspectSaving = true;
    this._inspectError = null;
    try {
      const resp = await fetch(this.graphUrl + '/expand', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type, id: origin._graphNode.data.id, depth: 1 }),
      });
      const text = await resp.text();
      if (!resp.ok) {
        this._inspectError = text || `HTTP ${resp.status}`;
        this._inspectSaving = false;
        return;
      }
      const result = JSON.parse(text);
      this._mergeGraphData(result, origin);

      // The just-expanded node itself is always the first thing
      // walked server-side, so it's always present in result.nodes --
      // show its real data in the inspect panel now, rather than
      // leaving the "Expand" button showing for a node that no longer
      // needs it. Only touches the panel if this node is the one
      // currently selected -- a halo-triggered expand of some other,
      // unselected node shouldn't hijack whatever's currently shown.
      const expanded = (result.nodes || []).find((n) => n.id === id);
      if (expanded && this._lastSelectedObject === origin) {
        this._inspecting = { kind: 'node', type: expanded.type, label: origin.text, data: expanded.data };
        this._inspectEdits = {};
        for (const [k, v] of Object.entries(expanded.data)) {
          if (!isNodeSystemField(k) && !isRefValue(v)) this._inspectEdits[k] = v;
        }
      }
    } catch (e) {
      this._inspectError = String(e);
    }
    this._inspectSaving = false;
  }

  // _collapseNode is expand's own inverse — purely client-side, no
  // server call needed, since collapsing only hides data already on
  // the canvas rather than fetching anything new. Removes every
  // outgoing edge from this node (the ones expand itself would have
  // introduced), then removes any node left with no remaining edges
  // touching it at all -- a node someone else still points to, or
  // that still points somewhere else via a different edge, stays
  // (something else on the canvas still needs it visible). The node
  // itself is never removed, only turned back into a collapsed stub.
  _collapseNode(target) {
    if (!target || !target._graphNode || target._graphNode.collapsed) return;
    const targetID = target._graphNode.id;

    const removed = window.links.filter((l) => l._graphEdge && l._graphEdge.from === targetID);
    const removedSet = new Set(removed);
    window.links = window.links.filter((l) => !removedSet.has(l));

    const candidateTargets = new Set(removed.map((l) => l.nodeB));
    window.nodes = window.nodes.filter((n) => {
      if (!candidateTargets.has(n)) return true;
      const stillTouched = window.links.some((l) => l.nodeA === n || l.nodeB === n || (l.node === n));
      return stillTouched;
    });

    target._graphNode = { id: targetID, type: target._graphNode.type, collapsed: true, data: { id: target._graphNode.data.id } };
    target.isAcceptState = true;
    target.text = this._labelForNode(target._graphNode);

    if (this._lastSelectedObject === target) {
      this._inspecting = { kind: 'collapsed-node', type: target._graphNode.type, label: target.text, data: target._graphNode.data };
      this._inspectEdits = {};
    }
    if (typeof draw === 'function') draw();
  }

  // _assignEntityType is the full answer to right-clicking a draft
  // node (a blank, local-only Node from double-click on empty canvas
  // — see the ondblclick wrap in firstUpdated): pick a real entity
  // type, fill in and submit the real form for it, and on success
  // turn the local draft into a genuinely persisted xolu entity.
  //
  // Deliberately three separate steps, not one combined form:
  //   1. showFSMDialog with a select field — the entity type itself,
  //      picked from real, known-to-exist types (this._graphEntity-
  //      TypeOptions, already loaded on mount for the toolbar's own
  //      picker), not free text a person could mistype.
  //   2. Once a type is chosen, fetch the real, unmodified NewForm
  //      fragment for it (the same form the main entity list already
  //      uses, via the same HX-Request mechanism WriteModalAware
  //      already understands — no duplicate field-rendering logic
  //      anywhere) and show it in showHTMLDialog.
  //   3. On submit, POST the form's own field values to
  //      CreateGraphNode (not to the form's own action, which points
  //      at entities.go's Create and would redirect rather than
  //      return the new ID this needs) — see that handler's own doc
  //      comment for why it's a separate endpoint from Create at all.
  //
  // A node cannot be persisted with no type at all (structurally --
  // xolu's own REST routes have no create endpoint without an entity
  // type in the path), so this is the mandatory first step, not an
  // optional enhancement -- the node stays a red, invalid draft on
  // the canvas until it's been through this flow successfully.
  async _assignEntityType(node) {
    const typeResult = await showFSMDialog({
      title: 'Assign entity type',
      fields: [{
        key: 'entityType', label: 'Entity type', type: 'select',
        options: this._graphEntityTypeOptions, value: this._graphEntityTypeOptions[0] || '',
      }],
    });
    if (!typeResult || !typeResult.entityType) return; // cancelled -- node stays a draft
    const entityType = typeResult.entityType;

    let formHTML;
    try {
      const formResp = await fetch(`${this.entitiesBaseUrl}/${encodeURIComponent(entityType)}/new`, {
        headers: { 'HX-Request': 'true' },
      });
      formHTML = await formResp.text();
    } catch (e) {
      return; // best-effort -- node stays a draft, person can try again via the same right-click
    }
    if (!formHTML.includes('<form')) {
      // The known limitation this session found and named directly:
      // resolveEntityFields (shared with the main entity form) can't
      // build a form for a type with no schema and no existing rows
      // to infer fields from at all. Not reachable in practice here
      // since the type picker only ever offers types GraphEntityTypes
      // already knows have real data -- kept as a defensive check,
      // not expected to fire.
      return;
    }

    const formEl = await showHTMLDialog({ title: `New ${entityType}`, html: formHTML, saveLabel: 'Create' });
    if (!formEl) return; // cancelled -- node stays a draft

    const body = new URLSearchParams(new FormData(formEl));
    let resp;
    try {
      resp = await fetch(`${this.graphUrl}/create/${encodeURIComponent(entityType)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body.toString(),
      });
    } catch (e) {
      return; // best-effort -- node stays a draft
    }
    const result = await resp.json().catch(() => ({}));
    if (!resp.ok || !result.id) {
      // Best-effort for a first version -- the error itself isn't
      // shown anywhere yet (a real refinement, not this turn's own
      // scope); the node simply stays a draft, and right-clicking it
      // again starts the whole flow over.
      return;
    }

    // Success: fetch the real, server-computed document (correctly
    // structured REF fields, computed defaults, everything) rather
    // than approximating it from the raw strings just submitted --
    // reuses the existing Expand endpoint at depth 0, since that's
    // already exactly "fetch one real entity's own real data."
    const graphID = `${entityType}:${result.id}`;
    let realData = { id: result.id };
    try {
      const expandResp = await fetch(`${this.graphUrl}/expand`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: entityType, id: result.id, depth: 0 }),
      });
      const expandResult = await expandResp.json();
      const found = (expandResult.nodes || []).find((n) => n.id === graphID);
      if (found) realData = found.data;
    } catch (e) {
      // Best-effort -- the node still becomes real below, just with
      // minimal data until the next full query refreshes it.
    }

    node._graphNode = { id: graphID, type: entityType, data: realData };
    node._graphDraft = false;
    node.text = this._labelForNode(node._graphNode);
    nodeColors.delete(ensureNodeId(node));
    if (typeof draw === 'function') draw();
  }


  // live canvas engine's own global nodes[]/links[] arrays, in place --
  // see _expandCollapsedNode's own doc comment for why this isn't a
  // full re-query.
  _mergeGraphData(result, origin) {
    const byGraphId = {};
    window.nodes.forEach((n) => { if (n._graphNode) byGraphId[n._graphNode.id] = n; });

    const newNodes = (result.nodes || []);
    const placedCount = newNodes.length;
    newNodes.forEach((gn, i) => {
      const existing = byGraphId[gn.id];
      if (existing) {
        if (existing._graphNode.collapsed && !gn.collapsed) {
          // Upgrade in place: same node, real data has arrived.
          existing._graphNode = gn;
          existing.isAcceptState = false;
          existing.text = this._labelForNode(gn);
        }
        // Already present and already full -- nothing to do, and
        // definitely not a duplicate node for the same real entity.
        return;
      }
      const angle = (2 * Math.PI * i) / Math.max(placedCount, 1);
      const radius = 90;
      const node = new Node(origin.x + radius * Math.cos(angle), origin.y + radius * Math.sin(angle));
      node.text = this._labelForNode(gn);
      node.isAcceptState = !!gn.collapsed;
      node._graphNode = gn;
      window.nodes.push(node);
      byGraphId[gn.id] = node;
    });

    const existingEdgeKeys = new Set(
      window.links.filter((l) => l._graphEdge).map((l) => `${l._graphEdge.from}|${l._graphEdge.rel}|${l._graphEdge.to}`)
    );
    (result.edges || []).forEach((ge) => {
      const key = `${ge.from}|${ge.rel}|${ge.to}`;
      if (existingEdgeKeys.has(key)) return;
      const a = byGraphId[ge.from], b = byGraphId[ge.to];
      if (!a || !b) return; // both endpoints are always placed above first, but stay defensive
      const link = new Link(a, b);
      link.text = ge.rel;
      link._graphEdge = ge;
      window.links.push(link);
      existingEdgeKeys.add(key);
    });

    if (typeof draw === 'function') draw();
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

  async _loadGraphEntityTypes() {
    if (!this.graphEntityTypesUrl) return;
    try {
      const resp = await fetch(this.graphEntityTypesUrl);
      if (!resp.ok) return; // best-effort -- the picker just stays empty, not a page-level error
      this._graphEntityTypeOptions = await resp.json();
      if (!this._graphEntityType && this._graphEntityTypeOptions.length > 0) {
        this._graphEntityType = this._graphEntityTypeOptions[0];
      }
    } catch (e) {
      // Same best-effort reasoning -- an empty picker is a fine
      // degradation, not worth surfacing as this._error.
    }
  }

  async _runGraphQuery() {
    if (!this._graphEntityType || typeof clearCanvas !== 'function') return;
    this._graphLoading = true;
    this._error = null;
    this._inspecting = null;

    try {
      const resp = await fetch(this.graphUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ entityType: this._graphEntityType, depth: this._graphDepth }),
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
  // applyExternalGraphData is this component's own public entry point
  // for graph data that originated somewhere other than this
  // component's own query flow — specifically, the Sulpher tab living
  // beside the graph-walker tab on the same page (query.go's own
  // GraphView), whose own "View in graph" button dispatches an
  // xolu-view-in-graph event the page itself listens for and forwards
  // here. No different from any other graph render internally — same
  // _applyGraphData, same {nodes, edges} shape (Go's own
  // classifySulpherResult already produces exactly this contract) —
  // named and exposed without the underscore specifically to mark it
  // as the one method meant to be called from outside this component.
  applyExternalGraphData(data) {
    this._applyGraphData(data);
  }

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
          text: this._labelForNode(n), isAcceptState: !!n.collapsed, textOnly: false,
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

    const pageBg = dark ? '#0f172a' : '#ffffff';
    const maximizedStyle = this._maximized
      ? `position:fixed; inset:0; z-index:60; height:100vh; width:100vw; background:${pageBg}; padding:0.75rem; box-sizing:border-box;`
      : `height:640px; width:100%;`;

    return html`
      <style>
        xolu-fsm-editor { display:flex; flex-direction:column; ${maximizedStyle} }
        .fsm-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:0.75rem; }
        .fsm-fields { display:flex; gap:0.75rem; flex-wrap:wrap; padding:0.5rem 0 0.75rem; }
        .fsm-fields input, .fsm-fields select {
          padding:0.4rem 0.6rem; border:1px solid #d1d5db; border-radius:0.375rem; font-size:0.875rem;
          background:#fff; color:#111827;
        }
        .dark .fsm-fields input, .dark .fsm-fields select { border-color:#374151; background:#1f2937; color:#f3f4f6; }
        .fsm-toolbar { display:flex; align-items:center; gap:8px; padding:8px 12px; border-bottom:1px solid ${border}; background:${toolbarBg}; }
        .fsm-hint { flex:1; font-size:12px; color:${dark ? '#64748b' : '#94a3b8'}; }
        .fsm-canvas-wrap { flex:1; overflow:hidden; position:relative; border:1px solid ${border}; border-radius:0 0 0.5rem 0.5rem; }
        #fsm-canvas { display:block; width:100%; height:100%; cursor:crosshair; }
        .fsm-graph-body { display:flex; flex-direction:row; flex:1; overflow:hidden; gap:0.75rem; }
        .fsm-graph-body .fsm-canvas-wrap { border-radius:0 0 0 0.5rem; }
        .fsm-graph-sidebar { width:320px; flex-shrink:0; overflow-y:auto; }
        .fsm-graph-sidebar .fsm-panel { margin-top:0; }
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
        .fsm-inspect-hint { font-style:italic; color:${dark ? '#6b7280' : '#9ca3af'}; }
        .fsm-inspect-input {
          flex:1; min-width:0; padding:1px 6px; font-family:monospace; font-size:0.75rem;
          border:1px solid ${dark ? '#475569' : '#cbd5e1'}; border-radius:4px;
          background:${dark ? '#111827' : '#fff'}; color:${dark ? '#e5e7eb' : '#111827'};
        }
        .fsm-inspect-error {
          margin-top:6px; padding:4px 8px; border-radius:4px; font-size:0.75rem;
          background:#fef2f2; border:1px solid #fecaca; color:#991b1b;
        }
        .dark .fsm-inspect-error { background:rgba(127,29,29,0.3); border-color:rgba(248,113,113,0.3); color:#fca5a5; }
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
              <select class="fsm-graph-entity-type"
                .value=${this._graphEntityType}
                @change=${(e) => (this._graphEntityType = e.target.value)}>
                ${this._graphEntityTypeOptions.length === 0
                  ? html`<option value="">No entity types available</option>`
                  : this._graphEntityTypeOptions.map((t) => html`<option value=${t}>${t}</option>`)}
              </select>
              <label style="display:flex;align-items:center;gap:0.4rem;font-size:0.875rem;color:#6b7280">
                Depth
                <input type="number" class="fsm-graph-depth" min="0" max="4" style="width:4rem"
                  .value=${String(this._graphDepth)}
                  @input=${(e) => (this._graphDepth = parseInt(e.target.value, 10) || 0)} />
              </label>
              <button class="${btnStyle}" style="background:#4f46e5;color:#fff;border-color:#4f46e5"
                ?disabled=${this._graphLoading || !this._graphEntityType} @click=${this._runGraphQuery}>
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
        <button class="${btnStyle}" @click=${this._toggleMaximized}
          title=${this._maximized ? 'Exit full screen' : 'Full screen'}>
          ${this._maximized ? '⛶ Exit' : '⛶'}
        </button>
        ${!isGraph && this.idValue
          ? html`
              <a class="${btnStyle}" style="text-decoration:none;display:inline-flex;align-items:center"
                href="${this._exportBaseUrl}/svg" title="Export as SVG">SVG</a>
              <a class="${btnStyle}" style="text-decoration:none;display:inline-flex;align-items:center"
                href="${this._exportBaseUrl}/png" title="Export as PNG">PNG</a>
              <a class="${btnStyle}" style="text-decoration:none;display:inline-flex;align-items:center"
                href="${this._exportBaseUrl}/latex" title="Export as LaTeX/TikZ — uses the saved layout; save one first if this fails">TeX</a>
            `
          : ''}
      </div>

      ${isGraph
        ? html`
            <div class="fsm-graph-body">
              <div class="fsm-canvas-wrap">
                <canvas id="fsm-canvas"></canvas>
              </div>
              <div class="fsm-graph-sidebar">${this._renderInspectPanel()}</div>
            </div>
          `
        : html`
            <div class="fsm-canvas-wrap">
              <canvas id="fsm-canvas"></canvas>
            </div>
            ${this._renderValidationPanel()}
          `}
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
  // edges and see the data contained in the edges" — and, since this
  // turn's own ask, editable and saveable, not read-only. System
  // fields (_id/_version/id/type) and REF-typed fields stay display-
  // only here — REF fields specifically because retargeting a
  // relationship is what the edge form is for, not a side effect of
  // editing a node field that happens to hold a REF (see
  // isNodeSystemField/isRefValue's own comment).
  _renderInspectPanel() {
    if (!this._inspecting) {
      return html`<div class="text-xs text-gray-400 dark:text-gray-500 mt-2">Run a query, then click a node or edge to inspect it.</div>`;
    }
    const { kind, label, data } = this._inspecting;
    const btnStyle = `display:inline-flex;align-items:center;gap:4px;padding:4px 10px;font-size:12px;font-weight:500;border-radius:6px;border:1px solid #4f46e5;background:#4f46e5;color:#fff;cursor:pointer`;
    const titleForKind = { node: 'Node', edge: 'Edge', 'collapsed-node': 'Collapsed node' };

    return html`
      <div class="fsm-panel fsm-inspect">
        <div class="fsm-inspect-title">${titleForKind[kind]}: ${label}</div>

        ${kind === 'collapsed-node'
          ? html`
              <div class="text-xs text-gray-500 dark:text-gray-400 mb-2">
                Its own REF fields haven't been fetched yet.
              </div>
              ${this._inspectError ? html`<div class="fsm-inspect-error">${this._inspectError}</div>` : ''}
              <button class="${btnStyle}" ?disabled=${this._inspectSaving} @click=${this._expandCollapsedNode}>
                ${this._inspectSaving ? 'Expanding…' : 'Expand'}
              </button>
            `
          : html`
              ${kind === 'node' ? this._renderNodeForm(data) : this._renderEdgeForm(data)}

              ${this._inspectError ? html`<div class="fsm-inspect-error">${this._inspectError}</div>` : ''}

              <button class="${btnStyle}" style="margin-top:8px"
                ?disabled=${this._inspectSaving}
                @click=${() => (kind === 'node' ? this._saveInspectedNode() : this._saveInspectedEdge())}>
                ${this._inspectSaving ? 'Saving…' : 'Save'}
              </button>
            `}
      </div>
    `;
  }

  _renderNodeForm(data) {
    const entries = Object.entries(data || {});
    return entries.map(([k, v]) => {
      if (isNodeSystemField(k)) {
        return html`
          <div class="fsm-inspect-row">
            <span class="fsm-inspect-key">${k}</span>
            <span class="fsm-inspect-val">${String(v)}</span>
          </div>
        `;
      }
      if (isRefValue(v)) {
        return html`
          <div class="fsm-inspect-row">
            <span class="fsm-inspect-key">${k}</span>
            <span class="fsm-inspect-val">${v.entity}:${v.id} <em class="fsm-inspect-hint">(retarget via its edge, not here)</em></span>
          </div>
        `;
      }
      return html`
        <div class="fsm-inspect-row">
          <span class="fsm-inspect-key">${k}</span>
          <input class="fsm-inspect-input" type="text" .value=${String(this._inspectEdits[k] ?? '')}
            @input=${(e) => (this._inspectEdits = { ...this._inspectEdits, [k]: e.target.value })} />
        </div>
      `;
    });
  }

  _renderEdgeForm(edge) {
    const [targetType] = (edge.to || '').split(':');
    return html`
      <div class="fsm-inspect-row">
        <span class="fsm-inspect-key">from</span>
        <span class="fsm-inspect-val">${edge.from}</span>
      </div>
      <div class="fsm-inspect-row">
        <span class="fsm-inspect-key">rel</span>
        <span class="fsm-inspect-val">${edge.rel}</span>
      </div>
      <div class="fsm-inspect-row">
        <span class="fsm-inspect-key">to</span>
        <span class="fsm-inspect-val">${targetType}:</span>
        <input class="fsm-inspect-input" type="text" style="max-width:6rem" .value=${String(this._inspectEdits.targetID ?? '')}
          @input=${(e) => (this._inspectEdits = { ...this._inspectEdits, targetID: e.target.value })} />
      </div>
      <div class="text-xs text-gray-400 dark:text-gray-500 mt-1">
        Retargeting saves, then re-runs the current query — the graph's own shape changed.
      </div>
    `;
  }
}

customElements.define('xolu-fsm-editor', XoluFsmEditor);
