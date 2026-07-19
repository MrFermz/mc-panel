"use client";

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "@/lib/api";
import { jobsResponseSchema, type Job } from "@/lib/types";
import { formatDateTime } from "@/lib/format";
import { useT, type TranslationKey } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

const jobStatusClasses: Record<Job["status"], string> = {
  pending: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
  running: "bg-yellow-500/15 text-yellow-400 border-yellow-500/30",
  succeeded: "bg-green-500/15 text-green-400 border-green-500/30",
  failed: "bg-red-500/15 text-red-400 border-red-500/30",
};

function JobStatusBadge({ status }: { status: Job["status"] }) {
  const t = useT();
  return (
    <Badge variant="outline" className={cn(jobStatusClasses[status])}>
      {t(`jobStatus.${status}` as TranslationKey)}
    </Badge>
  );
}

// มือถือ: แสดงแต่ละ job เป็นการ์ด key/value แทน table เพราะ 6 คอลัมน์ล้นที่ 375px
function JobCard({ job }: { job: Job }) {
  const t = useT();
  return (
    <div className="grid gap-2 rounded-md border p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">
          {t(`jobType.${job.type}` as TranslationKey)}
        </span>
        <JobStatusBadge status={job.status} />
      </div>
      <dl className="grid gap-1 text-xs">
        <div className="flex justify-between gap-2">
          <dt className="text-muted-foreground shrink-0">
            {t("jobs.requestedBy")}
          </dt>
          <dd className="truncate text-right" title={job.requested_by_email ?? undefined}>
            {job.requested_by_email ?? (
              <span className="text-muted-foreground">{t("jobs.system")}</span>
            )}
          </dd>
        </div>
        {job.error && (
          <div className="flex justify-between gap-2">
            <dt className="text-muted-foreground shrink-0">{t("jobs.error")}</dt>
            <dd
              className="text-destructive truncate text-right"
              title={job.error}
            >
              {job.error}
            </dd>
          </div>
        )}
        <div className="flex justify-between gap-2">
          <dt className="text-muted-foreground shrink-0">{t("jobs.created")}</dt>
          <dd className="text-muted-foreground text-right">
            {formatDateTime(job.created_at)}
          </dd>
        </div>
        <div className="flex justify-between gap-2">
          <dt className="text-muted-foreground shrink-0">
            {t("jobs.completed")}
          </dt>
          <dd className="text-muted-foreground text-right">
            {formatDateTime(job.completed_at)}
          </dd>
        </div>
      </dl>
    </div>
  );
}

export default function ServerJobs({ serverId }: { serverId: string }) {
  const t = useT();
  // ไม่ poll แล้ว — control-plane ส่ง event server_jobs ผ่าน WS แล้ว invalidate query นี้ (useEvents)
  const jobs = useQuery({
    queryKey: ["servers", serverId, "jobs"],
    queryFn: () =>
      apiGet(`/api/servers/${serverId}/jobs?limit=20`, jobsResponseSchema),
  });

  if (jobs.isPending) return <Skeleton className="h-32 w-full" />;
  if (jobs.isError)
    return <p className="text-destructive text-sm">{t("jobs.failedLoad")}</p>;
  if (jobs.data.jobs.length === 0)
    return <p className="text-muted-foreground text-sm">{t("jobs.none")}</p>;

  return (
    <>
      {/* มือถือ: การ์ดซ้อน */}
      <div className="grid gap-2 md:hidden">
        {jobs.data.jobs.map((job) => (
          <JobCard key={job.id} job={job} />
        ))}
      </div>

      {/* จอใหญ่: ตารางเดิม */}
      <div className="hidden md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("jobs.type")}</TableHead>
              <TableHead>{t("jobs.status")}</TableHead>
              <TableHead>{t("jobs.requestedBy")}</TableHead>
              <TableHead>{t("jobs.error")}</TableHead>
              <TableHead>{t("jobs.created")}</TableHead>
              <TableHead>{t("jobs.completed")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {jobs.data.jobs.map((job) => (
              <TableRow key={job.id}>
                <TableCell className="text-xs">
                  {t(`jobType.${job.type}` as TranslationKey)}
                </TableCell>
                <TableCell>
                  <JobStatusBadge status={job.status} />
                </TableCell>
                <TableCell
                  className="max-w-48 truncate text-xs"
                  title={job.requested_by_email ?? undefined}
                >
                  {job.requested_by_email ?? (
                    <span className="text-muted-foreground">
                      {t("jobs.system")}
                    </span>
                  )}
                </TableCell>
                <TableCell
                  className="text-destructive max-w-64 truncate"
                  title={job.error || undefined}
                >
                  {job.error || "-"}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs">
                  {formatDateTime(job.created_at)}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs">
                  {formatDateTime(job.completed_at)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  );
}
