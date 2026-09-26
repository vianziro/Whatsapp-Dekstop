'use strict';
// Executes the real injected init script against a WhatsApp-Web-shaped DOM.
// Two invariants are asserted:
//   1. no single failure locks the user out of Settings;
//   2. the spreadsheet preview's HTML sanitizer neutralizes attribute-breakout
//      payloads carried in cell values.
//
// Usage: node init_script_harness.js <path-to-init-script.js>
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

// Two DOM shapes. The bare one has no header, so the injected toolbar control
// cannot mount and the last-resort button takes over. The header variant looks
// like real WhatsApp Web, which is the shape that exposed the duplicate-gear
// bug: toolbar control present AND rail fallback present simultaneously.
const HEAD_BARE = '<!doctype html><html><head></head><body><div id="app"><div id="side"></div></div></body></html>';
const HEAD_WITH_HEADER =
  '<!doctype html><html><head></head><body><div id="app"><div id="side">' +
  '<header><div></div><div id="wa-header-actions"></div></header>' +
  '</div></div></body></html>';
const BRIDGES = ['getDownloadDirNative', 'openDownloadDirNative', 'sendNativeNotification', 'openExternalLink'];

// jsdom reports 0x0 for every getBoundingClientRect, which would make the real
// visibility check treat every element as hidden. Return a plausible rect for
// elements that are not explicitly hidden, so the script's own isElementVisible
// logic is what decides what the user sees.
function patchLayout(window) {
  window.Element.prototype.getBoundingClientRect = function() {
    const cs = window.getComputedStyle(this);
    const hidden = cs.display === 'none' || cs.visibility === 'hidden' || cs.opacity === '0';
    const size = hidden ? 0 : 40;
    return { x: 0, y: 0, width: size, height: size, top: 0, left: 0, right: size, bottom: size };
  };
}

// The rail fallback is mounted from a requestAnimationFrame callback, so the
// DOM must be sampled after at least one frame. Sampling synchronously right
// after eval made an earlier version of this check pass even when two gears
// were on screen.
function nextFrame(window) {
  return new Promise((resolve) => {
    if (window.requestAnimationFrame) window.requestAnimationFrame(() => resolve());
    else setTimeout(resolve, 20);
  });
}

async function run(transform, envMutate, head) {
  const src = transform ? transform(script) : script;
  const dom = new JSDOM(head || HEAD_BARE, {
    runScripts: 'outside-only',
    pretendToBeVisual: true,
    url: 'https://web.whatsapp.com/',
    virtualConsole: new VirtualConsole(),
  });
  const { window } = dom;
  for (const n of BRIDGES) window[n] = () => Promise.resolve('');
  patchLayout(window);
  if (envMutate) envMutate(window);

  let threw = null;
  try { window.eval(src); } catch (e) { threw = e; }
  // Let the deferred entry-point mounts run before inspecting the DOM.
  await nextFrame(window);
  await nextFrame(window);
  await new Promise((r) => setTimeout(r, 30));

  const doc = window.document;
  const SETTINGS_IDS = ['wa-emergency-settings-btn', 'wa-settings-fallback-btn', 'wa-toolbar-settings-btn'];
  const entryPoints = SETTINGS_IDS.filter((id) => doc.getElementById(id));
  // Ids the user can actually see, judged by the same rules the script itself
  // uses: an element hidden with display:none / visibility:hidden / opacity:0
  // does not count. Presence alone is not enough — the duplicate-gear bug had
  // two elements present AND visible at once.
  const isShown = (el) => {
    if (!el) return false;
    const cs = window.getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden' || cs.opacity === '0') return false;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  };
  const visibleEntryPoints = SETTINGS_IDS.filter((id) => isShown(doc.getElementById(id)));

  window.dispatchEvent(new window.KeyboardEvent('keydown', { key: ',', ctrlKey: true, bubbles: true, cancelable: true }));
  const opened = doc.getElementById('wa-settings-overlay') || doc.getElementById('wa-recovery-overlay');

  return {
    threw: threw ? String(threw).split('\n')[0] : 'no',
    entryPoints,
    visibleEntryPoints,
    opened: opened ? opened.id : null,
    recoverable: (typeof window.__waRecoverable === 'function') ? window.__waRecoverable() : [],
  };
}

const cases = [
  ['baseline', null, null, script],
  ['early module failure', (s) => s.replace('\t\t// Emulate window.chrome',
    '\t\tthrow new Error("injected early failure");\n\t\t// Emulate window.chrome'), null, script],
  ['modal failure', (s) => s.replace('window.showSettingsModal = function() {',
    "window.showSettingsModal = function() { throw new Error('injected modal failure');"), null, script],
  ['localStorage denied', null,
    (w) => Object.defineProperty(w, 'localStorage', {
      get() { throw new Error('SecurityError: access denied'); }, configurable: true,
    }), script],
  // The real WhatsApp Web shape: a header exists, so the in-flow toolbar
  // control mounts. Before the fix the rail fallback mounted alongside it and
  // the user saw two identical gears.
  ['whatsapp header present', null, null, script, HEAD_WITH_HEADER],
  // Pre-DOM injection (WebView2 AddScriptToExecuteOnDocumentCreated) where
  // document.head and document.documentElement are both null at eval time.
  ['null head and docEl', null,
    (w) => {
      let active = true;
      const realHead = w.document.head;
      const realDocEl = w.document.documentElement;
      Object.defineProperty(w.document, 'head', {
        get() { return active ? null : realHead; },
        configurable: true,
      });
      Object.defineProperty(w.document, 'documentElement', {
        get() { return active ? null : realDocEl; },
        configurable: true,
      });
      setTimeout(() => { active = false; }, 20);
    }, script],
];

// The spreadsheet preview builds its table with XLSX.utils.sheet_to_html, which
// escapes cell text but writes the raw value into a data-v attribute. A cell
// whose value contains a double quote closes that attribute and injects markup,
// so the sanitizer wrapping it is a security boundary. Audit it here, in the
// jsdom that is already loaded, instead of paying for a second jsdom startup in
// the Go test.
function checkSpreadsheetSanitizer() {
  const failures = [];
  const start = script.indexOf('function sanitizeSheetHtml(html) {');
  if (start === -1) return ['sanitizeSheetHtml helper is missing from the init script'];
  const end = script.indexOf('\n\t\t}\n', start);
  if (end === -1) return ['sanitizeSheetHtml helper is truncated'];
  const fn = script.slice(start, end + '\n\t\t}'.length);

  const dom = new JSDOM('<!doctype html><html><body></body></html>');
  const sanitize = new Function('document', 'return (' + fn + ')')(dom.window.document);

  const BANNED = 'script,style,img,svg,iframe,frame,object,embed,link,meta,base,form,input,button,textarea,select,audio,video,source,track,math,template';
  const audit = (html) => {
    const d = new JSDOM('<!doctype html><html><body>' + html + '</body></html>');
    const doc = d.window.document;
    let bad = doc.querySelectorAll(BANNED).length;
    doc.querySelectorAll('*').forEach((el) => {
      for (const a of Array.from(el.attributes)) {
        if (/^on/i.test(a.name) || /^(src|href|srcdoc|xlink:href)$/i.test(a.name)) bad++;
      }
    });
    return bad;
  };
  const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  const payloads = [
    '" onmouseover="alert(1)',
    '"><img src=x onerror=alert(1)>',
    '"><iframe src=javascript:alert(1)>',
    '<svg onload=alert(1)>',
  ];
  for (const p of payloads) {
    const cell = '<table id="wa-xlsx-table"><tr><td data-t="s" data-v="' + p + '" id="A1">' + esc(p) + '</td></tr></table>';
    const bad = audit(sanitize(cell));
    if (bad) failures.push(`cell ${JSON.stringify(p)} leaves ${bad} dangerous node(s)/attribute(s)`);
    else console.log(`GREEN  ${'spreadsheet cell escaped'.padEnd(22)} payload=${JSON.stringify(p)}`);
  }

  const ok = sanitize('<table id="wa-xlsx-table"><tr><td id="A1">Revenue</td><td id="B1">42</td></tr></table>');
  if (ok.indexOf('Revenue') === -1 || ok.indexOf('42') === -1) failures.push('sanitizer dropped legitimate cell text');
  if (ok.indexOf('id="wa-xlsx-table"') === -1) failures.push('sanitizer dropped the table id');
  return failures;
}

async function main() {
  let failures = 0;
  for (const [label, transform, envMutate, , head] of cases) {
    const r = await run(transform, envMutate, head);
    const ok = r.entryPoints.length > 0 && !!r.opened;
    if (!ok) failures++;
    console.log(`${ok ? 'GREEN' : 'RED  '}  ${label.padEnd(22)} entry=${r.entryPoints.join(',') || 'NONE'} opened=${r.opened || 'NOTHING'} uncaught=${r.threw} recoverable=[${(r.recoverable || []).join('; ')}]`);

    const expectedOverlay = (label.includes('failure')) ? 'wa-recovery-overlay' : 'wa-settings-overlay';
    const overlayOk = r.opened === expectedOverlay;
    if (!overlayOk) {
      failures++;
      console.log(`RED    ${(label + ' overlay').padEnd(22)} got=${r.opened} (expected ${expectedOverlay})`);
    }

    // Regression: a settings control hidden with display:none is fine, but two
    // simultaneously visible gears are not — that is the duplicate button users
    // hit when the header control and the rail fallback were both on screen.
    const visible = r.visibleEntryPoints || [];
    if (visible.length > 1) {
      failures++;
      console.log(`RED    ${'duplicate settings gear'.padEnd(22)} visible=${visible.join(',')} (expected at most one)`);
    } else {
      console.log(`GREEN  ${'single settings gear'.padEnd(22)} visible=${visible.join(',') || 'NONE'}`);
    }
  }

  for (const reason of checkSpreadsheetSanitizer()) {
    failures++;
    console.log(`RED    ${'spreadsheet sanitizer'.padEnd(22)} ${reason}`);
  }

  if (failures) {
    console.log(`\nFAIL: ${failures} scenario(s) leave the user with no (or duplicate) Settings entry point, or expose the spreadsheet preview.`);
    process.exit(1);
  }
  console.log('\nPASS: no single injected-script failure removes the Settings entry point or the Ctrl+, shortcut, and the spreadsheet sanitizer neutralizes cell markup.');
  process.exit(0);
}

main().catch((e) => {
  console.error('harness error:', e && e.stack ? e.stack : e);
  process.exit(1);
});
