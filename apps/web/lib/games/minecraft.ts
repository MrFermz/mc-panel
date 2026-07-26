// Minecraft game profile ฝั่ง web — ทุกอย่างที่ "เป็น Minecraft" ใน UI อยู่ที่ไฟล์นี้ที่เดียว
// (เดา variant/version จาก jar, กติกาชื่อผู้เล่น, การลงสี log, ชื่อ EULA/metric)
//
// เพิ่มเกมใหม่ = เพิ่มไฟล์แบบเดียวกันนี้แล้วลงทะเบียนใน lib/games/index.ts

import type { DetectedInstance, GameProfile } from "@/lib/games/types";

// ชื่อผู้เล่น Minecraft — เช็คคร่าว ๆ ฝั่ง client เท่านั้น ตัวจริงถูก verify กับ Mojang ที่ backend
const USERNAME_RE = /^[A-Za-z0-9_]{3,16}$/;

// velocity เป็น proxy ไม่รัน server jar ของ Mojang — ไม่มี EULA ให้ยอมรับ
const VARIANTS_WITHOUT_LICENSE = new Set(["velocity"]);

// ---------- การเดา variant/version จาก archive ก่อน upload ----------
// backend ตรวจซ้ำตอน import อยู่แล้ว ที่นี่แค่ช่วยกรอกฟอร์ม พังก็ไม่ block

function guessVariant(jarName: string): string {
  const n = jarName.toLowerCase();
  if (/paper|purpur|spigot/.test(n)) return "paper";
  if (/fabric/.test(n)) return "fabric";
  if (/forge/.test(n)) return "forge";
  if (/velocity/.test(n)) return "velocity";
  return "vanilla";
}

// fallback: ดึงเลขเวอร์ชันจากชื่อไฟล์ jar
function versionFromName(name: string): string | undefined {
  const m = name.match(/\d+\.\d+(?:\.\d+)?/);
  return m ? m[0] : undefined;
}

// server jar เป็น zip — root มี version.json ที่ระบุเวอร์ชันจริง (id/name)
async function versionFromJarBytes(
  bytes: Uint8Array,
): Promise<string | undefined> {
  try {
    const { default: JSZip } = await import("jszip");
    const inner = await JSZip.loadAsync(bytes);
    const entry = inner.file("version.json");
    if (!entry) return undefined;
    const raw = await entry.async("string");
    const json = JSON.parse(raw) as { id?: string; name?: string };
    return json.id || json.name || undefined;
  } catch {
    return undefined;
  }
}

// jar ที่อยู่ root ของ archive (ไม่มี "/" ในชื่อ) — prefer ชื่อที่บอก variant ชัด
function pickRootJar(paths: string[]): string | undefined {
  const jars = paths.filter(
    (p) => p.toLowerCase().endsWith(".jar") && !p.includes("/"),
  );
  if (jars.length === 0) return undefined;
  return (
    jars.find((p) =>
      /paper|purpur|spigot|vanilla|fabric|forge|velocity|server/i.test(p),
    ) ?? jars[0]
  );
}

// path ของไฟล์ในโฟลเดอร์ โดยตัดชื่อโฟลเดอร์บนสุดออก (เทียบ root ของ archive)
function rootRelPath(f: File): string {
  const rel = f.webkitRelativePath || f.name;
  const slash = rel.indexOf("/");
  return slash >= 0 ? rel.slice(slash + 1) : rel;
}

async function detectFromZip(file: File): Promise<DetectedInstance> {
  const detected: DetectedInstance = { name: file.name.replace(/\.zip$/i, "") };
  try {
    const { default: JSZip } = await import("jszip");
    const outer = await JSZip.loadAsync(file);
    const jarPath = pickRootJar(Object.keys(outer.files));
    if (jarPath) {
      detected.variant = guessVariant(jarPath);
      const entry = outer.file(jarPath);
      const bytes = entry ? await entry.async("uint8array") : undefined;
      detected.gameVersion =
        (bytes ? await versionFromJarBytes(bytes) : undefined) ??
        versionFromName(jarPath);
    }
  } catch {
    // ล้มเหลว = ปล่อยให้กรอกเอง ไม่ block
  }
  return detected;
}

async function detectFromFolder(
  files: File[],
  folderName: string,
): Promise<DetectedInstance> {
  const detected: DetectedInstance = { name: folderName || undefined };
  try {
    const jarFiles = files.filter((f) => {
      const p = rootRelPath(f).toLowerCase();
      return p.endsWith(".jar") && !p.includes("/");
    });
    const jar =
      jarFiles.find((f) =>
        /paper|purpur|spigot|vanilla|fabric|forge|velocity|server/i.test(
          rootRelPath(f),
        ),
      ) ?? jarFiles[0];
    if (jar) {
      detected.variant = guessVariant(rootRelPath(jar));
      const bytes = new Uint8Array(await jar.arrayBuffer());
      detected.gameVersion =
        (await versionFromJarBytes(bytes)) ?? versionFromName(rootRelPath(jar));
    }
  } catch {
    // ล้มเหลว = ปล่อยให้กรอกเอง ไม่ block
  }
  return detected;
}

// ---------- การลงสีบรรทัด console ----------
// ตัว timestamp/level parse เป็นของกลาง (server-console.tsx) — ที่นี่คือรูปแบบข้อความของ MC

const SGR = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  gray: "\x1b[90m",
  green: "\x1b[32m",
  brightGreen: "\x1b[92m",
  cyan: "\x1b[36m",
} as const;

function highlightConsoleMessage(msg: string): string | null {
  if (/ (joined|joined the game)$/.test(msg))
    return `${SGR.green}${msg}${SGR.reset}`;
  if (/ left the game$/.test(msg)) return `${SGR.gray}${msg}${SGR.reset}`;
  if (/^Done \(/.test(msg))
    return `${SGR.brightGreen}${SGR.bold}${msg}${SGR.reset}`;
  const chat = /^<([^>]+)>\s([\s\S]*)$/.exec(msg);
  if (chat) return `${SGR.cyan}<${chat[1]}>${SGR.reset} ${chat[2]}`;
  return null;
}

export const minecraft: GameProfile = {
  id: "minecraft",
  label: "Minecraft",
  licenseName: "Minecraft EULA",
  licenseUrl: "https://www.minecraft.net/en-us/eula",
  // ต้อง white-list=true ใน server.properties ถึงจะ enforce จริง
  allowlistEnabledKey: "white-list",
  metricLabel: "TPS",
  metricUnsupportedHint: "Only Paper/Spigot report TPS",
  isValidPlayerName: (name) => USERNAME_RE.test(name),
  variantRequiresLicense: (variant) => !VARIANTS_WITHOUT_LICENSE.has(variant),
  detectFromZip,
  detectFromFolder,
  highlightConsoleMessage,
};
