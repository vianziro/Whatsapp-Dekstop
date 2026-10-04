'use strict';
// Exercises the idle memory-reload gate (window.waIdleReloadAllowed) that the
// native macOS watcher consults before it reloads the page.
//
// The gate is fail-closed on purpose. A reload that slips through while the
// user is looking at the window, or while an upload is in flight, throws away
// work the user can see -- and the reload exists only to hand back memory that
// nobody is using at that moment. Every check below therefore asserts a refusal
// unless all of the safe conditions hold together.
//
// Usage: node idle_reload_harness.js <path-to-init-script.js>
// Exits 0 on pass. Prints a JSON {"skipped": "..."} line and exits 0 when jsdom
// is unavailable, so the Go test can skip instead of failing.

const fs = require('fs');

let JSDOM, VirtualConsole;
try {
  ({ JSDOM, VirtualConsole } = require('jsdom'));
} catch (e) {
  console.log(JSON.stringify({ skipped: 'jsdom is not installed in this environment' }));
  process.exit(0);
}

const scriptPath = process.argv[2];
if (!scriptPath) {
  console.log(JSON.stringify({ skipped: 'no script path given' }));
  process.exit(0);
}
const script = fs.readFileSync(scriptPath, 'utf8');

// The chat list is what the gate reads to tell "loaded and signed in" from
// "still on the linking screen".
const HEAD_LOADED =
  '<!doctype html><html><head><title>WhatsApp</title></head><body>' +
  '<div id="app"><div id="side"><div id="pane-side"></div></div><div id="main"></div></div>' +
  '</body></html>';
const HEAD_LINKING = '<!doctype html><html><head></head><body><div id="app"></div></body></html>';

const BRIDGES = ['getDownloadDirNative', 'openDownloadDirNative', 'sendNativeNotification', 'openExternalLink'];

// jsdom reports 0x0 for every rect, which would make the script's own
// visibility helpers treat every element as hidden. Return a plausible rect so
// those helpers decide, rather than jsdom's default.
function patchLayout(window) {
  window.Element.prototype.getBoundingClientRect = function() {
    const cs = window.getComputedStyle(this);
    const hidden = cs.display === 'none' || cs.visibility === 'hidden' || cs.opacity === '0';
    const size = hidden ? 0 : 40;
    return { x: 0, y: 0, width: size, height: size, top: 0, left: 0, right: size, bottom: size };
  };
}

function makeWindow(html) {
  const dom = new JSDOM(html, {
    runScripts: 'outside-only',
    pretendToBeVisual: true,
    url: 'https://web.whatsapp.com/',
    virtualConsole: new VirtualConsole(),
  });
  const { window } = dom;
  for (const n of BRIDGES) window[n] = () => Promise.resolve('');
  window.waDiagNative = () => true;
  // jsdom has no createObjectURL, and the script's interceptor calls the
  // original outside its try block, so the stub must exist before eval.
  window.URL.createObjectURL = () => 'blob:jsdom/idle-reload';
  window.URL.revokeObjectURL = () => {};
  if (!window.TextDecoder && typeof TextDecoder === 'function') window.TextDecoder = TextDecoder;
  patchLayout(window);
  window.eval(script);
  return window;
}

// document.hidden and navigator.onLine are read-only in jsdom, so both are
// redefined to drive the gate.
function setHidden(window, hidden) {
  Object.defineProperty(window.document, 'hidden', { configurable: true, get: () => hidden });
  Object.defineProperty(window.document, 'visibilityState', {
    configurable: true,
    get: () => (hidden ? 'hidden' : 'visible'),
  });
}

function setOnline(window, online) {
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, get: () => online });
}

function gate(window) {
  if (typeof window.waIdleReloadAllowed !== 'function') return 'MISSING';
  return window.waIdleReloadAllowed();
}

const results = [];
function check(name, actual, expected) {
  results.push({ name, actual, expected, ok: actual === expected });
}

function withWindow(html, fn) {
  const window = makeWindow(html);
  try {
    fn(window);
  } finally {
    window.close();
  }
}

// 1. The gate must exist at all: without it the native watcher can never reload
//    and the whole feature is silently dead.
withWindow(HEAD_LOADED, (window) => {
  check('gate is defined', typeof window.waIdleReloadAllowed, 'function');
});

// 2. Visible window refuses, even with every other condition satisfied.
withWindow(HEAD_LOADED, (window) => {
  setHidden(window, false);
  setOnline(window, true);
  check('visible window refuses', gate(window), false);
});

// 3. The one combination that is allowed: hidden, online, loaded, idle.
withWindow(HEAD_LOADED, (window) => {
  setHidden(window, true);
  setOnline(window, true);
  check('hidden + idle + loaded allows', gate(window), true);
});

// 4. Offline refuses: the reload would replace a working page with an error.
withWindow(HEAD_LOADED, (window) => {
  setHidden(window, true);
  setOnline(window, false);
  check('offline refuses', gate(window), false);
});

// 5. A transfer in flight refuses. This is the data-loss case: reloading
//    mid-upload aborts it and the message never arrives.
for (const [label, html] of [
  ['progress bar', '<div role="progressbar"></div>'],
  ['busy region', '<div aria-busy="true"></div>'],
  ['progress testid', '<div data-testid="media-upload-progress"></div>'],
]) {
  withWindow(HEAD_LOADED, (window) => {
    setHidden(window, true);
    setOnline(window, true);
    const busy = window.document.createElement('div');
    busy.innerHTML = html;
    window.document.body.appendChild(busy.firstChild);
    check(label + ' refuses', gate(window), false);
  });
}

// 6. An open document preview refuses; the user is mid-read even if the window
//    has been hidden for a while.
withWindow(HEAD_LOADED, (window) => {
  setHidden(window, true);
  setOnline(window, true);
  const preview = window.document.createElement('div');
  preview.setAttribute('data-testid', 'document-preview');
  window.document.body.appendChild(preview);
  check('open document preview refuses', gate(window), false);
});

// 7. Still linking refuses: there is no media to reclaim yet.
withWindow(HEAD_LINKING, (window) => {
  setHidden(window, true);
  setOnline(window, true);
  check('not loaded refuses', gate(window), false);
});

// 8. Every refusal must name the condition that tripped. A bare false is what
//    made "the memory never came back" unactionable: the native log could only
//    say "the page refused", which is true of a dozen different situations.
function reason(window) {
  if (typeof window.waIdleReloadRefusalReason !== 'function') return 'MISSING';
  return window.waIdleReloadRefusalReason();
}

withWindow(HEAD_LOADED, (window) => {
  setHidden(window, true);
  setOnline(window, true);
  check('allowed case gives no reason', reason(window), '');
});

withWindow(HEAD_LOADED, (window) => {
  setHidden(window, false);
  setOnline(window, true);
  check('visible window names the condition', reason(window), 'the window is not hidden');
});

withWindow(HEAD_LOADED, (window) => {
  setHidden(window, true);
  setOnline(window, false);
  check('offline names the condition', reason(window), 'the browser reports offline');
});

withWindow(HEAD_LOADED, (window) => {
  setHidden(window, true);
  setOnline(window, true);
  const busy = window.document.createElement('div');
  busy.setAttribute('role', 'progressbar');
  window.document.body.appendChild(busy);
  check('busy page names the condition', reason(window),
    'a progress indicator or open dialog is on screen');
});

withWindow(HEAD_LINKING, (window) => {
  setHidden(window, true);
  setOnline(window, true);
  check('unloaded page names the condition', reason(window),
    'the chat list is not loaded (still starting up, or not signed in)');
});

const failed = results.filter((r) => !r.ok);
for (const r of results) {
  console.log(`${r.ok ? 'ok  ' : 'FAIL'}  ${r.name}: got ${JSON.stringify(r.actual)}, want ${JSON.stringify(r.expected)}`);
}
if (failed.length) {
  console.error(`${failed.length} idle-reload gate check(s) failed`);
  process.exit(1);
}
console.log('all idle-reload gate checks passed');
