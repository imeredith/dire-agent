(() => {
  const marker = "__DIRE_AGENT_PROJECT_PROXY__";
  const legacyMarker = "__GOAGENT_PROJECT_PROXY__";
  if (globalThis[marker]) {
    globalThis[legacyMarker] = globalThis[marker];
    return;
  }

  const currentScript = document.currentScript;
  const scriptURL = currentScript?.src ? new URL(currentScript.src, location.href) : null;
  const suffix = "/__dire_agent_project_proxy.js";
  const metaPrefix = document.querySelector('meta[name="dire-agent-proxy-prefix"]')?.content;
  const prefix = (metaPrefix || (scriptURL?.pathname.endsWith(suffix)
    ? scriptURL.pathname.slice(0, -suffix.length)
    : scriptURL?.pathname.replace(/\/$/, "")) || "").replace(/\/$/, "");
  if (!prefix) return;
  const upstreamPort = prefix.slice(prefix.lastIndexOf("/") + 1);
  const isLoopback = (hostname) => ["localhost", "127.0.0.1", "::1"].includes(hostname);
  const isPublicProtocol = (protocol) => ["http:", "https:", "ws:", "wss:"].includes(protocol);

  const rewriteURL = (value, websocket = false) => {
    if (value == null) return value;
    const source = value instanceof URL ? value.href : typeof value === "string" ? value : null;
    if (source == null) return value;
    let parsed;
    try {
      parsed = new URL(source, location.href);
    } catch {
      return value;
    }
    if (!isPublicProtocol(parsed.protocol)) return value;
    const samePublicHost = parsed.host === location.host;
    const directLoopback = isLoopback(parsed.hostname) && parsed.port === upstreamPort;
    if (!samePublicHost && !directLoopback) return value;
    if (samePublicHost && (parsed.pathname === prefix || parsed.pathname.startsWith(prefix + "/"))) {
      return source;
    }
    parsed.host = location.host;
    parsed.protocol = websocket
      ? location.protocol === "https:" ? "wss:" : "ws:"
      : location.protocol;
    parsed.pathname = prefix + (parsed.pathname.startsWith("/") ? parsed.pathname : "/" + parsed.pathname);
    return parsed.href;
  };

  const stripPrefix = (pathname) => {
    if (pathname === prefix) return "/";
    return pathname.startsWith(prefix + "/") ? pathname.slice(prefix.length) : pathname;
  };

  const projectProxy = {
    prefix,
    upstreamPort: Number(upstreamPort),
    rewriteURL,
    get pathname() { return stripPrefix(location.pathname); },
  };
  globalThis[marker] = projectProxy;
  globalThis[legacyMarker] = projectProxy;

  const NativeWebSocket = globalThis.WebSocket;
  if (NativeWebSocket) {
    globalThis.WebSocket = class WebSocket extends NativeWebSocket {
      constructor(url, protocols) {
        const rewritten = rewriteURL(url, true);
        if (protocols === undefined) super(rewritten);
        else super(rewritten, protocols);
      }
    };
  }

  const NativeEventSource = globalThis.EventSource;
  if (NativeEventSource) {
    globalThis.EventSource = class EventSource extends NativeEventSource {
      constructor(url, options) { super(rewriteURL(url), options); }
    };
  }

  const nativeFetch = globalThis.fetch?.bind(globalThis);
  if (nativeFetch) {
    globalThis.fetch = (input, init) => {
      if (input instanceof Request) {
        const rewritten = rewriteURL(input.url);
        if (rewritten !== input.url) input = new Request(rewritten, input);
      } else {
        input = rewriteURL(input);
      }
      return nativeFetch(input, init);
    };
  }

  const nativeOpen = globalThis.XMLHttpRequest?.prototype.open;
  if (nativeOpen) {
    globalThis.XMLHttpRequest.prototype.open = function(method, url, ...rest) {
      return nativeOpen.call(this, method, rewriteURL(url), ...rest);
    };
  }

  for (const name of ["Worker", "SharedWorker"]) {
    const NativeWorker = globalThis[name];
    if (!NativeWorker) continue;
    globalThis[name] = class extends NativeWorker {
      constructor(url, options) { super(rewriteURL(url), options); }
    };
  }

  for (const method of ["pushState", "replaceState"]) {
    const nativeMethod = history[method];
    history[method] = function(state, unused, url) {
      return nativeMethod.call(this, state, unused, url == null ? url : rewriteURL(url));
    };
  }

  const urlAttributes = new Set(["action", "href", "poster", "src"]);
  const rewriteAttribute = (element, name) => {
    if (!urlAttributes.has(name) || !element.hasAttribute(name)) return;
    const value = element.getAttribute(name);
    const rewritten = rewriteURL(value);
    if (typeof rewritten === "string" && rewritten !== value) element.setAttribute(name, rewritten);
  };
  const nativeSetAttribute = Element.prototype.setAttribute;
  Element.prototype.setAttribute = function(name, value) {
    const normalized = String(name).toLowerCase();
    return nativeSetAttribute.call(this, name, urlAttributes.has(normalized) ? rewriteURL(String(value)) : value);
  };

  const rewriteTree = (root) => {
    if (root.nodeType === Node.ELEMENT_NODE) {
      for (const name of urlAttributes) rewriteAttribute(root, name);
    }
    root.querySelectorAll?.("[action], [href], [poster], [src]").forEach((element) => {
      for (const name of urlAttributes) rewriteAttribute(element, name);
    });
  };
  const observe = () => {
    rewriteTree(document.documentElement);
    new MutationObserver((records) => {
      for (const record of records) {
        if (record.type === "attributes") rewriteAttribute(record.target, record.attributeName);
        record.addedNodes.forEach(rewriteTree);
      }
    }).observe(document.documentElement, { subtree: true, childList: true, attributes: true, attributeFilter: [...urlAttributes] });
  };
  if (document.documentElement) observe();
  else addEventListener("DOMContentLoaded", observe, { once: true });
})();
