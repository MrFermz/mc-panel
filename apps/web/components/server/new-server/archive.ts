// สร้าง .zip จากโฟลเดอร์ที่ user เลือก โดยตัดชื่อโฟลเดอร์บนสุดออก เพื่อให้ไฟล์ของ server
// อยู่ที่ root ของ archive — ไม่รู้จักเกมใด ๆ (การเดา variant/version เป็นของ game profile)
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
