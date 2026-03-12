import { useState, useMemo, useRef, useEffect, useCallback } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { ViewHeader } from "@/components/layout/ViewHeader";
import { docs } from "virtual:docs";

interface TocEntry {
  id: string;
  text: string;
  level: number;
}

function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\w\s-]/g, "")
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .trim();
}

function extractHeadings(content: string): TocEntry[] {
  const entries: TocEntry[] = [];
  const re = /^(#{2,4})\s+(.+)$/gm;
  let m;
  while ((m = re.exec(content)) !== null) {
    entries.push({ id: slugify(m[2]), text: m[2], level: m[1].length });
  }
  return entries;
}

export function DocsView() {
  const grouped = useMemo(() => {
    const map = new Map<string, typeof docs>();
    for (const doc of docs) {
      const list = map.get(doc.group) ?? [];
      list.push(doc);
      map.set(doc.group, list);
    }
    return map;
  }, []);

  const [activePath, setActivePath] = useState(() => {
    const gs = docs.find((d) => d.path === "getting-started.md");
    return gs?.path ?? docs[0]?.path ?? "";
  });

  const activeDoc = docs.find((d) => d.path === activePath);

  const headings = useMemo(
    () => (activeDoc ? extractHeadings(activeDoc.content) : []),
    [activeDoc],
  );

  const contentRef = useRef<HTMLDivElement>(null);
  const [activeHeading, setActiveHeading] = useState("");

  // Track which heading is currently in view
  useEffect(() => {
    const container = contentRef.current;
    if (!container || headings.length === 0) return;

    const handleScroll = () => {
      const scrollTop = container.scrollTop;
      let current = "";
      for (const h of headings) {
        const el = container.querySelector(`#${CSS.escape(h.id)}`);
        if (el) {
          const offset = (el as HTMLElement).offsetTop - container.offsetTop;
          if (offset <= scrollTop + 80) current = h.id;
        }
      }
      setActiveHeading(current);
    };

    container.addEventListener("scroll", handleScroll, { passive: true });
    handleScroll();
    return () => container.removeEventListener("scroll", handleScroll);
  }, [headings, activePath]);

  const scrollTo = useCallback((id: string) => {
    const container = contentRef.current;
    if (!container) return;
    const el = container.querySelector(`#${CSS.escape(id)}`);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, []);

  // Custom heading components that add ids for anchor linking
  const components = useMemo<Components>(
    () => ({
      h2: ({ children }) => {
        const text = extractText(children);
        return <h2 id={slugify(text)}>{children}</h2>;
      },
      h3: ({ children }) => {
        const text = extractText(children);
        return <h3 id={slugify(text)}>{children}</h3>;
      },
      h4: ({ children }) => {
        const text = extractText(children);
        return <h4 id={slugify(text)}>{children}</h4>;
      },
    }),
    [],
  );

  // Group ordering: General first, then alphabetical
  const groupOrder = useMemo(() => {
    const keys = [...grouped.keys()];
    return keys.sort((a, b) => {
      if (a === "General") return -1;
      if (b === "General") return 1;
      return a.localeCompare(b);
    });
  }, [grouped]);

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader title="Documentation" subtitle="Guides, references, and architecture docs" />
      <div className="flex-1 flex min-h-0">
        {/* Doc list sidebar */}
        <div className="w-64 border-r border-gray-200 overflow-y-auto py-2 shrink-0">
          {groupOrder.map((group) => (
            <div key={group}>
              <div className="px-4 pt-3 pb-1 text-[10px] font-semibold text-gray-400 uppercase tracking-wider">
                {group}
              </div>
              {grouped.get(group)!.map((doc) => (
                <button
                  key={doc.path}
                  onClick={() => setActivePath(doc.path)}
                  className={`w-full text-left px-4 py-1.5 text-sm transition-colors ${
                    activePath === doc.path
                      ? "bg-blue-50 text-blue-700 font-medium"
                      : "text-gray-600 hover:bg-gray-100 hover:text-gray-900"
                  }`}
                >
                  {doc.title}
                </button>
              ))}
            </div>
          ))}
        </div>

        {/* Markdown content */}
        <div ref={contentRef} className="flex-1 overflow-y-auto p-8">
          {activeDoc ? (
            <div className="max-w-3xl mx-auto docs-prose">
              <Markdown remarkPlugins={[remarkGfm]} components={components}>
                {activeDoc.content}
              </Markdown>
            </div>
          ) : (
            <div className="text-sm text-gray-400">Select a document to view.</div>
          )}
        </div>

        {/* Table of contents */}
        {activeDoc && headings.length > 0 && (
          <div className="w-56 border-l border-gray-200 overflow-y-auto py-4 px-3 shrink-0">
            <div className="text-[10px] font-semibold text-gray-400 uppercase tracking-wider mb-2">
              On this page
            </div>
            <nav className="space-y-0.5">
              {headings.map((h) => (
                <button
                  key={h.id}
                  onClick={() => scrollTo(h.id)}
                  className={`block w-full text-left text-xs py-1 transition-colors ${
                    h.level === 3 ? "pl-3" : h.level === 4 ? "pl-6" : ""
                  } ${
                    activeHeading === h.id
                      ? "text-blue-600 font-medium"
                      : "text-gray-500 hover:text-gray-900"
                  }`}
                >
                  {h.text}
                </button>
              ))}
            </nav>
          </div>
        )}
      </div>
    </div>
  );
}

/** Recursively extract plain text from React children. */
function extractText(children: React.ReactNode): string {
  if (typeof children === "string") return children;
  if (typeof children === "number") return String(children);
  if (Array.isArray(children)) return children.map(extractText).join("");
  if (children && typeof children === "object" && "props" in children) {
    const el = children as React.ReactElement<{ children?: React.ReactNode }>;
    return extractText(el.props.children);
  }
  return "";
}
