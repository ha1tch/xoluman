// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// theme.js — dark/light mode manager. Tailwind's `darkMode: 'class'`
// and every `dark:` utility class throughout xoluman have been in
// place from early on; this is specifically the missing toggle
// mechanism itself.
//
// Loaded as a plain, synchronous, non-module <script> — deliberately
// NOT type="module" (which defers execution until after parsing) and
// placed early in <head>, before the page body renders. Setting the
// `dark` class on <html> has to happen before first paint, or the page
// flashes the wrong theme for a frame while a deferred script catches
// up. An IIFE, not a module, for the same reason — no import
// resolution delay.
(function () {
  var KEY = 'xoluman-theme';

  function apply(theme) {
    document.documentElement.classList.toggle('dark', theme === 'dark');
  }

  function current() {
    var saved = null;
    try {
      saved = localStorage.getItem(KEY);
    } catch (e) {
      // Private browsing / storage disabled — fall back to system
      // preference below rather than throwing.
    }
    if (saved === 'dark' || saved === 'light') return saved;
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }

  apply(current());

  window.xoluTheme = {
    current: current,
    toggle: function () {
      var next = document.documentElement.classList.contains('dark') ? 'light' : 'dark';
      try {
        localStorage.setItem(KEY, next);
      } catch (e) {
        // Storage unavailable — theme still applies for this page
        // load, just won't persist across a reload.
      }
      apply(next);
      var btn = document.getElementById('theme-toggle-btn');
      if (btn) btn.textContent = next === 'dark' ? '☀' : '☾';
    },
  };
})();
