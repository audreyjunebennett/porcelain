const state = {
  items: [],
  index: 0,
  selection: null,
  audioSelection: {start: null, end: null},
  group: "all",
  before: null,
};

const $ = selector => document.querySelector(selector);
const transcript = $("#transcript");
const notice = $("#notice");
const audio = $("#audio");

function escapeHtml(value) {
  return String(value).replace(/[&<>'"]/g, character => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;",
  })[character]);
}

function formatDuration(seconds) {
  const value = Math.max(0, Math.round(Number(seconds) || 0));
  return `${Math.floor(value / 60)}:${String(value % 60).padStart(2, "0")}`;
}

function formatTimestamp(seconds) {
  const value = Math.max(0, Number(seconds) || 0);
  const minutes = Math.floor(value / 60);
  const remainder = (value % 60).toFixed(1).padStart(4, "0");
  return `${minutes}:${remainder}`;
}

function current() { return state.items[state.index]; }

function setNotice(message, error = false) {
  notice.textContent = message;
  notice.style.color = error ? "#e0928d" : "";
}

function renderProgress(progress) {
  const people = ["Ruby", "Lynn", "Raven"];
  $("#progress-people").innerHTML = people.map(name => {
    const value = progress[name] || {seconds: 0, clips: 0};
    const percent = Math.min(100, (value.seconds / 300) * 100);
    return `<div class="person"><div class="person-head"><span>${name}</span><span class="muted">${formatDuration(value.seconds)}</span></div><div class="meter"><span style="width:${percent}%"></span></div></div>`;
  }).join("");
}

function renderGroups(groups) {
  const select = $("#group");
  const active = select.value || state.group;
  select.innerHTML = `<option value="all">All suggestions</option>` + groups.map(group => {
    const count = group.clips == null ? "" : ` · ${group.clips}`;
    return `<option value="${encodeURIComponent(group.name)}">${group.name}${count}</option>`;
  }).join("");
  select.value = active;
}

function renderAnnotations(item) {
  $("#annotations").innerHTML = (item.annotations || []).map(annotation => {
    const hasAudioRange = annotation.audio_start_seconds != null && annotation.audio_end_seconds != null;
    const range = hasAudioRange
      ? `${formatTimestamp(annotation.audio_start_seconds)}–${formatTimestamp(annotation.audio_end_seconds)}`
      : "";
    const text = annotation.selected_text ? `“${annotation.selected_text}”` : "";
    const scope = [range, text].filter(Boolean).join(" · ") || "whole clip";
    const value = annotation.annotation_type === "transcript"
      ? `${scope} → “${annotation.replacement_text}”`
      : `${annotation.label}: ${scope}`;
    return `<button class="annotation" data-undo="${escapeHtml(annotation.annotation_id)}" title="Tap to undo">${escapeHtml(value)} ×</button>`;
  }).join("");
}

function activeAudioRange() {
  const {start, end} = state.audioSelection;
  return Number.isFinite(start) && Number.isFinite(end) && end > start
    ? {start, end}
    : null;
}

function renderAudioRange() {
  const range = activeAudioRange();
  const start = state.audioSelection.start;
  $("#audio-range").textContent = range
    ? `${formatTimestamp(range.start)} – ${formatTimestamp(range.end)}`
    : Number.isFinite(start)
      ? `${formatTimestamp(start)} – choose end`
      : "Whole clip";
}

function clearAudioRange() {
  state.audioSelection = {start: null, end: null};
  renderAudioRange();
}

function setAudioBoundary(boundary) {
  const item = current();
  if (!item) return;
  const mediaDuration = Number.isFinite(audio.duration) ? audio.duration : item.duration_seconds;
  const at = Math.max(0, Math.min(Number(audio.currentTime) || 0, Number(mediaDuration) || 30));
  if (boundary === "start") {
    state.audioSelection.start = at;
    if (Number.isFinite(state.audioSelection.end) && state.audioSelection.end <= at) {
      state.audioSelection.end = null;
    }
    setNotice(`Start set at ${formatTimestamp(at)}. Scrub forward and tap End here.`);
  } else {
    const start = Number.isFinite(state.audioSelection.start) ? state.audioSelection.start : 0;
    if (at <= start) {
      setNotice("The end must be after the selected start.", true);
      return;
    }
    state.audioSelection.start = start;
    state.audioSelection.end = at;
    setNotice(`Audio range selected: ${formatTimestamp(start)} – ${formatTimestamp(at)}.`);
  }
  renderAudioRange();
}

function renderCard() {
  const item = current();
  const card = $("#card");
  const empty = $("#empty");
  if (!item) {
    card.hidden = true;
    empty.hidden = false;
    $("#position").textContent = "0 clips";
    return;
  }
  empty.hidden = true;
  card.hidden = false;
  state.selection = null;
  clearAudioRange();
  const when = item.captured_at.replace("_", " · ").replaceAll("-", ":");
  $("#time").textContent = when;
  const proposal = item.proposal;
  $("#guess").textContent = proposal
    ? `${item.review_group} · ${Math.round((proposal.confidence || 0) * 100)}% model guess`
    : "Unsorted · teach me this one";
  audio.src = `/api/motox/review/audio/${encodeURIComponent(item.capture_id)}`;
  transcript.textContent = item.transcript;
  $("#selection").textContent = "Whole clip selected";
  $("#position").textContent = `${state.index + 1} of ${state.items.length}`;
  renderAnnotations(item);
  setNotice("");
}

function selectionInsideTranscript() {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return null;
  const range = selection.getRangeAt(0);
  if (!transcript.contains(range.commonAncestorContainer)) return null;
  const before = range.cloneRange();
  before.selectNodeContents(transcript);
  before.setEnd(range.startContainer, range.startOffset);
  const start = before.toString().length;
  const text = range.toString();
  return text.trim() ? {start, end: start + text.length, text} : null;
}

function captureSelection() {
  state.selection = selectionInsideTranscript();
  $("#selection").textContent = state.selection
    ? `Selected: “${state.selection.text}”`
    : "Whole clip selected";
}

async function requestJson(url, options) {
  const response = await fetch(url, options);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `Request failed (${response.status})`);
  return body;
}

async function saveAnnotation(type, label, replacementText = null) {
  const item = current();
  if (!item) return;
  // Dialog focus clears the browser's text selection. Preserve the range that
  // was captured when "Fix selected words" opened.
  const range = activeAudioRange();
  const selection = state.selection || (range
    ? {start: 0, end: 0, text: ""}
    : {start: 0, end: item.transcript.length, text: item.transcript});
  try {
    const annotation = await requestJson("/api/motox/review/annotations", {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: JSON.stringify({
        capture_id: item.capture_id,
        start_char: selection.start,
        end_char: selection.end,
        annotation_type: type,
        label,
        replacement_text: replacementText,
        audio_start_seconds: range?.start ?? null,
        audio_end_seconds: range?.end ?? null,
      }),
    });
    item.annotations = [...(item.annotations || []), annotation];
    renderAnnotations(item);
    window.getSelection()?.removeAllRanges();
    state.selection = null;
    $("#selection").textContent = "Whole clip selected";
    const rangeNotice = range ? ` for ${formatTimestamp(range.start)} – ${formatTimestamp(range.end)}` : "";
    setNotice(`${label || "Correction"} saved${rangeNotice}. Tap the chip below to undo.`);
    refreshProgress();
  } catch (error) {
    setNotice(error.message, true);
  }
}

async function undo(annotationId) {
  try {
    await requestJson(`/api/motox/review/annotations/${encodeURIComponent(annotationId)}`, {method: "DELETE"});
    const item = current();
    item.annotations = (item.annotations || []).filter(row => row.annotation_id !== annotationId);
    renderAnnotations(item);
    setNotice("Correction undone.");
    refreshProgress();
  } catch (error) {
    setNotice(error.message, true);
  }
}

async function refreshProgress() {
  const progress = await requestJson("/api/motox/review/progress");
  renderProgress(progress);
}

async function loadBatch({older = false} = {}) {
  setNotice("Loading review clips…");
  const params = new URLSearchParams({limit: "40", target_seconds: "300"});
  if (state.group !== "all") params.set("group", state.group);
  if (older && state.before) params.set("before", state.before);
  try {
    const [items, progress, groups] = await Promise.all([
      requestJson(`/api/motox/review/candidates?${params}`),
      requestJson("/api/motox/review/progress"),
      requestJson("/api/motox/review/groups"),
    ]);
    state.items = items;
    state.index = 0;
    if (items.length) state.before = items[items.length - 1].captured_at;
    renderProgress(progress);
    renderGroups(groups);
    renderCard();
  } catch (error) {
    setNotice(error.message, true);
  }
}

function nextClip() {
  if (!state.items.length) return;
  state.index += 1;
  if (state.index >= state.items.length) {
    loadBatch({older: true});
  } else {
    renderCard();
  }
}

transcript.addEventListener("mouseup", captureSelection);
transcript.addEventListener("touchend", () => setTimeout(captureSelection, 100));
$("#range-start").addEventListener("click", () => setAudioBoundary("start"));
$("#range-end").addEventListener("click", () => setAudioBoundary("end"));
$("#range-clear").addEventListener("click", () => {
  clearAudioRange();
  setNotice("Audio selection cleared; labels will use selected text or the whole clip.");
});
$("#labels").addEventListener("click", event => {
  const button = event.target.closest("[data-label]");
  if (button) saveAnnotation(button.dataset.type, button.dataset.label);
});
$("#annotations").addEventListener("click", event => {
  const button = event.target.closest("[data-undo]");
  if (button) undo(button.dataset.undo);
});
$("#next").addEventListener("click", nextClip);
$("#skip").addEventListener("click", nextClip);
$("#older").addEventListener("click", () => loadBatch({older: true}));
$("#refresh").addEventListener("click", () => loadBatch());
$("#group").addEventListener("change", event => {
  state.group = decodeURIComponent(event.target.value);
  state.before = null;
  loadBatch();
});
$("#fix").addEventListener("click", () => {
  captureSelection();
  const item = current();
  if (!item) return;
  const selected = state.selection?.text || item.transcript;
  $("#original").textContent = selected;
  $("#replacement").value = selected;
  $("#fix-dialog").showModal();
  $("#replacement").focus();
  $("#replacement").select();
});
$("#fix-form").addEventListener("submit", event => {
  if (event.submitter?.value === "cancel") return;
  event.preventDefault();
  const replacement = $("#replacement").value.trim();
  if (!replacement) return;
  $("#fix-dialog").close();
  saveAnnotation("transcript", null, replacement);
});

loadBatch();
