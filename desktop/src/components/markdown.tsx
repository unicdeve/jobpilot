import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { openExternal } from "../lib/platform";

/// Markdown renders untrusted markdown — job descriptions arrive from job
/// boards and imported pages, and `docs/07_security/00_threat_model.md`
/// treats that text as hostile data rather than content to be trusted.
///
/// Three deliberate restrictions follow from that:
///
///   - Raw HTML is never rendered. react-markdown ignores embedded HTML
///     unless a rehype-raw plugin is added; none is, and none should be.
///   - Images are not loaded, only their alt text is shown. A remote image
///     in a job posting is a request to a third-party server that reveals
///     the user is reading that posting — which `docs/07_security/01_privacy.md`
///     rules out, since no such request is needed by any service the user
///     configured. The app's CSP (`default-src 'self'`) would block the
///     fetch anyway; this keeps the page from showing broken images and
///     does not rely on the CSP alone.
///   - Links open in the user's browser rather than navigating the webview,
///     which would otherwise replace the app with the job board. Only
///     http(s) links are offered at all: react-markdown already strips
///     dangerous URL schemes, and this refuses anything else that survives.
export function Markdown({ children }: { children: string }) {
  return (
    <div className="space-y-2 text-sm text-subtle-foreground select-text">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}

function ExternalLink({ href, children }: { href?: string; children?: React.ReactNode }) {
  const safe = href && /^https?:\/\//i.test(href);
  if (!safe) return <>{children}</>;

  return (
    <a
      href={href}
      className="text-foreground underline underline-offset-2"
      onClick={(event) => {
        // The webview would otherwise navigate away from the app itself.
        event.preventDefault();
        void openExternal(href);
      }}
    >
      {children}
    </a>
  );
}

const components: Components = {
  a: ({ href, children }) => <ExternalLink href={href}>{children}</ExternalLink>,
  img: ({ alt }) => (alt ? <span className="text-muted-foreground">[image: {alt}]</span> : null),

  h1: ({ children }) => <h2 className="mt-4 text-base font-semibold text-foreground">{children}</h2>,
  h2: ({ children }) => <h3 className="mt-4 text-sm font-semibold text-foreground">{children}</h3>,
  h3: ({ children }) => <h4 className="mt-3 text-sm font-semibold text-subtle-foreground">{children}</h4>,
  h4: ({ children }) => <h5 className="mt-3 text-sm font-medium text-subtle-foreground">{children}</h5>,
  h5: ({ children }) => <h6 className="mt-3 text-sm font-medium text-subtle-foreground">{children}</h6>,
  h6: ({ children }) => <h6 className="mt-3 text-sm font-medium text-subtle-foreground">{children}</h6>,

  p: ({ children }) => <p className="leading-relaxed">{children}</p>,
  ul: ({ children }) => <ul className="list-outside list-disc space-y-1 pl-5">{children}</ul>,
  ol: ({ children }) => <ol className="list-outside list-decimal space-y-1 pl-5">{children}</ol>,
  li: ({ children }) => <li className="leading-relaxed">{children}</li>,
  strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
  blockquote: ({ children }) => (
    <blockquote className="border-l-2 border-border pl-3 text-subtle-foreground">{children}</blockquote>
  ),
  hr: () => <hr className="border-border" />,

  code: ({ children }) => <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">{children}</code>,
  pre: ({ children }) => <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">{children}</pre>,

  // Job descriptions converted from HTML often carry salary or benefit
  // tables, which need to scroll rather than widen the page.
  table: ({ children }) => (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-left text-xs">{children}</table>
    </div>
  ),
  th: ({ children }) => <th className="border-b border-border py-1 pr-3 font-medium text-foreground">{children}</th>,
  td: ({ children }) => <td className="border-b border-border py-1 pr-3">{children}</td>,
};
