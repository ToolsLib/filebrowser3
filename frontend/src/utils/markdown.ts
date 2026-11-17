import DOMPurify from "dompurify";
import { marked } from "marked";
import markedKatex from "marked-katex-extension";

marked.use(markedKatex({ output: "mathml", throwOnError: false }));
export async function renderMarkdown(source: string): Promise<string> {
  return DOMPurify.sanitize(await marked(source));
}
