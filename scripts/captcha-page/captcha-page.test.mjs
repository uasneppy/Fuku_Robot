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
