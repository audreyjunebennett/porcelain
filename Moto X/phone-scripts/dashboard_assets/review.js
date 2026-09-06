const state = {
  items: [],
  index: 0,
  selection: null,
  audioSelection: {start: null, end: null},
  mode: "identify",
  group: "all",
  before: null,
  showOriginal: false,
  loopSelection: false,
  contextRadius: 2,
  seenTurnIds: new Set(),
  seenCaptureIds: new Set(),
  identifiedThisSession: 0,
  pendingSpeakerLabel: null,
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

function formatCapturedAt(value) {
  const source = String(value);
  const capture = source.match(/^(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})-(\d{2})$/);
  const parsed = capture
    ? new Date(...capture.slice(1).map((part, index) => Number(part) - (index === 1 ? 1 : 0)))
    : new Date(source);
  if (!Number.isNaN(parsed.valueOf())) {
    return parsed.toLocaleString(undefined, {
      month: "short", day: "numeric", hour: "numeric", minute: "2-digit", second: "2-digit",
    });
  }
  return source.replace("_", " · ");
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

function annotationsForCurrentCard(item) {
  const identification = item?.identification;
  if (!identification) return item?.annotations || [];
  const turnStart = Number(identification.audio_start_seconds);
  const turnEnd = Number(identification.audio_end_seconds);
  return (item.annotations || []).filter(annotation => {
    if (annotation.speaker_turn_id) {
      return annotation.speaker_turn_id === identification.turn_id;
    }
    const annotationStart = Number(annotation.audio_start_seconds);
    const annotationEnd = Number(annotation.audio_end_seconds);
    return Number.isFinite(annotationStart) && Number.isFinite(annotationEnd)
      && annotationEnd > turnStart && annotationStart < turnEnd;
  });
}

function renderAnnotations(item) {
  $("#annotations").innerHTML = annotationsForCurrentCard(item).map(annotation => {
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

function hasSavedNonSpeakerLabel() {
  return annotationsForCurrentCard(current()).some(annotation =>
    ["sound", "overlap", "boundary"].includes(annotation.annotation_type)
  );
}

function renderIdentificationAction() {
  const button = $("#confirm-speaker");
  if (!current()?.identification) {
    button.hidden = true;
    return;
  }
  button.hidden = false;
  if (state.pendingSpeakerLabel) {
    button.disabled = false;
    button.textContent = `Confirm ${state.pendingSpeakerLabel} & next`;
  } else if (hasSavedNonSpeakerLabel()) {
    button.disabled = false;
    button.textContent = "Next clip";
  } else {
    button.disabled = true;
    button.textContent = "Choose a voice or sound first";
  }
}

function renderIdentificationLabels() {
  const item = current();
  if (!item?.identification) return;
  const annotations = annotationsForCurrentCard(item);
  document.querySelectorAll("#sound-labels [data-label]").forEach(button => {
    const saved = annotations.some(annotation =>
      annotation.annotation_type === button.dataset.type
      && annotation.label === button.dataset.label
    );
    button.classList.toggle("saved", saved);
    button.disabled = saved;
    button.setAttribute("aria-pressed", String(saved));
  });
}

function correctedTranscript(item) {
  let value = item.transcript;
  const corrections = (item.annotations || [])
    .filter(row => row.annotation_type === "transcript" && row.replacement_text != null)
    .sort((left, right) => right.start_char - left.start_char);
  for (const correction of corrections) {
    value = value.slice(0, correction.start_char) + correction.replacement_text + value.slice(correction.end_char);
  }
  return value;
}

function scopeDescription() {
  const item = current();
  const range = activeAudioRange();
  if (range && state.selection) {
    return [`${formatTimestamp(range.start)}–${formatTimestamp(range.end)}`, `“${state.selection.text}”`];
  }
  if (range) return [`${formatTimestamp(range.start)}–${formatTimestamp(range.end)}`, "selected audio"];
  if (state.selection) return ["SELECTED WORDS", `“${state.selection.text}”`];
  return ["WHOLE CLIP", item ? formatDuration(item.duration_seconds) : ""];
}

function renderScope() {
  const [title, detail] = scopeDescription();
  const whole = !state.selection && !activeAudioRange();
  const scope = $("#scope");
  scope.classList.toggle("whole", whole);
  scope.innerHTML = `<span>Applying to</span><strong>${escapeHtml(title)}</strong><small>${escapeHtml(detail)}</small>`;
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
  renderScope();
}

function clearAudioRange() {
  state.audioSelection = {start: null, end: null};
  renderAudioRange();
}

function restoreIdentificationRange() {
  const identification = current()?.identification;
  if (!identification) {
    clearAudioRange();
    return;
  }
  state.audioSelection = {
    start: Number(identification.audio_start_seconds),
    end: Number(identification.audio_end_seconds),
  };
  renderAudioRange();
}

function setLoop(enabled) {
  state.loopSelection = Boolean(enabled);
  const button = $("#range-loop");
  button.setAttribute("aria-pressed", String(state.loopSelection));
  button.textContent = state.loopSelection ? "Loop on" : "Loop off";
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

function renderTranscript(item) {
  if (item.identification) {
    transcript.textContent = item.identification.selected_text
      || "No word-timed excerpt is available for this older capture. Identify the voice from the selected audio; the full transcript remains in Browse clips.";
    transcript.classList.remove("corrected");
    $("#transcript-version").hidden = true;
    return;
  }
  const corrected = item.corrected_transcript ?? item.transcript;
  const hasCorrections = corrected !== item.transcript;
  const button = $("#transcript-version");
  button.hidden = !hasCorrections;
  button.textContent = state.showOriginal ? "Show corrected" : "Show original";
  transcript.textContent = state.showOriginal ? item.transcript : corrected;
  transcript.classList.toggle("corrected", hasCorrections && !state.showOriginal);
}

function renderCard() {
  const item = current();
  const card = $("#card");
  const empty = $("#empty");
  if (!item) {
    card.hidden = true;
    empty.hidden = false;
    $("#position").textContent = state.mode === "identify" ? "0 questions" : "0 clips";
    $("#empty-message").textContent = state.mode === "identify"
      ? state.seenCaptureIds.size
        ? "You finished this pass across distinct recordings. Tap Refresh for another pass through any remaining turns."
        : "No unlabeled diarized turns are ready yet. Import a diarization report, or switch to Browse clips."
      : "No review clips are available in this group.";
    $("#older").hidden = state.mode === "identify";
    return;
  }
  empty.hidden = true;
  card.hidden = false;
  $("#older").hidden = state.mode === "identify";
  state.selection = null;
  state.pendingSpeakerLabel = null;
  state.showOriginal = false;
  state.contextRadius = 2;
  $("#context-items").innerHTML = "";
  $("#more-context").hidden = true;
  const identification = item.identification;
  card.classList.toggle("smart-identification", Boolean(identification));
  if (identification) {
    state.audioSelection = {
      start: Number(identification.audio_start_seconds),
      end: Number(identification.audio_end_seconds),
    };
    renderAudioRange();
    setLoop(true);
  } else {
    clearAudioRange();
    setLoop(false);
  }
  $("#time").textContent = identification
    ? `${formatCapturedAt(identification.occurred_at)} · source ${formatTimestamp(identification.audio_start_seconds)}–${formatTimestamp(identification.audio_end_seconds)}`
    : formatCapturedAt(item.captured_at);
  const proposal = item.proposal;
  $("#guess").textContent = identification
    ? `${identification.prompt} · ${identification.duration_seconds.toFixed(1)} seconds`
    : proposal
      ? `${item.review_group} · ${Math.round((proposal.confidence || 0) * 100)}% model guess`
      : "Unsorted · teach me this one";
  $("#learning-reason").hidden = !identification;
  $("#learning-reason").textContent = identification?.reason || "";
  $("#audio-range-actions").hidden = Boolean(identification);
  $("#instruction").textContent = identification
    ? "This exact diarized turn is selected and will loop while it plays."
    : "Select words to select and loop their audio automatically.";
  $("#speaker-prompt").innerHTML = identification
    ? `${escapeHtml(identification.prompt)} <span>Other and Not sure are always okay</span>`
    : "Who is vocalizing? <span>one identity per region</span>";
  document.querySelectorAll("#labels [data-label]").forEach(button => {
    if (button.dataset.label === "Not sure") {
      button.textContent = identification ? "Not sure / mixed voices" : "Not sure";
    }
    button.classList.toggle("suggested", Boolean(identification?.alternatives.includes(button.dataset.label)));
    button.classList.toggle("pending", Boolean(
      identification && state.pendingSpeakerLabel === button.dataset.label
    ));
    button.setAttribute("aria-pressed", String(
      Boolean(identification && state.pendingSpeakerLabel === button.dataset.label)
    ));
  });
  audio.src = `/api/motox/review/audio/${encodeURIComponent(item.capture_id)}`;
  renderTranscript(item);
  $("#selection").textContent = identification
    ? identification.selected_text
      ? `Turn text: “${identification.selected_text}”`
      : "Exact audio turn selected"
    : "Whole clip selected";
  $("#position").textContent = state.mode === "identify"
    ? `Question ${state.identifiedThisSession + 1}`
    : `${state.index + 1} of ${state.items.length}`;
  $("#next").textContent = "Next clip";
  $("#next").className = "primary";
  $("#fix").hidden = Boolean(identification);
  $("#next").hidden = Boolean(identification);
  renderAnnotations(item);
  renderIdentificationLabels();
  renderIdentificationAction();
  renderScope();
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
  if (current()?.identification) {
    setNotice("Smart identification keeps this diarized turn's exact audio boundary.");
    return;
  }
  state.selection = selectionInsideTranscript();
  $("#selection").textContent = state.selection
    ? `Selected: “${state.selection.text}”`
    : "Whole clip selected";
  if (state.selection) syncAudioToSelectedWords();
  renderScope();
}

function syncAudioToSelectedWords() {
  const item = current();
  if (!item || !state.selection || !item.words?.length) return;
  if (!state.showOriginal && item.corrected_transcript !== item.transcript) {
    setNotice("Show the original transcript to sync edited words to their exact audio.");
    return;
  }
  const selectedWords = item.words.filter(word =>
    Number(word.char_end) > state.selection.start && Number(word.char_start) < state.selection.end
  );
  if (!selectedWords.length) return;
  state.audioSelection = {
    start: Number(selectedWords[0].start_seconds),
    end: Number(selectedWords[selectedWords.length - 1].end_seconds),
  };
  audio.currentTime = state.audioSelection.start;
  setLoop(true);
  renderAudioRange();
  setNotice(`Selected words synced to ${formatTimestamp(state.audioSelection.start)}–${formatTimestamp(state.audioSelection.end)}. Press play to loop them.`);
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
  const wholeClip = !state.selection && !range;
  if (wholeClip && type !== "transcript" && !window.confirm(
    `Apply “${label}” to the WHOLE ${formatDuration(item.duration_seconds)} clip?`
  )) return;
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
        speaker_turn_id: type === "speaker" ? item.identification?.turn_id ?? null : null,
      }),
    });
    item.annotations = [...(item.annotations || []), annotation];
    if (type === "transcript") {
      item.corrected_transcript = correctedTranscript(item);
      state.showOriginal = false;
      renderTranscript(item);
    }
    renderAnnotations(item);
    renderIdentificationLabels();
    renderIdentificationAction();
    window.getSelection()?.removeAllRanges();
    state.selection = null;
    if (item.identification) {
      restoreIdentificationRange();
      $("#selection").textContent = item.identification.selected_text
        ? `Turn text: “${item.identification.selected_text}”`
        : "Exact audio turn selected";
    } else {
      clearAudioRange();
      $("#selection").textContent = "Whole clip selected";
    }
    const rangeNotice = range ? ` for ${formatTimestamp(range.start)} – ${formatTimestamp(range.end)}` : "";
    await refreshProgress();
    if (item.identification && type === "speaker") {
      state.seenTurnIds.add(item.identification.turn_id);
      state.seenCaptureIds.add(item.capture_id);
      state.identifiedThisSession += 1;
      await loadBatch();
      setNotice(`${label} learned from ${item.identification.duration_seconds.toFixed(1)} seconds. The next question was re-ranked.`);
    } else {
      setNotice(`${label || "Correction"} saved${rangeNotice}. Tap the chip below to undo.`);
    }
  } catch (error) {
    setNotice(error.message, true);
  }
}

async function loadContext({more = false} = {}) {
  const item = current();
  if (!item) return;
  if (more) state.contextRadius = Math.min(12, state.contextRadius * 2);
  try {
    const rows = await requestJson(
      `/api/motox/review/context/${encodeURIComponent(item.capture_id)}?radius=${state.contextRadius}`
    );
    $("#context-items").innerHTML = rows.map(row => {
      const currentClass = row.capture_id === item.capture_id ? " current" : "";
      const label = row.capture_id === item.capture_id ? "current clip" : row.captured_at.slice(11).replaceAll("-", ":");
      return `<article class="context-item${currentClass}"><div><strong>${escapeHtml(label)}</strong><span>${escapeHtml(row.corrected_transcript || row.transcript)}</span></div><audio controls preload="none" src="/api/motox/review/audio/${encodeURIComponent(row.capture_id)}"></audio></article>`;
    }).join("");
    $("#more-context").hidden = state.contextRadius >= 12;
    setNotice(`Showing ${rows.length} connected clip${rows.length === 1 ? "" : "s"}. Labels still attach to the current source clip.`);
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
    renderIdentificationLabels();
    renderIdentificationAction();
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
  const identify = state.mode === "identify";
  setNotice(identify ? "Choosing the next useful voice question…" : "Loading review clips…");
  const params = new URLSearchParams({limit: "40"});
  let candidateUrl;
  if (identify) {
    for (const turnId of state.seenTurnIds) params.append("exclude", turnId);
    for (const captureId of state.seenCaptureIds) params.append("exclude_capture", captureId);
    candidateUrl = `/api/motox/review/identification?${params}`;
  } else {
    params.set("target_seconds", "300");
    if (state.group !== "all") params.set("group", state.group);
    if (older && state.before) params.set("before", state.before);
    candidateUrl = `/api/motox/review/candidates?${params}`;
  }
  try {
    const [items, progress, groups] = await Promise.all([
      requestJson(candidateUrl),
      requestJson("/api/motox/review/progress"),
      requestJson("/api/motox/review/groups"),
    ]);
    state.items = items;
    state.index = 0;
    if (!identify && items.length) state.before = items[items.length - 1].captured_at;
    renderProgress(progress);
    renderGroups(groups);
    renderCard();
  } catch (error) {
    setNotice(error.message, true);
  }
}

function nextClip() {
  if (!state.items.length) return;
  const identification = current()?.identification;
  if (identification) state.seenTurnIds.add(identification.turn_id);
  if (identification) state.seenCaptureIds.add(current().capture_id);
  if (identification) state.identifiedThisSession += 1;
  state.index += 1;
  if (state.index >= state.items.length) {
    loadBatch({older: !identification});
  } else {
    renderCard();
  }
}

function chooseSpeaker(label) {
  const item = current();
  if (!item?.identification) return;
  state.pendingSpeakerLabel = label;
  document.querySelectorAll("#labels [data-label]").forEach(button => {
    const pending = button.dataset.label === label;
    button.classList.toggle("pending", pending);
    button.setAttribute("aria-pressed", String(pending));
  });
  renderIdentificationAction();
  setNotice(`${label} selected. Confirm when you're ready.`);
}

async function confirmSpeakerAndNext() {
  const label = state.pendingSpeakerLabel;
  if (!label) {
    if (hasSavedNonSpeakerLabel()) {
      await saveAnnotation("speaker", "Not sure");
      return;
    }
    setNotice("Choose a voice or sound first.", true);
    return;
  }
  await saveAnnotation("speaker", label);
}

transcript.addEventListener("mouseup", captureSelection);
transcript.addEventListener("touchend", () => setTimeout(captureSelection, 100));
$("#range-start").addEventListener("click", () => setAudioBoundary("start"));
$("#range-end").addEventListener("click", () => setAudioBoundary("end"));
$("#range-clear").addEventListener("click", () => {
  clearAudioRange();
  setNotice("Audio selection cleared; labels will use selected text or the whole clip.");
});
$("#range-loop").addEventListener("click", () => setLoop(!state.loopSelection));
audio.addEventListener("timeupdate", () => {
  const range = activeAudioRange();
  if (state.loopSelection && range && audio.currentTime >= range.end) {
    audio.currentTime = range.start;
    audio.play().catch(() => {});
  }
});
audio.addEventListener("loadedmetadata", () => {
  const range = activeAudioRange();
  if (range) audio.currentTime = range.start;
});
const labelClick = event => {
  const button = event.target.closest("[data-label]");
  if (!button) return;
  if (current()?.identification && button.dataset.type === "speaker") {
    chooseSpeaker(button.dataset.label);
    return;
  }
  saveAnnotation(button.dataset.type, button.dataset.label);
};
$("#labels").addEventListener("click", labelClick);
$("#sound-labels").addEventListener("click", labelClick);
$("#boundary-labels").addEventListener("click", labelClick);
$("#annotations").addEventListener("click", event => {
  const button = event.target.closest("[data-undo]");
  if (button) undo(button.dataset.undo);
});
$("#next").addEventListener("click", nextClip);
$("#skip").addEventListener("click", nextClip);
$("#confirm-speaker").addEventListener("click", confirmSpeakerAndNext);
$("#older").addEventListener("click", () => loadBatch({older: true}));
$("#refresh").addEventListener("click", () => {
  state.seenTurnIds.clear();
  state.seenCaptureIds.clear();
  state.identifiedThisSession = 0;
  loadBatch();
});
$("#load-context").addEventListener("click", () => loadContext());
$("#more-context").addEventListener("click", () => loadContext({more: true}));
$("#transcript-version").addEventListener("click", () => {
  state.showOriginal = !state.showOriginal;
  window.getSelection()?.removeAllRanges();
  state.selection = null;
  restoreIdentificationRange();
  renderTranscript(current());
  $("#selection").textContent = current()?.identification
    ? "Exact audio turn selected"
    : "Whole clip selected";
});
$("#exclude").addEventListener("click", () => saveAnnotation("privacy", "Exclude"));
$("#group").addEventListener("change", event => {
  state.group = decodeURIComponent(event.target.value);
  state.before = null;
  loadBatch();
});
$("#mode").addEventListener("change", event => {
  state.mode = event.target.value;
  state.before = null;
  state.seenTurnIds.clear();
  state.seenCaptureIds.clear();
  state.identifiedThisSession = 0;
  $("#group").hidden = state.mode === "identify";
  loadBatch();
});
$("#fix").addEventListener("click", () => {
  const item = current();
  if (!item) return;
  if (!state.showOriginal && item.corrected_transcript !== item.transcript) {
    state.showOriginal = true;
    renderTranscript(item);
    setNotice("Showing the original machine text. Select the words you want to correct, then tap Fix again.");
    return;
  }
  captureSelection();
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
