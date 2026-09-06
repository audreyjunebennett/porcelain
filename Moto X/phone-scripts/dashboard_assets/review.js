const state = {
  items: [],
  index: 0,
  selection: null,
  audioSelection: {start: null, end: null},
  trimWindow: {start: 0, end: 30},
  mode: "identify",
  group: "all",
  before: null,
  showOriginal: false,
  loopSelection: false,
  contextRadius: 2,
  seenTurnIds: new Set(),
  seenCaptureIds: new Set(),
  identifiedThisSession: 0,
  autoPlayNext: false,
  waveform: null,
  waveformToken: 0,
  pendingSpeakerLabel: null,
  pendingNonSpeakerLabels: new Map(),
};

const $ = selector => document.querySelector(selector);
const transcript = $("#transcript");
const notice = $("#notice");
const audio = $("#audio");
const trimStage = $("#trim-stage");

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
    if (annotation.audio_start_seconds == null && annotation.audio_end_seconds == null) {
      return true;
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
  const pendingSounds = [...state.pendingNonSpeakerLabels.values()];
  if (state.pendingSpeakerLabel && pendingSounds.length) {
    button.disabled = false;
    button.textContent = `Confirm ${state.pendingSpeakerLabel} + ${pendingSounds.length} sound${pendingSounds.length === 1 ? "" : "s"} & next`;
  } else if (state.pendingSpeakerLabel) {
    button.disabled = false;
    button.textContent = `Confirm ${state.pendingSpeakerLabel} & next`;
  } else if (pendingSounds.length === 1) {
    button.disabled = false;
    button.textContent = `Confirm ${pendingSounds[0].label} & next`;
  } else if (pendingSounds.length > 1) {
    button.disabled = false;
    button.textContent = `Confirm ${pendingSounds.length} sounds & next`;
  } else if (hasSavedNonSpeakerLabel()) {
    button.disabled = false;
    button.textContent = "Confirm sound only & next";
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
    const key = `${button.dataset.type}:${button.dataset.label}`;
    const saved = annotations.some(annotation =>
      annotation.annotation_type === button.dataset.type
      && annotation.label === button.dataset.label
    );
    const pending = state.pendingNonSpeakerLabels.has(key);
    button.classList.toggle("saved", saved);
    button.classList.toggle("pending", pending);
    button.disabled = saved;
    button.setAttribute("aria-pressed", String(saved || pending));
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
  if (range) return [`${formatTimestamp(range.start)}–${formatTimestamp(range.end)}`, "Use whole clip"];
  if (state.selection) return ["SELECTED WORDS", "Use whole clip"];
  return ["WHOLE CLIP", item ? formatDuration(item.duration_seconds) : ""];
}

function renderScope() {
  const [title, detail] = scopeDescription();
  const whole = !state.selection && !activeAudioRange();
  const scope = $("#scope");
  scope.classList.toggle("whole", whole);
  scope.innerHTML = `<span>Applying to</span><strong>${escapeHtml(title)}</strong>${detail ? `<small>${escapeHtml(detail)}</small>` : ""}`;
  scope.setAttribute("aria-label", whole
    ? "Applying to whole clip"
    : `Applying to ${title}. Tap to use the whole clip.`);
}

function clearScopeToWholeClip() {
  if (!state.selection && !activeAudioRange()) {
    setNotice("Already applying to the whole clip.");
    return;
  }
  audio.pause();
  window.getSelection()?.removeAllRanges();
  state.selection = null;
  clearAudioRange();
  configureTrimWindow();
  setLoop(false);
  renderTranscript(current());
  setNotice("Selection cleared. Labels will apply to the whole clip.");
}

function activeAudioRange() {
  const {start, end} = state.audioSelection;
  return Number.isFinite(start) && Number.isFinite(end) && end > start
    ? {start, end}
    : null;
}

function currentAudioDuration() {
  const duration = Number(audio.duration);
  if (Number.isFinite(duration) && duration > 0) return duration;
  return Math.max(0.5, Number(current()?.duration_seconds) || 30);
}

function configureTrimWindow() {
  const duration = currentAudioDuration();
  const range = activeAudioRange();
  if (!current()?.identification || !range) {
    state.trimWindow = {start: 0, end: duration};
    renderTrimEditor();
    return;
  }
  const padding = Math.max(2, (range.end - range.start) * 0.75);
  let start = Math.max(0, range.start - padding);
  let end = Math.min(duration, range.end + padding);
  const targetWidth = Math.min(duration, Math.max(8, range.end - range.start + padding * 2));
  if (end - start < targetWidth) {
    if (start === 0) end = Math.min(duration, targetWidth);
    else if (end === duration) start = Math.max(0, duration - targetWidth);
  }
  state.trimWindow = {start, end};
  renderTrimEditor();
}

function trimPercent(seconds) {
  const width = state.trimWindow.end - state.trimWindow.start;
  return width > 0
    ? Math.max(0, Math.min(100, ((seconds - state.trimWindow.start) / width) * 100))
    : 0;
}

function drawWaveform() {
  const canvas = $("#trim-wave");
  const width = Math.max(1, Math.round(canvas.clientWidth * window.devicePixelRatio));
  const height = Math.max(1, Math.round(canvas.clientHeight * window.devicePixelRatio));
  if (canvas.width !== width) canvas.width = width;
  if (canvas.height !== height) canvas.height = height;
  const context = canvas.getContext("2d");
  context.clearRect(0, 0, width, height);
  const waveform = state.waveform;
  if (!waveform?.samples?.length) return;
  const center = height / 2;
  const bars = Math.max(24, Math.floor(width / (5 * window.devicePixelRatio)));
  const windowDuration = state.trimWindow.end - state.trimWindow.start;
  context.fillStyle = "#8b7b96";
  for (let index = 0; index < bars; index += 1) {
    const at = state.trimWindow.start + ((index + 0.5) / bars) * windowDuration;
    const sampleIndex = Math.max(0, Math.min(
      waveform.samples.length - 1,
      Math.floor((at / waveform.duration) * waveform.samples.length),
    ));
    const amplitude = Math.max(0.06, waveform.samples[sampleIndex]);
    const barHeight = Math.max(2, amplitude * height * 0.88);
    const barWidth = Math.max(1, 2 * window.devicePixelRatio);
    const x = ((index + 0.5) / bars) * width;
    context.fillRect(x - barWidth / 2, center - barHeight / 2, barWidth, barHeight);
  }
}

async function loadWaveform(item) {
  const token = ++state.waveformToken;
  state.waveform = null;
  drawWaveform();
  const AudioContextClass = window.AudioContext || window.webkitAudioContext;
  if (!AudioContextClass) return;
  let context;
  try {
    const response = await fetch(`/api/motox/review/audio/${encodeURIComponent(item.capture_id)}`);
    if (!response.ok) return;
    const bytes = await response.arrayBuffer();
    context = new AudioContextClass();
    const buffer = await context.decodeAudioData(bytes.slice(0));
    if (token !== state.waveformToken) return;
    const channel = buffer.getChannelData(0);
    const sampleCount = 360;
    const stride = Math.max(1, Math.floor(channel.length / sampleCount));
    const samples = [];
    let peak = 0;
    for (let index = 0; index < sampleCount; index += 1) {
      let value = 0;
      const start = index * stride;
      const end = Math.min(channel.length, start + stride);
      for (let cursor = start; cursor < end; cursor += Math.max(1, Math.floor(stride / 64))) {
        value = Math.max(value, Math.abs(channel[cursor]));
      }
      peak = Math.max(peak, value);
      samples.push(value);
    }
    const scale = peak > 0 ? peak : 1;
    state.waveform = {duration: buffer.duration, samples: samples.map(value => value / scale)};
    drawWaveform();
  } catch (_) {
    // The trim handles and audio remain fully usable if waveform decoding is
    // unavailable for a browser/codec combination.
  } finally {
    context?.close().catch(() => {});
  }
}

function renderTrimEditor() {
  if (!trimStage) return;
  const duration = currentAudioDuration();
  const range = activeAudioRange() || {start: 0, end: duration};
  const startPercent = trimPercent(range.start);
  const endPercent = trimPercent(range.end);
  $("#trim-start").style.left = `${startPercent}%`;
  $("#trim-end").style.left = `${endPercent}%`;
  $("#trim-selection").style.left = `${startPercent}%`;
  $("#trim-selection").style.width = `${Math.max(0, endPercent - startPercent)}%`;
  $("#trim-start").setAttribute("aria-valuemax", String(duration));
  $("#trim-end").setAttribute("aria-valuemax", String(duration));
  $("#trim-start").setAttribute("aria-valuenow", range.start.toFixed(2));
  $("#trim-end").setAttribute("aria-valuenow", range.end.toFixed(2));
  $("#trim-window-start").textContent = formatTimestamp(state.trimWindow.start);
  $("#trim-window-end").textContent = formatTimestamp(state.trimWindow.end);
  const playhead = Number(audio.currentTime);
  const inWindow = Number.isFinite(playhead)
    && playhead >= state.trimWindow.start && playhead <= state.trimWindow.end;
  $("#trim-playhead").style.opacity = inWindow ? "1" : "0";
  if (inWindow) $("#trim-playhead").style.left = `${trimPercent(playhead)}%`;
  $("#trim-play").textContent = audio.paused ? "▶" : "❚❚";
  drawWaveform();
}

function trimTimeFromPointer(event) {
  const rect = trimStage.getBoundingClientRect();
  const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
  return state.trimWindow.start + ratio * (state.trimWindow.end - state.trimWindow.start);
}

function setTrimBoundary(boundary, seconds) {
  const duration = currentAudioDuration();
  const range = activeAudioRange() || {start: 0, end: duration};
  const minimum = 0.5;
  if (boundary === "start") {
    state.audioSelection.start = Math.max(0, Math.min(seconds, range.end - minimum));
    state.audioSelection.end = range.end;
  } else {
    state.audioSelection.start = range.start;
    state.audioSelection.end = Math.min(duration, Math.max(seconds, range.start + minimum));
  }
  syncTranscriptToAudioRange();
  renderAudioRange();
}

function bindTrimHandle(selector, boundary) {
  const handle = $(selector);
  let dragOffset = 0;
  let pointerStartX = 0;
  let dragged = false;
  handle.addEventListener("pointerdown", event => {
    event.preventDefault();
    handle.classList.add("dragging");
    handle.setPointerCapture(event.pointerId);
    const range = activeAudioRange() || {start: 0, end: currentAudioDuration()};
    const boundaryTime = boundary === "start" ? range.start : range.end;
    dragOffset = boundaryTime - trimTimeFromPointer(event);
    pointerStartX = event.clientX;
    dragged = false;
  });
  handle.addEventListener("pointermove", event => {
    if (!handle.hasPointerCapture(event.pointerId)) return;
    if (Math.abs(event.clientX - pointerStartX) < 3 && !dragged) return;
    if (!dragged) audio.pause();
    dragged = true;
    setTrimBoundary(boundary, trimTimeFromPointer(event) + dragOffset);
  });
  const finish = event => {
    if (!handle.hasPointerCapture(event.pointerId)) return;
    handle.releasePointerCapture(event.pointerId);
    handle.classList.remove("dragging");
    if (!dragged) return;
    const range = activeAudioRange();
    if (range) audio.currentTime = range.start;
    setLoop(true);
    audio.play().catch(() => {});
    setNotice(`Crop adjusted to ${formatTimestamp(range.start)} – ${formatTimestamp(range.end)}. This exact audio will be saved.`);
  };
  handle.addEventListener("pointerup", finish);
  handle.addEventListener("pointercancel", finish);
  handle.addEventListener("keydown", event => {
    if (!["ArrowLeft", "ArrowRight"].includes(event.key)) return;
    event.preventDefault();
    const range = activeAudioRange() || {start: 0, end: currentAudioDuration()};
    const currentValue = boundary === "start" ? range.start : range.end;
    setTrimBoundary(boundary, currentValue + (event.key === "ArrowRight" ? 0.1 : -0.1));
  });
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
  renderTrimEditor();
}

function syncTranscriptToAudioRange() {
  const item = current();
  const range = activeAudioRange();
  if (!item || !range) return false;
  const words = (item.words || []).filter(word =>
    Number(word.end_seconds) > range.start && Number(word.start_seconds) < range.end
  );
  if (!words.length) {
    state.selection = null;
    renderTranscript(item);
    return false;
  }
  const start = Number(words[0].char_start);
  const end = Number(words[words.length - 1].char_end);
  const text = item.transcript.slice(start, end).trim();
  state.selection = {start, end, text};
  renderTranscript(item);
  return true;
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
}

function playNextClipIfArmed() {
  if (!state.autoPlayNext) return;
  state.autoPlayNext = false;
  const playFromSelection = () => {
    const range = activeAudioRange();
    if (range) audio.currentTime = range.start;
    audio.play().catch(() => {
      setNotice("Your browser blocked automatic playback. Tap play once and the next clips should continue automatically.", true);
    });
  };
  if (audio.readyState >= HTMLMediaElement.HAVE_METADATA) {
    playFromSelection();
  } else {
    audio.addEventListener("loadedmetadata", playFromSelection, {once: true});
  }
}

function renderTranscriptText(value, highlight) {
  value = String(value || "");
  transcript.replaceChildren();
  const start = Number(highlight?.start);
  const end = Number(highlight?.end);
  if (!Number.isInteger(start) || !Number.isInteger(end) || start < 0 || end <= start || end > value.length) {
    transcript.textContent = value;
    return;
  }
  transcript.append(document.createTextNode(value.slice(0, start)));
  const selected = document.createElement("mark");
  selected.className = "transcript-selection";
  selected.textContent = value.slice(start, end);
  transcript.append(selected, document.createTextNode(value.slice(end)));
}

function renderTranscriptActions(item) {
  const fix = $("#fix");
  const status = $("#selection");
  if (item.identification) {
    fix.hidden = true;
    status.hidden = true;
    return;
  }
  const audioOnly = Boolean(activeAudioRange() && !state.selection);
  fix.hidden = audioOnly;
  const selected = state.selection?.text?.trim();
  const abbreviated = selected && selected.length > 28 ? `${selected.slice(0, 27)}…` : selected;
  fix.textContent = abbreviated ? `Correct “${abbreviated}”` : "Correct transcript";
  status.hidden = !audioOnly;
  status.textContent = audioOnly
    ? "No timed words overlap this crop; its speaker label will apply to audio only."
    : "";
}

function renderTranscript(item) {
  const corrected = item.corrected_transcript ?? item.transcript;
  const hasCorrections = corrected !== item.transcript;
  const showingOriginal = item.identification || state.showOriginal || !hasCorrections;
  const value = item.identification || showingOriginal ? item.transcript : corrected;
  const highlight = showingOriginal ? state.selection : null;
  if (item.identification) {
    renderTranscriptText(value || item.identification.selected_text
      || "No word-timed transcript is available for this older capture. Identify the voice from the selected audio.", highlight);
    transcript.classList.remove("corrected");
    $("#transcript-version").hidden = true;
    renderTranscriptActions(item);
    return;
  }
  const button = $("#transcript-version");
  button.hidden = !hasCorrections;
  button.textContent = state.showOriginal ? "Show corrected" : "Show original";
  renderTranscriptText(value, highlight);
  transcript.classList.toggle("corrected", hasCorrections && !state.showOriginal);
  renderTranscriptActions(item);
}

function renderCard() {
  const item = current();
  const card = $("#card");
  const empty = $("#empty");
  if (!item) {
    state.autoPlayNext = false;
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
  state.pendingNonSpeakerLabels.clear();
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
  $("#instruction").textContent = identification
    ? "Drag either bracket to clean the crop. Matching timed words stay highlighted and the selection loops while it plays."
    : "Select words to move the audio brackets around them, or drag the brackets to highlight matching words.";
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
  configureTrimWindow();
  loadWaveform(item);
  playNextClipIfArmed();
  renderTranscript(item);
  if (identification) {
    syncTranscriptToAudioRange();
  }
  $("#position").textContent = state.mode === "identify"
    ? `Question ${state.identifiedThisSession + 1}`
    : `${state.index + 1} of ${state.items.length}`;
  $("#next").textContent = "Next clip";
  $("#next").className = "primary";
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
    setNotice("Use the waveform brackets to adjust this suggested audio crop.");
    return;
  }
  state.selection = selectionInsideTranscript();
  if (state.selection) syncAudioToSelectedWords();
  renderTranscript(current());
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

async function saveAnnotation(type, label, replacementText = null, options = {}) {
  const {
    advanceIdentificationSpeaker = true,
    preserveScope = false,
    quiet = false,
  } = options;
  const item = current();
  if (!item) return;
  // Dialog focus clears the browser's text selection. Preserve the range that
  // was captured when "Fix selected words" opened.
  const range = activeAudioRange();
  const selection = state.selection || (range
    ? {start: 0, end: 0, text: ""}
    : {start: 0, end: item.transcript.length, text: item.transcript});
  const wholeClip = !state.selection && !range;
  if (wholeClip && type === "speaker" && !window.confirm(
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
    if (!preserveScope) {
      window.getSelection()?.removeAllRanges();
      state.selection = null;
      if (item.identification) {
        if (range) {
          state.audioSelection = {start: range.start, end: range.end};
          syncTranscriptToAudioRange();
          renderAudioRange();
        } else if (wholeClip) {
          clearAudioRange();
          configureTrimWindow();
          setLoop(false);
        } else {
          restoreIdentificationRange();
        }
      } else {
        clearAudioRange();
        renderTranscript(item);
      }
    }
    const rangeNotice = range ? ` for ${formatTimestamp(range.start)} – ${formatTimestamp(range.end)}` : "";
    if (advanceIdentificationSpeaker && item.identification && type === "speaker") {
      state.seenTurnIds.add(item.identification.turn_id);
      state.seenCaptureIds.add(item.capture_id);
      state.identifiedThisSession += 1;
      audio.pause();
      state.autoPlayNext = true;
      await loadBatch();
      const acceptedSeconds = Number(annotation.audio_end_seconds) - Number(annotation.audio_start_seconds);
      setNotice(`${label} learned from ${acceptedSeconds.toFixed(1)} clean seconds. The next question was re-ranked.`);
    } else if (!quiet) {
      setNotice(`${label || "Correction"} saved${rangeNotice}. Tap the chip below to undo.`);
    }
    return annotation;
  } catch (error) {
    setNotice(error.message, true);
    return null;
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
  } catch (error) {
    setNotice(error.message, true);
  }
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
    const [items, groups] = await Promise.all([
      requestJson(candidateUrl),
      requestJson("/api/motox/review/groups"),
    ]);
    state.items = items;
    state.index = 0;
    if (!identify && items.length) state.before = items[items.length - 1].captured_at;
    renderGroups(groups);
    renderCard();
  } catch (error) {
    setNotice(error.message, true);
  }
}

function nextClip() {
  if (!state.items.length) return;
  audio.pause();
  state.autoPlayNext = true;
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
  const cleared = state.pendingSpeakerLabel === label;
  state.pendingSpeakerLabel = cleared ? null : label;
  document.querySelectorAll("#labels [data-label]").forEach(button => {
    const pending = button.dataset.label === state.pendingSpeakerLabel;
    button.classList.toggle("pending", pending);
    button.setAttribute("aria-pressed", String(pending));
  });
  renderIdentificationAction();
  setNotice(cleared
    ? `${label} cleared. You can choose another voice, or confirm a saved sound by itself.`
    : `${label} selected. Tap it again to clear, or confirm when you're ready.`);
}

function chooseNonSpeaker(type, label) {
  if (!current()?.identification) return;
  const key = `${type}:${label}`;
  if (state.pendingNonSpeakerLabels.has(key)) {
    state.pendingNonSpeakerLabels.delete(key);
  } else {
    state.pendingNonSpeakerLabels.set(key, {type, label});
  }
  renderIdentificationLabels();
  renderIdentificationAction();
  setNotice("");
}

async function confirmSpeakerAndNext() {
  const label = state.pendingSpeakerLabel;
  const pendingSounds = [...state.pendingNonSpeakerLabels.values()];
  if (!label && !pendingSounds.length) {
    if (hasSavedNonSpeakerLabel()) {
      nextClip();
      return;
    }
    setNotice("Choose a voice or sound first.", true);
    return;
  }
  for (const pending of pendingSounds) {
    const saved = await saveAnnotation(pending.type, pending.label, null, {
      advanceIdentificationSpeaker: false,
      preserveScope: true,
      quiet: true,
    });
    if (!saved) return;
  }
  if (label) {
    const saved = await saveAnnotation("speaker", label, null, {
      advanceIdentificationSpeaker: false,
      preserveScope: true,
      quiet: true,
    });
    if (!saved) return;
  }
  state.pendingSpeakerLabel = null;
  state.pendingNonSpeakerLabels.clear();
  const item = current();
  state.seenTurnIds.add(item.identification.turn_id);
  state.seenCaptureIds.add(item.capture_id);
  state.identifiedThisSession += 1;
  audio.pause();
  state.autoPlayNext = true;
  await loadBatch();
}

transcript.addEventListener("mouseup", captureSelection);
transcript.addEventListener("touchend", () => setTimeout(captureSelection, 100));
$("#scope").addEventListener("click", clearScopeToWholeClip);
audio.addEventListener("timeupdate", () => {
  renderTrimEditor();
  const range = activeAudioRange();
  if (state.loopSelection && range && audio.currentTime >= range.end) {
    audio.currentTime = range.start;
    audio.play().catch(() => {});
  }
});
audio.addEventListener("loadedmetadata", () => {
  configureTrimWindow();
  const range = activeAudioRange();
  if (range) audio.currentTime = range.start;
});
audio.addEventListener("play", renderTrimEditor);
audio.addEventListener("pause", renderTrimEditor);
$("#trim-play").addEventListener("click", () => {
  if (!audio.paused) {
    audio.pause();
    return;
  }
  const range = activeAudioRange();
  if (range && (audio.currentTime < range.start || audio.currentTime >= range.end)) {
    audio.currentTime = range.start;
  }
  audio.play().catch(() => setNotice("Tap the standard audio play button once to allow playback.", true));
});
bindTrimHandle("#trim-start", "start");
bindTrimHandle("#trim-end", "end");
window.addEventListener("resize", drawWaveform);
const labelClick = event => {
  const button = event.target.closest("[data-label]");
  if (!button) return;
  if (current()?.identification && button.dataset.type === "speaker") {
    chooseSpeaker(button.dataset.label);
    return;
  }
  if (current()?.identification && ["sound", "overlap"].includes(button.dataset.type)) {
    chooseNonSpeaker(button.dataset.type, button.dataset.label);
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
    window.getSelection()?.removeAllRanges();
    state.selection = null;
    clearAudioRange();
    renderTranscript(item);
    setNotice("Showing the original machine text. Select the words you want to correct, then tap Fix again.");
    return;
  }
  state.selection = selectionInsideTranscript() || state.selection;
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

async function loadInitialView() {
  const parameters = new URLSearchParams(window.location.search);
  const captureId = parameters.get("capture");
  if (!captureId) {
    await loadBatch();
    return;
  }
  try {
    const [item, groups] = await Promise.all([
      requestJson(`/api/motox/review/capture/${encodeURIComponent(captureId)}`),
      requestJson("/api/motox/review/groups"),
    ]);
    state.mode = "browse";
    state.items = [item];
    state.index = 0;
    state.before = null;
    $("#mode").value = "browse";
    $("#group").hidden = false;
    renderGroups(groups);
    renderCard();

    const start = Number(parameters.get("start"));
    const end = Number(parameters.get("end"));
    if (Number.isFinite(start) && Number.isFinite(end) && end > start) {
      state.audioSelection = {start, end};
      configureTrimWindow();
      syncTranscriptToAudioRange();
      renderAudioRange();
      setLoop(true);
    }
    setNotice("Opened from Today’s Journal. Tap an existing label chip to undo it, then choose the corrected speaker.");
  } catch (error) {
    setNotice(error.message, true);
  }
}

loadInitialView();
