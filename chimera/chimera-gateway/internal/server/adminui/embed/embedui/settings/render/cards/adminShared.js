/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAdminShared = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var getFlat = ctx.getFlat;
  var entryCache = ctx.entryCache;
  var sumEvlogRowTrHtml = ctx.sumEvlogRowTrHtml;
  var sumEvlogPanelHtml = ctx.sumEvlogPanelHtml;
  var CHIMERA_BROKER_PROVIDER_STALE_MS = ctx.CHIMERA_BROKER_PROVIDER_STALE_MS || 90000;

  function inferServiceBadge(ev) {
    if (typeof ctx.inferServiceBadge === "function") return ctx.inferServiceBadge(ev);
    return { cls: "", lab: "" };
  }
  var sumEvlogHttpCode = ctx.sumEvlogHttpCode;
  var sumEvlogIsWarnish = ctx.sumEvlogIsWarnish;
  var sumEvlogIsFailish = ctx.sumEvlogIsFailish;

  if (globalThis.ChimeraShared && ChimeraShared.ScopedEvlog && typeof ChimeraShared.ScopedEvlog.setDeps === "function") {
    ChimeraShared.ScopedEvlog.setDeps({
      escapeHtml: escapeHtml,
      getFlat: getFlat,
      sumEvlogRowTrHtml: sumEvlogRowTrHtml,
      sumEvlogPanelHtml: sumEvlogPanelHtml,
      sumEvlogHttpCode: sumEvlogHttpCode,
      sumEvlogIsWarnish: sumEvlogIsWarnish,
      sumEvlogIsFailish: sumEvlogIsFailish,
      inferServiceBadge: function (ev) {
        if (typeof ctx.inferServiceBadge === "function") return ctx.inferServiceBadge(ev);
        return { cls: "", lab: "" };
      }
    });
  }

  function fallbackChainToYAML(ids) {
    if (!ids || !ids.length) return "";
    return ids
      .map(function (id) {
        var s = String(id);
        if (/^[\w./-]+$/.test(s)) return "- " + s;
        return "- " + JSON.stringify(s);
      })
      .join("\n");
  }

  function parseFallbackChainInput(text) {
    var t = String(text || "").trim();
    if (t.length > 0 && t[0] === "[") {
      try {
        var j = JSON.parse(t);
        if (Array.isArray(j)) return j.map(function (x) { return String(x); });
      } catch (_e) {}
    }
    var lines = String(text || "").split(/\r?\n/);
    var out = [];
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i].replace(/#.*$/, "").trim();
      if (!line) continue;
      if (line[0] !== "-") throw new Error("each non-empty line must start with '-' (line " + (i + 1) + ")");
      var rest = line.slice(1).trim();
      if (!rest) throw new Error("empty list item (line " + (i + 1) + ")");
      if (rest[0] === '"') {
        try {
          out.push(JSON.parse(rest));
        } catch (e) {
          throw new Error("bad double-quoted string (line " + (i + 1) + "): " + e.message);
        }
        continue;
      }
      if (rest[0] === "'") {
        if (rest.length < 2 || rest[rest.length - 1] !== "'") throw new Error("unclosed single-quoted string (line " + (i + 1) + ")");
        out.push(rest.slice(1, -1).replace(/''/g, "'"));
        continue;
      }
      out.push(rest);
    }
    return out;
  }
  function providerKeyIconBtn(action, providerId, title, iconName, extraAttrs) {
    extraAttrs = extraAttrs || {};
    var cls = "sg-op-yaml-ov-btn";
    if (extraAttrs.extraClass) cls += " " + String(extraAttrs.extraClass);
    var attrs = "";
    if (extraAttrs.name != null) {
      attrs += ' data-name="' + escapeHtml(String(extraAttrs.name)) + '"';
    }
    return (
      '<button type="button" class="' +
      cls +
      '" data-admin-action="' +
      escapeHtml(String(action || "")) +
      '" data-provider="' +
      escapeHtml(providerId) +
      '"' +
      attrs +
      ' title="' +
      escapeHtml(title) +
      '" aria-label="' +
      escapeHtml(title) +
      '"><span class="material-symbols-outlined" aria-hidden="true">' +
      escapeHtml(String(iconName || "")) +
      "</span></button>"
    );
  }
  function providerRowsHtml(providerId, p) {
    var rows = p && Array.isArray(p.keys) ? p.keys : [];
    if (!rows.length) {
      return '<p class="muted sg-op-provider-key-empty">No keys saved yet.</p>';
    }
    var out = '<ul class="sg-op-provider-key-list">';
    for (var i = 0; i < rows.length; i++) {
      var nm = rows[i] && rows[i].name != null ? String(rows[i].name) : "";
      out +=
        '<li class="sg-op-provider-key-row">' +
        '<div class="sg-op-provider-key-row__meta">' +
        '<span class="sg-op-provider-key-row__name">' +
        escapeHtml(nm || "(unnamed)") +
        "</span>" +
        '<code class="sg-op-provider-key-row__hint sum-mono-id">' +
        escapeHtml((rows[i] && rows[i].key_hint) || "—") +
        "</code></div>" +
        providerKeyIconBtn("provider-key-delete", providerId, "Remove key", "delete_forever", {
          extraClass: "sg-op-provider-key-btn-delete",
          name: nm
        }) +
        "</li>";
    }
    return out + "</ul>";
  }

  function providerKeyAddBlockHtml(providerId) {
    if (globalThis.ChimeraShared && ChimeraShared.ProviderCredentials && typeof ChimeraShared.ProviderCredentials.keyAddBlockHtml === "function") {
      return ChimeraShared.ProviderCredentials.keyAddBlockHtml(escapeHtml, ctx, providerId);
    }
    return "";
  }

  function adminProviderIntro(providerId, subtitle) {
    var links = {
      groq: { href: "https://groq.com/", label: "groq.com" },
      gemini: { href: "https://ai.google.dev/gemini-api/docs", label: "Gemini API docs" },
      ollama: { href: "https://ollama.com/", label: "ollama.com" }
    };
    var meta = links[providerId] || null;
    var out =
      '<p class="sg-op-provider-intro" data-ui-part="admin-provider.intro">' +
      escapeHtml(subtitle || "");
    if (meta) {
      out += ' Public reference: <a href="' + escapeHtml(meta.href) + '" target="_blank" rel="noopener noreferrer">' + escapeHtml(meta.label) + "</a>.";
    }
    return out + "</p>";
  }

  function adminProviderAvatarClass(providerId) {
    if (providerId === "groq") return "sum-av-a";
    if (providerId === "gemini") return "sum-av-b";
    if (providerId === "ollama") return "sum-av-c";
    return "sum-av-svc-chimera-broker";
  }

  function adminProviderHealthEntry(providerId) {
    if (!providerId || !ctx.chimeraBrokerProviderSnapshot || !ctx.chimeraBrokerProviderSnapshot.data || !Array.isArray(ctx.chimeraBrokerProviderSnapshot.data.providers)) {
      return null;
    }
    var snapshotAgeMs = Date.now() - Number(ctx.chimeraBrokerProviderSnapshot.fetchedClientMs || 0);
    if (snapshotAgeMs > CHIMERA_BROKER_PROVIDER_STALE_MS) return null;
    var list = ctx.chimeraBrokerProviderSnapshot.data.providers;
    for (var i = 0; i < list.length; i++) {
      var row = list[i] || {};
      if (String(row.id || "").toLowerCase() === String(providerId).toLowerCase()) return row;
    }
    return null;
  }

  function operatorSectionHeadHtml(title, icon, opts) {
    opts = opts || {};
    var idAttr = opts.id ? ' id="' + escapeHtml(String(opts.id)) + '"' : "";
    var iconCls = opts.iconPrimary ? " sg-op-section-icon--primary" : "";
    var action = opts.actionHtml != null ? String(opts.actionHtml) : "";
    return (
      '<div class="sg-op-section-head"' +
      idAttr +
      ">" +
      '<div class="sg-op-section-head__left">' +
      '<span class="material-symbols-outlined sg-op-section-icon' +
      iconCls +
      '" aria-hidden="true">' +
      escapeHtml(icon || "dashboard") +
      "</span>" +
      '<h3 class="sg-op-section-title">' +
      escapeHtml(title || "") +
      "</h3></div>" +
      action +
      "</div>"
    );
  }

  function operatorSectionAddBtn(attrs, label, opts) {
    attrs = attrs || {};
    opts = opts || {};
    var attrStr = "";
    for (var k in attrs) {
      if (!Object.prototype.hasOwnProperty.call(attrs, k)) continue;
      attrStr += " " + k + '="' + escapeHtml(String(attrs[k])) + '"';
    }
    var lab = label != null ? String(label) : "Add";
    var title = opts.title != null ? String(opts.title) : lab;
    var disabled = opts.disabled ? ' disabled aria-disabled="true"' : "";
    var btn =
      '<button type="button" class="sg-op-section-action"' +
      attrStr +
      disabled +
      ' aria-label="' +
      escapeHtml(lab) +
      '" title="' +
      escapeHtml(title) +
      '">' +
      '<span class="material-symbols-outlined" aria-hidden="true">add_2</span>' +
      "<span>" +
      escapeHtml(lab) +
      "</span></button>";
    if (opts.desktopLocked) {
      return (
        '<span class="ws-desktop-only-locked" title="' +
        escapeHtml(title) +
        '">' +
        btn +
        "</span>"
      );
    }
    return btn;
  }

  /** Full-width add control under section title row (same width as cards). */
  function operatorSectionAddBarHtml(attrs, label) {
    attrs = attrs || {};
    var attrStr = "";
    for (var k in attrs) {
      if (!Object.prototype.hasOwnProperty.call(attrs, k)) continue;
      attrStr += " " + k + '="' + escapeHtml(String(attrs[k])) + '"';
    }
    var lab = label != null ? String(label) : "Add";
    return (
      '<button type="button" class="sg-op-section-action-bar"' +
      attrStr +
      ' aria-label="' +
      escapeHtml(lab) +
      '" title="' +
      escapeHtml(lab) +
      '">' +
      '<span class="material-symbols-outlined" aria-hidden="true">add_2</span>' +
      "<span>" +
      escapeHtml(lab) +
      "</span></button>"
    );
  }

  function operatorCardChevronHtml() {
    return (
      '<span class="material-symbols-outlined sg-op-chev-icon" aria-hidden="true">chevron_right</span>' +
      '<span class="sum-chev" aria-hidden="true"></span>'
    );
  }

  function operatorConfigureBtnInline(action, ariaLabel, title) {
    if (globalThis.ChimeraShared && ChimeraShared.ConfigureEdit && typeof ChimeraShared.ConfigureEdit.configureBtnInline === "function") {
      return ChimeraShared.ConfigureEdit.configureBtnInline(escapeHtml, action, ariaLabel, title);
    }
    return "";
  }

  function sgOpHealthPillHtml(label, variant, opts) {
    opts = opts || {};
    var cls = "sg-op-health-pill";
    if (variant === "ok") cls += " sg-op-health-pill--ok";
    else if (variant === "metric") cls += " sg-op-health-pill--metric";
    else if (variant === "down") cls += " sg-op-health-pill--down";
    else if (variant === "unknown") cls += " sg-op-health-pill--unknown";
    else if (variant === "not_configured") cls += " sg-op-health-pill--not_configured";
    else if (variant === "warn") cls += " sg-op-health-pill--warn";
    if (opts.pulse) cls += " sg-op-health-pill--pulse";
    var labelStr = label != null ? String(label) : "";
    var inner = escapeHtml(labelStr);
    if (opts.icon) {
      inner +=
        ' <span class="material-symbols-outlined material-symbols-outlined--sm sg-op-health-pill__icon" aria-hidden="true">' +
        escapeHtml(String(opts.icon)) +
        "</span>";
    }
    var attrs = "";
    if (opts.title) attrs += ' title="' + escapeHtml(String(opts.title)) + '"';
    if (opts.icon && opts.title) {
      attrs += ' aria-label="' + escapeHtml(String(opts.title) + ": " + labelStr) + '"';
    }
    return "<span class=\"" + cls + "\"" + attrs + ">" + inner + "</span>";
  }

  function adminProviderIsConfigured(providerId) {
    var row = ((ctx.adminStateCache || {}).providers || {})[providerId] || {};
    if (providerId === "ollama") {
      return !!(String(row.ollama_base_url || "").trim());
    }
    if (row.key_configured === true) return true;
    var keys = row.keys;
    if (Array.isArray(keys)) {
      for (var ki = 0; ki < keys.length; ki++) {
        var kr = keys[ki] || {};
        if (kr.key_configured === true) return true;
      }
    }
    return false;
  }

  function adminProviderAvailabilityHtml(providerId) {
    var hp = adminProviderHealthEntry(providerId);
    var st;
    if (hp && hp.state) {
      st = String(hp.state).toLowerCase();
    } else if (!adminProviderIsConfigured(providerId)) {
      st = "not_configured";
    } else {
      st = "unknown";
    }
    var map = {
      up: { variant: "ok", label: "reachable" },
      key_missing: { variant: "warn", label: "key missing" },
      down: { variant: "down", label: "offline" },
      unknown: { variant: "unknown", label: "configured" },
      not_configured: { variant: "not_configured", label: "not configured" }
    };
    var meta = map[st] || map.unknown;
    return sgOpHealthPillHtml(meta.label, meta.variant);
  }

  function adminProviderModelCount(providerId) {
    var prow = ((ctx.adminStateCache || {}).providers || {})[providerId] || {};
    var editing = ctx.adminProviderModelsEditingId === providerId;
    if (editing && ctx.adminProviderModelsDraft && ctx.adminProviderModelsDraft[providerId]) {
      var draftMap = ctx.adminProviderModelsDraft[providerId].models || {};
      var draftAvail = 0;
      for (var dk in draftMap) {
        if (Object.prototype.hasOwnProperty.call(draftMap, dk) && draftMap[dk]) draftAvail++;
      }
      return draftAvail;
    }
    if (prow.models_configured && prow.models_available_count != null) {
      return Number(prow.models_available_count) || 0;
    }
    var listed = adminProviderCatalogModels(providerId);
    if (listed.length) return listed.length;
    var data = ctx.metricsCache || {};
    var rows = [];
    if (Array.isArray(data.day_rollups) && data.day_rollups.length) rows = data.day_rollups;
    else if (Array.isArray(data.minute_rollups) && data.minute_rollups.length) rows = data.minute_rollups;
    var seen = {};
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i] || {};
      var rp = String(r.provider || "").toLowerCase();
      var mid = String(r.model_id || "");
      if (rp && rp !== String(providerId).toLowerCase()) continue;
      if (!rp && mid.toLowerCase().indexOf(String(providerId).toLowerCase() + "/") !== 0) continue;
      if (mid) seen[mid] = true;
    }
    var n = 0;
    for (var k in seen) {
      if (Object.prototype.hasOwnProperty.call(seen, k)) n++;
    }
    return n;
  }

  function countRoutingRulesFromYAML(yamlText) {
    var src = String(yamlText || "");
    if (!src.trim()) return 0;
    var lines = src.split(/\r?\n/);
    var inRules = false;
    var rulesIndent = 0;
    var itemIndent = -1;
    var n = 0;
    for (var i = 0; i < lines.length; i++) {
      var ln = lines[i];
      if (!inRules) {
        var mHead = /^(\s*)rules\s*:\s*$/.exec(ln);
        if (mHead) {
          inRules = true;
          rulesIndent = mHead[1].length;
        }
        continue;
      }
      if (!ln.trim()) continue;
      var indent = ln.match(/^\s*/)[0].length;
      if (indent <= rulesIndent && /^[^\s#]/.test(ln)) break;
      if (!/^\s*-\s+/.test(ln)) continue;
      if (itemIndent < 0) itemIndent = indent;
      if (indent === itemIndent) n++;
    }
    return n;
  }

  function parseRoutingYamlScalar(value) {
    var s = String(value || "").replace(/#.*$/, "").trim();
    if (!s) return "";
    if (s[0] === '"') {
      try { return String(JSON.parse(s)); } catch (_eScalar) {}
    }
    if (s[0] === "'" && s[s.length - 1] === "'") {
      return s.slice(1, -1).replace(/''/g, "'");
    }
    return s;
  }

  function parseRoutingRulesFromYAML(yamlText) {
    var src = String(yamlText || "");
    if (!src.trim()) return [];
    var lines = src.split(/\r?\n/);
    var inRules = false;
    var rulesIndent = 0;
    var out = [];
    var cur = null;
    var itemIndent = -1;
    var inWhen = false;
    var whenIndent = 0;
    var inModels = false;
    var modelsIndent = 0;

    function pushCurrent() {
      if (!cur) return;
      out.push(cur);
      cur = null;
    }

    for (var i = 0; i < lines.length; i++) {
      var ln = lines[i] || "";
      if (!inRules) {
        var mHead = /^(\s*)rules\s*:\s*$/.exec(ln);
        if (mHead) {
          inRules = true;
          rulesIndent = mHead[1].length;
        }
        continue;
      }
      if (!ln.trim()) continue;
      var indent = ln.match(/^\s*/)[0].length;
      if (indent <= rulesIndent && /^[^\s#]/.test(ln)) break;

      var mItem = /^\s*-\s*(.*)$/.exec(ln);
      if (mItem) {
        if (itemIndent < 0) itemIndent = indent;
        if (indent !== itemIndent) {
          if (inModels) {
            var midNested = parseRoutingYamlScalar(mItem[1]);
            if (midNested) cur.models.push(midNested);
          }
          continue;
        }
        pushCurrent();
        cur = {
          name: "unnamed",
          whenInline: "",
          whenParts: [],
          models: []
        };
        inWhen = false;
        inModels = false;
        var itemRest = String(mItem[1] || "").trim();
        if (itemRest) {
          var mNameInline = /^name\s*:\s*(.*)$/.exec(itemRest);
          var mWhenInline = /^when\s*:\s*(.*)$/.exec(itemRest);
          var mModelsInline = /^models\s*:\s*(.*)$/.exec(itemRest);
          if (mNameInline) cur.name = parseRoutingYamlScalar(mNameInline[1]) || "unnamed";
          else if (mWhenInline) cur.whenInline = parseRoutingYamlScalar(mWhenInline[1]);
          else if (mModelsInline) {
            var mv = parseRoutingYamlScalar(mModelsInline[1]);
            if (mv && mv !== "[]") cur.models.push(mv);
            inModels = !String(mModelsInline[1] || "").trim();
            modelsIndent = indent;
          }
        }
        continue;
      }
      if (!cur) continue;

      if (inWhen && indent <= whenIndent) inWhen = false;
      if (inModels && indent <= modelsIndent) inModels = false;

      var mName = /^\s*name\s*:\s*(.*)$/.exec(ln);
      if (mName) {
        cur.name = parseRoutingYamlScalar(mName[1]) || cur.name || "unnamed";
        continue;
      }

      var mWhen = /^\s*when\s*:\s*(.*)$/.exec(ln);
      if (mWhen) {
        var whenRest = String(mWhen[1] || "").trim();
        cur.whenInline = parseRoutingYamlScalar(whenRest);
        inWhen = !whenRest;
        whenIndent = indent;
        inModels = false;
        continue;
      }

      var mModels = /^\s*models\s*:\s*$/.exec(ln);
      if (mModels) {
        inModels = true;
        modelsIndent = indent;
        inWhen = false;
        continue;
      }

      if (inWhen) {
        var whenLn = ln.replace(/^\s+/, "").trim();
        if (whenLn && whenLn[0] !== "#") cur.whenParts.push(whenLn);
        continue;
      }

      if (inModels) {
        var mModel = /^\s*-\s*(.+)$/.exec(ln);
        if (mModel) {
          var mid = parseRoutingYamlScalar(mModel[1]);
          if (mid) cur.models.push(mid);
        }
      }
    }

    pushCurrent();
    return out;
  }

  function adminPrincipalForFlat(f) {
    if (!f) return "";
    return String(f.tenant_id || f.principal_id || f.tenant || "").trim();
  }

  function adminExtractProviderModel(mid) {
    var s = String(mid || "").trim();
    var slash = s.indexOf("/");
    if (slash <= 0) return { provider: "", model: s };
    return { provider: s.slice(0, slash), model: s.slice(slash + 1) };
  }

  function adminProviderCatalogModels(providerId) {
    var pid = String(providerId || "").toLowerCase();
    if (!pid) return [];
    if (!ctx.chimeraBrokerProviderSnapshot || !ctx.chimeraBrokerProviderSnapshot.data || !Array.isArray(ctx.chimeraBrokerProviderSnapshot.data.providers)) return [];
    var snapshotAgeMs = Date.now() - Number(ctx.chimeraBrokerProviderSnapshot.fetchedClientMs || 0);
    if (snapshotAgeMs > CHIMERA_BROKER_PROVIDER_STALE_MS) return [];
    var providers = ctx.chimeraBrokerProviderSnapshot.data.providers;
    for (var i = 0; i < providers.length; i++) {
      var row = providers[i] || {};
      if (String(row.id || "").toLowerCase() !== pid) continue;
      var mids = Array.isArray(row.model_ids) ? row.model_ids : [];
      var seen = {};
      var out = [];
      for (var j = 0; j < mids.length; j++) {
        var mid = String(mids[j] || "").trim();
        if (!mid) continue;
        var key = mid.toLowerCase();
        if (seen[key]) continue;
        seen[key] = true;
        out.push(mid);
      }
      return out;
    }
    return [];
  }

  function adminProviderUsageRows(providerId) {
    var out = {};
    var listedModels = adminProviderCatalogModels(providerId);
    for (var li = 0; li < listedModels.length; li++) {
      var listed = String(listedModels[li] || "").trim();
      if (!listed) continue;
      out[listed] = { model_id: listed, calls: 0, errors: 0 };
    }
    var data = ctx.metricsCache || {};
    var rows = Array.isArray(data.day_rollups) && data.day_rollups.length
      ? data.day_rollups
      : (Array.isArray(data.minute_rollups) ? data.minute_rollups : []);
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i] || {};
      var mid = String(r.model_id || "");
      var pm = adminExtractProviderModel(mid);
      var provider = String(r.provider || pm.provider || "").toLowerCase();
      if (!provider || provider !== String(providerId).toLowerCase()) continue;
      var key = mid || provider + "/(unknown)";
      if (!out[key]) out[key] = { model_id: key, calls: 0, errors: 0 };
      out[key].calls += Number(r.calls) || 0;
      var status = Number(r.status);
      if (!isNaN(status) && (status < 200 || status >= 300)) out[key].errors += Number(r.calls) || 0;
    }
    var list = [];
    for (var k in out) {
      if (Object.prototype.hasOwnProperty.call(out, k)) list.push(out[k]);
    }
    list.sort(function (a, b) {
      var dc = (b.calls || 0) - (a.calls || 0);
      if (dc !== 0) return dc;
      var de = (b.errors || 0) - (a.errors || 0);
      if (de !== 0) return de;
      return String(a.model_id || "").localeCompare(String(b.model_id || ""));
    });
    return list;
  }

  function adminProviderModelsDraftMap(providerId) {
    if (ctx.adminProviderModelsEditingId !== providerId) return null;
    if (!ctx.adminProviderModelsDraft || !ctx.adminProviderModelsDraft[providerId]) return null;
    return ctx.adminProviderModelsDraft[providerId].models || null;
  }

  function adminProviderModelsMapFromResponse(doc) {
    var out = {};
    var rows = doc && Array.isArray(doc.models) ? doc.models : [];
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i] || {};
      var mid = String(row.model_id || "").trim();
      if (!mid) continue;
      out[mid] = !!row.available;
    }
    return out;
  }

  function adminProviderModelsAvailabilityMap(providerId) {
    var draft = adminProviderModelsDraftMap(providerId);
    if (draft) return draft;
    if (ctx.adminProviderModelsCache && ctx.adminProviderModelsCache[providerId]) {
      return adminProviderModelsMapFromResponse(ctx.adminProviderModelsCache[providerId]);
    }
    return null;
  }

  function adminProviderModelEditRows(providerId) {
    var availMap = adminProviderModelsAvailabilityMap(providerId);
    var usageRows = adminProviderUsageRows(providerId);
    if (!availMap) {
      return usageRows.map(function (ur) {
        return {
          model_id: ur.model_id,
          calls: ur.calls,
          errors: ur.errors,
          available: true
        };
      });
    }
    var seen = {};
    var out = [];
    for (var ui = 0; ui < usageRows.length; ui++) {
      var ur = usageRows[ui];
      var mid = String(ur.model_id || "").trim();
      if (!mid) continue;
      seen[mid] = true;
      out.push({
        model_id: mid,
        calls: ur.calls,
        errors: ur.errors,
        available: availMap[mid] !== false
      });
    }
    for (var dm in availMap) {
      if (!Object.prototype.hasOwnProperty.call(availMap, dm)) continue;
      if (seen[dm]) continue;
      out.push({ model_id: dm, calls: 0, errors: 0, available: !!availMap[dm] });
    }
    out.sort(function (a, b) {
      var dc = (b.calls || 0) - (a.calls || 0);
      if (dc !== 0) return dc;
      var de = (b.errors || 0) - (a.errors || 0);
      if (de !== 0) return de;
      return String(a.model_id || "").localeCompare(String(b.model_id || ""));
    });
    return out;
  }

  function adminProviderSupportsFreeTierAssist(providerId) {
    var pid = String(providerId || "").toLowerCase();
    return pid === "groq" || pid === "gemini";
  }

  function adminModelUsageById() {
    var out = {};
    var data = ctx.metricsCache || {};
    var rows = Array.isArray(data.day_rollups) && data.day_rollups.length
      ? data.day_rollups
      : (Array.isArray(data.minute_rollups) ? data.minute_rollups : []);
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i] || {};
      var mid = String(r.model_id || "").trim();
      if (!mid) continue;
      out[mid] = (out[mid] || 0) + (Number(r.calls) || 0);
    }
    return out;
  }

  function adminProviderTierSpan(provider) {
    var p = String(provider || "").toLowerCase();
    var tier = "sum-conv-tier--inferred";
    var label = provider || "";
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
    return '<span class="sum-conv-tier ' + tier + '">' + escapeHtml(label) + "</span>";
  }

  function adminScopedEventsForPrincipal(principalId, maxN) {
    var want = String(principalId || "").trim();
    var out = [];
    for (var i = entryCache.length - 1; i >= 0; i--) {
      var ev = entryCache[i];
      var f = getFlat(ev.parsed);
      if (adminPrincipalForFlat(f) !== want) continue;
      out.push(ev);
      if (out.length >= maxN) break;
    }
    return out;
  }

  function adminUserStatsByPrincipal() {
    var map = {};
    for (var i = 0; i < entryCache.length; i++) {
      var ev = entryCache[i];
      var f = getFlat(ev.parsed);
      var pid = adminPrincipalForFlat(f);
      if (!pid) continue;
      if (!map[pid]) map[pid] = { conv: {}, ws: {} };
      var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
      if (cid) map[pid].conv[cid] = true;
      var proj = String(
        f.scope_project_id != null ? f.scope_project_id
          : f.project_id != null ? f.project_id
          : f.ingest_project != null ? f.ingest_project
          : ""
      ).trim();
      if (proj) map[pid].ws[proj] = true;
    }
    return map;
  }

  function adminProviderEventMatches(f, providerId) {
    var pid = String(providerId || "").trim().toLowerCase();
    if (!pid) return false;
    var prov = String(f.provider_id || f.provider || f.upstream_provider || "")
      .trim()
      .toLowerCase();
    if (prov === pid) return true;
    var model = String(f.upstreamModel || f.model || "")
      .trim()
      .toLowerCase();
    if (model.indexOf(pid + "/") === 0) return true;
    var msgEv = String(f.msg || f.message || "").toLowerCase();
    if (msgEv.indexOf(pid) >= 0) return true;
    var det = String(f.progress_detail || f.progressDetail || "").toLowerCase();
    if (det.indexOf(pid) >= 0) return true;
    return false;
  }

  function adminScopedEventsForRouting(kind) {
    var out = [];
    var want = String(kind || "").toLowerCase();
    for (var i = entryCache.length - 1; i >= 0 && out.length < 18; i--) {
      var ev = entryCache[i];
      var f = getFlat(ev.parsed);
      var msg = String(f.msg || f.message || "").toLowerCase();
      var hit = false;
      if (want === "rules") {
        hit = msg.indexOf("routing") >= 0 || msg.indexOf("virtual model") >= 0;
      } else if (want === "fallback") {
        hit = msg.indexOf("fallback") >= 0 || msg.indexOf("failover") >= 0;
      } else if (want === "router") {
        hit = msg.indexOf("router") >= 0 || msg.indexOf("tool_router") >= 0 || msg.indexOf("tool-router") >= 0;
      }
      if (hit) out.push(ev);
    }
    return out;
  }

  function adminScopedEvlogPanelFromEvents(title, scopeId, evs, opts) {
    if (globalThis.ChimeraShared && ChimeraShared.ScopedEvlog && typeof ChimeraShared.ScopedEvlog.panelFromEvents === "function") {
      return ChimeraShared.ScopedEvlog.panelFromEvents(title, scopeId, evs, opts);
    }
    return "";
  }

  ctx.operatorSectionHeadHtml = operatorSectionHeadHtml;
  ctx.operatorSectionAddBtn = operatorSectionAddBtn;
  ctx.operatorSectionAddBarHtml = operatorSectionAddBarHtml;
  ctx.operatorCardChevronHtml = operatorCardChevronHtml;
  ctx.operatorConfigureBtnInline = operatorConfigureBtnInline;
  ctx.sgOpHealthPillHtml = sgOpHealthPillHtml;
  ctx.fallbackChainToYAML = fallbackChainToYAML;
  ctx.parseFallbackChainInput = parseFallbackChainInput;
  ctx.providerRowsHtml = providerRowsHtml;
  ctx.providerKeyAddBlockHtml = providerKeyAddBlockHtml;
  ctx.adminProviderIntro = adminProviderIntro;
  ctx.adminProviderAvatarClass = adminProviderAvatarClass;
  ctx.adminProviderHealthEntry = adminProviderHealthEntry;
  ctx.adminProviderAvailabilityHtml = adminProviderAvailabilityHtml;
  ctx.adminProviderModelCount = adminProviderModelCount;
  ctx.countRoutingRulesFromYAML = countRoutingRulesFromYAML;
  ctx.parseRoutingYamlScalar = parseRoutingYamlScalar;
  ctx.parseRoutingRulesFromYAML = parseRoutingRulesFromYAML;
  ctx.adminPrincipalForFlat = adminPrincipalForFlat;
  ctx.adminExtractProviderModel = adminExtractProviderModel;
  ctx.adminProviderCatalogModels = adminProviderCatalogModels;
  ctx.adminProviderUsageRows = adminProviderUsageRows;
  ctx.adminProviderModelsDraftMap = adminProviderModelsDraftMap;
  ctx.adminProviderModelsMapFromResponse = adminProviderModelsMapFromResponse;
  ctx.adminProviderModelEditRows = adminProviderModelEditRows;
  ctx.adminProviderSupportsFreeTierAssist = adminProviderSupportsFreeTierAssist;
  ctx.adminModelUsageById = adminModelUsageById;
  ctx.adminProviderTierSpan = adminProviderTierSpan;
  ctx.adminProviderEventMatches = adminProviderEventMatches;
  ctx.adminScopedEventsForPrincipal = adminScopedEventsForPrincipal;
  ctx.adminUserStatsByPrincipal = adminUserStatsByPrincipal;
  ctx.adminScopedEventsForRouting = adminScopedEventsForRouting;
  ctx.adminScopedEvlogPanelFromEvents = adminScopedEvlogPanelFromEvents;
};
