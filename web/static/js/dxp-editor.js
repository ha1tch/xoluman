// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// dxp-editor.js — Lit shell for T-25's design
// (docs/proposals/dxp-transactions.md). Deliberately not a code
// editor: a DXP transaction has no query language, only a chosen
// definition and the parameter values ("bindings") its participants
// reference via {"$ref": "..."} — this is a def picker plus a
// dynamically-generated form, same architecture split as
// grid-editor.js and query-editor.js (a thin Lit wrapper), but with no
// CodeMirror at all, since there's no code to highlight.
//
// Data contract with the server (internal/ui/dxp.go):
//   GET  {defs-url}         -> [{id, name, createdAt}, ...]
//   GET  {defs-url}/{id}    -> {id, name, pattern, participants,
//                                bindings: [<name>, ...], bindingsSchema?}
//                              bindings is the server-side extracted,
//                              de-duplicated, sorted list of every
//                              {"$ref": "..."} name any participant's
//                              Params reference — this component does
//                              not re-walk participants itself.
//   POST {run-url} <- {defId, bindings: {<name>: <value>, ...}}
//                   -> the real DxpTxn: {id, defId, status, reason?,
//                      committedThrough, snapshot, ...}. A
//                      non-committed status (released/expired) is a
//                      normal response, not an HTTP error — rendered
//                      distinctly, not as a failure state.

import { LitElement, html } from 'lit';

class XoluDxpEditor extends LitElement {
  static properties = {
    defsUrl: { attribute: 'defs-url' },
    runUrl: { attribute: 'run-url' },
  };

  // Light DOM — same reasoning as grid-editor.js/query-editor.js: no
  // real CSS isolation need here, and a shadow root only risks
  // surprises with the rest of the page's own styling.
  createRenderRoot() {
    return this;
  }

  constructor() {
    super();
    this._defs = [];
    this._selectedDefId = null;
    this._defDetail = null;
    this._bindingValues = {};
    this._running = false;
    this._result = null;
    this._error = null;
    this._loadingDef = false;
  }

  connectedCallback() {
    super.connectedCallback();
    this._fetchDefs();
  }

  async _fetchDefs() {
    try {
      const resp = await fetch(this.defsUrl);
      if (resp.ok) this._defs = await resp.json();
    } catch (e) {
      this._error = 'Could not load definitions: ' + String(e);
    }
    this.requestUpdate();
  }

  async _selectDef(idStr) {
    const id = Number(idStr);
    this._selectedDefId = id || null;
    this._defDetail = null;
    this._bindingValues = {};
    this._result = null;
    this._error = null;
    if (!id) {
      this.requestUpdate();
      return;
    }
    this._loadingDef = true;
    this.requestUpdate();
    try {
      const resp = await fetch(this.defsUrl + '/' + id);
      if (!resp.ok) {
        this._error = await resp.text();
      } else {
        this._defDetail = await resp.json();
        const values = {};
        for (const name of this._defDetail.bindings || []) values[name] = '';
        this._bindingValues = values;
      }
    } catch (e) {
      this._error = String(e);
    } finally {
      this._loadingDef = false;
      this.requestUpdate();
    }
  }

  // _parseBindingValue lets someone type a bare number, true/false, or
  // a quoted/unquoted string and get the right JSON type through to
  // Bindings (a map[string]interface{} server-side) — a raw text
  // input is the simplest thing to render for an arbitrary binding
  // without knowing its type ahead of time (bindingsSchema, when
  // present, describes types but this first pass doesn't yet render
  // type-specific inputs from it — see the proposal's own scoping).
  _parseBindingValue(raw) {
    const trimmed = raw.trim();
    if (trimmed === '') return '';
    try {
      return JSON.parse(trimmed);
    } catch {
      return raw; // not valid JSON — treat as a plain string, unquoted typing works
    }
  }

  async _runTransaction() {
    if (this._running || !this._selectedDefId) return;
    this._running = true;
    this._error = null;
    this._result = null;
    this.requestUpdate();

    const bindings = {};
    for (const [name, raw] of Object.entries(this._bindingValues)) {
      bindings[name] = this._parseBindingValue(raw);
    }

    try {
      const resp = await fetch(this.runUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ defId: this._selectedDefId, bindings }),
      });
      const text = await resp.text();
      if (!resp.ok) {
        this._error = text || `HTTP ${resp.status}`;
      } else {
        this._result = JSON.parse(text);
      }
    } catch (err) {
      this._error = String(err);
    } finally {
      this._running = false;
      this.requestUpdate();
    }
  }

  _statusClass(status) {
    if (status === 'committed') return 'xolu-dxp-status-committed';
    if (status === 'released' || status === 'expired') return 'xolu-dxp-status-declined';
    return '';
  }

  render() {
    return html`
      <style>
        .xolu-dxp-picker { margin-bottom: 1rem; }
        .xolu-dxp-picker select {
          width: 100%; max-width: 24rem; padding: 0.5rem 0.75rem; border: 1px solid #d1d5db;
          border-radius: 0.5rem; font-size: 0.875rem; background: #fff; color: #111827;
        }
        .dark .xolu-dxp-picker select { border-color: #374151; background: #1f2937; color: #f3f4f6; }
        .xolu-dxp-def-info {
          margin-bottom: 1rem; padding: 0.75rem 1rem; border-radius: 0.5rem;
          background: #f9fafb; border: 1px solid #e5e7eb; font-size: 0.8125rem; color: #374151;
        }
        .dark .xolu-dxp-def-info { background: #1f2937; border-color: #374151; color: #d1d5db; }
        .xolu-dxp-def-info dt { font-weight: 600; color: #6b7280; margin-top: 0.5rem; }
        .dark .xolu-dxp-def-info dt { color: #9ca3af; }
        .xolu-dxp-def-info dt:first-child { margin-top: 0; }
        .xolu-dxp-participant { padding: 0.25rem 0; font-family: monospace; font-size: 0.75rem; }
        .xolu-dxp-bindings-form { margin-bottom: 1rem; }
        .xolu-dxp-binding-row { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem; }
        .xolu-dxp-binding-label {
          width: 10rem; flex-shrink: 0; font-size: 0.8125rem; font-family: monospace;
          color: #374151; text-align: right;
        }
        .dark .xolu-dxp-binding-label { color: #d1d5db; }
        .xolu-dxp-binding-input {
          flex: 1; padding: 0.4rem 0.6rem; border: 1px solid #d1d5db; border-radius: 0.375rem;
          font-size: 0.875rem; background: #fff; color: #111827;
        }
        .dark .xolu-dxp-binding-input { border-color: #374151; background: #1f2937; color: #f3f4f6; }
        .xolu-dxp-run-btn {
          padding: 0.5rem 1.25rem; font-size: 0.875rem; font-weight: 500; border-radius: 0.5rem;
          border: none; cursor: pointer; background: #4f46e5; color: #fff;
        }
        .xolu-dxp-run-btn:disabled { background: #a5b4fc; cursor: default; }
        .dark .xolu-dxp-run-btn:disabled { background: #4338ca; color: #c7d2fe; }
        .xolu-dxp-result {
          margin-top: 1rem; padding: 0.75rem 1rem; border-radius: 0.5rem; font-size: 0.8125rem;
        }
        .xolu-dxp-status-committed { background: #f0fdf4; border: 1px solid #bbf7d0; }
        .dark .xolu-dxp-status-committed { background: rgba(20, 83, 45, 0.3); border-color: rgba(74, 222, 128, 0.3); }
        .xolu-dxp-status-declined { background: #fef2f2; border: 1px solid #fecaca; }
        .dark .xolu-dxp-status-declined { background: rgba(127, 29, 29, 0.3); border-color: rgba(248, 113, 113, 0.3); }
        .xolu-dxp-status-label { font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em; font-size: 0.75rem; }
        .xolu-dxp-status-committed .xolu-dxp-status-label { color: #16a34a; }
        .dark .xolu-dxp-status-committed .xolu-dxp-status-label { color: #4ade80; }
        .xolu-dxp-status-declined .xolu-dxp-status-label { color: #dc2626; }
        .dark .xolu-dxp-status-declined .xolu-dxp-status-label { color: #f87171; }
        .xolu-dxp-result pre { margin: 0.5rem 0 0; white-space: pre-wrap; font-size: 0.75rem; }
        .xolu-dxp-error {
          margin-top: 1rem; padding: 0.75rem; background: #fef2f2; color: #b91c1c; border-radius: 0.5rem;
          font-family: monospace; font-size: 0.8125rem; white-space: pre-wrap;
        }
        .dark .xolu-dxp-error { background: rgba(127, 29, 29, 0.3); color: #f87171; }
        .xolu-dxp-empty { color: #6b7280; font-size: 0.875rem; }
        .dark .xolu-dxp-empty { color: #9ca3af; }
      </style>

      ${this._defs.length === 0
        ? html`<div class="xolu-dxp-empty">No DXP definitions registered on this connection yet.</div>`
        : html`
            <div class="xolu-dxp-picker">
              <select @change=${(e) => this._selectDef(e.target.value)}>
                <option value="">Choose a definition…</option>
                ${this._defs.map((d) => html`<option value=${d.id}>${d.name}</option>`)}
              </select>
            </div>
          `}
      ${this._loadingDef ? html`<div class="xolu-dxp-empty">Loading…</div>` : ''}
      ${this._defDetail
        ? html`
            <dl class="xolu-dxp-def-info">
              <dt>Pattern</dt>
              <dd>${this._defDetail.pattern}</dd>
              <dt>Participants (${(this._defDetail.participants || []).length})</dt>
              <dd>
                ${(this._defDetail.participants || []).map(
                  (p) => html`<div class="xolu-dxp-participant">${p.id}: ${p.primitive}.${p.op}</div>`
                )}
              </dd>
            </dl>
            ${(this._defDetail.bindings || []).length === 0
              ? html`<div class="xolu-dxp-empty">This definition takes no parameters.</div>`
              : html`
                  <div class="xolu-dxp-bindings-form">
                    ${this._defDetail.bindings.map(
                      (name) => html`
                        <div class="xolu-dxp-binding-row">
                          <label class="xolu-dxp-binding-label">${name}</label>
                          <input
                            class="xolu-dxp-binding-input"
                            type="text"
                            .value=${this._bindingValues[name] || ''}
                            @input=${(e) => (this._bindingValues = { ...this._bindingValues, [name]: e.target.value })}
                          />
                        </div>
                      `
                    )}
                  </div>
                `}
            <button class="xolu-dxp-run-btn" ?disabled=${this._running} @click=${this._runTransaction}>
              ${this._running ? 'Running…' : 'Run transaction'}
            </button>
          `
        : ''}
      ${this._error ? html`<div class="xolu-dxp-error">${this._error}</div>` : ''}
      ${this._result
        ? html`
            <div class="xolu-dxp-result ${this._statusClass(this._result.status)}">
              <span class="xolu-dxp-status-label">${this._result.status}</span>
              ${this._result.reason ? html` — ${this._result.reason}` : ''}
              <pre>${JSON.stringify(this._result, null, 2)}</pre>
            </div>
          `
        : ''}
    `;
  }
}

customElements.define('xolu-dxp-editor', XoluDxpEditor);
