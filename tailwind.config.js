/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: [
    './internal/**/*.go',
    './web/static/js/*.js',
  ],
  // Real bug this closed, found via visual verification (Playwright
  // screenshot, not something curl or reading source could ever have
  // caught): mi.DarkModeSVGIcons()'s two icon SVGs use "w-5 h-5" for
  // their size, but that markup is generated inside minty's own
  // package (github.com/ha1tch/minty, a Go module dependency), never
  // inside anything content-scanning above actually looks at — so
  // Tailwind never generated CSS for w-5/h-5 at all, and the theme
  // toggle icon rendered at zero size. Functionally clickable the
  // whole time, but invisible — plausibly the real explanation for a
  // direct report of "we lost the theme switcher." Safelisted here
  // rather than pointing content-scanning at the module cache path
  // directly, which would be version-specific and not portable across
  // machines/CI. Extend this list if minty's own generated markup
  // ever uses additional classes xoluman's own source doesn't
  // otherwise reference.
  safelist: ['w-5', 'h-5'],
  theme: { extend: {} },
  plugins: [],
}
