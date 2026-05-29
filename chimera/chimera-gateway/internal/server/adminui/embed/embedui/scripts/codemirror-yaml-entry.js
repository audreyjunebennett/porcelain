/**
 * CodeMirror 6 bundle entry — exposes globalThis.ChimeraCodeMirror for operator YAML panels.
 */
import { EditorState, Compartment } from "@codemirror/state";
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLineGutter,
  highlightActiveLine,
  drawSelection,
  rectangularSelection,
  highlightSpecialChars,
} from "@codemirror/view";
import { yaml } from "@codemirror/lang-yaml";
import { defaultKeymap, indentWithTab } from "@codemirror/commands";
import { syntaxHighlighting, defaultHighlightStyle, indentOnInput, foldGutter } from "@codemirror/language";

var readOnlyCompartment = new Compartment();

var chimeraEditorTheme = EditorView.theme(
  {
    "&": {
      backgroundColor: "var(--embed-surface-page)",
      color: "var(--embed-text-secondary)",
      fontSize: "0.76rem",
      height: "100%",
    },
    "&.cm-focused": {
      outline: "none",
    },
    ".cm-scroller": {
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      lineHeight: "1.35",
      overflow: "auto",
    },
    ".cm-content": {
      padding: "0.5rem 0.45rem 0.45rem 0.15rem",
      caretColor: "var(--embed-text-primary)",
    },
    ".cm-gutters": {
      backgroundColor: "var(--embed-surface-muted-2)",
      color: "var(--embed-text-muted)",
      border: "none",
      borderRight: "1px solid var(--embed-border-medium)",
    },
    ".cm-activeLineGutter": {
      backgroundColor: "var(--embed-surface-row-hover)",
    },
    ".cm-activeLine": {
      backgroundColor: "var(--embed-surface-muted)",
    },
    ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
      backgroundColor: "var(--embed-surface-row-hover) !important",
    },
    ".cm-cursor, .cm-dropCursor": {
      borderLeftColor: "var(--embed-text-primary)",
    },
    ".cm-foldPlaceholder": {
      backgroundColor: "var(--embed-surface-muted-2)",
      border: "none",
      color: "var(--embed-text-muted)",
    },
  },
  { dark: false }
);

function baseExtensions(readOnly) {
  return [
    lineNumbers(),
    highlightActiveLineGutter(),
    highlightActiveLine(),
    drawSelection(),
    rectangularSelection(),
    highlightSpecialChars(),
    indentOnInput(),
    foldGutter(),
    yaml(),
    syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
    keymap.of([indentWithTab, ...defaultKeymap]),
    chimeraEditorTheme,
    EditorView.lineWrapping,
    readOnlyCompartment.of(EditorState.readOnly.of(!!readOnly)),
  ];
}

/**
 * @param {HTMLElement} parent
 * @param {{ value?: string, readOnly?: boolean, onChange?: function(string): void, onFocus?: function(): void, onBlur?: function(): void, onScroll?: function(): void }} opts
 * @returns {EditorView}
 */
function createYamlEditor(parent, opts) {
  opts = opts || {};
  var initial = opts.value != null ? String(opts.value) : "";

  var view = new EditorView({
    parent: parent,
    state: EditorState.create({
      doc: initial,
      extensions: baseExtensions(opts.readOnly).concat([
        EditorView.updateListener.of(function (update) {
          if (update.docChanged && typeof opts.onChange === "function") {
            opts.onChange(update.state.doc.toString());
          }
          if (update.focusChanged) {
            if (update.view.hasFocus && typeof opts.onFocus === "function") opts.onFocus();
            if (!update.view.hasFocus && typeof opts.onBlur === "function") opts.onBlur();
          }
          if (update.geometryChanged && typeof opts.onScroll === "function") {
            opts.onScroll();
          }
        }),
      ]),
    }),
  });

  return view;
}

function getValue(view) {
  return view.state.doc.toString();
}

function setValue(view, value) {
  var next = value != null ? String(value) : "";
  if (next === view.state.doc.toString()) return;
  view.dispatch({
    changes: { from: 0, to: view.state.doc.length, insert: next },
  });
}

function setReadOnly(view, readOnly) {
  view.dispatch({
    effects: readOnlyCompartment.reconfigure(EditorState.readOnly.of(!!readOnly)),
  });
}

function focus(view) {
  view.focus();
}

function requestMeasure(view) {
  view.requestMeasure();
}

globalThis.ChimeraCodeMirror = {
  createYamlEditor: createYamlEditor,
  getValue: getValue,
  setValue: setValue,
  setReadOnly: setReadOnly,
  focus: focus,
  requestMeasure: requestMeasure,
};
