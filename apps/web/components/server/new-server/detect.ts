// ตรวจ type/version จาก archive ฝั่ง client ก่อน upload — เดาให้ user แล้วให้แก้ต่อได้
// (backend ตรวจซ้ำตอน import อยู่แล้ว ที่นี่แค่ช่วยกรอกฟอร์ม พังก็ไม่ block)

export type Detected = {
  name?: string;
  serverType?: string;
  mcVersion?: string;
};

// สร้าง .zip จากโฟลเดอร์ที่ user เลือก โดยตัดชื่อโฟลเดอร์บนสุดออก
// เพื่อให้ไฟล์เซิร์ฟเวอร์ (server.properties, world/ ...) อยู่ที่ root ของ archive
export async function zipFolder(files: File[]): Promise<Blob> {
  const { default: JSZip } = await import("jszip");
  const zip = new JSZip();
  for (const file of files) {
    const rel = file.webkitRelativePath || file.name;
    const slash = rel.indexOf("/");
    const path = slash >= 0 ? rel.slice(slash + 1) : rel;
    if (path === "") continue;
    zip.file(path, file);
  }
  return zip.generateAsync({ type: "blob", compression: "DEFLATE" });
}

// เดา server_type จากชื่อ jar
function guessServerType(jarName: string): string {
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

// server jar เป็น zip — root มี version.json ที่ระบุ mc version จริง (id/name)
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

// jar ที่อยู่ root ของ archive (ไม่มี "/" ในชื่อ) — prefer ชื่อที่บอก type ชัด
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

// โหมด zip: อ่านตรงจากไฟล์ .zip ที่เลือก (ไม่ต้อง extract ทั้งก้อน)
export async function detectFromZip(file: File): Promise<Detected> {
  const detected: Detected = { name: file.name.replace(/\.zip$/i, "") };
  try {
    const { default: JSZip } = await import("jszip");
    const outer = await JSZip.loadAsync(file);
    const jarPath = pickRootJar(Object.keys(outer.files));
    if (jarPath) {
      detected.serverType = guessServerType(jarPath);
      const entry = outer.file(jarPath);
      const bytes = entry ? await entry.async("uint8array") : undefined;
      detected.mcVersion =
        (bytes ? await versionFromJarBytes(bytes) : undefined) ??
        versionFromName(jarPath);
    }
  } catch {
    // ล้มเหลว = ปล่อยให้กรอกเอง ไม่ block
  }
  return detected;
}

// โหมด folder: หา jar ที่ root ตรงจาก File[] แล้วอ่าน version.json ข้างใน
export async function detectFromFolder(
  files: File[],
  folderName: string,
): Promise<Detected> {
  const detected: Detected = { name: folderName || undefined };
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
      detected.serverType = guessServerType(rootRelPath(jar));
      const bytes = new Uint8Array(await jar.arrayBuffer());
      detected.mcVersion =
        (await versionFromJarBytes(bytes)) ??
        versionFromName(rootRelPath(jar));
    }
  } catch {
    // ล้มเหลว = ปล่อยให้กรอกเอง ไม่ block
  }
  return detected;
}
