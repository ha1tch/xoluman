// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// query-editor.js — Lit shell around CodeMirror 6 for xoluman's three
// query modes (OQL, Sulpher, REST). Same architecture as
// grid-editor.js: a thin Lit toolbar/theming/lifecycle wrapper around a
// framework-agnostic engine.
//
// Language choice per mode: OQL uses @codemirror/lang-sql's MSSQL
// dialect (OQL is a T-SQL subset — anything valid OQL is valid T-SQL by
// construction). Sulpher uses @neo4j-cypher/codemirror's real ANTLR4
// Cypher grammar (Sulpher is near-exactly openCypher9). REST request
// bodies use @codemirror/lang-json. All three bundled together into one
// vendored file — see web/static/vendor/VENDOR.md's Asset 4.
//
// Data contract with the server (internal/ui/query.go):
//   POST {run-url} <- {mode, query, method?, path?, contentType?, body?}
//                   -> raw JSON result, shape depends on mode:
//                      oql/sulpher: xolu's own OQLResult/GraphQueryResult
//                      rest: {statusCode, body, headers}
//   This component does not reshape or interpret the result beyond
//   pretty-printing it — the three modes have genuinely different
//   natural result shapes, and inventing one unified display format
//   would lose information a raw JSON view doesn't.

import { LitElement, html } from 'lit';
import { EditorView, basicSetup, EditorState, Compartment, sql, MSSQL, json, cypherExtensions, oneDark } from 'codemirror-bundle';

const MODES = ['oql', 'sulpher', 'rest'];

class XoluQueryEditor extends LitElement {
  static properties = {
    runUrl: { attribute: 'run-url' },
    // modes restricts which of the three query modes this instance
    // shows tabs for — a comma-separated subset of MODES, e.g.
    // "oql,rest" on the query view (query.go's own /query page) or
    // "sulpher" alone on the graph view (query.go's own GraphView),
    // since the graph view's own Sulpher tab has no use for OQL/REST
    // at all. Unset (the default) means all three, matching this
    // component's own original, single-page behavior.
    modes: {},
    // Every one of these drives something in render() — without a
    // declared reactive property, Lit has no way to know any of them
    // changed, and a plain `this._x = y` assignment silently does
    // nothing visible at all. Confirmed the hard way: clicking "Save…"
    // set _showSaveForm to true correctly (the assignment itself
    // works fine), but nothing on screen ever changed, because Lit
    // never re-rendered — the actual, root cause of "saved queries
    // don't work," found by testing the real UI click flow directly
    // rather than only the server-side save endpoint in isolation.
    _mode: { state: true },
    _running: { state: true },
    _result: { state: true },
    _resultParsed: { state: true },
    _resultViewMode: { state: true },
    _error: { state: true },
    _recentByMode: { state: true },
    _savedByMode: { state: true },
    _showSaveForm: { state: true },
    _saveNameDraft: { state: true },
    _selectedSavedId: { state: true },
    _restMethod: { state: true },
    _restPath: { state: true },
  };

  // Light DOM — CodeMirror renders real DOM nodes into whatever
  // container it's given; a shadow root adds nothing here and only
  // risks CSS isolation surprises, same reasoning as grid-editor.js.
  createRenderRoot() {
    return this;
  }

  // _activeModes is the subset of MODES this instance actually shows
  // tabs for — see the modes property's own doc comment. Falls back
  // to every mode when the attribute is unset, matching this
  // component's original, single-page behavior before it was reused
  // on the graph view too.
  get _activeModes() {
    if (!this.modes) return MODES;
    const requested = this.modes.split(',').map((m) => m.trim()).filter(Boolean);
    const active = MODES.filter((m) => requested.includes(m));
    return active.length > 0 ? active : MODES;
  }

  constructor() {
    super();
    this._mode = 'oql';
    this._running = false;
    this._result = null;
    this._resultParsed = null;
    this._resultViewMode = 'raw';
    this._error = null;
    this._languageConf = new Compartment();
    // Same live-reactivity reasoning theme.js's toggle button needs
    // elsewhere is needed here too, but for a different reason: the
    // query editor is a persistent widget someone might have open
    // across a theme toggle, unlike the modal (open-then-closed,
    // check-once-at-build-time is correct for that). A
    // MutationObserver on <html>'s class attribute (set up in
    // firstUpdated, torn down in disconnectedCallback) keeps this
    // Compartment's contents in sync live, not just at creation.
    this._themeConf = new Compartment();
    // Each mode keeps its own document — a real bug before this fix:
    // switching modes only reconfigured the language extension
    // (this._languageConf.reconfigure(...)), never the document
    // itself, so all three modes shared one CodeMirror document.
    // Editing in Sulpher, then switching back to OQL, showed the
    // Sulpher text under OQL's language — confirmed and fixed here by
    // saving the outgoing mode's text before switching, and doing a
    // full EditorView.setState() with the incoming mode's own saved
    // text on switch, rather than reconfiguring in place.
    this._docs = { oql: '', sulpher: '', rest: '' };
    // REST-mode-only fields, kept outside CodeMirror since they're
    // plain form inputs, not editable code.
    this._restMethod = 'GET';
    this._restPath = '';
    // Recent queries: local, ephemeral, per browser — the last N run
    // per mode, most recent first, kept in localStorage (safe here,
    // unlike inside a Claude-generated artifact: this is a real
    // xoluman-authored component, not subject to that sandbox's own
    // restriction). Keyed by connection (derived from runUrl, which
    // is already per-connection) + mode, so switching connections
    // doesn't mix histories together.
    this._recentByMode = { oql: [], sulpher: [], rest: [] };
    // Saved queries: explicit, named, server-backed (see
    // internal/ui/query.go's savedQuery API) — deliberately the
    // opposite persistence choice from recent queries: a saved query
    // is a deliberate, shared artifact, visible to anyone else using
    // xoluman against the same xolu instance, not a private browser
    // history.
    this._savedByMode = { oql: [], sulpher: [], rest: [] };
    this._showSaveForm = false;
    this._saveNameDraft = '';
    this._selectedSavedId = null;
  }

  firstUpdated() {
    if (!this._activeModes.includes(this._mode)) {
      this._mode = this._activeModes[0];
    }
    this._editorHost = document.createElement('div');
    this._editorHost.className = 'xolu-query-editor-host';
    this.querySelector('.xolu-query-editor-mount').appendChild(this._editorHost);

    this._view = new EditorView({
      state: EditorState.create({
        doc: '',
        extensions: [
          basicSetup,
          this._languageConf.of(this._languageExtensionFor(this._mode)),
          this._themeConf.of(this._themeExtensionFor()),
        ],
      }),
      parent: this._editorHost,
    });
    // @neo4j-cypher/codemirror's own cypherExtensions (via getExtensions)
    // includes a lint source that calls view.newContentVersion() —
    // confirmed directly against the real, unminified package source
    // (es/codemirror.js): this method is never provided by
    // getExtensions() itself, only by the package's own separate,
    // higher-level createCypherEditor() helper, which attaches it to
    // the EditorView it creates internally as exactly this same
    // version-counter closure. Using getExtensions() directly (as this
    // component does, for the same Compartment-based reconfiguration
    // the OQL/REST modes already need) means completing that same
    // attachment ourselves — without it, the lint source throws
    // "newContentVersion is not a function" the first time it runs
    // (on a debounced timer after any doc change, not only on Run;
    // confirmed by reproducing it from typing alone with no Run click
    // at all). Attached once, unconditionally, regardless of which
    // language mode is active — harmless for OQL/REST, and persists
    // correctly across _setMode's own setState calls since those keep
    // the same EditorView instance.
    this._view.version = 1;
    this._view.newContentVersion = function () {
      this.version += 1;
      return this.version;
    };

    this._themeObserver = new MutationObserver(() => {
      this._view.dispatch({ effects: this._themeConf.reconfigure(this._themeExtensionFor()) });
    });
    this._themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });

    this._loadRecentFromStorage();
    this._fetchAllSaved();
  }

  // savedUrl is derived from run-url rather than a second attribute —
  // internal/ui/query.go registers both under the same
  // /connections/{name}/query/... prefix, so the relationship is
  // structural, not something the caller needs to state twice.
  get _savedUrl() {
    return this.runUrl.replace(/\/run$/, '/saved');
  }

  get _recentStorageKey() {
    // The run URL already encodes the connection name
    // (/connections/{name}/query/run) — reusing it as the localStorage
    // namespace means recent-query history is automatically scoped
    // per connection without parsing the name back out separately.
    return 'xolu-recent-queries:' + this.runUrl;
  }

  _loadRecentFromStorage() {
    try {
      const raw = localStorage.getItem(this._recentStorageKey);
      if (raw) {
        const parsed = JSON.parse(raw);
        for (const m of MODES) {
          if (Array.isArray(parsed[m])) this._recentByMode[m] = parsed[m];
        }
      }
    } catch (e) {
      // Storage unavailable or corrupt — start empty rather than
      // breaking the editor over a history feature.
    }
    this.requestUpdate();
  }

  _saveRecentToStorage() {
    try {
      localStorage.setItem(this._recentStorageKey, JSON.stringify(this._recentByMode));
    } catch (e) {
      // Storage full or unavailable — the current session's history
      // still works via _recentByMode in memory, just won't persist.
    }
  }

  // _pushRecent records a query that was actually run — called from
  // _run() on any completed request, success or failure, since even a
  // failed query is one someone might want to pull back up and fix
  // rather than retype from scratch.
  _pushRecent(mode, entry) {
    const list = this._recentByMode[mode];
    // De-duplicate: an identical query bubbles to the front rather
    // than appearing twice in the history.
    const key = JSON.stringify(entry);
    const withoutDup = list.filter((e) => JSON.stringify(e) !== key);
    withoutDup.unshift(entry);
    this._recentByMode[mode] = withoutDup.slice(0, 15);
    this._saveRecentToStorage();
    this.requestUpdate();
  }

  async _fetchAllSaved() {
    for (const m of MODES) {
      try {
        const resp = await fetch(this._savedUrl + '?mode=' + m);
        if (!resp.ok) continue;
        this._savedByMode[m] = await resp.json();
      } catch (e) {
        // A saved-query fetch failing shouldn't block the editor
        // itself from being usable — that list just stays empty.
      }
    }
    this.requestUpdate();
  }

  // _loadEntry applies a recent or saved entry to the current editor
  // state — the query text (or, for REST, method+path+body) — without
  // switching modes; both listboxes are already scoped to the active
  // mode, so applying one implies "into what's currently open."
  _loadEntry(entry) {
    if (this._mode === 'rest') {
      this._restMethod = entry.method || 'GET';
      this._restPath = entry.path || '';
    }
    const doc = this._mode === 'rest' ? entry.body || '' : entry.query || '';
    this._view.dispatch({
      changes: { from: 0, to: this._view.state.doc.length, insert: doc },
    });
    this._result = null;
    this._error = null;
    this.requestUpdate();
  }

  async _saveCurrentQuery() {
    const name = this._saveNameDraft.trim();
    if (!name) return;
    const query = this._view.state.doc.toString();
    const payload = { mode: this._mode, name };
    if (this._mode === 'rest') {
      payload.method = this._restMethod;
      payload.path = this._restPath;
      payload.contentType = 'application/json';
      payload.body = query;
    } else {
      payload.query = query;
    }
    try {
      const resp = await fetch(this._savedUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (resp.ok) {
        const listResp = await fetch(this._savedUrl + '?mode=' + this._mode);
        if (listResp.ok) this._savedByMode[this._mode] = await listResp.json();
      }
    } catch (e) {
      // Best-effort — the query the person typed is still sitting in
      // the editor either way, nothing is lost by a save failing.
    }
    this._showSaveForm = false;
    this._saveNameDraft = '';
    this.requestUpdate();
  }

  async _deleteSaved(id) {
    try {
      await fetch(this._savedUrl + '/' + id, { method: 'DELETE' });
      this._savedByMode[this._mode] = this._savedByMode[this._mode].filter((s) => s.id !== id);
      this.requestUpdate();
    } catch (e) {
      // Best-effort — the entry just stays in the list if this fails,
      // no different from before the click.
    }
  }

  // _describeEntry builds a short, single-line label for a recent
  // (unnamed) history entry's dropdown option — saved entries already
  // have a real name a person gave them, so this is only used for the
  // Recent list. Truncated: a dropdown option isn't the place for a
  // multi-line query to be read in full, just recognised.
  _describeEntry(entry) {
    const text = this._mode === 'rest' ? `${entry.method || 'GET'} ${entry.path || ''}` : entry.query || '';
    const oneLine = text.replace(/\s+/g, ' ').trim();
    return oneLine.length > 60 ? oneLine.slice(0, 57) + '…' : oneLine || '(empty)';
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._themeObserver) this._themeObserver.disconnect();
  }

  _themeExtensionFor() {
    return document.documentElement.classList.contains('dark') ? oneDark : [];
  }

  _languageExtensionFor(mode) {
    switch (mode) {
      case 'oql':
        return sql({ dialect: MSSQL });
      case 'sulpher':
        return cypherExtensions(); // returns a plain extensions array directly — confirmed against the real source, not assumed
      case 'rest':
        return json();
      default:
        return [];
    }
  }

  _setMode(mode) {
    if (mode === this._mode) return;
    this._docs[this._mode] = this._view.state.doc.toString();
    this._mode = mode;
    this._result = null;
    this._resultParsed = null;
    this._resultViewMode = 'raw';
    this._error = null;
    this._showSaveForm = false;
    this._saveNameDraft = '';
    if (this._view) {
      this._view.setState(
        EditorState.create({
          doc: this._docs[mode] || '',
          extensions: [
            basicSetup,
            this._languageConf.of(this._languageExtensionFor(mode)),
            this._themeConf.of(this._themeExtensionFor()),
          ],
        })
      );
    }
    this.requestUpdate();
  }

  async _run() {
    if (this._running || !this._view) return;
    this._running = true;
    this._error = null;
    this._resultParsed = null;
    this._resultViewMode = 'raw';
    this.requestUpdate();

    const query = this._view.state.doc.toString();
    const payload = { mode: this._mode, query };
    if (this._mode === 'rest') {
      payload.method = this._restMethod;
      payload.path = this._restPath;
      payload.contentType = 'application/json';
      payload.body = query; // REST mode's editor holds the request body (JSON), path/method come from the form fields above it
    }
    if (query.trim()) {
      this._pushRecent(this._mode, this._mode === 'rest' ? { method: this._restMethod, path: this._restPath, body: query } : { query });
    }

    try {
      const resp = await fetch(this.runUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const text = await resp.text();
      if (!resp.ok) {
        this._error = text || `HTTP ${resp.status}`;
      } else {
        try {
          const parsed = JSON.parse(text);
          this._resultParsed = parsed;
          this._result = JSON.stringify(parsed, null, 2);
        } catch {
          this._result = text;
        }
      }
    } catch (err) {
      this._error = String(err);
    } finally {
      this._running = false;
      this.requestUpdate();
    }
  }

  // _entitiesUrlFor derives the real entity list/grid page's own URL
  // from runUrl ("/connections/{name}/query/run"), the same string-
  // derivation convention this component already uses for savedQueriesUrl
  // and its own localStorage key. Only ever called when the OQL
  // classifier (server-side, internal/oqlclassify) has already
  // confirmed a query is a genuine simple select — this is where the
  // live-editable grid for a table actually lives, not a new view
  // built for this feature.
  _entitiesUrlFor(table) {
    return this.runUrl.replace(/\/query\/run$/, '/entities/' + encodeURIComponent(table));
  }

  // _viewInGraph is the Sulpher tab's own bridge to the graph-walker
  // tab living beside it on the same page (query.go's own GraphView,
  // not this component's own concern how the two tabs are switched or
  // laid out) — dispatches a plain DOM event carrying the already-
  // classified {nodes, edges} (see internal/ui/query.go's own
  // classifySulpherResult, shared with the dormant standalone
  // endpoint) rather than re-classifying client-side, since the
  // server already did that work once. bubbles+composed so a page-
  // level listener catches it regardless of exactly where in the DOM
  // this component sits; light DOM here anyway (createRenderRoot
  // returns this), so composed doesn't strictly matter, but it's
  // correct regardless of that implementation detail.
  _viewInGraph() {
    if (!this._resultParsed || !this._resultParsed.graphData) return;
    this.dispatchEvent(
      new CustomEvent('xolu-view-in-graph', {
        detail: this._resultParsed.graphData,
        bubbles: true,
        composed: true,
      })
    );
  }

  // _renderResultTable builds a plain, read-only <table> from an OQL
  // result's own data rows — the generic "view as table" path for any
  // query shape that isn't a simple select (a JOIN, an aggregate, an
  // explicit field list), where routing to the live-editable entity
  // grid isn't safe (see oqlclassify's own doc comment for why).
  // Deliberately not editable: these rows don't correspond to a
  // single, complete, real record a PATCH could safely target.
  _renderResultTable(rows) {
    if (!rows || rows.length === 0) {
      return html`<div class="xolu-query-result-empty">No rows.</div>`;
    }
    const columns = [];
    const seen = new Set();
    for (const row of rows) {
      for (const key of Object.keys(row)) {
        if (!seen.has(key)) {
          seen.add(key);
          columns.push(key);
        }
      }
    }
    return html`
      <table class="xolu-query-result-table">
        <thead>
          <tr>${columns.map((c) => html`<th>${c}</th>`)}</tr>
        </thead>
        <tbody>
          ${rows.map(
            (row) => html`<tr>${columns.map((c) => html`<td>${this._cellText(row[c])}</td>`)}</tr>`
          )}
        </tbody>
      </table>
    `;
  }

  _cellText(value) {
    if (value === null || value === undefined) return '';
    if (typeof value === 'object') return JSON.stringify(value);
    return String(value);
  }

  render() {
    return html`
      <style>
        .xolu-query-tabs { display: flex; gap: 0.5rem; margin-bottom: 0.75rem; }
        .xolu-query-tab {
          padding: 0.4rem 0.9rem; font-size: 0.875rem; border-radius: 0.5rem 0.5rem 0 0;
          border: 1px solid #d1d5db; border-bottom: none; background: #f3f4f6; cursor: pointer;
          color: #111827;
        }
        .dark .xolu-query-tab { border-color: #374151; background: #1f2937; color: #f3f4f6; }
        .xolu-query-tab.active { background: #fff; font-weight: 600; border-color: #4f46e5; color: #4f46e5; }
        .dark .xolu-query-tab.active { background: #111827; border-color: #818cf8; color: #818cf8; }
        .xolu-query-editor-mount { border: 1px solid #d1d5db; border-radius: 0.5rem; overflow: hidden; }
        .dark .xolu-query-editor-mount { border-color: #374151; }
        .xolu-query-editor-host .cm-editor { min-height: 160px; }
        .xolu-query-rest-fields { display: flex; gap: 0.5rem; margin-bottom: 0.5rem; }
        .xolu-query-rest-fields select, .xolu-query-rest-fields input {
          padding: 0.4rem 0.6rem; border: 1px solid #d1d5db; border-radius: 0.375rem; font-size: 0.875rem;
          background: #fff; color: #111827;
        }
        .dark .xolu-query-rest-fields select, .dark .xolu-query-rest-fields input {
          border-color: #374151; background: #1f2937; color: #f3f4f6;
        }
        .xolu-query-run-btn {
          margin-top: 0.75rem; padding: 0.5rem 1.25rem; font-size: 0.875rem; font-weight: 500;
          border-radius: 0.5rem; border: none; cursor: pointer; background: #4f46e5; color: #fff;
        }
        .xolu-query-run-btn:disabled { background: #a5b4fc; cursor: default; }
        .dark .xolu-query-run-btn:disabled { background: #4338ca; color: #c7d2fe; }
        .xolu-query-history-row { display: flex; gap: 0.5rem; margin-bottom: 0.5rem; align-items: center; }
        .xolu-query-history-select {
          flex: 1; min-width: 0; padding: 0.35rem 0.5rem; border: 1px solid #d1d5db; border-radius: 0.375rem;
          font-size: 0.8125rem; background: #fff; color: #111827;
        }
        .dark .xolu-query-history-select { border-color: #374151; background: #1f2937; color: #f3f4f6; }
        .xolu-query-history-delete {
          padding: 0.25rem 0.55rem; font-size: 0.9rem; line-height: 1; border-radius: 0.375rem;
          border: 1px solid #fecaca; background: #fef2f2; color: #b91c1c; cursor: pointer;
        }
        .dark .xolu-query-history-delete { border-color: rgba(248, 113, 113, 0.3); background: rgba(127, 29, 29, 0.3); color: #f87171; }
        .xolu-query-actions-row { display: flex; gap: 0.5rem; align-items: center; margin-top: 0.75rem; }
        .xolu-query-save-btn {
          padding: 0.5rem 1rem; font-size: 0.875rem; font-weight: 500; border-radius: 0.5rem;
          border: 1px solid #d1d5db; background: #fff; color: #374151; cursor: pointer;
        }
        .dark .xolu-query-save-btn { border-color: #374151; background: #1f2937; color: #f3f4f6; }
        .xolu-query-save-name {
          padding: 0.5rem 0.75rem; font-size: 0.875rem; border: 1px solid #d1d5db; border-radius: 0.5rem;
          background: #fff; color: #111827; min-width: 12rem;
        }
        .dark .xolu-query-save-name { border-color: #374151; background: #1f2937; color: #f3f4f6; }
        .xolu-query-save-confirm {
          padding: 0.5rem 1rem; font-size: 0.875rem; font-weight: 500; border-radius: 0.5rem;
          border: none; cursor: pointer; background: #4f46e5; color: #fff;
        }
        .xolu-query-save-confirm:disabled { background: #a5b4fc; cursor: default; }
        .dark .xolu-query-save-confirm:disabled { background: #4338ca; color: #c7d2fe; }
        .xolu-query-result { margin-top: 1rem; padding: 0.75rem; background: #f9fafb; border-radius: 0.5rem; font-family: monospace; font-size: 0.8125rem; white-space: pre-wrap; max-height: 400px; overflow: auto; color: #111827; }
        .dark .xolu-query-result { background: #1f2937; color: #f3f4f6; }
        .xolu-query-result-actions { display: flex; align-items: center; gap: 0.75rem; margin-top: 1rem; flex-wrap: wrap; }
        .xolu-query-view-toggle { padding: 0.35rem 0.75rem; font-size: 0.8125rem; border-radius: 0.375rem; border: 1px solid #d1d5db; background: #fff; cursor: pointer; }
        .dark .xolu-query-view-toggle { border-color: #4b5563; background: #1f2937; color: #f3f4f6; }
        .xolu-query-edit-grid-link { font-size: 0.8125rem; color: #4f46e5; text-decoration: none; }
        .xolu-query-edit-grid-link:hover { text-decoration: underline; }
        .xolu-query-view-graph-btn { padding: 0.35rem 0.75rem; font-size: 0.8125rem; border-radius: 0.375rem; border: 1px solid #4f46e5; background: #4f46e5; color: #fff; cursor: pointer; }
        .xolu-query-result-table { margin-top: 0; width: 100%; border-collapse: collapse; font-size: 0.8125rem; max-height: 400px; display: block; overflow: auto; }
        .xolu-query-result-table th, .xolu-query-result-table td { padding: 0.4rem 0.6rem; border: 1px solid #e5e7eb; text-align: left; white-space: nowrap; }
        .xolu-query-result-table th { background: #f9fafb; font-weight: 600; position: sticky; top: 0; }
        .dark .xolu-query-result-table th, .dark .xolu-query-result-table td { border-color: #374151; }
        .dark .xolu-query-result-table th { background: #1f2937; }
        .xolu-query-result-empty { margin-top: 1rem; font-size: 0.8125rem; color: #6b7280; }
        .xolu-query-error { margin-top: 1rem; padding: 0.75rem; background: #fef2f2; color: #b91c1c; border-radius: 0.5rem; font-family: monospace; font-size: 0.8125rem; white-space: pre-wrap; }
        .dark .xolu-query-error { background: rgba(127, 29, 29, 0.3); color: #f87171; }
      </style>
      <div class="xolu-query-tabs">
        ${this._activeModes.length > 1
          ? this._activeModes.map(
              (m) => html`
                <div class="xolu-query-tab ${m === this._mode ? 'active' : ''}" @click=${() => this._setMode(m)}>
                  ${m.toUpperCase()}
                </div>
              `
            )
          : ''}
      </div>
      <div class="xolu-query-history-row">
        <select
          class="xolu-query-history-select"
          @change=${(e) => {
            const i = Number(e.target.value);
            e.target.value = '';
            if (!Number.isNaN(i) && this._recentByMode[this._mode][i]) this._loadEntry(this._recentByMode[this._mode][i]);
          }}
        >
          <option value="">Recent…</option>
          ${this._recentByMode[this._mode].map((entry, i) => html`<option value=${i}>${this._describeEntry(entry)}</option>`)}
        </select>
        <select
          class="xolu-query-history-select"
          .value=${this._selectedSavedId || ''}
          @change=${(e) => {
            const id = Number(e.target.value);
            this._selectedSavedId = id || null;
            const entry = this._savedByMode[this._mode].find((s) => s.id === id);
            if (entry) this._loadEntry(entry);
          }}
        >
          <option value="">Saved…</option>
          ${this._savedByMode[this._mode].map((s) => html`<option value=${s.id}>${s.name}</option>`)}
        </select>
        ${this._selectedSavedId
          ? html`<button
              class="xolu-query-history-delete"
              title="Delete this saved query"
              @click=${() => {
                this._deleteSaved(this._selectedSavedId);
                this._selectedSavedId = null;
              }}
            >
              ×
            </button>`
          : ''}
      </div>
      ${this._mode === 'rest'
        ? html`
            <div class="xolu-query-rest-fields">
              <select .value=${this._restMethod} @change=${(e) => (this._restMethod = e.target.value)}>
                <option>GET</option>
                <option>POST</option>
                <option>PUT</option>
                <option>PATCH</option>
                <option>DELETE</option>
              </select>
              <input
                type="text"
                placeholder="/api/v1/..."
                .value=${this._restPath}
                @input=${(e) => (this._restPath = e.target.value)}
                style="flex:1;"
              />
            </div>
          `
        : ''}
      <div class="xolu-query-editor-mount"></div>
      <div class="xolu-query-actions-row">
        <button class="xolu-query-run-btn" ?disabled=${this._running} @click=${this._run}>
          ${this._running ? 'Running…' : 'Run'}
        </button>
        <button class="xolu-query-save-btn" @click=${() => (this._showSaveForm = !this._showSaveForm)}>Save…</button>
        ${this._showSaveForm
          ? html`
              <input
                type="text"
                class="xolu-query-save-name"
                placeholder="Name this query"
                .value=${this._saveNameDraft}
                @input=${(e) => (this._saveNameDraft = e.target.value)}
                @keydown=${(e) => {
                  if (e.key === 'Enter') this._saveCurrentQuery();
                  if (e.key === 'Escape') this._showSaveForm = false;
                }}
              />
              <button class="xolu-query-save-confirm" ?disabled=${!this._saveNameDraft.trim()} @click=${this._saveCurrentQuery}>
                Save
              </button>
            `
          : ''}
      </div>
      ${this._error ? html`<div class="xolu-query-error">${this._error}</div>` : ''}
      ${this._resultParsed && this._mode === 'oql' && Array.isArray(this._resultParsed.data) && this._resultParsed.data.length > 0
        ? html`
            <div class="xolu-query-result-actions">
              <button
                class="xolu-query-view-toggle"
                @click=${() => (this._resultViewMode = this._resultViewMode === 'table' ? 'raw' : 'table')}>
                ${this._resultViewMode === 'table' ? 'View as JSON' : 'View as table'}
              </button>
              ${this._resultParsed.classification && this._resultParsed.classification.isSimpleSelect
                ? html`
                    <a class="xolu-query-edit-grid-link" href=${this._entitiesUrlFor(this._resultParsed.classification.sourceTable)}>
                      Open ${this._resultParsed.classification.sourceTable} as an editable table →
                    </a>
                  `
                : ''}
            </div>
          `
        : ''}
      ${this._resultParsed && this._mode === 'sulpher' && this._resultParsed.graphData && this._resultParsed.graphData.nodes && this._resultParsed.graphData.nodes.length > 0
        ? html`
            <div class="xolu-query-result-actions">
              <button class="xolu-query-view-graph-btn" @click=${this._viewInGraph}>
                View in graph →
              </button>
            </div>
          `
        : ''}
      ${this._result
        ? this._resultViewMode === 'table' && this._resultParsed
          ? this._renderResultTable(this._resultParsed.data)
          : html`<div class="xolu-query-result">${this._result}</div>`
        : ''}
    `;
  }
}

customElements.define('xolu-query-editor', XoluQueryEditor);
