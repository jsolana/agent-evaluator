import ReactMarkdown from "react-markdown";

/** Agent answers are markdown; links open in a new tab. */
export function Markdown({ children }: { children: string }) {
  return (
    <div className="prose-answer space-y-2 text-sm leading-relaxed [&_a]:text-indigo-600 [&_a]:underline [&_li]:ml-4 [&_ol]:list-decimal [&_strong]:font-semibold [&_ul]:list-disc">
      <ReactMarkdown components={{ a: ({ children, href }) => <a href={href} target="_blank" rel="noreferrer">{children}</a> }}>{children}</ReactMarkdown>
    </div>
  );
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** Plain text with every occurrence of the evidence quotes highlighted. */
export function Highlight({ text, marks }: { text: string; marks: string[] }) {
  const quotes = marks.map((m) => m.replace(/^["“”']|["“”']$/g, "").trim()).filter((m) => m.length > 2);
  if (!quotes.length) return <p className="whitespace-pre-wrap text-sm leading-relaxed">{text}</p>;
  const parts = text.split(new RegExp(`(${quotes.map(escape).join("|")})`, "g"));
  return (
    <p className="whitespace-pre-wrap text-sm leading-relaxed">
      {parts.map((p, i) => (quotes.includes(p) ? <mark key={i} className="rounded bg-amber-200/70 px-0.5">{p}</mark> : p))}
    </p>
  );
}
