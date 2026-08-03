/**
 * xoluman modal — trimmed from Seam AMS's SeamModal (web/static/js/app.js)
 * to the interaction pattern xoluman actually needs: a backdrop, a
 * title, and an htmx-loaded body. Dropped relative to the original:
 * display-adaptive 4K sizing, dark-mode detection, the fixed-footer
 * extraction step, and inline-script re-execution — none of which
 * xoluman's simple forms need yet.
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

  function _css(el, props) { Object.assign(el.style, props); }

  function _build() {
    _backdrop = document.createElement('div');
    _css(_backdrop, {
      position: 'fixed', inset: '0',
      background: 'rgba(0,0,0,0.48)',
      zIndex: '9998',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    });

    _modal = document.createElement('div');
    _css(_modal, {
      background: 'Canvas', color: 'CanvasText',
      borderRadius: '8px',
      boxShadow: '0 20px 60px rgba(0,0,0,0.35)',
      width: '32rem', maxWidth: 'calc(100vw - 2rem)',
      maxHeight: 'calc(100vh - 4rem)',
      display: 'flex', flexDirection: 'column', overflow: 'hidden',
    });

    var header = document.createElement('div');
    _css(header, {
      padding: '0.9rem 1.1rem', borderBottom: '1px solid #8884',
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
      fontSize: '1.3rem', lineHeight: '1', padding: '0.2rem 0.4rem',
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
    if (_backdrop) close();
    _build();
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

  return { open: open, close: close, setTitle: setTitle };
}());
