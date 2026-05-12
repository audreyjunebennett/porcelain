/**
 * Unified operator gallery: editable routing YAML + fallback chain YAML + router
 * model list with refresh/save overlays; optional /api/ui/* when the page is served over HTTP(S)
 * from the gateway origin (same-origin session cookie).
 */
(function () {
  "use strict";

  function canUseGatewayAPI() {
    try {
      var p = window.location && window.location.protocol;
      return p === "http:" || p === "https:";
    } catch (e) {
      return false;
    }
  }

  function fallbackChainToYAML(ids) {
    if (!ids || !ids.length) return "";
    return ids.map(function (id) {
      var s = String(id);
      if (/^[\w./-]+$/.test(s)) return "- " + s;
      return "- " + JSON.stringify(s);
    }).join("\n");
  }

  function parseFallbackChainInput(text) {
    var t = String(text || "").trim();
    if (t.length > 0 && t[0] === "[") {
      try {
        var j = JSON.parse(t);
        if (Array.isArray(j)) {
          return j.map(function (x) {
            return String(x);
          });
        }
      } catch (e0) {
        /* fall through */
      }
    }
    var lines = String(text || "").split(/\r?\n/);
    var out = [];
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i].replace(/#.*$/, "").trim();
      if (!line) continue;
      if (line[0] !== "-") {
        throw new Error("each non-empty line must start with '-' (line " + (i + 1) + ")");
      }
      var rest = line.slice(1).trim();
      if (!rest) {
        throw new Error("empty list item (line " + (i + 1) + ")");
      }
      if (rest[0] === '"') {
        try {
          out.push(JSON.parse(rest));
        } catch (e1) {
          throw new Error("bad double-quoted string (line " + (i + 1) + "): " + e1.message);
        }
        continue;
      }
      if (rest[0] === "'") {
        if (rest.length < 2 || rest[rest.length - 1] !== "'") {
          throw new Error("unclosed single-quoted string (line " + (i + 1) + ")");
        }
        out.push(rest.slice(1, -1).replace(/''/g, "'"));
        continue;
      }
      out.push(rest);
    }
    return out;
  }

  function wireYamlOverlay(wrap, ta, opts) {
    var lastSaved = opts.initialLastSaved;
    var refreshBtn = wrap.querySelector("[data-sg-op-yaml-refresh]");
    var saveBtn = wrap.querySelector("[data-sg-op-yaml-save]");
    if (!refreshBtn || !saveBtn) return;

    function setDirty(on) {
      wrap.classList.toggle("sg-op-yaml-wrap--dirty", !!on);
    }

    function setActive(on) {
      wrap.classList.toggle("sg-op-yaml-wrap--active", !!on);
    }

    function syncDirtyFromValue() {
      setDirty(String(ta.value) !== String(lastSaved));
    }

    /** Vertical overflow → class for overlay inset (see gallery-shell.css). */
    function syncVScroll() {
      wrap.classList.toggle("sg-op-yaml-wrap--vscroll", ta.scrollHeight > ta.clientHeight + 1);
    }

    function scrollEditorToTop() {
      try {
        ta.scrollTop = 0;
      } catch (e) {
        /* ignore */
      }
    }

    ta.addEventListener("focus", function () {
      setActive(true);
      window.requestAnimationFrame(syncVScroll);
    });
    ta.addEventListener("blur", function () {
      window.setTimeout(function () {
        if (document.activeElement === ta) return;
        setActive(false);
      }, 0);
    });
    ta.addEventListener("input", function () {
      syncDirtyFromValue();
      syncVScroll();
    });
    ta.addEventListener("scroll", syncVScroll);
    window.addEventListener("resize", syncVScroll);
    if (typeof ResizeObserver === "function") {
      try {
        new ResizeObserver(syncVScroll).observe(ta);
      } catch (eRo) {
        /* ignore */
      }
    }

    refreshBtn.addEventListener("click", function () {
      ta.value = lastSaved;
      scrollEditorToTop();
      setDirty(false);
      syncVScroll();
      ta.focus();
    });

    saveBtn.addEventListener("click", function () {
      if (!opts.onSave) return;
      var cur = String(ta.value);
      opts.onSave(cur, function (err, newLast) {
        if (err) {
          window.alert(err);
          return;
        }
        lastSaved = newLast != null ? String(newLast) : cur;
        ta.value = lastSaved;
        scrollEditorToTop();
        setDirty(false);
        syncVScroll();
        if (typeof opts.afterSave === "function") opts.afterSave(lastSaved);
      });
    });

    syncDirtyFromValue();
    syncVScroll();
    window.requestAnimationFrame(syncVScroll);
  }

  function postJSON(path, body, cb) {
    if (!canUseGatewayAPI()) {
      cb("Open this page from the gateway over HTTP(S) with a UI session to save to disk (file:// cannot call /api/ui).");
      return;
    }
    fetch(path, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    })
      .then(function (res) {
        if (res.status === 401) {
          window.location.href = "/ui/login?next=" + encodeURIComponent(window.location.pathname);
          return null;
        }
        return res.json().then(function (j) {
          return { res: res, j: j || {} };
        });
      })
      .then(function (pack) {
        if (!pack) return;
        if (!pack.res.ok) {
          var msg =
            (pack.j.error && pack.j.error.message) ||
            pack.j.detail ||
            "HTTP " + pack.res.status;
          cb(msg);
          return;
        }
        cb(null, pack.j);
      })
      .catch(function (e) {
        cb(e && e.message ? String(e.message) : String(e));
      });
  }

  function providerTierSpan(provider) {
    var p = String(provider || "").toLowerCase();
    var tier = "sum-conv-tier--inferred";
    var label = String(provider || "");
    if (p === "groq") {
      tier = "sum-conv-tier--request_id";
      label = "Groq";
    } else if (p === "gemini") {
      tier = "sum-conv-tier--ingest";
      label = "Gemini";
    } else if (p === "ollama") {
      tier = "sum-conv-tier--anchored_inferred";
      label = "Ollama";
    }
    return '<span class="sum-conv-tier ' + tier + '">' + label + "</span>";
  }

  function splitProviderModel(mid) {
    var s = String(mid || "").trim();
    var i = s.indexOf("/");
    if (i <= 0) return { provider: "", model: s };
    return { provider: s.slice(0, i), model: s.slice(i + 1) };
  }

  /** Static demo usage counts keyed by full model id (gallery only). */
  var fbUsageDemo = {
    "groq/llama-3.3-70b": 1842,
    "gemini/gemini-2.0-flash": 612,
    "ollama/llama3.1:8b": 104,
    "ollama/nomic-embed-text": 9021,
    "ollama/mxbai-embed-large": 1204
  };

  function rebuildFallbackTable(tbody, chain) {
    if (!tbody) return;
    var rowsHtml = "";
    for (var i = 0; i < chain.length; i++) {
      var mid = chain[i];
      var pm = splitProviderModel(mid);
      var uses = fbUsageDemo[mid] != null ? fbUsageDemo[mid] : "—";
      rowsHtml +=
        "<tr>" +
        '<td class="num">' +
        (i + 1) +
        "</td>" +
        "<td>" +
        providerTierSpan(pm.provider) +
        "</td>" +
        '<td><code class="sum-mono-id">' +
        mid +
        "</code></td>" +
        '<td class="num">' +
        uses +
        "</td>" +
        "</tr>";
    }
    tbody.innerHTML = rowsHtml;
  }

  function init() {
    var policyTa = document.getElementById("sg-op-routing-policy-yaml");
    var policyWrap = document.getElementById("sg-op-routing-policy-wrap");
    var fbTa = document.getElementById("sg-op-fallback-yaml");
    var fbWrap = document.getElementById("sg-op-fallback-yaml-wrap");
    var fbTableView = document.getElementById("sg-op-fb-table-view");
    var fbYamlView = document.getElementById("sg-op-fb-yaml-view");
    var fbTbody = document.querySelector("[data-sg-op-fallback-tbody]");
    var btnConfigure = document.getElementById("sg-op-fb-configure");
    var btnFbCancel = document.getElementById("sg-op-fb-yaml-cancel");
    var routerTa = document.getElementById("sg-op-router-models-yaml");
    var routerWrap = document.getElementById("sg-op-router-models-wrap");
    var routerThr = document.getElementById("sg-op-router-threshold");
    var routerEn = document.getElementById("sg-op-router-enabled");

    var lastFbChain = ["groq/llama-3.3-70b", "gemini/gemini-2.0-flash", "ollama/llama3.1:8b"];

    function wirePolicyWhenReady() {
      if (!policyTa || !policyWrap) return;
      wireYamlOverlay(policyWrap, policyTa, {
        initialLastSaved: String(policyTa.value),
        onSave: function (yaml, done) {
          postJSON("/api/ui/routing/policy", { routing_policy_yaml: yaml }, function (err, j) {
            if (err) return done(err);
            var next = (j && j.routing_policy_yaml) || yaml;
            done(null, next);
          });
        }
      });
    }

    function wireFbWhenReady() {
      if (!fbTa || !fbWrap) return;
      wireYamlOverlay(fbWrap, fbTa, {
        initialLastSaved: fallbackChainToYAML(lastFbChain),
        onSave: function (_yaml, done) {
          var chain;
          try {
            chain = parseFallbackChainInput(fbTa.value);
          } catch (e3) {
            return done(e3.message || String(e3));
          }
          postJSON("/api/ui/routing/fallback_chain", { fallback_chain: chain }, function (err, j) {
            if (err) return done(err);
            var fc = (j && j.fallback_chain) || chain;
            lastFbChain = fc;
            rebuildFallbackTable(fbTbody, lastFbChain);
            showFbTable();
            done(null, fallbackChainToYAML(lastFbChain));
          });
        },
        afterSave: function () {
          showFbTable();
        }
      });
    }

    function wireRouterWhenReady() {
      if (!routerTa || !routerWrap) return;
      wireYamlOverlay(routerWrap, routerTa, {
        initialLastSaved: String(routerTa.value),
        onSave: function (_yaml, done) {
          var models;
          try {
            models = parseFallbackChainInput(routerTa.value);
          } catch (e4) {
            return done(e4.message || String(e4));
          }
          var thrRaw = routerThr && routerThr.value != null ? String(routerThr.value) : "0.35";
          var thr = parseFloat(thrRaw);
          if (isNaN(thr) || thr < 0 || thr > 1) {
            return done("Confidence threshold must be a number between 0 and 1.");
          }
          var enabled = routerEn ? !!routerEn.checked : true;
          postJSON(
            "/api/ui/routing/router_tooling",
            {
              router_models: models,
              tool_router_enabled: enabled,
              confidence_threshold: thr
            },
            function (err, j) {
              if (err) return done(err);
              var rm = (j && j.router_models) || models;
              done(null, fallbackChainToYAML(rm));
            }
          );
        }
      });
    }

    function showFbTable() {
      if (fbTableView) fbTableView.hidden = false;
      if (fbYamlView) fbYamlView.hidden = true;
    }

    function showFbYaml() {
      if (fbTableView) fbTableView.hidden = true;
      if (fbYamlView) fbYamlView.hidden = false;
      if (fbTa) {
        fbTa.value = fallbackChainToYAML(lastFbChain);
        try {
          fbTa.scrollTop = 0;
        } catch (e2a) {
          /* ignore */
        }
        fbTa.dispatchEvent(new Event("input", { bubbles: true }));
        try {
          fbTa.focus();
        } catch (e2) {}
      }
    }

    function hydrateFromGatewayThenWireEditors() {
      var p = Promise.resolve(null);
      if (canUseGatewayAPI()) {
        p = fetch("/api/ui/state", { credentials: "same-origin" }).then(function (res) {
          if (res.status === 401) return null;
          return res.json();
        });
      }
      p.then(function (j) {
        if (j && j.gateway) {
          var y = j.gateway.routing_policy_yaml;
          if (policyTa && typeof y === "string" && y.trim() !== "") {
            policyTa.value = y;
            try {
              policyTa.scrollTop = 0;
            } catch (eH) {
              /* ignore */
            }
          }
          var fc = j.gateway.fallback_chain;
          if (Array.isArray(fc) && fc.length) {
            lastFbChain = fc.slice();
            rebuildFallbackTable(fbTbody, lastFbChain);
          }
          var rmods = j.gateway.router_models;
          if (Array.isArray(rmods) && routerTa) {
            routerTa.value = fallbackChainToYAML(rmods);
            try {
              routerTa.scrollTop = 0;
            } catch (eR) {
              /* ignore */
            }
          }
          if (routerThr && typeof j.gateway.tool_router_confidence_threshold === "number") {
            routerThr.value = String(j.gateway.tool_router_confidence_threshold);
          }
          if (routerEn && typeof j.gateway.tool_router_enabled === "boolean") {
            routerEn.checked = j.gateway.tool_router_enabled;
          }
        }
      })
        .catch(function () {})
        .finally(function () {
          wirePolicyWhenReady();
          wireFbWhenReady();
          wireRouterWhenReady();
        });
    }

    hydrateFromGatewayThenWireEditors();

    if (btnConfigure) {
      btnConfigure.addEventListener("click", function () {
        showFbYaml();
      });
    }
    if (btnFbCancel) {
      btnFbCancel.addEventListener("click", function () {
        if (fbTa) {
          fbTa.value = fallbackChainToYAML(lastFbChain);
          try {
            fbTa.scrollTop = 0;
          } catch (e3) {
            /* ignore */
          }
          fbTa.dispatchEvent(new Event("input", { bubbles: true }));
        }
        showFbTable();
      });
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
