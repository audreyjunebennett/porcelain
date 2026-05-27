/**
 * Provider catalog client — loads GET /api/ui/providers/catalog (Go BFF source of truth).
 *
 * ctx.adminProviderCatalog: ProviderCatalogEntry[] from last fetch.
 * ctx.adminVisibleProviderIds: ordered provider ids for summarized cards (session).
 *   Seeded once from configured_ids when still empty after the first successful catalog fetch.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Providers = globalThis.ChimeraSettings.Providers || {};

(function () {
  var catalogById = Object.create(null);
  var catalogList = [];
  var loadPromise = null;

  function normalizeId(id) {
    return String(id || "")
      .trim()
      .toLowerCase();
  }

  function rebuildIndex(entries) {
    catalogById = Object.create(null);
    catalogList = [];
    for (var i = 0; i < entries.length; i++) {
      var e = entries[i];
      if (!e || !e.id) continue;
      catalogById[normalizeId(e.id)] = e;
      catalogList.push(e);
    }
  }

  function lookupProviderSpec(id) {
    var e = catalogById[normalizeId(id)];
    if (!e) return null;
    return {
      id: e.id,
      title: e.title,
      avatar: e.avatar,
      subtitle: e.subtitle,
      kind: e.kind,
      key_placeholder: e.key_placeholder
    };
  }

  function providerCatalogEntries() {
    return catalogList.slice();
  }

  function visibleIdSet(ctx) {
    var set = Object.create(null);
    var visible = ctx && ctx.adminVisibleProviderIds ? ctx.adminVisibleProviderIds : [];
    for (var i = 0; i < visible.length; i++) {
      var id = normalizeId(visible[i]);
      if (id) set[id] = true;
    }
    return set;
  }

  function addableProviderEntries(ctx) {
    var seen = visibleIdSet(ctx);
    var out = [];
    for (var i = 0; i < catalogList.length; i++) {
      var e = catalogList[i];
      if (!e || !e.id) continue;
      if (!seen[normalizeId(e.id)]) out.push(e);
    }
    return out;
  }

  function hasAddableProviders(ctx) {
    return addableProviderEntries(ctx).length > 0;
  }

  function addVisibleProviderId(ctx, id) {
    if (!ctx) return false;
    id = normalizeId(id);
    if (!id || !lookupProviderSpec(id)) return false;
    if (!Array.isArray(ctx.adminVisibleProviderIds)) ctx.adminVisibleProviderIds = [];
    for (var i = 0; i < ctx.adminVisibleProviderIds.length; i++) {
      if (normalizeId(ctx.adminVisibleProviderIds[i]) === id) return false;
    }
    ctx.adminVisibleProviderIds.push(id);
    ctx.adminVisibleProviderIdsSeeded = true;
    return true;
  }

  function buildSpecsFromVisibleIds(visibleIds) {
    var specs = [];
    if (!visibleIds || !visibleIds.length) return specs;
    for (var i = 0; i < visibleIds.length; i++) {
      var spec = lookupProviderSpec(visibleIds[i]);
      if (spec) specs.push(spec);
    }
    return specs;
  }

  function applyVisibleProviderIds(ctx, ids) {
    if (!ctx) return;
    if (!Array.isArray(ctx.adminVisibleProviderIds)) ctx.adminVisibleProviderIds = [];
    ctx.adminVisibleProviderIds.length = 0;
    for (var vi = 0; vi < ids.length; vi++) ctx.adminVisibleProviderIds.push(ids[vi]);
  }

  function seedVisibleProviderIds(ctx, configuredIds) {
    if (!ctx) return;
    if (ctx.adminVisibleProviderIdsSeeded && ctx.adminVisibleProviderIds && ctx.adminVisibleProviderIds.length > 0) {
      return;
    }
    var ids = [];
    if (configuredIds && configuredIds.length) {
      for (var i = 0; i < configuredIds.length; i++) {
        var id = normalizeId(configuredIds[i]);
        if (id && lookupProviderSpec(id)) ids.push(id);
      }
    }
    applyVisibleProviderIds(ctx, ids);
    if (ids.length > 0) ctx.adminVisibleProviderIdsSeeded = true;
  }

  function fetchProviderCatalog(ctx, opts) {
    opts = opts || {};
    if (ctx && ctx.uiUnauthorized) return Promise.resolve(null);
    if (opts.force) loadPromise = null;
    if (loadPromise) return loadPromise;
    loadPromise = fetch("/api/ui/providers/catalog", { credentials: "same-origin" })
      .then(function (res) {
        if (res.status === 401) {
          if (ctx && typeof ctx.markUiUnauthorized === "function") ctx.markUiUnauthorized();
          return null;
        }
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (data) {
        if (!data) return null;
        var entries = Array.isArray(data.providers) ? data.providers : [];
        rebuildIndex(entries);
        if (ctx) {
          ctx.adminProviderCatalog = entries;
          seedVisibleProviderIds(ctx, data.configured_ids);
          ctx.adminProviderCatalogReady = true;
        }
        return data;
      })
      .catch(function (err) {
        loadPromise = null;
        throw err;
      });
    return loadPromise;
  }

  /** Install catalog rows without a network round-trip (tests, gallery fixtures). */
  function installCatalogEntries(entries) {
    rebuildIndex(Array.isArray(entries) ? entries : []);
  }

  globalThis.ChimeraSettings.Providers.Catalog = {
    installCatalogEntries: installCatalogEntries,
    lookupProviderSpec: lookupProviderSpec,
    providerCatalogEntries: providerCatalogEntries,
    addableProviderEntries: addableProviderEntries,
    hasAddableProviders: hasAddableProviders,
    addVisibleProviderId: addVisibleProviderId,
    buildSpecsFromVisibleIds: buildSpecsFromVisibleIds,
    fetchProviderCatalog: fetchProviderCatalog,
    seedVisibleProviderIds: seedVisibleProviderIds
  };
})();
