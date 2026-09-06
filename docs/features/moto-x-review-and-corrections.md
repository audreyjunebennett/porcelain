# Feature: Moto X review and corrections

| Field | Value |
|---|---|
| **Doc kind** | `feature-record` |
| **Areas** | Moto X transcript review, speaker enrollment, corrections |
| **Status** | `partial` |
| **Introduced** | Moto X roadmap Phases 3 and 6 |
| **Related features** | [Moto X receiver manager](moto-x-receiver-manager.md) |

## At a glance

The private Moto X web app includes a phone-first **Teach Claudia** review
surface. It plays real captures, shows selectable transcript text, and stores
Ruby's speaker, cat, sound, overlap, and word corrections as reversible derived
annotations. Model guesses remain separate from accepted human labels.

The default **Smart identification** mode presents one short diarized turn at a
time with its wall-clock time and exact source-audio range, asking a focused
question such as **Ruby or Lynn?**. Each accepted identity becomes a verified
voice example; the next question is then ranked again against the updated
Ruby/Lynn/Raven profiles instead of walking backward through recent clips.

## Behavior and invariants

- `/review` requests approximately five-minute review batches and streams the
  original audio with range support for phone seeking.
- The review HTML, JavaScript, and CSS are served without browser caching so a
  normal reload picks up operator UI fixes immediately.
- New quick-pass transcripts retain word-level timestamps. Selecting words
  automatically selects and can loop their source-audio interval. Historical
  clips without timings retain the manual audio-range fallback.
- A prominent scope banner shows whether the next action targets words, timed
  audio, both, or the whole clip. Whole-clip labels require confirmation.
- Audio can also be scoped independently: scrub to a point, set a start, scrub
  forward, and set an end. A correction may carry a text span, an audio span,
  both, or neither; multiple labels may overlap the same seconds so speech over
  music is represented without flattening either event.
- Quick labels include Ruby, Lynn, Raven, Other, Hahli, Lam, television, not
  speech, overlap, Ruby singing, and played music/song.
- New people can receive a named speaker profile through an explicit
  consent-and-enrollment action. A typed name alone never converts uncertain
  historical audio into that identity.
- Word corrections preserve both the original machine text and replacement.
- The review card projects accepted word corrections immediately and provides
  a **Show original / Show corrected** switch. Original ASR remains immutable.
- A progressively expandable context drawer loads neighboring speech chunks
  from the same conversation. Boundary labels record whether a sentence
  continues before, after, or across both sides of the current capture.
- **Exclude from memory & training** is a reversible privacy annotation. Active
  exclusions are omitted from review queues, anonymous grouping, and the
  Community-1 experiment, recent dashboard text, and regenerated readable
  journals without deleting source evidence.
- Every correction can be undone without changing raw audio or the `chunks`
  evidence row.
- Anonymous model proposals use names such as `Voice group A`. Duration,
  microphone proximity, and cluster order never assign a household identity.
- Smart identification considers only 1.5–8 second diarized regions, loops the
  exact region, and keeps **Other** and **Not sure** available even when the two
  nearest verified profiles are shown in the question.
- Smart identification presents the diarizer's region as an editable proposed
  crop in a large touch-first waveform trimmer. Ruby can drag either bracket,
  preview the looping result, and remove an adjacent interjection before
  confirming; the initial crop remains unchanged when no edit is needed. The
  native browser audio controls remain hidden behind this single larger player.
  Each bracket has one visual edge and a larger invisible touch target; tapping
  inside that target does not move or visually duplicate the boundary.
- Tapping the **Applying to** scope bubble clears an automatic or edited crop
  and switches the answer back to the whole source clip. That explicit whole-
  clip scope remains active after accepting the safety confirmation and saving
  an additional sound label.
- A named answer stores a WavLM embedding extracted from the exact accepted
  crop. The deterministic diarized turn ID remains attached as provenance, but
  no longer overwrites a human-adjusted audio boundary. Compatible CPU and CUDA
  builds of the same torchaudio release share one model family.
- Smart identification is a two-step choice: select an identity, then use
  **Confirm <identity> & next**. Ambiguous or overlapping speech can be marked
  **Not sure / mixed voices** instead of forcing an identity into training.
  Tapping the selected identity again clears it.
- Sound-only labels such as television, music, cats, or overlapping voices do
  not require a speaker answer; after saving one, the primary action becomes
  **Confirm sound only & next**. Whole-clip sound, overlap, and boundary labels
  save without an additional browser confirmation; whole-clip named speaker
  assignments retain the safety confirmation.
- In Smart identification, annotation chips are scoped to the current
  diarized turn. Labels from another turn in the same 30-second source capture
  do not remain visible or enable its primary action.
- Saving a sound or overlap label in Smart identification preserves the exact
  diarized audio range, allowing a speaker identity and overlapping sound to
  be stacked on the same turn without falling back to the whole source clip.
- Repeating the same active label for the same text/audio scope is idempotent.
  Smart-identification sound buttons show their saved state and cannot create
  a duplicate annotation for the current turn.
- Advancing a Smart-identification turn with only sound/overlap labels does not
  invent a speaker answer. The saved sound/overlap annotation itself keeps that
  exact completed question from returning later.
- Existing sound and overlap annotations also count that exact diarized turn as
  handled, so older television or mixed-voice answers do not re-enter the queue.
- When television is marked in at least three captures and at least 60% of the
  reviewed captures from one diarization report, the remaining turns from that
  report are suppressed as television-contaminated. The inference is derived
  from active annotations, so undoing labels can make the source eligible again.
- Named answers that also carry a sound or overlap annotation remain visible as
  corrections but are excluded from Ruby/Lynn/Raven voice-profile centroids.
- A Smart-identification batch asks at most one question from each 30-second
  source capture, and the page avoids that capture again until it is refreshed.
  This prevents one noisy room or television recording from dominating a run;
  the empty state explicitly offers another pass when remaining turns may exist.
- The Smart-identification question number counts advances across adaptive
  re-ranking reloads instead of resetting to one after every confirmed answer.
- A manual **Next**, **Skip**, or confirmed Smart-identification answer arms the
  following clip to play automatically from its selected audio boundary. The
  initial page load remains quiet, and browser autoplay rejection is surfaced.
- Accepted smart-identification answers are anchored to a deterministic turn
  ID plus source capture seconds. Undoing the answer removes it from the learned
  profile and makes that turn eligible for review again.
- The dashboard's **Today's journal** view is an exact rolling 24-hour window,
  so midnight does not hide the preceding evening. Date-addressed journals stay
  available as deterministic archive views.
- Today's journal projects conservative speaker enrichment from prepared
  diarized turns and the current clean identity centroids. Confirmed identities
  and high-confidence predictions can replace the provisional chunk speaker;
  low-confidence turns keep the original display instead of forcing a name.
  Identity readiness is independent per person, so a missing Raven profile does
  not prevent Ruby or Lynn predictions.
- Where word timestamps are available, the journal projects words onto those
  diarized turns and renders separate chat bubbles: Ruby on the right in purple,
  Lynn on the left in green, and Raven on the left in orange. Unassigned or
  genuinely mixed words remain neutral instead of collapsing names into a
  combined `Ruby / Lynn` chunk label.
- A journal bubble's colored speaker name links its exact source-audio range to
  Teach Claudia; the journal has no redundant separate edit link. The main
  listening dashboard alone uses the lightweight Unsorted/Ruby/Lynn picker.
- Adjusting a Teach Claudia audio crop recomputes its visible excerpt from word
  timestamps and saves matching character boundaries. Confirmed journal turns
  use the accepted human crop rather than expanding back to the diarizer's
  original boundary. When word timings are unavailable, a narrow crop remains
  audio-only instead of presenting the whole transcript as though it matched.
- The waveform and transcript are one bidirectional selection: dragging either
  audio bracket paints the overlapping timed words in mint, while selecting
  transcript words snaps the audio brackets to their word times and loops that
  range. The full transcript remains visible for context. Older captures without
  word timings state that the selection is audio-only.
- The touch trimmer uses only play/pause and its draggable brackets. Active
  crops loop automatically; the compact **Applying to** row is the sole action
  that clears text and audio scope back to the whole clip. Transcript correction
  appears beside the selected text, and the clip advance/confirmation action is
  sticky at the bottom of the viewport.
- Speaker corrections project as non-overlapping region paint. A newer Ruby,
  Lynn, Raven, Other, Unknown, or Not sure range replaces an older identity only
  where their audio overlaps, preserving the older speaker on either side and
  producing separate timed chat bubbles where word timestamps are available.
- Conversation status appears only as **recording now** while a conversation is
  active. Ended conversations show no lifecycle badge, avoiding any implication
  that speaker review is complete. Each journal bubble links its source capture
  and exact timed range back to Teach Claudia for reversible speaker correction.
- Journal audio controls are created lazily only after **Play turn** is tapped,
  avoiding hundreds of costly native media elements during initial mobile page
  rendering. Rolling views also calculate their cross-midnight speaker layer in
  one pass and load timed words only for clips that will actually be enriched.
- The rolling journal initially renders the newest 12 conversations and offers
  Older/Newer paging, keeping hundreds of chat bubbles out of the initial mobile
  document while preserving access to the complete 24-hour window.
- The main listening dashboard shows the eight most recent speech captures as
  the same voice-colored chat bubbles, newest first so the latest transcript is
  visible without scrolling. Teach Claudia and Today's journal remain above the
  transcript feed for one-tap phone access. The colored capture-health line is
  the only listening-status heading; speech/ambient/silent/queued totals and the
  refreshed time sit below the feed. Prepared confident regions use the
  Ruby/Lynn/Raven projection; recordings awaiting diarization remain neutral
  and explicitly Unsorted. Tapping a bubble's speaker badge opens a lightweight
  Unsorted/Ruby/Lynn picker; choosing a name replaces any conflicting speaker
  correction for that audio range, while choosing Unsorted clears it. Tapping
  or scrolling away closes the picker without changing anything.
- The installed PWA manifest and every in-scope page declare the same opaque
  black theme/background plus a dark color scheme, allowing Android's
  standalone system chrome to choose its dark treatment consistently. The
  manifest is served without a long cache so installed-shell updates are not
  held behind the ordinary static-asset cache.
- Compatible WavLM embeddings form normalized per-person centroids from human
  Ruby/Lynn/Raven answers. The queue favors clean, uncertain comparisons and
  spreads bootstrap questions across diarization clusters. `Other` is not used
  as one global voice profile because it may contain many different people.
- Five-minute progress is estimated from labeled text coverage and detected
  speech duration until word-level timestamps are available.

## Current automatic bootstrap

`build_review_groups.py` decodes derived in-memory PCM with FFmpeg, extracts
public WavLM Base+ features from energetic regions, and groups recent clips
with deterministic K-means. These groups are triage suggestions, not identity
claims. Pyannote Community-1 remains the intended diarization baseline after
its gated model terms are accepted.

`run_community_diarization.py` is the first isolated Community-1 experiment
worker. It reads the journal database in read-only mode, selects a strong
contiguous conversation window, decodes source captures to in-memory mono PCM,
and writes source-mapped ordinary and exclusive turn layers under the private,
Git-ignored `Moto X/diarization_data/` tree. It never assigns household names.

`build_identification_turns.py` is the additive bridge from one of those reports
to Smart identification. It merges adjacent same-cluster fragments, discards
very short regions, trims unusually long regions to a centered four-second
question, extracts normalized WavLM Base+ embeddings, and stores the derived
turns in the review database. Privacy-excluded captures are never decoded or
stored by this worker.

Example dry run and import:

```powershell
python build_identification_turns.py `
  --database ../motox_audio_data/motox_v1.sqlite3 `
  --report ../diarization_data/derived/diarization/community-1-YYYYMMDD-HHMMSS.json `
  --dry-run

python build_identification_turns.py `
  --database ../motox_audio_data/motox_v1.sqlite3 `
  --report ../diarization_data/derived/diarization/community-1-YYYYMMDD-HHMMSS.json
```

## Storage

The existing Moto X SQLite database uses additive, in-place migrations:

- `review_annotations`: active and reverted human corrections, with optional
  `audio_start_seconds`, `audio_end_seconds`, and `speaker_turn_id`. Existing
  databases add these columns in place without rewriting earlier annotations.
- `review_proposals`: versioned model/cluster guesses and provenance.
- `review_speaker_turns`: deterministic source-mapped diarized regions,
  normalized embeddings, quality, compatible-model provenance, and scoped
  cluster IDs used by the adaptive question ranker.
- `transcription_passes`: versioned quick, checkpoint, boundary, or final ASR
  hypotheses. A newer pass supersedes only the earlier derived pass.
- `transcription_words`: exact source-audio time and character anchors for each
  word in a transcription pass.

The original `chunks.transcript`, source audio, timestamps, and conversation
records remain unchanged.

## Code map

| Concern | Location |
|---|---|
| Review persistence | `Moto X/phone-scripts/motox_review.py` |
| Private APIs and page route | `Moto X/phone-scripts/receiver.py` |
| Mobile UI | `Moto X/phone-scripts/dashboard_assets/review.*` |
| Anonymous grouping worker | `Moto X/phone-scripts/build_review_groups.py` |
| Community-1 experiment worker | `Moto X/phone-scripts/run_community_diarization.py` |
| Adaptive identification importer | `Moto X/phone-scripts/build_identification_turns.py` |
| Tests | `test_motox_review.py`, `test_receiver_review.py` |

## Remaining work

- Backfill word timestamps for worthwhile historical captures and add a
  draggable waveform/nudge editor around the shipped text-to-audio sync.
- Run Community-1 and identification-turn import automatically when a finalized
  conversation becomes available; the current worker is an explicit offline
  step.
- Calibrate similarity thresholds on held-out real Moto X examples before any
  high-confidence identity can bypass human review.
- Add timeline-wide editing and corrected journal projection.
- Implement checkpoint/boundary/final conversation ASR workers. Every final
  turn must source-map back to capture IDs and seconds so existing corrections
  can be projected forward instead of being invalidated.
- Add cat-specific proposals after enough Hahli and Lam examples are labeled.
- Add singing-versus-played-music corrections, including explicit overlap.
- Add the consent-based new-speaker enrollment flow for friends and visitors.
