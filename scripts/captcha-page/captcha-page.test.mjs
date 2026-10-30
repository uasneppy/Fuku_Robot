// Tests of the Turnstile captcha page's script. Run with: node --test scripts/captcha-page/*.test.mjs
// (Node 22 reads the argument as a glob, so a bare directory name does not run anything.)
//
// The script is read verbatim out of the embedded page and run in a node:vm context whose
// window is the context itself, so what is tested is the file the bot serves. Nothing is
// installed: only Node's built-in runner and modules are used. This suite is not part of
// `make test`; what only a real Telegram client can show is on the spike checklist.

import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import fs from 'node:fs';
import { createHash, webcrypto } from 'node:crypto';
import { fileURLToPath } from 'node:url';

const pagePath = fileURLToPath(new URL('../../alita/utils/captchaweb/page/captcha.html', import.meta.url));
const pageHTML = fs.readFileSync(pagePath, 'utf8');
const scriptMatch = /<script[^>]*id="captcha-main"[^>]*>([\s\S]*?)<\/script>/.exec(pageHTML);
assert.ok(scriptMatch, 'the page has no script with id captcha-main');
const pageScript = scriptMatch[1];

// loadPage runs the page script in a fresh context. The stub document never fires
// DOMContentLoaded, so nothing boots until a test calls CaptchaPage.boot().
function loadPage(extra = {}) {
  const ctx = {
    document: {
      readyState: 'loading',
      addEventListener() {},
      documentElement: {},
      title: '',
    },
    navigator: { language: 'en-US' },
    location: { search: '', hash: '', reload() {} },
    crypto: webcrypto,
    Promise,
    ...extra,
  };
  ctx.window = ctx;
  vm.createContext(ctx);
  vm.runInContext(pageScript, ctx, { filename: 'captcha-main.js' });
  return ctx;
}

const page = loadPage().CaptchaPage;

test('page script exposes the pure functions', () => {
  for (const name of [
    'resolveLanguage', 'readChallenge', 'cdataFor', 'turnstileLanguage', 'stateForAnswer',
    'parseColor', 'contrastRatio', 'widgetSizeFor', 'createSubmitLatch',
  ]) {
    assert.equal(typeof page[name], 'function', name);
  }
});

test('language', () => {
  assert.equal(page.resolveLanguage('pt-BR'), 'pt');
  assert.equal(page.resolveLanguage('in'), 'id');
  assert.equal(page.resolveLanguage('ID'), 'id');
  assert.equal(page.resolveLanguage('de'), 'en');
  assert.equal(page.resolveLanguage('hi_IN'), 'hi');
  assert.equal(page.resolveLanguage(undefined, 'fr-CA'), 'fr');
  assert.equal(page.resolveLanguage('', 'ru'), 'ru');
  assert.equal(page.resolveLanguage(undefined), 'en');
  assert.equal(page.resolveLanguage(undefined, undefined), 'en');
  assert.equal(page.resolveLanguage(42, null), 'en');
  for (const code of ['en', 'es', 'fr', 'hi', 'id', 'pt', 'ru']) {
    assert.equal(page.resolveLanguage(code), code);
  }
});

test('challenge', () => {
  assert.equal(page.readChallenge({ start_param: 'abc_-1' }, ''), 'abc_-1');
  assert.equal(page.readChallenge({ start_param: '' }, '?tgWebAppStartParam=tok123'), 'tok123');
  assert.equal(page.readChallenge({}, '?x=1&tgWebAppStartParam=tok123'), 'tok123');
  assert.equal(page.readChallenge(undefined, '&tgWebAppStartParam=from_hash'), 'from_hash');
  assert.equal(page.readChallenge({ start_param: 'has space' }, ''), null);
  assert.equal(page.readChallenge({ start_param: 'a'.repeat(64) }, ''), 'a'.repeat(64));
  assert.equal(page.readChallenge({ start_param: 'a'.repeat(65) }, ''), null);
  assert.equal(page.readChallenge({ start_param: '' }, ''), null);
  assert.equal(page.readChallenge({}, '?tgWebAppStartParam=%20bad'), null);
  assert.equal(page.readChallenge({}, '?tgWebAppStartParam=%E0%A4%A'), null);
  // a malformed start_param is never papered over by the query parameter
  assert.equal(page.readChallenge({ start_param: 'bad value' }, '?tgWebAppStartParam=tok123'), null);
});

test('cData', async () => {
  const got = await page.cdataFor('cd1:', 'tok123');
  const want = createHash('sha256').update('cd1:tok123').digest('hex');
  assert.equal(got, want);
  assert.equal(got.length, 64);
  assert.equal(got, got.toLowerCase());
  await assert.rejects(() => loadPage({ crypto: {} }).CaptchaPage.cdataFor('cd1:', 'tok123'));
});

test('turnstile language', () => {
  for (const code of ['en', 'es', 'fr', 'hi', 'id', 'pt', 'ru']) {
    assert.equal(page.turnstileLanguage(code), code);
  }
  assert.equal(page.turnstileLanguage('de'), 'auto');
  assert.equal(page.turnstileLanguage(undefined), 'auto');
});

test('answers', () => {
  const ans = (status, body) => page.stateForAnswer({ status, body });
  const pick = (r) => [r.state, r.titleKey, r.bodyKey];

  assert.deepEqual(pick(ans(200, { result: 'passed' })),
    ['passed', 'captcha_web_passed_title', 'captcha_web_passed_body']);
  assert.deepEqual(pick(ans(200, { result: 'failed', action: 'ban' })),
    ['failed', 'captcha_web_failed_title', 'captcha_web_failed_body_ban']);
  assert.deepEqual(pick(ans(200, { result: 'failed', action: 'mute' })),
    ['failed', 'captcha_web_failed_title', 'captcha_web_failed_body_mute']);
  assert.deepEqual(pick(ans(200, { result: 'failed', action: 'kick' })),
    ['failed', 'captcha_web_failed_title', 'captcha_web_failed_body_kick']);
  assert.deepEqual(pick(ans(200, { result: 'expired', action: 'mute' })),
    ['expired', 'captcha_web_expired_title', 'captcha_web_expired_body_mute']);
  assert.deepEqual(pick(ans(200, { result: 'expired', action: 'ban' })),
    ['expired', 'captcha_web_expired_title', 'captcha_web_expired_body_ban']);

  // a missing or unknown action is the group default, a kick
  assert.equal(ans(200, { result: 'failed' }).bodyKey, 'captcha_web_failed_body_kick');
  assert.equal(ans(200, { result: 'failed', action: 'nuke' }).bodyKey, 'captcha_web_failed_body_kick');
  assert.equal(ans(200, { result: 'expired' }).bodyKey, 'captcha_web_expired_body_kick');
  assert.equal(ans(200, { result: 'expired', action: 7 }).bodyKey, 'captcha_web_expired_body_kick');

  // invalid and not_for_you are one state, with 200 or 400
  for (const [status, result] of [[200, 'invalid'], [200, 'not_for_you'], [400, 'invalid'], [400, 'not_for_you']]) {
    assert.equal(ans(status, { result }).state, 'invalid', `${status} ${result}`);
  }

  // everything that is a transport or server problem is a retry, never a failed solve
  assert.equal(ans(200, { result: 'retry' }).state, 'retry');
  assert.equal(ans(503, { result: 'retry' }).state, 'retry');
  assert.equal(ans(429, null).state, 'retry');
  assert.equal(ans(429, { result: 'invalid' }).state, 'retry');
  assert.equal(ans(500, null).state, 'retry');
  assert.equal(ans(502, { result: 'failed', action: 'ban' }).state, 'retry');
  assert.equal(ans(200, { result: 'something_new' }).state, 'retry');
  assert.equal(ans(200, {}).state, 'retry');
  assert.equal(ans(200, null).state, 'retry');
  assert.equal(page.stateForAnswer(null).state, 'retry');
  assert.equal(page.stateForAnswer(undefined).state, 'retry');

  // the page never reports a pass the server did not send with status 200
  assert.equal(ans(500, { result: 'passed' }).state, 'retry');
  assert.equal(ans(400, { result: 'passed' }).state, 'retry');
});

test('contrast', () => {
  assert.ok(Math.abs(page.contrastRatio('#000000', '#ffffff') - 21) < 0.01);
  assert.ok(Math.abs(page.contrastRatio([255, 255, 255], [0, 0, 0]) - 21) < 0.01);
  assert.equal(page.contrastRatio('nonsense', '#ffffff'), null);

  assert.deepEqual(Array.from(page.parseColor('#fff')), [255, 255, 255]);
  assert.deepEqual(Array.from(page.parseColor('#ffffff')), [255, 255, 255]);
  assert.deepEqual(Array.from(page.parseColor('#2481CC')), [0x24, 0x81, 0xcc]);
  assert.deepEqual(Array.from(page.parseColor('rgb(255, 255, 255)')), [255, 255, 255]);
  assert.deepEqual(Array.from(page.parseColor(' rgba(10, 20, 30, 0.5) ')), [10, 20, 30]);
  assert.equal(page.parseColor('nonsense'), null);
  assert.equal(page.parseColor('rgb(300, 0, 0)'), null);
  assert.equal(page.parseColor(''), null);
  assert.equal(page.parseColor(undefined), null);

  const light = { bg: '#ffffff', surface: '#f1f1f4', text: '#000000', hint: '#5f6368', accent: '#1c6fc2', 'on-accent': '#ffffff' };
  // Telegram's default blue with white text is about 4.1:1 and is replaced as a pair
  const tgBlue = { ...light, accent: '#2481cc' };
  assert.ok(page.contrastRatio(tgBlue.accent, tgBlue['on-accent']) < 4.5);
  const replaced = page.guardTheme(tgBlue, 'dark');
  assert.equal(replaced.accent, '#8ab4f8');
  assert.equal(replaced['on-accent'], '#0b1a2e');
  assert.equal(replaced.text, undefined);
  assert.equal(replaced.bg, undefined);
  // under the light scheme the white label already equals the fallback, so only the fill changes
  const lightReplaced = page.guardTheme(tgBlue, 'light');
  assert.equal(lightReplaced.accent, '#1c6fc2');
  assert.equal(lightReplaced['on-accent'], undefined);

  // a readable 5:1 pair is kept
  assert.ok(page.contrastRatio(light.accent, light['on-accent']) >= 4.5);
  assert.equal(Object.keys(page.guardTheme(light, 'light')).length, 0);

  // a hint colour too pale for the panel goes back to the fallback of the active scheme
  const pale = page.guardTheme({ ...light, hint: '#cccccc' }, 'dark');
  assert.equal(pale.hint, '#aaaaaa');
  assert.equal(pale.surface, '#181818');
});

test('widget size', () => {
  assert.equal(page.widgetSizeFor(288), 'compact');
  assert.equal(page.widgetSizeFor(299), 'compact');
  assert.equal(page.widgetSizeFor(300), 'flexible');
  assert.equal(page.widgetSizeFor(360), 'flexible');
});

test('one POST per token', () => {
  const calls = [];
  const latch = page.createSubmitLatch((token) => calls.push(token));
  assert.equal(latch('tok-1'), true);
  assert.equal(latch('tok-1'), false);
  assert.equal(latch('tok-1'), false);
  assert.deepEqual(calls, ['tok-1']);
  assert.equal(latch.isBusy(), true);
  latch.release();
  assert.equal(latch.isBusy(), false);
  assert.equal(latch('tok-2'), true);
  assert.deepEqual(calls, ['tok-1', 'tok-2']);
});

// ---------------------------------------------------------------------------------------
// boot: the real boot path against stub DOM, Telegram, Turnstile, fetch and timers.
// ---------------------------------------------------------------------------------------

const PAGE_KEYS = [
  'captcha_web_page_title', 'captcha_web_loading_title', 'captcha_web_loading_body',
  'captcha_web_ready_title', 'captcha_web_ready_body', 'captcha_web_widget_label',
  'captcha_web_verifying_title', 'captcha_web_verifying_body', 'captcha_web_passed_title',
  'captcha_web_passed_body', 'captcha_web_failed_title', 'captcha_web_failed_body_kick',
  'captcha_web_failed_body_ban', 'captcha_web_failed_body_mute', 'captcha_web_expired_title',
  'captcha_web_expired_body_kick', 'captcha_web_expired_body_ban', 'captcha_web_expired_body_mute',
  'captcha_web_retry_title', 'captcha_web_retry_body', 'captcha_web_retry_button',
  'captcha_web_invalid_title', 'captcha_web_invalid_body', 'captcha_web_unsupported_title',
  'captcha_web_unsupported_body', 'captcha_web_close_button',
];

const INIT_DATA = 'query_id=SECRET_INIT_DATA&hash=abc';
const CHALLENGE = 'chal_1-x';
const WIDGET_TOKEN = 'WIDGET_TOKEN_VALUE';

function tablesJSON() {
  const tables = {};
  for (const lang of ['en', 'pt']) {
    tables[lang] = {};
    for (const key of PAGE_KEYS) { tables[lang][key] = `${lang.toUpperCase()} ${key}`; }
  }
  return JSON.stringify(tables);
}

// Every text and attribute a stub element receives is logged, so the leak test can read it.
function makeEl(log, extra = {}) {
  let text = '';
  const el = {
    hidden: false,
    attrs: {},
    focused: 0,
    listeners: {},
    clientWidth: 288,
    setAttribute(k, v) { el.attrs[k] = String(v); log.push(String(v)); },
    getAttribute(k) { return Object.prototype.hasOwnProperty.call(el.attrs, k) ? el.attrs[k] : null; },
    addEventListener(type, fn) { el.listeners[type] = fn; },
    focus() { el.focused += 1; },
    ...extra,
  };
  Object.defineProperty(el, 'textContent', {
    get() { return text; },
    set(v) { text = String(v); log.push(text); },
  });
  return el;
}

function startPage(opts = {}) {
  const log = [];
  const timers = [];
  let now = 0;
  let nextId = 1;
  const calls = { console: [], fetch: [], render: [], reset: [], close: 0, reload: 0, abort: 0, ready: 0, haptic: [], props: [] };

  const els = {
    'captcha-root': makeEl(log, {}),
    i18n: makeEl(log, {}),
    icon: makeEl(log, {}),
    status: makeEl(log, { classList: { remove() {}, add() {} }, offsetWidth: 0 }),
    headline: makeEl(log, {}),
    'status-body': makeEl(log, {}),
    widget: makeEl(log, {}),
    action: makeEl(log, { hidden: true }),
  };
  els['captcha-root'].attrs = { 'data-sitekey': 'SITEKEY', 'data-action': 'captcha_join', 'data-cdata-prefix': 'cd1:' };
  els.i18n.textContent = tablesJSON();
  log.length = 0;

  const root = makeEl(log, {
    style: {
      setProperty(k, v) { calls.props.push([k, v]); },
      removeProperty() {},
    },
  });

  const fetchResponders = [];
  const ctx = {
    document: {
      readyState: 'loading',
      addEventListener() {},
      documentElement: root,
      title: '',
      getElementById: (id) => els[id],
    },
    navigator: { language: opts.navigatorLanguage ?? 'en-US' },
    location: { search: '', hash: '', reload() { calls.reload += 1; }, ...(opts.location || {}) },
    crypto: webcrypto,
    Promise,
    innerWidth: 320,
    console: new Proxy({}, { get: (_t, name) => (...args) => { calls.console.push([String(name), args]); } }),
    setTimeout(fn, ms) {
      const id = nextId++;
      timers.push({ id, at: now + (ms || 0), fn });
      return id;
    },
    clearTimeout(id) {
      const i = timers.findIndex((t) => t.id === id);
      if (i >= 0) { timers.splice(i, 1); }
    },
    AbortController: class {
      constructor() { this.signal = { aborted: false }; }
      abort() { calls.abort += 1; this.signal.aborted = true; }
    },
    fetch(url, init) {
      calls.fetch.push({ url, init });
      return new Promise((resolve, reject) => { fetchResponders.push({ resolve, reject, url, init }); });
    },
    getComputedStyle: () => ({ getPropertyValue: (name) => (opts.themeValues || {})[name] ?? '#ffffff' }),
  };
  ctx.window = ctx;

  if (opts.telegram !== false) {
    ctx.Telegram = {
      WebApp: {
        initData: opts.initData ?? INIT_DATA,
        initDataUnsafe: { user: { language_code: opts.userLanguage ?? 'pt-BR' }, start_param: opts.startParam ?? CHALLENGE },
        colorScheme: 'dark',
        ready() { calls.ready += 1; },
        close() { calls.close += 1; },
        isVersionAtLeast: () => true,
        disableVerticalSwipes() {},
        setHeaderColor() {},
        setBackgroundColor() {},
        onEvent() {},
        HapticFeedback: { notificationOccurred(kind) { calls.haptic.push(kind); } },
      },
    };
  }
  if (opts.turnstile !== false) {
    ctx.turnstile = {
      render(container, options) { calls.render.push({ container, options }); return 'widget-1'; },
      reset(id) { calls.reset.push(id); },
    };
  }

  vm.createContext(ctx);
  vm.runInContext(pageScript, ctx, { filename: 'captcha-main.js' });

  const env = {
    ctx, els, root, calls, log, fetchResponders,
    boot() { ctx.CaptchaPage.boot(); },
    advance(ms) {
      const target = now + ms;
      for (;;) {
        timers.sort((a, b) => a.at - b.at || a.id - b.id);
        const next = timers[0];
        if (!next || next.at > target) { break; }
        timers.shift();
        now = next.at;
        next.fn();
      }
      now = target;
    },
    // webcrypto digests finish on a worker thread, so a few macrotask turns are not enough
    // to be sure; a short real delay is.
    async flush() {
      for (let i = 0; i < 3; i++) { await new Promise((r) => setImmediate(r)); }
      await new Promise((r) => globalThis.setTimeout(r, 30));
    },
    text() { return [els.headline.textContent, els['status-body'].textContent]; },
    async loadTurnstile() { ctx.captchaTurnstileReady(); await env.flush(); },
    answer(status, body) {
      const r = fetchResponders[fetchResponders.length - 1];
      r.resolve({ status, json: async () => body });
    },
  };
  return env;
}

test('boot: no Telegram object after 8 s is unsupported', async () => {
  const env = startPage({ telegram: false });
  env.boot();
  assert.deepEqual(env.text(), ['EN captcha_web_loading_title', 'EN captcha_web_loading_body']);
  env.advance(7999);
  assert.equal(env.els.headline.textContent, 'EN captcha_web_loading_title');
  env.advance(1);
  assert.deepEqual(env.text(), ['EN captcha_web_unsupported_title', 'EN captcha_web_unsupported_body']);
  assert.equal(env.els.icon.attrs['data-icon'], 'info');
  assert.equal(env.els.action.hidden, true);
  // the 10 s Turnstile timer cannot move a terminal state
  env.advance(5000);
  assert.equal(env.els.headline.textContent, 'EN captcha_web_unsupported_title');
  assert.equal(env.calls.render.length, 0);
  assert.equal(env.calls.fetch.length, 0);
});

test('boot: Telegram that appears late is used', async () => {
  const env = startPage({ telegram: false });
  env.boot();
  env.advance(2000);
  env.ctx.Telegram = { WebApp: { initData: INIT_DATA, initDataUnsafe: { start_param: CHALLENGE }, colorScheme: 'light' } };
  env.advance(250);
  await env.loadTurnstile();
  assert.equal(env.calls.render.length, 1);
  assert.equal(env.els.headline.textContent, 'EN captcha_web_ready_title');
});

test('boot: empty initData is unsupported', async () => {
  const env = startPage({ initData: '' });
  env.boot();
  assert.deepEqual(env.text(), ['EN captcha_web_unsupported_title', 'EN captcha_web_unsupported_body']);
  await env.loadTurnstile();
  assert.equal(env.calls.render.length, 0);
  assert.equal(env.calls.fetch.length, 0);
});

test('boot: a malformed challenge is invalid with no widget and no POST', async () => {
  for (const startParam of ['bad value', 'a'.repeat(65), '']) {
    const env = startPage({ startParam });
    env.boot();
    await env.loadTurnstile();
    assert.deepEqual(env.text(), ['PT captcha_web_invalid_title', 'PT captcha_web_invalid_body'], startParam);
    assert.equal(env.els.icon.attrs['data-icon'], 'slash');
    assert.equal(env.els.action.hidden, false);
    assert.equal(env.els.action.textContent, 'PT captcha_web_close_button');
    assert.equal(env.els.action.focused, 1);
    assert.equal(env.calls.render.length, 0);
    assert.equal(env.calls.fetch.length, 0);
  }
});

test('boot: a valid challenge renders the widget once with the planned options', async () => {
  const env = startPage();
  env.boot();
  assert.equal(env.root.attrs.lang, 'pt');
  assert.equal(env.root.attrs['data-theme'], 'dark');
  assert.equal(env.calls.ready, 1);
  // cData is computed, but nothing renders until the Turnstile script has loaded
  await env.flush();
  assert.equal(env.calls.render.length, 0);

  await env.loadTurnstile();
  assert.equal(env.calls.render.length, 1);
  const { container, options } = env.calls.render[0];
  assert.equal(container, env.els.widget);
  assert.equal(options.sitekey, 'SITEKEY');
  assert.equal(options.action, 'captcha_join');
  assert.equal(options.cData, createHash('sha256').update(`cd1:${CHALLENGE}`).digest('hex'));
  assert.equal(options.theme, 'dark');
  assert.equal(options.size, 'compact');
  assert.equal(options.language, 'pt');
  assert.equal(options.appearance, 'always');
  assert.equal(options.retry, 'never');
  assert.equal(options['refresh-expired'], 'never');
  assert.equal(options['refresh-timeout'], 'never');
  for (const name of ['callback', 'error-callback', 'timeout-callback', 'expired-callback']) {
    assert.equal(typeof options[name], 'function', name);
  }
  assert.equal(env.els.widget.attrs['data-size'], 'compact');
  assert.equal(env.els.widget.hidden, false);
  assert.deepEqual(env.text(), ['PT captcha_web_ready_title', 'PT captcha_web_ready_body']);
  assert.equal(env.els.icon.attrs['data-icon'], 'shield');
  assert.equal(env.els.widget.attrs['aria-label'], 'PT captcha_web_widget_label');
  assert.equal(env.els.action.hidden, true);
  assert.equal(env.ctx.document.title, 'PT captcha_web_page_title');

  // a second load callback does not render a second widget
  env.ctx.captchaTurnstileReady();
  await env.flush();
  assert.equal(env.calls.render.length, 1);

  // a wide container gets the flexible size
  const wide = startPage();
  wide.els.widget.clientWidth = 340;
  wide.boot();
  await wide.loadTurnstile();
  assert.equal(wide.calls.render[0].options.size, 'flexible');
});

test('boot: one widget token sends exactly one POST, and a pass closes after 2000 ms', async () => {
  const env = startPage();
  env.boot();
  await env.loadTurnstile();
  const { callback } = env.calls.render[0].options;

  callback(WIDGET_TOKEN);
  callback(WIDGET_TOKEN);
  callback('another');
  assert.equal(env.calls.fetch.length, 1);
  const { url, init } = env.calls.fetch[0];
  assert.equal(url, '/captcha/verify');
  assert.equal(init.method, 'POST');
  assert.equal(init.credentials, 'omit');
  assert.equal(init.cache, 'no-store');
  assert.equal(init.headers['Content-Type'], 'application/json');
  assert.deepEqual(JSON.parse(init.body), { init_data: INIT_DATA, turnstile_token: WIDGET_TOKEN, challenge: CHALLENGE });
  assert.ok(init.signal, 'the request carries an abort signal');
  assert.deepEqual(env.text(), ['PT captcha_web_verifying_title', 'PT captcha_web_verifying_body']);
  assert.equal(env.els.widget.hidden, true);
  assert.equal(env.els.action.hidden, true);

  env.answer(200, { result: 'passed' });
  await env.flush();
  assert.deepEqual(env.text(), ['PT captcha_web_passed_title', 'PT captcha_web_passed_body']);
  assert.equal(env.els.icon.attrs['data-icon'], 'check');
  assert.deepEqual(env.calls.haptic, ['success']);
  env.advance(1999);
  assert.equal(env.calls.close, 0);
  env.advance(1);
  assert.equal(env.calls.close, 1);
  // a late widget callback after the pass changes nothing
  callback('late');
  assert.equal(env.calls.fetch.length, 1);
});

test('boot: failed and expired pick the body of the answer action and offer Close', async () => {
  for (const [result, action] of [['failed', 'ban'], ['expired', 'mute'], ['failed', undefined]]) {
    const env = startPage();
    env.boot();
    await env.loadTurnstile();
    env.calls.render[0].options.callback(WIDGET_TOKEN);
    env.answer(200, { result, action });
    await env.flush();
    const suffix = action || 'kick';
    assert.deepEqual(env.text(), [`PT captcha_web_${result}_title`, `PT captcha_web_${result}_body_${suffix}`]);
    assert.equal(env.els.action.hidden, false);
    assert.equal(env.els.action.textContent, 'PT captcha_web_close_button');
    assert.ok(env.els.action.focused >= 1);
    assert.deepEqual(env.calls.haptic, ['error']);
    env.els.action.listeners.click();
    assert.equal(env.calls.close, 1);
    // terminal: no countdown, no auto close
    env.advance(60000);
    assert.equal(env.calls.close, 1);
  }
});

test('boot: a retry answer then Try again resets the widget and allows a new POST', async () => {
  const env = startPage();
  env.boot();
  await env.loadTurnstile();
  const { callback } = env.calls.render[0].options;

  callback(WIDGET_TOKEN);
  env.answer(503, { result: 'retry' });
  await env.flush();
  assert.deepEqual(env.text(), ['PT captcha_web_retry_title', 'PT captcha_web_retry_body']);
  assert.equal(env.els.icon.attrs['data-icon'], 'retry');
  assert.equal(env.els.action.hidden, false);
  assert.equal(env.els.action.textContent, 'PT captcha_web_retry_button');
  assert.equal(env.els.action.focused, 1);

  env.els.action.listeners.click();
  assert.deepEqual(env.calls.reset, ['widget-1']);
  assert.equal(env.calls.reload, 0);
  assert.equal(env.els.headline.textContent, 'PT captcha_web_ready_title');
  assert.equal(env.els.widget.hidden, false);
  assert.equal(env.els.action.hidden, true);

  callback('second-token');
  assert.equal(env.calls.fetch.length, 2);
  assert.equal(JSON.parse(env.calls.fetch[1].init.body).turnstile_token, 'second-token');
});

test('boot: network errors, timeouts and widget problems are a retry, never a failed solve', async () => {
  const rejected = startPage();
  rejected.boot();
  await rejected.loadTurnstile();
  rejected.calls.render[0].options.callback(WIDGET_TOKEN);
  rejected.fetchResponders[0].reject(new Error('offline'));
  await rejected.flush();
  assert.equal(rejected.els.headline.textContent, 'PT captcha_web_retry_title');

  const slow = startPage();
  slow.boot();
  await slow.loadTurnstile();
  slow.calls.render[0].options.callback(WIDGET_TOKEN);
  slow.advance(11999);
  assert.equal(slow.els.headline.textContent, 'PT captcha_web_verifying_title');
  slow.advance(1);
  assert.equal(slow.calls.abort, 1);
  assert.equal(slow.els.headline.textContent, 'PT captcha_web_retry_title');
  // the late answer of the aborted request cannot overwrite the state
  slow.answer(200, { result: 'passed' });
  await slow.flush();
  assert.equal(slow.els.headline.textContent, 'PT captcha_web_retry_title');

  const notJSON = startPage();
  notJSON.boot();
  await notJSON.loadTurnstile();
  notJSON.calls.render[0].options.callback(WIDGET_TOKEN);
  notJSON.fetchResponders[0].resolve({ status: 500, json: async () => { throw new Error('not json'); } });
  await notJSON.flush();
  assert.equal(notJSON.els.headline.textContent, 'PT captcha_web_retry_title');

  for (const name of ['error-callback', 'timeout-callback', 'expired-callback']) {
    const env = startPage();
    env.boot();
    await env.loadTurnstile();
    env.calls.render[0].options[name]();
    assert.equal(env.els.headline.textContent, 'PT captcha_web_retry_title', name);
    assert.equal(env.calls.fetch.length, 0);
  }
});

test('boot: no Turnstile script after 10 s is a retry whose button reloads', async () => {
  const env = startPage({ turnstile: false });
  env.boot();
  await env.flush();
  env.advance(9999);
  assert.equal(env.els.headline.textContent, 'PT captcha_web_loading_title');
  env.advance(1);
  assert.deepEqual(env.text(), ['PT captcha_web_retry_title', 'PT captcha_web_retry_body']);
  env.els.action.listeners.click();
  assert.equal(env.calls.reload, 1);
  assert.equal(env.calls.reset.length, 0);
});

test('boot: the challenge may come from the tgWebAppStartParam query parameter', async () => {
  const env = startPage({ startParam: '', location: { search: '?tgWebAppStartParam=query_tok', hash: '' } });
  env.boot();
  await env.loadTurnstile();
  assert.equal(env.calls.render.length, 1);
  assert.equal(env.calls.render[0].options.cData, createHash('sha256').update('cd1:query_tok').digest('hex'));
});

test('boot: without crypto.subtle the page is unsupported', async () => {
  const env = startPage();
  env.ctx.crypto = {};
  env.boot();
  await env.flush();
  assert.equal(env.els.headline.textContent, 'PT captcha_web_unsupported_title');
  await env.loadTurnstile();
  assert.equal(env.calls.render.length, 0);
});

test('boot: the contrast guard replaces an unreadable button pair', async () => {
  const env = startPage({
    themeValues: {
      '--c-bg': '#ffffff', '--c-surface': '#f1f1f4', '--c-text': '#000000', '--c-hint': '#5f6368',
      '--c-accent': '#2481cc', '--c-on-accent': '#ffffff',
    },
  });
  env.boot();
  const props = Object.fromEntries(env.calls.props);
  // the stub reports colorScheme dark, so the dark fallbacks replace the failing pair
  assert.equal(props['--c-accent'], '#8ab4f8');
  assert.equal(props['--c-on-accent'], '#0b1a2e');
  assert.equal(props['--c-text'], undefined);
});

test('boot: nothing is logged and nothing relayed is ever shown', async () => {
  const env = startPage();
  env.boot();
  await env.loadTurnstile();
  env.calls.render[0].options.callback(WIDGET_TOKEN);
  env.answer(200, { result: 'failed', action: 'ban', detail: 'SECRET_BODY_TEXT', user_id: 123456789 });
  await env.flush();
  env.els.action.listeners.click();
  env.advance(30000);

  assert.deepEqual(env.calls.console, [], 'no console method may be called');
  assert.ok(env.log.length > 5, 'the stub elements recorded the page text');
  for (const secret of [INIT_DATA, 'SECRET_INIT_DATA', WIDGET_TOKEN, CHALLENGE, 'SECRET_BODY_TEXT', '123456789']) {
    for (const entry of env.log) {
      assert.ok(!entry.includes(secret), `a rendered value contains ${secret}: ${entry}`);
    }
  }
  assert.ok(!env.ctx.document.title.includes(WIDGET_TOKEN));
});

test('page source: no console call, no innerHTML, no eval', () => {
  assert.ok(!/\bconsole\s*\./.test(pageScript), 'the script calls console');
  assert.ok(!/\.innerHTML\b|\.outerHTML\b|\beval\s*\(|new\s+Function|document\.write/.test(pageScript));
});
