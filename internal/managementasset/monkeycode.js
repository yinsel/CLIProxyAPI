(() => {
  'use strict';
  const nativeFetch = window.fetch.bind(window);
  let authorization = '';
  const managementURL = value => {
    try {
      const url = new URL(value, location.href);
      return url.origin === location.origin && url.pathname.startsWith('/v0/management/');
    } catch { return false; }
  };
  const remember = (url, header, status) => {
    if (!managementURL(url) || !header) return;
    if (status >= 200 && status < 300) authorization = header;
    else if (status === 401 && authorization === header) authorization = '';
  };
  // Reuse successful same-origin WebUI authentication only in memory.
  window.fetch = async function(input, init) {
    const url = typeof input === 'string' || input instanceof URL ? String(input) : input.url;
    const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined));
    const response = await nativeFetch(input, init);
    remember(url, headers.get('Authorization') || (headers.has('X-Management-Key') ? `Bearer ${headers.get('X-Management-Key')}` : ''), response.status);
    return response;
  };
  const nativeOpen = XMLHttpRequest.prototype.open;
  const nativeSetHeader = XMLHttpRequest.prototype.setRequestHeader;
  const requests = new WeakMap();
  XMLHttpRequest.prototype.open = function(method, url, ...rest) {
    const result = nativeOpen.call(this, method, url, ...rest);
    requests.set(this, { url: String(url), authorization: '' });
    this.addEventListener('loadend', () => {
      const request = requests.get(this);
      if (request) remember(request.url, request.authorization, this.status);
    }, { once: true });
    return result;
  };
  XMLHttpRequest.prototype.setRequestHeader = function(name, value) {
    const result = nativeSetHeader.call(this, name, value);
    const request = requests.get(this);
    if (request && name.toLowerCase() === 'authorization') request.authorization = value;
    if (request && name.toLowerCase() === 'x-management-key' && !request.authorization) request.authorization = `Bearer ${value}`;
    return result;
  };

  function mount() {
    const zh = navigator.language.toLowerCase().startsWith('zh');
    const t = (cn, en) => zh ? cn : en;
    const host = document.createElement('div');
    host.id = 'cpa-monkeycode-settings';
    const root = host.attachShadow({ mode: 'open' });
    root.innerHTML = `
      <style>
        :host { font: 14px/1.5 system-ui, sans-serif; color-scheme: light dark; }
        button, input, select { font: inherit; box-sizing: border-box; }
        button { cursor: pointer; border: 1px solid #7776; border-radius: 8px; padding: 8px 14px; }
        button:disabled { cursor: default; opacity: .5; }
        #open { position: fixed; right: 20px; bottom: 20px; z-index: 1000; background: #2563eb; color: white; box-shadow: 0 2px 8px #0003; }
        dialog { width: min(500px, calc(100vw - 40px)); max-height: 85vh; overflow: auto; border: 1px solid #8886; border-radius: 14px; padding: 24px; }
        dialog::backdrop { background: #0006; }
        h2 { margin: 0; font-size: 20px; }
        p { opacity: .8; }
        label { display: block; margin-top: 16px; }
        input, select { display: block; width: 100%; margin-top: 6px; padding: 10px; border: 1px solid #8888; border-radius: 6px; }
        .actions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 20px; }
        #status { overflow-wrap: anywhere; min-height: 22px; }
        [hidden] { display: none !important; }
      </style>
      <button id="open" type="button">MonkeyCode</button>
      <dialog aria-labelledby="title">
        <h2 id="title">MonkeyCode · ${t('高级设置', 'Advanced settings')}</h2>
        <p>${t('选择已配置的 OpenAI、Codex 或 Claude 接入，设置与 API Key 配套的完整 signing_secret（含 omas_ 前缀）。留空保存可关闭签名。', 'Choose a configured OpenAI, Codex or Claude provider. Enter its complete signing_secret, including the omas_ prefix. Save an empty value to disable signing.')}</p>
        <form id="login" hidden>
          <label>${t('WebUI 管理密钥', 'WebUI management key')}<input id="management-key" type="password" autocomplete="off" required></label>
          <p>${t('请先登录当前实例的 WebUI，或在此输入同一管理密钥。', 'Log in to this instance in WebUI, or enter the same management key here.')}</p>
          <button type="submit">${t('连接', 'Connect')}</button>
        </form>
        <form id="settings">
          <label>${t('接入配置', 'Provider')}<select id="provider" disabled></select></label>
          <label>signing_secret<input id="secret" type="password" autocomplete="off" maxlength="4096" spellcheck="false" disabled></label>
          <p>${t('签名在协议转换后生成。启用后移除全部 URL 查询参数，Codex 使用 HTTP/SSE。', 'Signing uses the final translated request. All URL query parameters are removed, and Codex uses HTTP/SSE.')}</p>
          <div class="actions">
            <button id="close" type="button">${t('关闭', 'Close')}</button>
            <button id="reload" type="button">${t('刷新', 'Refresh')}</button>
            <button id="save" type="submit" disabled>${t('保存', 'Save')}</button>
          </div>
        </form>
        <p id="status" role="status" aria-live="polite"></p>
      </dialog>`;
    document.body.append(host);
    const el = id => root.getElementById(id);
    const dialog = root.querySelector('dialog');
    let providers = [];
    let busy = false;
    const status = message => { el('status').textContent = message; };
    const lock = value => {
      busy = value;
      el('reload').disabled = value;
      el('provider').disabled = value || !providers.length;
      el('secret').disabled = value || !providers.length;
      el('save').disabled = value || !providers.length;
    };
    const api = async (method = 'GET', body) => {
      const response = await nativeFetch('/v0/management/monkeycode', {
        method, cache: 'no-store', credentials: 'same-origin',
        headers: { Authorization: authorization, 'Content-Type': 'application/json' },
        ...(body === undefined ? {} : { body: JSON.stringify(body) })
      });
      if (response.status === 401 || response.status === 403) {
        authorization = '';
        el('login').hidden = false;
        throw new Error(t('管理密钥无效或无权访问当前实例。', 'Invalid management key or access denied.'));
      }
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
      return data;
    };
    const selectProvider = () => {
      el('secret').value = providers[Number(el('provider').value)]?.signing_secret ?? '';
    };
    async function load() {
      lock(true);
      providers = [];
      el('provider').replaceChildren();
      el('secret').value = '';
      try {
        el('login').hidden = !!authorization;
        if (!authorization) { status(t('请先连接 WebUI。', 'Connect to WebUI first.')); return false; }
        const data = await api();
        providers = data.providers;
        providers.forEach((provider, index) => {
          const option = document.createElement('option');
          option.value = String(index);
          option.textContent = `${provider.name || provider.section} #${provider.index + 1} · ${provider.base_url}`;
          el('provider').append(option);
        });
        selectProvider();
        status(providers.length ? '' : t('请先在 API 接入页面添加供应商，再点击刷新。', 'Add a provider in API Providers, then refresh.'));
        return true;
      } catch (error) { status(error.message); return false; }
      finally { lock(false); }
    }
    el('open').addEventListener('click', () => { dialog.showModal(); void load(); });
    el('close').addEventListener('click', () => dialog.close());
    dialog.addEventListener('close', () => {
      providers = [];
      el('secret').value = '';
      el('management-key').value = '';
    });
    el('provider').addEventListener('change', selectProvider);
    el('reload').addEventListener('click', () => void load());
    el('login').addEventListener('submit', async event => {
      event.preventDefault();
      if (busy) return;
      authorization = `Bearer ${el('management-key').value}`;
      el('management-key').value = '';
      await load();
    });
    el('settings').addEventListener('submit', async event => {
      event.preventDefault();
      const provider = providers[Number(el('provider').value)];
      if (busy || !provider) return;
      lock(true);
      try {
        await api('PATCH', { section: provider.section, index: provider.index, revision: provider.revision, signing_secret: el('secret').value });
        if (await load()) status(t('已保存，后续请求将使用新配置。', 'Saved. Subsequent requests will use the updated settings.'));
      } catch (error) { status(error.message); }
      finally { lock(false); }
    });
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mount, { once: true });
  else mount();
})();
