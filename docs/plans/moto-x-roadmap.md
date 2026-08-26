# Plan: Moto X memory companion

| Field | Value |
|---|---|
| **Doc kind** | `version-roadmap` |
| **Owners / areas** | Ruby / Moto X, Locus, Chimera, personal memory |
| **Status** | `active` |
| **Targets** | Reliable capture, daily memory, directed notes/tags, Porcelain handoff |
| **Last updated** | 2026-08-15 |
| **Supersedes / superseded by** | Old Claudia PWA experiments are historical, not an implementation base |
| **As-built** | [`Moto X receiver manager`](../features/moto-x-receiver-manager.md); [`Moto X review and corrections`](../features/moto-x-review-and-corrections.md) (`partial`) |

## Product shape

The Moto X should feel less like a recorder bolted to a transcription script and more like a quiet memory intake for Porcelain. It captures the day, makes the result pleasant to revisit, and helps organize explicitly spoken thoughts. Claudia branding is appropriate because this memory ultimately informs Ruby's Claudia persona and local-AI context, but the Moto X journal is not itself a live conversational agent.

The near-term product is also the primary lightweight reader for Moto X memory. Ruby should be able to open one home-screen icon on the Moto X or iPhone, skim the rolling last 24 hours like a calm messaging timeline, open a focused conversation, and search older material. Deeper Porcelain/Locus integration remains desirable, but it must not block a useful standalone reading surface now.

There are two Moto X interaction modes:

1. **Ambient memory.** Capture, transcribe, group, and index without treating nearby speech as a request.
2. **Directed memory.** A deliberate phrase or visible control marks nearby material as a note, tag, to-do, important item, or other organizational intent.

Mode boundaries must stay obvious. Ordinary journaling must not accidentally invoke an assistant, and directed memory does not begin a spoken dialogue. Live Claudia conversation belongs in Porcelain, where retrieval, tools, screen-on interaction, headphones, and any synthesized/cloned voice can share one coherent session without feeding speaker output back into the bag recorder.

## Canonical architecture and names

- **Porcelain** is the current suite.
- **Chimera** is the backend: gateway, model routing, ingest, embeddings, and retrieval.
- **Locus** is the current client/desktop surface, including the operator chat experience. A Moto X surface should grow with Locus rather than revive the old Claudia PWA.
- **Claudia** is Ruby's cat-girl alter-ego/persona. She may remain the face and metaphorical addressee of the Moto X memory reader, while actual conversational voice/agent behavior belongs to Porcelain rather than the recorder.
- **Miette** is Ruby's separate Pixel Cat pet on the MacBook. Miette is not Claudia.
- `.claude/worktrees` and old Claudia PWA code are archaeology unless a current plan explicitly adopts a small idea from them.

Moto X features should call Chimera through stable APIs and virtual model roles. They must not hard-code Gemini, Groq, Ollama, Qwen, or another provider into the memory workflow.

## At a glance

| Phase | Outcome | Status |
|---|---|---|
| [1 — Capture integrity](#phase-1--capture-integrity) | Recording continues independently of network and transcription | `active` |
| [2 — Deterministic daily journal](#phase-2--deterministic-daily-journal) | Correct 120-second conversations appear in one readable day | `active` |
| [3 — Moto X memory surface](#phase-3--moto-x-memory-surface) | A home-screen timeline makes recent memory pleasant to read | `active` |
| [4 — Chimera memory workspace](#phase-4--chimera-memory-workspace) | Current daily artifacts become searchable through existing ingest | `todo` |
| [5 — Stable local memory worker](#phase-5--stable-local-memory-worker) | Local models normally sort and connect notes; online models can escalate | `todo` |
| [6 — Directed notes, tags, and corrections](#phase-6--directed-notes-tags-and-corrections) | Explicit phrases or controls turn nearby memory into organized notes or correction items | `todo` |
| [7 — Porcelain / Claudia interaction boundary](#phase-7--porcelain--claudia-interaction-boundary) | Interactive Claudia uses indexed memory without living in the bag recorder | `todo` |
| [8 — Historical recovery](#phase-8--historical-recovery) | Useful old ideas are reconciled without reviving stale code | `deferred` |

## Phase 1 — Capture integrity

**Goal.** Preserve every successfully recorded chunk and remove network/transcription latency from the capture loop.

**Current implementation draft**

- Keep [`record.py`](../../Moto%20X/phone-scripts/record.py) untouched as the historical fallback.
- Use the physically validated [`record_v2.py`](../../Moto%20X/phone-scripts/record_v2.py) as the active recorder.
- Keep **30 seconds** as the durable capture and transport unit. It already feels immediate in Ruby's normal use, matches Whisper's natural context window, reduces stop/start gaps and request churn, and is not the unit shown as a chat bubble.
- Give every chunk a unique capture ID and the time recording began.
- Finalize audio into a durable phone-side queue before making it uploadable.
- Deliver and retry on a background thread while the next capture begins.
- Send capture metadata to the receiver so slow delivery cannot distort conversation timing.
- Make retry idempotent at the receiver.
- Recover stable IDs and original local capture times from legacy `record.py` epoch filenames so the existing backlog can drain into the new journal without being relabeled as processing-time audio.
- Treat the lav routed outside the zipped bag pouch, with its furry windscreen fitted, as a chosen high-quality field profile. Keep the windscreen from rubbing against fabric, zipper, or the bag itself; contact noise can be worse than the wind it removes.
- Preserve microphone route/placement observations alongside evaluation clips. A clipped lav, a lav hanging outside the pouch, a phone mic inside the bag, and a room-positioned mic are different acoustic domains and must not be compared as if they were interchangeable.

**Acceptance**

- A PC, Wi-Fi, or transcription outage does not discard finalized audio.
- Upload/transcription time no longer creates the ordinary gap between chunks.
- Logs expose capture-cycle duration and queue delivery.
- The old recorder remains available as a deliberate fallback while normal `record_v2.py` operation accumulates evidence.

**Deployment boundary.** Syncthing copied the draft and Ruby deliberately switched the physical phone to `record_v2.py`. The supervised offline/restore test proved that finalized audio remains queued, capture continues independently, and restored delivery acknowledges original capture IDs and timestamps once.

## Phase 2 — Deterministic daily journal

**Goal.** Make one generated Markdown file the primary readable memory of a day.

**Rules**

- Speech starts a conversation.
- A pause shorter than 120 seconds keeps it open.
- At exactly 120 seconds without speech, the conversation closes at the last speech time.
- Ambient audio may attach as context to an open conversation but never starts one or resets its speech clock.
- Silence can prove that the gap has elapsed but is otherwise discarded from the readable journal.
- Literal silent capture files are already discarded after VAD classification.
  Final transcript and note projections should additionally collapse empty
  turns and non-semantic pauses while preserving timestamps and never deleting
  speech merely because an ASR pass returned empty text.
- Duplicate capture IDs do not duplicate journal entries.
- Daily files are projections of SQLite state and can be rebuilt deterministically.
- Raw audio and existing transcript/conversation files remain available during migration.

### Two-stage transcription and turn projection

Thirty-second recorder files are durable capture/transport pieces, not readable conversation turns. The PC owns a two-stage derived transcript:

1. **Quick pass:** classify and transcribe each arriving 30-second chunk immediately, retain its exact capture/audio provenance, and publish provisional text so the recent-memory surface feels current.
2. **Final conversation pass:** when the 120-second conversation state closes—or an explicit finalize action occurs—process the complete neighboring audio sequence with cross-chunk context, overlapping boundary repair, punctuation, diarization, and known-speaker identification. Preserve raw audio, original chunk timestamps, quick-pass output, and model/version provenance.
3. **Boundary reconciliation pass:** re-transcribe overlapping windows that straddle every 30-second recorder cutoff, then compare the left-chunk, right-chunk, overlap-window, and whole-conversation hypotheses using word timestamps and confidence where available. Stitch repeated words conservatively, recover sentence fragments that were clipped or misheard without future context, and retain a visible uncertain/correction-needed state when the passes materially disagree rather than inventing one seamless sentence.
4. **Long-open checkpoint:** if speech keeps one conversation open for an extended period, permit a bounded provisional consolidation around every 10–15 minutes so stable earlier turns become readable without falsely closing the conversation. Conversation close still owns the authoritative final pass and may supersede these checkpoint projections atomically.
5. **Atomic UI projection:** replace provisional fragments with versioned finalized speaker turns without losing anchors back to their original time ranges and audio. Never mutate evidence and never expose a half-replaced conversation while Ruby is reading it.
6. **Correction projection:** human labels always target source capture IDs plus
   exact seconds (and source-pass character spans when available). A checkpoint,
   boundary, or final conversation pass produces an explicit source map. The
   projection worker carries compatible speaker, sound, privacy, boundary, and
   transcript corrections onto new turns; disagreements become visible review
   items rather than silently dropping a correction or forcing it onto the
   wrong words.
7. **Expandable review context:** the review UI starts compact but may load
   progressively larger neighboring capture windows or the whole conversation.
   Context audio is reference material; annotations remain anchored to their
   actual source clip/range. Human `continues before/after/both` flags become
   boundary-pass evaluation cases.

One finalized turn may span several recorder chunks, and one recorder chunk may contain the end of one person's turn and the beginning of another's. Chat bubbles follow complete human turns, not files, Whisper windows, sentences, or arbitrary chunk boundaries. Manual or spoken corrections update a versioned derived turn layer and may later contribute clean enrollment examples; they do not rewrite the captured source or erase the original machine transcript. Preserve the chain **original audio → original machine transcript → corrected transcript**.

### ASR evaluation contract

- Keep faster-whisper `large-v3` in CUDA float16 as the production baseline until real Moto X evidence justifies a change. It is already the largest standard Whisper model and fits alongside ordinary use on the 16 GB RTX 5060 Ti.
- Evaluate NVIDIA Parakeet TDT 0.6B as the first serious English-specialized challenger now that the lav outside the pouch can produce clean close-mic speech. Do not assume clean-benchmark leadership transfers to distant, obstructed, windy, fabric-rubbing, television, or overlapping-conversation audio.
- Build a manually corrected evaluation set from Ruby's actual recordings across mic placements, quiet and noisy environments, cross-chunk sentences, names, low-volume speech, intentional voices, and known silence/background hallucination cases.
- Score word errors, quiet-speech omissions, hallucinated insertions, names, punctuation, timestamps, boundary repair, latency, and peak VRAM. Operational steadiness while Minecraft and ordinary desktop workloads are running is part of the result.
- Quick and final passes may use different ASR models if the measured tradeoff supports it. Model names remain replaceable implementation choices; raw audio, chunk IDs, conversation rules, and projection contracts do not depend on one vendor.

**Current implementation draft**

- [`motox_v1.py`](../../Moto%20X/phone-scripts/motox_v1.py) contains a dependency-light, tested state machine and SQLite journal.
- [`receiver.py`](../../Moto%20X/phone-scripts/receiver.py) dual-writes into it without making journal failure fatal to audio receipt.
- Generated days live under `motox_audio_data/daily/YYYY-MM-DD.md` by default.

**Acceptance**

- One file shows the day's speech conversations in chronological order with source-audio links.
- Active and completed conversations are distinguishable.
- Boundary, ambient, duplicate, rebuild, and status behavior remains covered by tests.

## Phase 3 — Moto X memory surface

**Goal.** Give the bag-mounted phone a low-brightness, OLED-friendly surface for capture confidence, the rolling last 24 hours, conversation browsing, and search.

The receiver serves `/dashboard` plus JSON status and recent-transcript endpoints. The physical Moto X validation on 2026-07-21 proved the black dashboard, health indicator, live refresh, safe Markdown speaker rendering, CUDA transcription, legacy retry idempotency, and original-time recovery while the old queue drains. This remains a lightweight Moto-native reader rather than the final Locus design.

The home-screen identity now uses the selected canonical purple CRT Claudia artwork with a restrained mint listening/healthy dot. The receiver serves ordinary 192/512 px icons, an Android-safe maskable icon, an Apple touch icon, a favicon, and an installable web-app manifest; the canonical source files remain untouched.

The Windows receiver is now owned by a hidden per-user tray manager. It starts
after login, prevents duplicate managed starts, restarts failures, exposes
status/start/stop/restart controls, and links directly to the private dashboard
and receiver log. See the
[`Moto X receiver manager`](../features/moto-x-receiver-manager.md) feature
record for the as-built contract.

### Chosen reading experience

- The home screen is a **rolling last 24 hours**, not a calendar-day view that resets at midnight. Older material remains available in the archive; it is never deleted merely because it leaves the front timeline.
- Open at the newest turn and scroll upward into the past. Provide a floating return-to-now control and never pull the viewport downward while Ruby is reading older material.
- Use a restrained messaging layout: Ruby turns on the right; Lynn, Raven, and Other turns on the left with stable individual color accents; useful sound, ambient, and cat context is centered between them.
- Group complete speaker turns rather than creating a bubble for every sentence. Show compact time markers and conversation dividers in the stream.
- Keep navigation tiny: **Timeline**, **Conversations**, and **Search**.
- Conversation cards preserve full transcript access and can later add a concise summary, speakers, important notes, and explicit to-dos.
- Load older timeline sections progressively for smoothness and battery use while making the experience feel like one continuous 24-hour stream.
- Use one installable home-screen icon and one stable private HTTPS URL through **Tailscale Serve**. Tailscale is expected to remain connected; do not build separate LAN/Tailscale URL detection or redirect logic.
- The PC performs transcription, diarization, search, and enrichment. The Moto X remains a recorder and lightweight web client; its 4 GB RAM is not the inference constraint.

### Deferred speaker and sound enrichment

- Speaker diarization and identity are separate stages. Diarization finds distinct voices; identification maps them to **Ruby**, **Lynn**, **Raven**, or conservatively **Other**.
- Ruby's intentional or relaxed voices are **voice modes**, not new speaker identities. Speaker identification must be able to retain `Ruby` while a separate, editable layer describes a voice as relaxed/low-effort, ordinary conversational, animated/playful, character voice, deliberately polished/social, sleepy/low-energy, strained/congested, or uncertain.
- Build Ruby/Lynn/Raven enrollment material later from real Moto X recordings, not sterile studio prompts. Include clean isolated turns across actual mic placements: lav clipped to Ruby, outside the bag, between Ruby and Lynn, and farther across the room; include Raven at both ordinary and typical farther distances.
- FL Studio may assemble one clean reference track per person, but references should not add music, reverb, EQ, aggressive denoising, compression, or other effects that erase the real capture conditions. Keep separate evaluation clips out of enrollment.
- Use confidence thresholds, temporal smoothing across neighboring turns, and manual correction. Never force an uncertain voice into Ruby, Lynn, or Raven; overlapping speech remains explicitly difficult.
- Treat cat recognition as a separate sound-event/individual-classification path rather than human diarization. Enroll real Moto X examples for **Hahli** (spelled like the BIONICLE character) and **Lam** (spelled like Aleister Crowley's entity), using pitch, contour, timbre, harmonics, duration, call type, and context rather than sex or pitch alone. Render high-confidence identities as centered cat events and fall back to **unknown cat** or **cat vocalization** instead of guessing.
- Use a layered sound pipeline rather than forcing one model to solve everything:
  a general sound-event detector proposes music, television, cat vocalization,
  traffic, bangs, and other useful events; a cat-specific identity classifier
  runs only on accepted/proposed cat regions to distinguish Hahli, Lam, or
  unknown cat. Both remain versioned proposals until reviewed.
- Treat Ruby singing and externally played songs as distinct editable event types. Singing may coexist with music, so the system must preserve overlap and uncertainty instead of treating every melodic vocal as either ordinary speech or a recording.
- Let Ruby create a new named speaker profile when a new friend appears, but only through an explicit consent-and-enrollment flow. Until enough accepted examples exist, keep that voice as **Other/Uncertain** rather than guessing from a name or social context.
- Sound annotations must be narratively useful, not acoustic telemetry. Prefer rare/high-confidence events that affect nearby speech, such as a loud bang followed by a reaction. Collapse or omit repetitive room noise.
- Preserve raw audio and raw transcript as immutable evidence. Speaker names, sound events, summaries, notes, and tasks are editable/re-runnable enrichment layers.

### Oblivious Mode

- Add a prominent phone-dashboard control named **Oblivious Mode**, described in Lynn's words as **“🙈 fingers in ears, la la la mode.”** It temporarily pauses recording without requiring Termux.
- The screen must make paused versus recording state unmistakable and provide a one-tap resume action.
- Pausing must stop new capture intentionally, finalize or preserve any already durable chunk safely, and record only a minimal local pause/resume audit marker—not ambient audio from the paused interval.
- This privacy control must remain available even if transcription, diarization, Chimera, or the wider Porcelain stack is unavailable.

### Deferred personal acoustic signals

- Treat coughs, sniffs, throat clearing, nose blowing, sneezes, snoring/breathing sounds, and sleep movement as a separate sound-event classification path rather than words for Whisper to transcribe.
- Keep event detections anchored to source audio with confidence and permit correction. Aggregate useful counts and trends without flooding the narrative timeline with every repetitive event.
- Learn voice-mode correlations from Ruby's own optional labels and longitudinal baseline. Microphone placement, distance, wind protection, congestion, mood, performance/character voices, and background noise are confounders that must remain visible to evaluation.
- Use observational language such as **“resembles Ruby's personally observed relaxed voice pattern”** or **“more throat-clearing events than Ruby's recent baseline.”** Never turn a voice mode into a medical state: relaxed or low-effort speech may simply mean Ruby is comfortable and having a good time.
- Sleep audio may support a private diary of audible cough, snore, breathing, movement, and wake-like events. It is not sleep staging, a diagnosis, or a replacement for validated medical measurement unless a future separately reviewed feature establishes those claims.
- Personal acoustic signals are reversible enrichment. Raw captures and deterministic journal state stay valid when every detector is disabled or wrong.

Next steps:

- Replace the latest-four probe with the rolling 24-hour messaging timeline and chronological pagination.
- Add conversation-card and transcript-detail endpoints/views, followed by phrase/speaker/date search.
- Add queue state and a measured inter-capture-gap signal from `record_v2.py`.
- Add honest microphone confidence (`lav likely`, `phone mic likely`, `unsure`) only if Android route data or repeatable acoustic evidence supports it.
- Assemble the corrected Whisper/Parakeet evaluation set before choosing different quick-pass or final-pass ASR roles.
- Design correction and optional self-label controls for sound events and voice modes before using them in summaries or trends.
- Decide how the Moto X surface joins Locus without coupling capture to UI availability.
- Add subtle layout movement or blanking if needed for OLED burn-in protection.
- Keep future icon variants derived from the selected canonical Claudia sources in `D:\assets\Canonical Icons`; do not import an unrelated old PWA asset by accident.

## Phase 4 — Chimera memory workspace

**Goal.** Make current Moto X memory searchable through Porcelain's existing ingest and retrieval contracts.

- Configure the current indexer to watch deterministic daily journal files in a dedicated workspace.
- Use the current manifest-only ingest path, embeddings, Qdrant scoping, and Locus chat retrieval.
- Do not revive the receiver's legacy direct-ingest JSON shape; it does not match the current Chimera ingest contract.
- Retain capture IDs, day, timestamps, source paths, and conversation anchors as provenance.
- Decide whether active journal projections are indexed or only completed conversations are promoted.

## Phase 5 — Stable local memory worker

**Goal.** Use local models as the normal worker for repetitive memory organization because they are stable infrastructure Ruby and Lynn control and can keep experimenting with. This is not primarily a privacy feature.

Online free models such as Gemini and Groq may remain useful, but their free tiers, quotas, names, and availability are outside Porcelain's control. Chimera virtual-model roles should therefore select a local route by default for bounded tasks and permit explicit fallback or escalation when a harder task benefits from an online model.

Candidate tasks:

- Detect explicit notes and recall requests.
- Produce a concise title, category, people/projects, and source links.
- Choose create, append, merge, link, or leave in an inbox.
- Describe ambient context conservatively when classification is genuinely useful.
- Summarize a closed conversation in one to three sentences.
- Separate explicit commitments, possible ideas, and important facts instead of flattening all extracted text into generic tasks.
- Produce a calm daily digest of reviewed to-dos, important notes, and things worth remembering while deduplicating repeated mentions.
- Turn corrections into a reusable rule or an evaluation example.
- Verify automatically extracted notes, reminders, to-dos, people, and filing
  suggestions through lightweight accept/edit/dismiss cards. The user should
  see useful work already proposed rather than manually constructing every
  note or editing backend artifacts.
- Support an optional **Pestering Mode** for accepted reminders: when capture is
  quiet and the configured reminder window is active, surface a gentle prompt
  without beginning a conversational-agent session. Enforce cooldowns,
  snooze/dismiss, quiet hours, and no prompting during Oblivious Mode.
- Show due reminders on the memory home screen as a compact bottom-anchored
  bubble that survives scrolling. A restrained pulsing red outline may signal
  urgency, must respect reduced-motion preferences, and must not cover capture
  health or primary controls.

Implementation constraints:

- Require structured output with deterministic validation and conservative failure behavior.
- Keep routing by capability role, not model brand. Qwen, Gemma, or another local family can change without changing Moto X code.
- Benchmark a small candidate set on Ruby's real corrected examples on each 16 GB VRAM computer. Prefer task accuracy, latency, and operational steadiness over social-media leaderboards.
- Keep the raw memory and deterministic journal valid when every model is unavailable.

## Phase 6 — Directed notes, tags, and corrections

**Goal.** Let Ruby intentionally mark nearby recorded material with phrases such as **“Claudia, save this as a note,” “tag this Porcelain,” “this is important,” “make that a to-do,”** or **“Claudia, mark that transcript wrong”** without turning ordinary conversation into a live assistant session.

- Start with explicit phrase patterns and/or a visible press-to-mark control that can be tested without continuous wake-word dialogue.
- Attach the intent to the relevant preceding/following transcript span rather than storing only the command sentence. Preserve exact source conversation, timestamps, and audio alongside every extracted note.
- Support voice tags, note titles, importance markers, project/person/topic labels, to-dos, and “remember this” anchors. The quick command result may appear visually; the bag does not need to speak a confirmation.
- Support deliberate spoken correction markers such as **“mark that transcript wrong,” “I said Parakeet, not parakeets,” “the previous speaker was Lynn,”** and **“that was coughing, not speech.”** A correction marker attaches to the most plausible preceding turn/audio span while retaining enough neighboring audio to repair a boundary mistake.
- Store a correction item even when Ruby does not speak the replacement text. A later review surface can replay the anchored audio, show the original transcript and metadata, and accept a typed or spoken correction.
- Permit a marked span to be retranscribed with additional neighboring context, different decoding settings, or another candidate ASR model. Present comparisons as proposals; a rerun never silently overwrites an accepted human correction.
- Corrections may target words, spelling/proper names, speaker identity, sound-event class, voice mode, conversation boundary, or a complete false-positive transcript. Spoken spellings such as **“Hahli, spelled H-A-H-L-I”** may propose a reusable vocabulary entry after review.
- Promote accepted corrections conservatively into proper-name vocabulary, context/hotword rules, hallucination filters, speaker or sound-event enrollment candidates, and the held-out Whisper/Parakeet evaluation corpus. Do not treat every one-off correction as a global rule or direct model-weight update.
- Provide a small **Notes** surface alongside Timeline, Conversations, and Search, with filters for tag, person, project, date, importance, and unresolved to-dos.
- Provide a lightweight **Corrections** queue, either within Notes initially or as its own small surface, with source navigation and clear unresolved/accepted/dismissed state.
- Offer automatic filing for confident decisions and a lightweight review view for uncertain ones.
- Support rename, recategorize, merge, split, dismiss, correction, and source navigation without a mandatory approval queue.
- Keep ordinary transcript text even when a phrase also creates metadata. Directed-note extraction is a reversible derived layer, not deletion or rewriting of what Ruby actually said.
- Keep reminder verification and presentation separate from reminder
  execution: accepting/editing a suggested reminder creates durable structured
  state; Pestering Mode and the floating bubble are optional delivery surfaces.

## Phase 7 — Porcelain / Claudia interaction boundary

**Goal.** Make Moto X memory available to interactive Claudia through Porcelain while keeping capture/journal behavior independent from full-duplex agent voice.

- Index finalized transcripts, notes, tags, summaries, tasks, people, and source anchors through current Chimera/Porcelain contracts so Claudia can recall and reason over the journal from the main client.
- Porcelain owns interactive dialogue, model/tool routing, screen-on walk mode, headphone output, interruption controls, and any Ruby-authorized cloned voice. Voice cloning is a separate Porcelain feature, not part of Moto X capture.
- Prefer headphones for walk conversation so Claudia's output is private and is not acoustically recycled through the bag microphone. Any coincidental pickup remains ordinary captured environment, not an intended agent feedback channel.
- Moto X never needs Spotify control, recent-audio replay, Bluetooth-speaker orchestration, TTS responses, or general Siri-style commands for this roadmap.
- Keep the systems loosely coupled: capture remains valid if Porcelain is closed; Porcelain can query memory without controlling the recorder; failures in chat, retrieval, or synthesized voice never interrupt durable recording.
- Claudia branding may remain on the reader because Ruby is metaphorically speaking into Claudia's memory and the results feed her local personality/context map. Branding does not imply that the webpage is a live Claudia session.

## Phase 8 — Historical recovery

**Goal.** Recover useful Moto X decisions from old chats while treating old generated code as untrusted historical material.

- Ingest available histories with service, date, conversation, and source provenance.
- Extract candidate decisions, abandoned ideas, and open questions.
- Reconcile candidates against this roadmap and current Porcelain feature records.
- Do not assume Claude-era worktrees or the Claudia PWA are canonical merely because they contain more code.

## Current deployment and next physical validation

Validated on 2026-07-21:

- Syncthing delivered the new receiver, journal state machine, dashboard, and `record_v2.py`; queue/audio/log data remains intentionally unsynced.
- The dedicated PC `.venv-motox` loads Silero VAD plus faster-whisper `large-v3` with CTranslate2 CUDA float16 support.
- The physical Moto X opened `/dashboard`, refreshed live, displayed rendered speaker labels, and showed healthy capture state.
- Old `record.py` drained legacy epoch-named chunks with original capture times. A live logging-induced HTTP 500 during validation was retried, acknowledged once by stable ID, and not duplicated.
- Ruby deliberately switched the physical phone to `record_v2.py`. A supervised receiver outage produced queued captures while recording continued; after restart, the backlog drained to zero with healthy 30-second capture status.

Validated on 2026-07-27:

- Enabled tailnet HTTPS certificates and configured persistent, private Tailscale Serve from `https://sunset.tailfa86ac.ts.net` to the receiver on `127.0.0.1:8765`; no Funnel or public exposure was enabled.
- Changed `record_v2.py`'s default receiver base URL to the stable HTTPS origin while preserving `MOTOX_PC_URL` as an override and keeping 30 seconds canonical.
- The Moto X loaded the HTTPS dashboard and delivered fresh chunks through Tailscale while connected through Ruby's iPhone hotspot rather than the home LAN.
- Android offered the dashboard as an installable app, and iPhone Safari installed it as a web app from the same origin using the selected purple CRT Claudia icon with mint listening dot.
- A second physical connectivity interruption grew the durable queue to 11 captures while recording continued. Restoring Tailscale drained the queue to zero with original capture IDs and timestamps, then returned recorder health to healthy with normal roughly 31-second cycles.
- The focused recorder/journal suite passed all 15 tests. On Windows, the test child process must redirect output rather than inherit the Codex host's output stream because inherited unittest output caused the ChatGPT host to stop unexpectedly.

Observed on 2026-08-01:

- Ruby reported that the lav routed outside the zipped pouch, with its furry windscreen available, captured her voice at the intended quality during normal bag use. This establishes a promising close-mic field profile for evaluation, not a conclusion that every outdoor or fabric-contact condition is solved.
- Confirmed that the receiver already uses faster-whisper `large-v3` in CUDA float16; the next ASR decision is an evidence-based Whisper/Parakeet comparison rather than selecting a nominally larger Whisper checkpoint.
- Added personal acoustic signals and Ruby-specific voice modes to deferred enrichment. Relaxed/low-effort speech is explicitly not equivalent to medical fatigue.

Remaining physical validation:

1. Continue normal 30-second `record_v2.py` use and measure longer-run capture gaps, battery, heat, and queue behavior rather than shortening chunks for artificial live-caption latency.
2. Exercise actual mic placements (clipped lav, lav hanging outside the pouch with windscreen, phone mic inside the bag, between Ruby and Lynn, and farther room speech) and record route/acoustic observations plus fabric-contact failures before trusting any microphone-confidence or speaker label.
3. Build and evaluate the quick-pass/final-pass transcript projection on real cross-boundary sentences before presenting finalized chat bubbles as canonical.
4. Leave speaker and cat enrollment deferred until distinct training and held-out evaluation clips have been collected from real Moto X use for Ruby, Lynn, Raven, Hahli, and Lam.

## Open decisions

- Whether active daily projections should be searchable immediately or only after conversation close.
- Where corrected note examples and reusable filing rules should live.
- Which local runtime best fits the two 16 GB VRAM machines and whether work should be independent, replicated, or deliberately distributed.
- Which Claudia voice becomes canonical in Porcelain; voice selection is outside the Moto X roadmap, while the selected purple CRT Claudia art remains the canonical memory-reader icon.
- How manual speaker corrections should update enrollment profiles without contaminating held-out evaluation clips.
- Which sound-event labels are useful enough to appear in the narrative timeline and what confidence/context rules suppress clutter.
- Whether Whisper or Parakeet should own the quick and final ASR roles after evaluation on corrected Moto X audio.
- Which small set of Ruby-authored voice-mode/self-state labels is useful without encouraging medicalized or overconfident inference.
- Which spoken correction phrases are distinct enough for reliable detection, how far backward each marker should attach by default, and whether ambiguous markers should create an unresolved item without guessing a target.

## References

- Current gateway ingest and retrieval: [`gateway-rag-ingest-and-retrieval.md`](../features/gateway-rag-ingest-and-retrieval.md)
- Current operator chat surface: [`operator-chat-ui.md`](../features/operator-chat-ui.md)
- Current conversation history: [`operator-conversation-history.md`](../features/operator-conversation-history.md)
- Current virtual model routing: [`operator-virtual-models.md`](../features/operator-virtual-models.md)
- Planning conventions: [`README.md`](README.md)
- Private stable web routing: [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve)
