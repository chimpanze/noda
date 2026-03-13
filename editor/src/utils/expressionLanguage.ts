import type * as Monaco from "monaco-editor";
import * as api from "@/api/client";

let registered = false;

/**
 * Register the Noda expression language (completion provider) with Monaco.
 * Safe to call multiple times — only registers once.
 */
export function registerExpressionLanguage(monaco: typeof Monaco) {
  if (registered) return;
  registered = true;

  // Register a completion provider for plaintext (used by expression editors)
  monaco.languages.registerCompletionItemProvider("plaintext", {
    triggerCharacters: ["{", ".", "("],
    provideCompletionItems: async (model, position) => {
      const textUntilPosition = model.getValueInRange({
        startLineNumber: 1,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      });

      // Only provide completions inside {{ }} expressions
      const lastOpen = textUntilPosition.lastIndexOf("{{");
      const lastClose = textUntilPosition.lastIndexOf("}}");
      if (lastOpen === -1 || lastClose > lastOpen) {
        return { suggestions: [] };
      }

      const word = model.getWordUntilPosition(position);
      const range: Monaco.IRange = {
        startLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endLineNumber: position.lineNumber,
        endColumn: word.endColumn,
      };

      // Get context from the expression inside {{ }}
      const exprText = textUntilPosition.slice(lastOpen + 2).trim();

      const suggestions: Monaco.languages.CompletionItem[] = [];

      // Fetch context if we have workflow info (stored on window by the widget)
      const ctx = getCachedContext();
      if (ctx) {
        // Variables
        for (const v of ctx.variables) {
          suggestions.push({
            label: v.name,
            kind: monaco.languages.CompletionItemKind.Variable,
            insertText: v.name,
            detail: v.type,
            documentation: v.description,
            range,
          });
        }

        // Upstream node references
        for (const u of ctx.upstream) {
          suggestions.push({
            label: u.ref,
            kind: monaco.languages.CompletionItemKind.Reference,
            insertText: u.ref,
            detail: u.node_type,
            documentation: `Output from node "${u.node_id}" (${u.node_type})`,
            range,
          });
        }

        // Functions
        for (const f of ctx.functions) {
          suggestions.push({
            label: f.name,
            kind: monaco.languages.CompletionItemKind.Function,
            insertText: f.name,
            detail: "function",
            documentation: f.description,
            range,
          });
        }
      } else {
        // Fallback: provide basic suggestions without context
        const basicVars = ["input", "trigger", "auth", "nodes"];
        for (const name of basicVars) {
          suggestions.push({
            label: name,
            kind: monaco.languages.CompletionItemKind.Variable,
            insertText: name,
            detail: "object",
            range,
          });
        }

        const basicFns = [
          { name: "$uuid()", desc: "Generate UUID v4" },
          { name: "now()", desc: "Current timestamp" },
          { name: "upper(s)", desc: "Uppercase string" },
          { name: "lower(s)", desc: "Lowercase string" },
          { name: "len(v)", desc: "Length of array/string/map" },
          { name: "toInt(v)", desc: "Convert to integer" },
          { name: "toFloat(v)", desc: "Convert to float" },
        ];
        for (const f of basicFns) {
          suggestions.push({
            label: f.name,
            kind: monaco.languages.CompletionItemKind.Function,
            insertText: f.name,
            documentation: f.desc,
            range,
          });
        }
      }

      // If user typed "input." or "nodes.", suggest dot access
      if (exprText.endsWith("input.")) {
        suggestions.push(
          {
            label: "body",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "body",
            range,
          },
          {
            label: "params",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "params",
            range,
          },
          {
            label: "query",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "query",
            range,
          },
          {
            label: "headers",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "headers",
            range,
          },
        );
      }

      if (exprText.endsWith("trigger.")) {
        suggestions.push(
          {
            label: "method",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "method",
            range,
          },
          {
            label: "path",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "path",
            range,
          },
          {
            label: "trace_id",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "trace_id",
            range,
          },
          {
            label: "route_id",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "route_id",
            range,
          },
        );
      }

      if (exprText.endsWith("auth.")) {
        suggestions.push(
          {
            label: "user_id",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "user_id",
            range,
          },
          {
            label: "roles",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "roles",
            range,
          },
          {
            label: "claims",
            kind: monaco.languages.CompletionItemKind.Field,
            insertText: "claims",
            range,
          },
        );
      }

      return { suggestions };
    },
  });
}

// Simple cache for expression context to avoid excessive API calls
let cachedContext: api.ExpressionContext | null = null;
let cacheKey = "";

export function updateExpressionContext(workflow: string, node: string) {
  const key = `${workflow}:${node}`;
  if (key === cacheKey) return;
  cacheKey = key;
  api
    .getExpressionContext(workflow, node)
    .then((ctx) => {
      cachedContext = ctx;
    })
    .catch(() => {
      // Silently fail — autocomplete just won't have context
    });
}

export function getCachedContext(): api.ExpressionContext | null {
  return cachedContext;
}
