"use client";

import * as React from "react";
import { ActivityIcon } from "lucide-react";
import type { Node } from "@/lib/types";
import { useT } from "@/lib/i18n";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { NodeStatsChart } from "@/components/node/node-stats-chart";
import { cn } from "@/lib/utils";

// mirror ServerStatsAccordion (icon divider + same collapse behavior) เพื่อให้การ์ด node
// หน้าตาเหมือนการ์ด instance เป๊ะ ต่างแค่ content เป็นกราฟของ node
export function NodeStatsAccordion({
  node,
  className,
}: {
  node: Node;
  className?: string;
}) {
  const t = useT();
  const online = node.status === "online";

  return (
    <Accordion type="single" collapsible className={cn("border-t", className)}>
      <AccordionItem value="resources" className="border-b-0">
        <AccordionTrigger className="text-muted-foreground hover:text-foreground px-0">
          <span className="flex items-center gap-1.5">
            <ActivityIcon className="size-3.5" />
            {t("stats.resources")}
          </span>
        </AccordionTrigger>
        <AccordionContent>
          {online ? (
            <NodeStatsChart node={node} />
          ) : (
            <p className="text-muted-foreground text-xs">
              {t("nodes.offlineNoStats")}
            </p>
          )}
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  );
}
