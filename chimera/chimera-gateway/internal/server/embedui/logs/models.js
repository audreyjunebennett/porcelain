/**
 * Logs UI models (JSDoc-only; no runtime behavior).
 *
 * Keep these typedefs stable so components render consistently across the log view
 * (summarized feed, structured table, raw textarea).
 */

/**
 * @typedef {Object} BadgeModel
 * @property {string} text
 * @property {string=} title
 * @property {"neutral"|"info"|"success"|"warn"|"error"|"svc-gateway"|"svc-chimera-broker"|"svc-chimera-indexer"|"svc-chimera-vectorstore"} variant
 */

/**
 * @typedef {Object} MetricPillModel
 * @property {string} text
 * @property {string=} title
 * @property {"neutral"|"info"|"success"|"warn"|"error"} variant
 */

/**
 * @typedef {Object} ExtraKV
 * @property {string} k
 * @property {string} v
 */

/**
 * @typedef {Object} ParsedEntry
 * @property {number=} seq
 * @property {string=} source
 * @property {string=} ts
 * @property {string=} shape
 * @property {string=} levelCanon
 * @property {Object=} flat
 * @property {ExtraKV[]=} extras
 */

