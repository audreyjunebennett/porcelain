/**
 * Gallery-only: toggle body.gallery-show-parts and honor ?parts=1 for shareable review links.
 */
(function () {
  var STORAGE_KEY = "chimera-gallery-show-parts";

  function readStored() {
    try {
      return localStorage.getItem(STORAGE_KEY) === "1";
    } catch (_e) {
      return false;
    }
  }

  function writeStored(on) {
    try {
      if (on) localStorage.setItem(STORAGE_KEY, "1");
      else localStorage.removeItem(STORAGE_KEY);
    } catch (_e) {
      /* ignore */
    }
  }

  function queryWantsParts() {
    try {
      var q = new URLSearchParams(window.location.search);
      return q.get("parts") === "1" || q.get("parts") === "true";
    } catch (_e) {
      return false;
    }
  }

  function setShowParts(on) {
    document.body.classList.toggle("gallery-show-parts", !!on);
    var btn = document.getElementById("gallery-parts-toggle");
    if (btn) btn.setAttribute("aria-pressed", on ? "true" : "false");
    writeStored(!!on);
  }

  function init() {
    var initial = queryWantsParts() || readStored();
    setShowParts(initial);
    var btn = document.getElementById("gallery-parts-toggle");
    if (!btn) return;
    btn.addEventListener("click", function () {
      setShowParts(!document.body.classList.contains("gallery-show-parts"));
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
