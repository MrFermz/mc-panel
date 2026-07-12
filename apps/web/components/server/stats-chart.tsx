"use client";

import * as React from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { CpuIcon, MemoryStickIcon } from "lucide-react";
import type { ServerStats } from "@/lib/types";
import type { StatPoint } from "@/lib/settings/stats-history";
import { formatCpuPercent, formatMb } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface ChartRow {
  t: number;
  cpu: number;
  memUsed: number;
}

function formatClock(ms: number): string {
  const d = new Date(ms);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function TooltipCard({
  label,
  value,
  time,
}: {
  label: string;
  value: string;
  time: string;
}) {
  return (
    <div className="bg-popover text-popover-foreground rounded-md border px-2.5 py-1.5 text-xs shadow-md">
      <div className="text-muted-foreground">{time}</div>
      <div className="font-mono font-medium">
        {label}: {value}
      </div>
    </div>
  );
}

// recharts ส่ง payload เป็น readonly generic — cast แคบ ๆ ตอนใช้แทนการ type param
type TipShape = {
  active?: boolean;
  payload?: readonly { payload: ChartRow }[];
};

// 3 ระดับตามค่าปัจจุบัน — สีมาจาก CSS token (--chart-*) อ่านได้ทั้ง light/dark
type Level = "low" | "medium" | "high";

const LEVEL_COLOR: Record<Level, string> = {
  low: "var(--chart-low)",
  medium: "var(--chart-medium)",
  high: "var(--chart-high)",
};

function cpuLevel(cpuPercent: number): Level {
  if (cpuPercent >= 80) return "high";
  if (cpuPercent >= 50) return "medium";
  return "low";
}

function memLevel(usedMb: number, limitMb: number): Level {
  if (limitMb <= 0) return "low";
  const ratio = usedMb / limitMb;
  if (ratio >= 0.85) return "high";
  if (ratio >= 0.6) return "medium";
  return "low";
}

function MiniChart({
  title,
  icon,
  valueText,
  color,
  height,
  children,
}: {
  title: string;
  icon: React.ReactNode;
  valueText: string;
  color: string;
  height: number;
  children: React.ReactElement;
}) {
  return (
    <div className="grid gap-1.5">
      <div className="text-muted-foreground flex items-center justify-between text-xs">
        <span className="flex items-center gap-1.5">
          {icon}
          {title}
        </span>
        <span className="font-mono" style={{ color }}>
          {valueText}
        </span>
      </div>
      <div style={{ height }}>
        <ResponsiveContainer width="100%" height="100%">
          {children}
        </ResponsiveContainer>
      </div>
    </div>
  );
}

export function StatsChart({
  stats,
  history,
  height = 88,
  className,
}: {
  stats: ServerStats;
  history: StatPoint[];
  height?: number;
  className?: string;
}) {
  const t = useT();

  const data: ChartRow[] = React.useMemo(
    () =>
      history.map((p) => ({
        t: p.t,
        cpu: p.cpu,
        memUsed: p.memUsed,
      })),
    [history],
  );

  const memLimit = stats.memory_limit_mb;
  const cpuColor = LEVEL_COLOR[cpuLevel(stats.cpu_percent)];
  const memColor = LEVEL_COLOR[memLevel(stats.memory_used_mb, memLimit)];

  // id ไม่ซ้ำต่อ instance เพื่อไม่ให้ gradient ชนกันเมื่อมีหลายกราฟในหน้าเดียว
  const gid = React.useId().replace(/:/g, "");

  const cpuText = formatCpuPercent(stats.cpu_percent);
  const memText = `${formatMb(stats.memory_used_mb)} / ${formatMb(memLimit)}`;

  if (history.length === 0) {
    return (
      <p className={cn("text-muted-foreground text-xs", className)}>
        {t("stats.waiting")}
      </p>
    );
  }

  return (
    <div className={cn("grid gap-4", className)}>
      <MiniChart
        title={t("stats.cpu")}
        icon={<CpuIcon className="size-3.5" />}
        valueText={cpuText}
        color={cpuColor}
        height={height}
      >
        <AreaChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={`cpu-${gid}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={cpuColor} stopOpacity={0.35} />
              <stop offset="100%" stopColor={cpuColor} stopOpacity={0.02} />
            </linearGradient>
          </defs>
          <CartesianGrid
            vertical={false}
            stroke="var(--border)"
            strokeDasharray="2 4"
          />
          <XAxis
            dataKey="t"
            tickFormatter={formatClock}
            tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
            tickLine={false}
            axisLine={false}
            minTickGap={40}
            interval="preserveStartEnd"
          />
          <YAxis
            // CPU multi-core เกิน 100% ได้ — ปล่อยเพดานยืดขึ้นเป็นหลักร้อยที่ปัดสวย (ต่ำสุดคง 100)
            domain={[0, (dataMax: number) => Math.max(100, Math.ceil(dataMax / 100) * 100)]}
            width={52}
            tickFormatter={(v: number) => `${Math.round(v)}%`}
            tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
            tickLine={false}
            axisLine={false}
          />
          <Tooltip
            cursor={{ stroke: "var(--border)" }}
            content={(props) => {
              const { active, payload } = props as TipShape;
              if (!active || !payload?.length) return null;
              const row = payload[0]!.payload;
              return (
                <TooltipCard
                  time={formatClock(row.t)}
                  label={t("stats.cpu")}
                  value={formatCpuPercent(row.cpu)}
                />
              );
            }}
          />
          <Area
            type="monotone"
            dataKey="cpu"
            stroke={cpuColor}
            strokeWidth={1.75}
            fill={`url(#cpu-${gid})`}
            isAnimationActive={false}
            dot={false}
          />
        </AreaChart>
      </MiniChart>

      <MiniChart
        title={t("stats.ram")}
        icon={<MemoryStickIcon className="size-3.5" />}
        valueText={memText}
        color={memColor}
        height={height}
      >
        <AreaChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={`mem-${gid}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={memColor} stopOpacity={0.35} />
              <stop offset="100%" stopColor={memColor} stopOpacity={0.02} />
            </linearGradient>
          </defs>
          <CartesianGrid
            vertical={false}
            stroke="var(--border)"
            strokeDasharray="2 4"
          />
          <XAxis
            dataKey="t"
            tickFormatter={formatClock}
            tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
            tickLine={false}
            axisLine={false}
            minTickGap={40}
            interval="preserveStartEnd"
          />
          <YAxis
            domain={[0, memLimit > 0 ? memLimit : "auto"]}
            // กว้างพอให้ "3.0 GB"/"800 MB" อยู่บรรทัดเดียว ไม่ตกบรรทัด/โดนตัด
            width={52}
            tickFormatter={(v: number) => formatMb(v)}
            tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
            tickLine={false}
            axisLine={false}
          />
          <Tooltip
            cursor={{ stroke: "var(--border)" }}
            content={(props) => {
              const { active, payload } = props as TipShape;
              if (!active || !payload?.length) return null;
              const row = payload[0]!.payload;
              return (
                <TooltipCard
                  time={formatClock(row.t)}
                  label={t("stats.ram")}
                  value={`${formatMb(row.memUsed)} / ${formatMb(memLimit)}`}
                />
              );
            }}
          />
          <Area
            type="monotone"
            dataKey="memUsed"
            stroke={memColor}
            strokeWidth={1.75}
            fill={`url(#mem-${gid})`}
            isAnimationActive={false}
            dot={false}
          />
        </AreaChart>
      </MiniChart>
    </div>
  );
}

export default StatsChart;
