/**
 * xoluman modal — trimmed from Seam AMS's SeamModal (web/static/js/app.js)
 * to the interaction pattern xoluman actually needs: a backdrop, a
 * title, and an htmx-loaded body. Dropped relative to the original:
 * display-adaptive 4K sizing, the fixed-footer extraction step, and
 * inline-script re-execution — none of which xoluman's simple forms
 * need yet.
 *
 * Dark-mode awareness was ALSO dropped in that original trim (with a
 * "not yet needed" note right here) and that was a real mistake once
 * xoluman actually shipped a theme toggle — the CSS system-color
 * keywords `Canvas`/`CanvasText` used in its place follow the OS/
 * browser's own light-or-dark preference, not xoluman's explicit
 * localStorage-backed choice (theme.js), so overriding the theme via
 * the toggle button left every modal still following whatever the OS
 * happened to be set to — a visible, reported inconsistency. Restored
 * here using Seam's exact proven pattern (`document.documentElement.
 * classList.contains('dark')`, checked once at build time — Seam's
 * own SeamModal/SeamPopover never re-theme a modal that's already
 * open either, so this matches that behaviour deliberately, not by
 * oversight), with colours matching xoluman's own Tailwind gray scale
 * (the same palette every `dark:` utility class elsewhere in the app
 * already uses) rather than Seam's slate — a JS-computed style needs
 * to look consistent with THIS app's own classes, not the app it was
 * borrowed from.
 *
 * Usage (matches Seam's convention so the pattern transfers directly):
 *   <button hx-get="/connections/new" hx-target="#modal-body"
 *           onclick="XModal.open('New connection')">+ New connection</button>
 *
 * The modal body element is <div id="modal-body">; htmx targets it to
 * load form partials. XModal.close() dismisses programmatically.
 */
window.XModal = (function () {
  'use strict';

  var _backdrop = null;
  var _modal = null;
  var _onKey = null;

  function _dark() {
    return document.documentElement.classList.contains('dark');
  }

  function _css(el, props) { Object.assign(el.style, props); }

  function _build() {
    var dark = _dark();

    _backdrop = document.createElement('div');
    _css(_backdrop, {
      position: 'fixed', inset: '0',
      background: 'rgba(0,0,0,0.48)',
      zIndex: '9998',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    });

    _modal = document.createElement('div');
    _css(_modal, {
      // Tailwind's gray-800/gray-100 (dark) and white/gray-900 (light)
      // — the exact hex values already used throughout the rest of
      // xoluman via `dark:bg-gray-900`, `dark:text-gray-100`, etc.
      // gray-800 rather than gray-900 for the modal surface itself: a
      // shade lighter than the page background gives the modal visual
      // depth/separation from what's behind it, the same convention
      // any elevated surface (a card, a dropdown) uses over a page.
      background: dark ? '#1f2937' : '#ffffff',
      color: dark ? '#f3f4f6' : '#111827',
      borderRadius: '8px',
      boxShadow: '0 20px 60px rgba(0,0,0,0.35)',
      width: '32rem', maxWidth: 'calc(100vw - 2rem)',
      maxHeight: 'calc(100vh - 4rem)',
      display: 'flex', flexDirection: 'column', overflow: 'hidden',
    });

    var header = document.createElement('div');
    _css(header, {
      padding: '0.9rem 1.1rem',
      borderBottom: '1px solid ' + (dark ? '#374151' : '#e5e7eb'), // gray-700 / gray-200
      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      flexShrink: '0',
    });

    var titleEl = document.createElement('div');
    titleEl.id = 'modal-title';
    _css(titleEl, { fontWeight: '600' });

    var closeBtn = document.createElement('button');
    closeBtn.textContent = '×';
    closeBtn.setAttribute('aria-label', 'Close');
    _css(closeBtn, {
      background: 'none', border: 'none', cursor: 'pointer',
      color: 'inherit', fontSize: '1.3rem', lineHeight: '1', padding: '0.2rem 0.4rem',
    });
    closeBtn.addEventListener('click', function () { close(); });
    header.append(titleEl, closeBtn);

    var body = document.createElement('div');
    body.id = 'modal-body';
    _css(body, { flex: '1', overflowY: 'auto', padding: '1.1rem' });
    body.textContent = 'Loading…';

    _modal.append(header, body);
    _backdrop.appendChild(_modal);

    _backdrop.addEventListener('click', function (e) {
      if (e.target === _backdrop) close();
    });
    _onKey = function (e) { if (e.key === 'Escape') close(); };
    document.addEventListener('keydown', _onKey);

    document.body.appendChild(_backdrop);
  }

  function open(title) {
    if (!_backdrop) {
      _build();
    }
    // When a modal is already open (the lightning-icon jump case —
    // opening a linked entity from within an already-open modal),
    // #modal-body's content is deliberately left untouched here. A
    // real bug, found only by actually clicking it (Playwright), and
    // found TWICE: the first fix (rebuilding the whole modal) removed
    // the clicked button from the DOM before htmx's own click handler
    // on that same element could fire its request. The second attempt
    // (leaving the modal alone but resetting #modal-body's own
    // textContent to a loading message) made the identical mistake
    // one level down — the lightning button lives INSIDE #modal-body,
    // so clearing its content also removes the button mid-click,
    // synchronously, within this same onclick handler. The fix is to
    // not touch #modal-body's content here at all: the old content
    // stays visible until htmx's own swap replaces it once the
    // response actually arrives, which is a perfectly fine transition
    // and doesn't require touching the DOM synchronously inside a
    // handler that fired because of a click on an element inside the
    // very subtree being modified.
    setTitle(title || '');
  }

  function close() {
    if (!_backdrop) return;
    if (_onKey) { document.removeEventListener('keydown', _onKey); _onKey = null; }
    if (_backdrop.parentNode) _backdrop.parentNode.removeChild(_backdrop);
    _backdrop = null; _modal = null;
  }

  function setTitle(t) {
    var el = document.getElementById('modal-title');
    if (el) el.textContent = t;
  }

  // Promote the fragment's own <h1> (e.g. "Edit companies #1") to the
  // modal's title bar, replacing whatever static, less specific title
  // the trigger button set at click time (e.g. "Edit companies" — set
  // before the specific row's id was even known). Reported directly
  // as two redundant titles stacked on top of each other; this closes
  // it centrally, once, for every modal-loaded form, rather than
  // requiring each Go handler to coordinate its own title text with
  // its own trigger button's title param.
  // Listens on document, not document.body — a real bug caught only by
  // actually running this in a browser (Playwright), not from reading
  // the code: modal.js loads via a synchronous <script src> in <head>,
  // executing before <body> has been parsed at all, so
  // document.body.addEventListener would throw "Cannot read
  // properties of null" immediately — an uncaught exception inside
  // this IIFE that silently prevented the whole `return {...}`
  // statement from ever running, meaning window.XModal was never
  // defined and every single modal (New/Edit/Delete) broke. document
  // itself always exists regardless of parse order, and htmx's own
  // events bubble up to it the same as they would to body.
  document.addEventListener('htmx:afterSwap', function (e) {
    if (!e.detail || !e.detail.target || e.detail.target.id !== 'modal-body') return;
    var h1 = e.detail.target.querySelector('h1');
    if (!h1) return;
    setTitle(h1.textContent);
    h1.remove();
  });

  return { open: open, close: close, setTitle: setTitle };
}());
