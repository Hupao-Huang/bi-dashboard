// 数据关联三维图 (仅超管) — 可旋转的 3D 数据架构网络
// 外部数据源 → 数据域 → 看板模块 三层, 点节点看它的上下游。
import React, { useRef, useState, useEffect, useCallback, useMemo, Suspense } from 'react';
import { Card, Spin, Typography, Tag, Empty } from 'antd';
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

const DataMap: React.FC = () => {
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<any>(null);
  const [size, setSize] = useState({ w: 800, h: 600 });
  const [selected, setSelected] = useState<DataMapNode | null>(null);
  const [highlightLinks, setHighlightLinks] = useState<Set<any>>(new Set());

  // 克隆一份给图库内部改坐标, 不动原模型
  const graphData = useMemo(
    () => ({
      nodes: DATA_MAP_NODES.map((n) => ({ ...n })),
      links: DATA_MAP_LINKS.map((l) => ({ ...l })),
    }),
    [],
  );

  // 有向邻接: 上游(数据来自) / 下游(流向)
  const { upstream, downstream } = useMemo(() => {
    const up: Record<string, string[]> = {};
    const down: Record<string, string[]> = {};
    DATA_MAP_LINKS.forEach((l) => {
      (down[l.source] ||= []).push(l.target);
      (up[l.target] ||= []).push(l.source);
    });
    return { upstream: up, downstream: down };
  }, []);

  // 容器尺寸自适应
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const apply = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    apply();
    const ro = new ResizeObserver(apply);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const linkId = (l: any) => {
    const s = typeof l.source === 'object' ? l.source.id : l.source;
    const t = typeof l.target === 'object' ? l.target.id : l.target;
    return `${s}->${t}`;
  };

  const handleNodeClick = useCallback(
    (node: any) => {
      const ll = new Set<any>();
      graphData.links.forEach((l: any) => {
        const s = typeof l.source === 'object' ? l.source.id : l.source;
        const t = typeof l.target === 'object' ? l.target.id : l.target;
        if (s === node.id || t === node.id) ll.add(linkId(l));
      });
      setHighlightLinks(ll);
      setSelected(NODE_BY_ID[node.id] || null);
      // 镜头飞向该节点
      const { x = 0, y = 0, z = 0 } = node;
      const r = Math.hypot(x, y, z) || 1;
      const k = 1 + 90 / r;
      fgRef.current?.cameraPosition({ x: x * k, y: y * k, z: z * k }, node, 900);
    },
    [graphData],
  );

  const clearSel = useCallback(() => {
    setSelected(null);
    setHighlightLinks(new Set());
  }, []);

  // 节点 = 彩色文字标签 (始终显示, 一眼看懂)
  const nodeThreeObject = useCallback((node: any) => {
    const meta = LAYER_META[node.layer as DataMapLayer];
    const sprite = new SpriteText(node.name);
    sprite.color = meta.color;
    sprite.textHeight =
      node.layer === 'source' ? 6 : node.layer === 'domain' ? 5 : 4.5;
    sprite.fontWeight = '600';
    sprite.backgroundColor = 'rgba(10,16,33,0.72)';
    sprite.padding = 2;
    sprite.borderRadius = 3;
    return sprite;
  }, []);

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
        数据从哪来 → 汇成哪些数据 → 喂给哪些看板的全景架构。鼠标拖动旋转、滚轮缩放、点节点看它的上下游。
      </Paragraph>

      {/* 图例 */}
      <div style={{ display: 'flex', gap: 18, marginBottom: 10, flexWrap: 'wrap' }}>
        {(Object.keys(LAYER_META) as DataMapLayer[]).map((k) => (
          <span key={k} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
            <span
              style={{
                width: 12,
                height: 12,
                borderRadius: '50%',
                background: LAYER_META[k].color,
                display: 'inline-block',
              }}
            />
            <Text>{LAYER_META[k].label}</Text>
          </span>
        ))}
      </div>

      <div style={{ display: 'flex', gap: 12 }}>
        {/* 3D 图 */}
        <div
          ref={containerRef}
          style={{
            flex: 1,
            height: 'calc(100vh - 280px)',
            minHeight: 460,
            background: '#0b1021',
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
              backgroundColor="#0b1021"
              nodeThreeObject={nodeThreeObject}
              nodeLabel={(n: any) => `${n.name}｜${n.desc}`}
              linkColor={(l: any) =>
                highlightLinks.has(linkId(l)) ? '#ffffff' : 'rgba(180,200,255,0.22)'
              }
              linkWidth={(l: any) => (highlightLinks.has(linkId(l)) ? 2.5 : 0.6)}
              linkOpacity={0.5}
              linkDirectionalArrowLength={3.5}
              linkDirectionalArrowRelPos={1}
              linkDirectionalParticles={(l: any) => (highlightLinks.has(linkId(l)) ? 3 : 0)}
              linkDirectionalParticleWidth={2.5}
              onNodeClick={handleNodeClick}
              onBackgroundClick={clearSel}
              enableNodeDrag={false}
            />
          </Suspense>
        </div>

        {/* 右侧节点详情 */}
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
