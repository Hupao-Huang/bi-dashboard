// 数据关联三维图 (仅超管)
// 节点 = 半透明方块(清晰亮边框) + 名字, 始终正对观测者(billboard), 靠透视近大远小区分远近;
// 额外纵深线索: 场景雾(远节点/连线往背景色淡化)。点节点高亮其上下游、淡化其余。
import React, { useRef, useState, useEffect, useCallback, useMemo, Suspense } from 'react';
import { Card, Spin, Typography, Tag, Empty } from 'antd';
import * as THREE from 'three';
import SpriteText from 'three-spritetext';
import {
  DATA_MAP_NODES,
  DATA_MAP_LINKS,
  LAYER_META,
  DataMapLayer,
  DataMapNode,
} from './dataMapModel';

const ForceGraph3D = React.lazy(() => import('react-force-graph-3d'));
const { Title, Text, Paragraph } = Typography;

const NODE_BY_ID: Record<string, DataMapNode> = Object.fromEntries(
  DATA_MAP_NODES.map((n) => [n.id, n]),
);

const BG = '#070b16'; // 深空底

const hexToRgba = (hex: string, a: number) => {
  const n = parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${a})`;
};

const DataMap: React.FC = () => {
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<any>(null);
  const [size, setSize] = useState({ w: 800, h: 600 });
  const [selected, setSelected] = useState<DataMapNode | null>(null);
  const [highlightNodes, setHighlightNodes] = useState<Set<string>>(new Set());
  const [highlightLinks, setHighlightLinks] = useState<Set<string>>(new Set());

  const graphData = useMemo(
    () => ({
      nodes: DATA_MAP_NODES.map((n) => ({ ...n })),
      links: DATA_MAP_LINKS.map((l) => ({ ...l })),
    }),
    [],
  );

  const { degree, upstream, downstream, neighbors } = useMemo(() => {
    const deg: Record<string, number> = {};
    const up: Record<string, string[]> = {};
    const down: Record<string, string[]> = {};
    const nb: Record<string, Set<string>> = {};
    DATA_MAP_LINKS.forEach((l) => {
      deg[l.source] = (deg[l.source] || 0) + 1;
      deg[l.target] = (deg[l.target] || 0) + 1;
      (down[l.source] ||= []).push(l.target);
      (up[l.target] ||= []).push(l.source);
      (nb[l.source] ||= new Set()).add(l.target);
      (nb[l.target] ||= new Set()).add(l.source);
    });
    return { degree: deg, upstream: up, downstream: down, neighbors: nb };
  }, []);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const apply = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    apply();
    const ro = new ResizeObserver(apply);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const linkEndId = (e: any) => (typeof e === 'object' ? e.id : e);
  const linkKey = (l: any) => `${linkEndId(l.source)}->${linkEndId(l.target)}`;

  // 场景雾 = 远近纵深线索 (远的往背景色淡化, 浓度已调淡到远处仍可读)
  const installFog = useCallback(() => {
    const scene = fgRef.current?.scene?.();
    if (scene && !scene.fog) scene.fog = new THREE.FogExp2(BG, 0.0012);
  }, []);

  const handleNodeClick = useCallback(
    (node: any) => {
      const nn = new Set<string>([node.id]);
      (neighbors[node.id] || new Set()).forEach((id) => nn.add(id));
      const ll = new Set<string>();
      graphData.links.forEach((l: any) => {
        const s = linkEndId(l.source);
        const t = linkEndId(l.target);
        if (s === node.id || t === node.id) ll.add(`${s}->${t}`);
      });
      setHighlightNodes(nn);
      setHighlightLinks(ll);
      setSelected(NODE_BY_ID[node.id] || null);
      const { x = 0, y = 0, z = 0 } = node;
      const r = Math.hypot(x, y, z) || 1;
      const k = 1 + 110 / r;
      fgRef.current?.cameraPosition({ x: x * k, y: y * k, z: z * k }, node, 900);
    },
    [neighbors, graphData],
  );

  const clearSel = useCallback(() => {
    setSelected(null);
    setHighlightNodes(new Set());
    setHighlightLinks(new Set());
  }, []);

  // 节点 = 半透明方块(亮色清晰边框) + 名字, SpriteText 自带 billboard + 透视近大远小
  const nodeThreeObject = useCallback(
    (node: any) => {
      const meta = LAYER_META[node.layer as DataMapLayer];
      const dim = highlightNodes.size > 0 && !highlightNodes.has(node.id);
      const s = new SpriteText(node.name);
      s.color = dim ? 'rgba(220,225,235,0.22)' : '#eaf0ff';
      const base = node.layer === 'source' ? 4.6 : node.layer === 'domain' ? 4 : 3.6;
      s.textHeight = base + Math.min(degree[node.id] || 0, 6) * 0.28; // 关联越多越大
      s.fontWeight = '600';
      s.backgroundColor = dim ? 'rgba(40,48,66,0.1)' : hexToRgba(meta.color, 0.18); // 半透明填充
      s.borderColor = dim ? 'rgba(120,130,150,0.3)' : meta.color; // 清晰亮边框(棱)
      s.borderWidth = 1.4;
      s.borderRadius = 1.5;
      s.padding = 3;
      return s;
    },
    [highlightNodes, degree],
  );

  return (
    <Card
      styles={{ body: { padding: 16 } }}
      title={
        <span>
          数据关联三维图{' '}
          <Tag color="purple" style={{ marginLeft: 4 }}>
            仅超管
          </Tag>
        </span>
      }
    >
      <Paragraph type="secondary" style={{ marginTop: -4, marginBottom: 12 }}>
        数据从哪来 → 汇成哪些数据 → 喂给哪些看板的全景架构。鼠标拖动旋转、滚轮缩放、点节点看它的上下游。方块越大代表关联越多；越近越大越清晰、越远越小越淡。
      </Paragraph>

      <div style={{ display: 'flex', gap: 18, marginBottom: 10, flexWrap: 'wrap' }}>
        {(Object.keys(LAYER_META) as DataMapLayer[]).map((k) => (
          <span key={k} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
            <span
              style={{
                width: 12,
                height: 12,
                borderRadius: 2,
                border: `1.5px solid ${LAYER_META[k].color}`,
                background: hexToRgba(LAYER_META[k].color, 0.25),
                display: 'inline-block',
              }}
            />
            <Text>{LAYER_META[k].label}</Text>
          </span>
        ))}
      </div>

      <div style={{ display: 'flex', gap: 12 }}>
        <div
          ref={containerRef}
          style={{
            flex: 1,
            height: 'calc(100vh - 280px)',
            minHeight: 460,
            background: BG,
            borderRadius: 8,
            overflow: 'hidden',
            position: 'relative',
          }}
        >
          <Suspense
            fallback={
              <div style={{ display: 'grid', placeItems: 'center', height: '100%' }}>
                <Spin tip="加载 3D 图…" />
              </div>
            }
          >
            <ForceGraph3D
              ref={fgRef}
              graphData={graphData}
              width={size.w}
              height={size.h}
              backgroundColor={BG}
              nodeThreeObject={nodeThreeObject}
              nodeThreeObjectExtend={false}
              nodeLabel={(n: any) => `${n.name}｜${n.desc}`}
              linkColor={(l: any) =>
                highlightLinks.has(linkKey(l)) ? '#a5b4fc' : 'rgba(140,160,200,0.16)'
              }
              linkWidth={(l: any) => (highlightLinks.has(linkKey(l)) ? 2 : 0.4)}
              linkDirectionalParticles={(l: any) => (highlightLinks.has(linkKey(l)) ? 4 : 2)}
              linkDirectionalParticleWidth={(l: any) => (highlightLinks.has(linkKey(l)) ? 2.4 : 1)}
              linkDirectionalParticleSpeed={0.006}
              onEngineStop={installFog}
              onNodeClick={handleNodeClick}
              onBackgroundClick={clearSel}
              enableNodeDrag={false}
            />
          </Suspense>
        </div>

        <div
          style={{
            width: 268,
            flexShrink: 0,
            border: '1px solid #f0f0f0',
            borderRadius: 8,
            padding: 14,
            height: 'calc(100vh - 280px)',
            minHeight: 460,
            overflow: 'auto',
          }}
        >
          {selected ? (
            <>
              <Tag color={LAYER_META[selected.layer].color}>
                {LAYER_META[selected.layer].label}
              </Tag>
              <Title level={5} style={{ marginTop: 8, marginBottom: 4 }}>
                {selected.name}
              </Title>
              <Paragraph type="secondary" style={{ fontSize: 13 }}>
                {selected.desc}
              </Paragraph>

              <Text strong>⬆ 数据来自</Text>
              <div style={{ margin: '6px 0 12px' }}>
                {(upstream[selected.id] || []).length ? (
                  (upstream[selected.id] || []).map((id) => (
                    <Tag key={id} style={{ marginBottom: 4 }}>
                      {NODE_BY_ID[id]?.name || id}
                    </Tag>
                  ))
                ) : (
                  <Text type="secondary">（最上游, 直接对接外部系统）</Text>
                )}
              </div>

              <Text strong>⬇ 流向</Text>
              <div style={{ marginTop: 6 }}>
                {(downstream[selected.id] || []).length ? (
                  (downstream[selected.id] || []).map((id) => (
                    <Tag key={id} style={{ marginBottom: 4 }}>
                      {NODE_BY_ID[id]?.name || id}
                    </Tag>
                  ))
                ) : (
                  <Text type="secondary">（最末端, 直接面向用户）</Text>
                )}
              </div>
            </>
          ) : (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description="点一个节点看它的上下游"
              style={{ marginTop: 80 }}
            />
          )}
        </div>
      </div>
    </Card>
  );
};

export default DataMap;
