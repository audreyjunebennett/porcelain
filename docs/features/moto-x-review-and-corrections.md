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

## Behavior and invariants

- `/review` requests approximately five-minute review batches and streams the
  original audio with range support for phone seeking.
- Selecting transcript text scopes the next label or correction. With no text
  selected, the action applies to the complete capture.
- Quick labels include Ruby, Lynn, Raven, Other, Hahli, Lam, television, not
  speech, overlap, Ruby singing, and played music/song.
- New people can receive a named speaker profile through an explicit
  consent-and-enrollment action. A typed name alone never converts uncertain
  historical audio into that identity.
- Word corrections preserve both the original machine text and replacement.
- Every correction can be undone without changing raw audio or the `chunks`
  evidence row.
- Anonymous model proposals use names such as `Voice group A`. Duration,
  microphone proximity, and cluster order never assign a household identity.
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

## Storage

The existing Moto X SQLite database gains two additive tables:

- `review_annotations`: active and reverted human corrections.
- `review_proposals`: versioned model/cluster guesses and provenance.

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
| Tests | `test_motox_review.py`, `test_receiver_review.py` |

## Remaining work

- Add word-level ASR timestamps so selected text maps to exact audio spans.
- Replace clip-level bootstrap groups with turn-level Community-1 diarization.
- Learn conservative Ruby/Lynn/Raven profiles from accepted clean spans.
- Add timeline-wide editing and corrected journal projection.
- Add cat-specific proposals after enough Hahli and Lam examples are labeled.
- Add singing-versus-played-music corrections, including explicit overlap.
- Add the consent-based new-speaker enrollment flow for friends and visitors.
