"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { apiGet, getNextPort } from "@/lib/api";
import {
  metaNodesResponseSchema,
  metaServerTypesResponseSchema,
  nodesResponseSchema,
  serversResponseSchema,
  versionsResponseSchema,
  type MetaNode,
  type MetaServerType,
} from "@/lib/types";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useMe } from "@/lib/use-me";

// งบ RAM ของโหนดที่เลือก — advisory ล้วน (backend เป็นคนปฏิเสธจริงด้วย insufficient_memory)
export interface RamBudget {
  usedMb: number;
  totalMb: number;
  freeMb: number;
  over: boolean;
}

export interface ServerMetadata {
  name: string;
  nodeId: string;
  serverType: string;
  mcVersion: string;
  memoryMb: string;
  hostPort: string;
  acceptEula: boolean;

  setName: (v: string) => void;
  setNodeId: (v: string) => void;
  setServerType: (v: string) => void;
  setMcVersion: (v: string) => void;
  setMemoryMb: (v: string) => void;
  setHostPort: (v: string) => void;
  setAcceptEula: (v: boolean) => void;

  needsEula: boolean;
  valid: boolean;

  nodes: MetaNode[];
  nodesPending: boolean;
  types: MetaServerType[];
  typesPending: boolean;
  versions: string[];
  versionsPending: boolean;
  versionsError: boolean;
  // null = ยังไม่ได้เลือกโหนด หรือไม่มีข้อมูลโหนดตัวเต็ม (ไม่มีสิทธิ์ /api/nodes)
  budget: RamBudget | null;
}

// state + query ของฟอร์ม metadata (name/node/type/version/memory/port/eula)
// เรียกที่ตัว wizard เพื่อให้ค่าที่กรอกอยู่รอดตอนเดินหน้า/ถอยหลัง step
// คืนเฉพาะ "ข้อมูล" — การ render อยู่ที่ step-general.tsx
export function useServerMetadata(): ServerMetadata {
  const me = useMe().data?.user;
  const [name, setName] = React.useState("");
  const [nodeId, setNodeId] = React.useState("");
  const [serverType, setServerType] = React.useState("");
  const [mcVersion, setMcVersion] = React.useState("");
  const [memoryMb, setMemoryMb] = React.useState("2048");
  const [hostPort, setHostPort] = React.useState("");
  // จำว่า user แตะช่อง port เองหรือยัง — ถ้าแตะแล้วห้าม auto-prefill ทับ
  const [portEdited, setPortEdited] = React.useState(false);
  const [acceptEula, setAcceptEula] = React.useState(false);

  const nodesQuery = useQuery({
    queryKey: ["meta", "nodes"],
    queryFn: () => apiGet("/api/meta/nodes", metaNodesResponseSchema),
  });
  const nodes = React.useMemo(
    () => nodesQuery.data?.nodes ?? [],
    [nodesQuery.data],
  );

  // มี node เดียว = ไม่มีอะไรให้เลือก เลือกให้เลย (หลายตัวปล่อยว่างเพื่อบังคับให้ตัดสินใจ)
  React.useEffect(() => {
    const only = nodes.length === 1 ? nodes[0] : undefined;
    if (nodeId === "" && only) setNodeId(only.id);
  }, [nodeId, nodes]);

  // แนะนำ host port ว่างของ node ที่เลือก — พังก็ปล่อยช่องว่างไว้เฉย ๆ (ไม่ crash)
  const nextPortQuery = useQuery({
    queryKey: ["meta", "next-port", nodeId],
    queryFn: () => getNextPort(nodeId),
    enabled: nodeId !== "",
    retry: false,
  });
  const suggestedPort = nextPortQuery.data;
  React.useEffect(() => {
    if (!portEdited && suggestedPort !== undefined) {
      setHostPort(String(suggestedPort));
    }
  }, [portEdited, suggestedPort]);

  const onHostPortChange = React.useCallback((v: string) => {
    setPortEdited(true);
    setHostPort(v);
  }, []);

  // เวอร์ชันที่เลือกไว้ผูกกับ type — เปลี่ยน type แล้วค่าเดิมอาจไม่มีในลิสต์ใหม่
  // (บังคับล้างที่นี่ ไม่ฝากไว้กับคนเรียก)
  const onServerTypeChange = React.useCallback((v: string) => {
    setServerType(v);
    setMcVersion("");
  }, []);

  // งบ RAM ต่อโหนด: total ของ node − ผลรวม memory_mb ของ server ที่มีอยู่บนโหนดนั้น
  // ทั้งสอง query แชร์ cache กับ dashboard (["nodes"], ["servers"]) — พังก็แค่ไม่โชว์ hint
  const nodesFullQuery = useQuery({
    queryKey: ["nodes"],
    queryFn: () => apiGet("/api/nodes", nodesResponseSchema),
    retry: false,
  });
  // งบ RAM ที่ backend คิดคือผลรวมของ **ทุก** server บน node (รวมตัวที่อยู่ในถังขยะ) — ใช้
  // scope=all ให้ตรงกัน ถ้าไม่มี servers.view_all ก็ตกไปใช้ list ของตัวเอง (hint จะต่ำกว่าจริง
  // แต่ backend ปฏิเสธด้วย insufficient_memory อยู่ดี — นี่เป็นแค่คำเตือนล่วงหน้า)
  const canViewAllServers = hasCapability(me, CAPABILITY.serversViewAll);
  const serversQuery = useQuery({
    queryKey: canViewAllServers ? ["servers", "all"] : ["servers"],
    queryFn: () =>
      apiGet(
        canViewAllServers ? "/api/servers?scope=all" : "/api/servers",
        serversResponseSchema,
      ),
    retry: false,
  });

  const typesQuery = useQuery({
    queryKey: ["meta", "server-types"],
    queryFn: () =>
      apiGet("/api/meta/server-types", metaServerTypesResponseSchema),
  });
  const versionsQuery = useQuery({
    queryKey: ["meta", "versions", serverType],
    queryFn: () =>
      apiGet(
        `/api/meta/versions?type=${encodeURIComponent(serverType)}`,
        versionsResponseSchema,
      ),
    enabled: serverType !== "",
  });

  const selectedType = typesQuery.data?.types.find((x) => x.id === serverType);
  const needsEula = selectedType?.needs_eula ?? serverType !== "velocity";

  const memory = Number(memoryMb);
  const port = hostPort === "" ? null : Number(hostPort);

  const selectedNode = nodesFullQuery.data?.nodes.find((n) => n.id === nodeId);
  const budget = React.useMemo<RamBudget | null>(() => {
    if (!selectedNode) return null;
    const usedMb = (serversQuery.data?.servers ?? [])
      .filter((s) => s.node_id === nodeId)
      .reduce((sum, s) => sum + s.memory_mb, 0);
    const totalMb = selectedNode.memory_total_mb ?? 0;
    const freeMb = Math.max(0, totalMb - usedMb);
    return {
      usedMb,
      totalMb,
      freeMb,
      over: Number.isInteger(memory) && memory > 0 && memory > freeMb,
    };
  }, [selectedNode, serversQuery.data, nodeId, memory]);

  const valid =
    name.trim().length > 0 &&
    nodeId !== "" &&
    serverType !== "" &&
    mcVersion !== "" &&
    Number.isInteger(memory) &&
    memory >= 512 &&
    (port === null ||
      (Number.isInteger(port) && port >= 1024 && port <= 65535)) &&
    (!needsEula || acceptEula);

  return {
    name,
    nodeId,
    serverType,
    mcVersion,
    memoryMb,
    hostPort,
    acceptEula,
    setName,
    setNodeId,
    setServerType: onServerTypeChange,
    setMcVersion,
    setMemoryMb,
    setHostPort: onHostPortChange,
    setAcceptEula,
    needsEula,
    valid,
    nodes,
    nodesPending: nodesQuery.isPending,
    types: typesQuery.data?.types ?? [],
    typesPending: typesQuery.isPending,
    versions: versionsQuery.data?.versions ?? [],
    versionsPending: versionsQuery.isPending,
    versionsError: versionsQuery.isError,
    budget,
  };
}
