export function formatMb(mb: number): string {
  if (mb >= 1024) {
    const gb = mb / 1024;
    return `${gb >= 100 ? Math.round(gb) : gb.toFixed(1)} GB`;
  }
  return `${Math.round(mb)} MB`;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let val = bytes / 1024;
  let i = 0;
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024;
    i += 1;
  }
  return `${val >= 100 ? Math.round(val) : val.toFixed(1)} ${units[i]}`;
}

export function formatBps(bytesPerSec: number): string {
  return `${formatBytes(bytesPerSec)}/s`;
}

export function formatCpuPercent(percent: number): string {
  // < 10% โชว์ทศนิยม 1 ตำแหน่งให้พอเห็นความเปลี่ยนแปลง, มากกว่านั้นปัดเต็ม
  return `${percent < 10 ? percent.toFixed(1) : Math.round(percent)}%`;
}

export function formatDateTime(iso: string | null): string {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

export function formatRelative(iso: string | null): string {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const diffSec = Math.round((d.getTime() - Date.now()) / 1000);
  const abs = Math.abs(diffSec);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (abs < 60) return rtf.format(diffSec, "second");
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), "hour");
  return rtf.format(Math.round(diffSec / 86400), "day");
}
