// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// grid-editor.js — Lit shell around Tabulator (window.Tabulator, a UMD
// global loaded via a plain <script> tag before this module runs — see
// internal/ui/grid.go's gridPage) for bulk entity editing.
//
// Same architecture as Seam AMS's own FSM editor: a thin Lit
// toolbar/theming/lifecycle wrapper around a swappable, framework-
// agnostic engine (there, Evan Wallace's canvas designer; here,
// Tabulator). Light DOM, not shadow DOM — same reason Seam's shell
// does the same (createRenderRoot returning `this`): Tabulator's CSS is
// loaded normally in <head>, and only applies to elements actually in
// the light DOM tree.
//
// Data contract with the server (internal/ui/grid.go):
//   - GET  {data-url}?page=N&per_page=M -> {data:[...], last_page, last_row}
//     (checked directly against the vendored Tabulator source's own
//     remote-pagination response parser, not assumed from docs)
//   - POST {save-url} <- [{id, changes:{field: value, ...}}, ...]
//                     -> [{id, success, message?}, ...]
//     "changes" only ever contains the fields actually edited — the
//     server Patches, never PUTs, per T-11's design pass. This file
//     must keep sending only edited fields per row for that guarantee
//     to hold; sending a full row here would defeat it just as surely
//     as the server using Update would.

import { LitElement, html } from 'lit';

class XoluGridEditor extends LitElement {
  static properties = {
    dataUrl: { attribute: 'data-url' },
    saveUrl: { attribute: 'save-url' },
  };

  // Light DOM: see file header. Tabulator's stylesheet is a normal
  // <link> in <head>, which a shadow root would not see.
  createRenderRoot() {
    return this;
  }

  constructor() {
    super();
    this._dirty = new Map(); // id -> {field: value, ...}, one entry per row with at least one edited cell
    this._lastResults = null;
    this._saving = false;
  }

  firstUpdated() {
    const columnsEl = document.getElementById('grid-columns');
    const columns = columnsEl ? JSON.parse(columnsEl.textContent) : [];

    this._gridEl = document.createElement('div');
    this.appendChild(this._gridEl);

    this._table = new window.Tabulator(this._gridEl, {
      ajaxURL: this.dataUrl,
      pagination: true,
      paginationMode: 'remote',
      paginationSize: 50,
      // Explicit param names on both the request and response side
      // (see grid.go), rather than relying on Tabulator's internal
      // defaults, which weren't confidently determinable from the
      // minified vendored source.
      dataSendParams: { page: 'page', size: 'per_page' },
      columns,
      layout: 'fitDataStretch',
      height: '600px',
    });

    this._table.on('cellEdited', (cell) => {
      const row = cell.getRow();
      const id = row.getData().id;
      const field = cell.getField();
      const value = cell.getValue();
      if (!this._dirty.has(id)) this._dirty.set(id, {});
      this._dirty.get(id)[field] = value;
      this._lastResults = null;
      this.requestUpdate();
    });
  }

  async _save() {
    if (this._dirty.size === 0 || this._saving) return;
    this._saving = true;
    this.requestUpdate();

    const rows = Array.from(this._dirty.entries()).map(([id, changes]) => ({
      id: Number(id),
      changes,
    }));

    try {
      const resp = await fetch(this.saveUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(rows),
      });
      const results = await resp.json();
      this._lastResults = results;
      // Only clear dirty-tracking for rows that actually succeeded —
      // a failed row's edits stay marked dirty so "Save changes" can
      // be retried without the person having to re-type anything.
      for (const r of results) {
        if (r.success) this._dirty.delete(r.id);
      }
    } catch (err) {
      this._lastResults = [{ id: 0, success: false, message: String(err) }];
    } finally {
      this._saving = false;
      this.requestUpdate();
    }
  }

  render() {
    const dirtyCount = this._dirty.size;
    return html`
      <style>
        .xolu-grid-toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 0.75rem; }
        .xolu-grid-status { font-size: 0.875rem; color: var(--xolu-grid-status-color, #64748b); }
        .dark .xolu-grid-status { color: var(--xolu-grid-status-color, #94a3b8); }
        .xolu-grid-save-btn {
          padding: 0.5rem 1rem; font-size: 0.875rem; font-weight: 500;
          border-radius: 0.5rem; border: none; cursor: pointer;
          background: #4f46e5; color: #fff;
        }
        .xolu-grid-save-btn:disabled { background: #a5b4fc; cursor: default; }
        .dark .xolu-grid-save-btn:disabled { background: #4338ca; color: #c7d2fe; }
        .xolu-grid-results { margin-top: 0.5rem; font-size: 0.875rem; }
        .xolu-grid-results ul { margin: 0.25rem 0 0 1.25rem; }
        /* Success/error colors match xoluman's own established
           green-600/dark:green-400 and red-600/dark:red-400
           convention used everywhere else in the app (e.g.
           connections.go's Test-connection status) — consistency
           with the rest of xoluman's own palette, not a one-off pair
           of colors picked just for this component. */
        .xolu-grid-results-ok { color: #16a34a; }
        .dark .xolu-grid-results-ok { color: #4ade80; }
        .xolu-grid-results-error { color: #dc2626; }
        .dark .xolu-grid-results-error { color: #f87171; }
      </style>
      <div class="xolu-grid-toolbar">
        <div class="xolu-grid-status">
          ${dirtyCount > 0 ? `${dirtyCount} row(s) changed` : 'No unsaved changes'}
        </div>
        <button
          class="xolu-grid-save-btn"
          ?disabled=${dirtyCount === 0 || this._saving}
          @click=${this._save}
        >
          ${this._saving ? 'Saving…' : 'Save changes'}
        </button>
      </div>
      ${this._renderResults()}
    `;
  }

  _renderResults() {
    if (!this._lastResults) return html``;
    const failed = this._lastResults.filter((r) => !r.success);
    if (failed.length === 0) {
      return html`<div class="xolu-grid-results xolu-grid-results-ok">All changes saved.</div>`;
    }
    return html`
      <div class="xolu-grid-results xolu-grid-results-error">
        ${failed.length} row(s) failed to save:
        <ul>
          ${failed.map((f) => html`<li>Row ${f.id}: ${f.message || 'unknown error'}</li>`)}
        </ul>
      </div>
    `;
  }
}

customElements.define('xolu-grid-editor', XoluGridEditor);
