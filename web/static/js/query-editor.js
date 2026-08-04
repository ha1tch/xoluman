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
  };

  // Light DOM — CodeMirror renders real DOM nodes into whatever
  // container it's given; a shadow root adds nothing here and only
  // risks CSS isolation surprises, same reasoning as grid-editor.js.
  createRenderRoot() {
    return this;
  }

  constructor() {
    super();
    this._mode = 'oql';
    this._running = false;
    this._result = null;
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
  }

  firstUpdated() {
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

    this._themeObserver = new MutationObserver(() => {
      this._view.dispatch({ effects: this._themeConf.reconfigure(this._themeExtensionFor()) });
    });
    this._themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
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
    this._error = null;
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
    this.requestUpdate();

    const query = this._view.state.doc.toString();
    const payload = { mode: this._mode, query };
    if (this._mode === 'rest') {
      payload.method = this._restMethod;
      payload.path = this._restPath;
      payload.contentType = 'application/json';
      payload.body = query; // REST mode's editor holds the request body (JSON), path/method come from the form fields above it
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
          this._result = JSON.stringify(JSON.parse(text), null, 2);
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
        .xolu-query-result { margin-top: 1rem; padding: 0.75rem; background: #f9fafb; border-radius: 0.5rem; font-family: monospace; font-size: 0.8125rem; white-space: pre-wrap; max-height: 400px; overflow: auto; color: #111827; }
        .dark .xolu-query-result { background: #1f2937; color: #f3f4f6; }
        .xolu-query-error { margin-top: 1rem; padding: 0.75rem; background: #fef2f2; color: #b91c1c; border-radius: 0.5rem; font-family: monospace; font-size: 0.8125rem; white-space: pre-wrap; }
        .dark .xolu-query-error { background: rgba(127, 29, 29, 0.3); color: #f87171; }
      </style>
      <div class="xolu-query-tabs">
        ${MODES.map(
          (m) => html`
            <div class="xolu-query-tab ${m === this._mode ? 'active' : ''}" @click=${() => this._setMode(m)}>
              ${m.toUpperCase()}
            </div>
          `
        )}
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
      <button class="xolu-query-run-btn" ?disabled=${this._running} @click=${this._run}>
        ${this._running ? 'Running…' : 'Run'}
      </button>
      ${this._error ? html`<div class="xolu-query-error">${this._error}</div>` : ''}
      ${this._result ? html`<div class="xolu-query-result">${this._result}</div>` : ''}
    `;
  }
}

customElements.define('xolu-query-editor', XoluQueryEditor);
