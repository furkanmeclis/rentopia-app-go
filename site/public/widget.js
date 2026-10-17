/*
 * Rentopia booking widget loader (TEC-732).
 *
 *   <script src="https://{site}/widget.js" async
 *           data-key="rtp_pk_…" data-branch="{uuid}" data-subgroup="{uuid}"
 *           data-locale="tr" data-target="#booking"></script>
 *
 * Inserts an iframe pointing at {site}/embed (the site origin is the origin
 * this script was loaded from), sizes it from the widget's resize messages
 * and forwards the widget's events to the host page as DOM events:
 *   rentopia:ready, rentopia:completed ({reference, status}), rentopia:error.
 * The payment step leaves the iframe: the widget asks for a top-level
 * navigation and the loader follows it.
 */
(function () {
  "use strict";

  var script = document.currentScript;
  if (!script || !script.src) return;

  var siteOrigin;
  try {
    siteOrigin = new URL(script.src).origin;
  } catch (e) {
    return;
  }

  var data = script.dataset || {};
  var widgetId =
    "rtw-" + Math.random().toString(36).slice(2, 10) + Date.now().toString(36);

  var params = new URLSearchParams();
  var map = {
    key: "key",
    branch: "branch",
    subgroup: "subgroup",
    pickup: "pickup",
    return: "return",
    pickupDate: "pickup_date",
    pickupTime: "pickup_time",
    returnDate: "return_date",
    returnTime: "return_time",
    locale: "locale",
  };
  Object.keys(map).forEach(function (attr) {
    var value = data[attr];
    if (value) params.set(map[attr], String(value).slice(0, 200));
  });
  params.set("parent", window.location.origin);
  params.set("widgetId", widgetId);

  var iframe = document.createElement("iframe");
  iframe.src = siteOrigin + "/embed?" + params.toString();
  iframe.title = data.title || "Rezervasyon";
  iframe.setAttribute("referrerpolicy", "no-referrer");
  iframe.setAttribute("loading", "lazy");
  iframe.setAttribute("allow", "payment");
  iframe.style.cssText =
    "display:block;width:100%;height:640px;border:0;overflow:hidden;color-scheme:normal;";

  var target = data.target ? document.querySelector(data.target) : null;
  if (target) {
    target.appendChild(iframe);
  } else if (script.parentNode) {
    script.parentNode.insertBefore(iframe, script.nextSibling);
  } else {
    return;
  }

  function emit(name, detail) {
    var host = target || iframe;
    try {
      host.dispatchEvent(
        new CustomEvent("rentopia:" + name, { detail: detail, bubbles: true }),
      );
    } catch (e) {
      /* old browsers: no CustomEvent */
    }
  }

  // The pay page lives on the platform's origin, not the site's: accept any
  // https URL (http only on localhost for development), never javascript: etc.
  function isNavigable(url) {
    try {
      var u = new URL(url);
      return (
        u.protocol === "https:" ||
        (u.protocol === "http:" &&
          (u.hostname === "localhost" || u.hostname.endsWith(".localhost")))
      );
    } catch (e) {
      return false;
    }
  }

  window.addEventListener("message", function (event) {
    if (event.origin !== siteOrigin) return;
    if (event.source !== iframe.contentWindow) return;
    var msg = event.data;
    if (!msg || typeof msg !== "object") return;
    if (msg.source !== "rentopia-widget" || msg.v !== 1) return;
    if (msg.widgetId !== widgetId) return;
    var payload = msg.payload || {};

    switch (msg.type) {
      case "ready":
        emit("ready", {});
        break;
      case "resize": {
        var h = Number(payload.height);
        if (isFinite(h)) {
          h = Math.min(100000, Math.max(100, Math.ceil(h)));
          iframe.style.height = h + "px";
        }
        break;
      }
      case "navigate":
        if (
          payload.target === "_top" &&
          typeof payload.url === "string" &&
          isNavigable(payload.url)
        ) {
          window.location.assign(payload.url);
        }
        break;
      case "completed":
        emit("completed", {
          reference: String(payload.reference || ""),
          status: String(payload.status || ""),
        });
        break;
      case "error":
        emit("error", { code: String(payload.code || "") });
        break;
    }
  });
})();
