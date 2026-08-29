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
- Accepted smart-identification answers are anchored to a deterministic turn
  ID plus source capture seconds. Undoing the answer removes it from the learned
  profile and makes that turn eligible for review again.
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
- Resolve the speaker layer as region painting (newer identities replace older
  identities only where they overlap) while sound labels remain additive.
- Add timeline-wide editing and corrected journal projection.
- Implement checkpoint/boundary/final conversation ASR workers. Every final
  turn must source-map back to capture IDs and seconds so existing corrections
  can be projected forward instead of being invalidated.
- Add cat-specific proposals after enough Hahli and Lam examples are labeled.
- Add singing-versus-played-music corrections, including explicit overlap.
- Add the consent-based new-speaker enrollment flow for friends and visitors.
