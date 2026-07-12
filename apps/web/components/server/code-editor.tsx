"use client";

import * as React from "react";
import CodeMirror from "@uiw/react-codemirror";
import { StreamLanguage } from "@codemirror/language";
import { json } from "@codemirror/lang-json";
import { yaml } from "@codemirror/lang-yaml";
import { javascript } from "@codemirror/lang-javascript";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { xml } from "@codemirror/lang-xml";
import { markdown } from "@codemirror/lang-markdown";
import { java } from "@codemirror/lang-java";
import { properties } from "@codemirror/legacy-modes/mode/properties";
import { toml } from "@codemirror/legacy-modes/mode/toml";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { githubDark, githubLight } from "@uiw/codemirror-theme-github";
import { useTheme } from "@/lib/settings/theme";

// ดึงชนิด Extension ผ่าน props ของ CodeMirror แทน import จาก @codemirror/state
// (เป็น transitive dep ที่ pnpm ไม่ hoist ให้ import ตรงได้)
type CMExtension = NonNullable<
  React.ComponentProps<typeof CodeMirror>["extensions"]
>[number];

function baseName(path: string): string {
  const idx = path.lastIndexOf("/");
  return idx < 0 ? path : path.slice(idx + 1);
}

// เลือก language extension จากนามสกุลไฟล์ — ไม่รู้จัก = plain (ไม่ใส่ extension)
function languageFor(filename: string): CMExtension | null {
  const name = baseName(filename).toLowerCase();
  const dot = name.lastIndexOf(".");
  const ext = dot < 0 ? "" : name.slice(dot + 1);
  switch (ext) {
    case "json":
      return json();
    case "yml":
    case "yaml":
      return yaml();
    case "js":
    case "mjs":
    case "cjs":
    case "jsx":
    case "ts":
    case "tsx":
      return javascript({
        jsx: ext === "jsx" || ext === "tsx",
        typescript: ext === "ts" || ext === "tsx",
      });
    case "html":
    case "htm":
      return html();
    case "css":
      return css();
    case "xml":
      return xml();
    case "md":
    case "markdown":
      return markdown();
    case "java":
      return java();
    case "properties":
    case "conf":
    case "ini":
    case "cfg":
      return StreamLanguage.define(properties);
    case "toml":
      return StreamLanguage.define(toml);
    case "sh":
    case "bash":
      return StreamLanguage.define(shell);
    default:
      return null;
  }
}

export default function CodeEditor({
  value,
  onChange,
  readOnly,
  filename,
}: {
  value: string;
  onChange: (value: string) => void;
  readOnly: boolean;
  filename: string;
}) {
  const { resolvedTheme } = useTheme();
  const extensions = React.useMemo(() => {
    const lang = languageFor(filename);
    return lang ? [lang] : [];
  }, [filename]);

  return (
    <CodeMirror
      value={value}
      onChange={onChange}
      readOnly={readOnly}
      editable={!readOnly}
      theme={resolvedTheme === "dark" ? githubDark : githubLight}
      extensions={extensions}
      height="20rem"
      className="overflow-hidden rounded-md border text-xs"
      basicSetup={{
        lineNumbers: true,
        highlightActiveLine: !readOnly,
        highlightActiveLineGutter: !readOnly,
      }}
    />
  );
}
