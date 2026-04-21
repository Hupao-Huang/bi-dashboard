import React, { useState, useMemo } from 'react';
import { Input, Tag, Card, Modal, Button } from 'antd';
import {
  SearchOutlined,
  UserOutlined,
  CheckCircleOutlined,
} from '@ant-design/icons';

const { Search } = Input;

interface McpItem {
  name: string;
  mcpId: number;
  desc: string;
  category: string;
  free: boolean;
  type: string;
  color: string;
}

const categories = ['全部', '办公协同', '文档处理', 'AI模型', '地图服务', '项目管理'];

const mcpItems: McpItem[] = [
  { name: '机器人消息', mcpId: 9595, desc: '钉钉机器人消息MCP服务，支持创建企业机器人、搜索群会话、发送群消息和单聊消息、取消发送等能力。', category: '办公协同', free: true, type: 'Remote', color: '#3b82f6' },
  { name: '钉钉日志', mcpId: 9639, desc: '包含获取日志模板、读取日志内容、写日志等功能。', category: '办公协同', free: true, type: 'Remote', color: '#0ea5e9' },
  { name: '钉钉 AI 表格', mcpId: 9555, desc: '让AI直接操作表格数据与字段，快速打通查询、维护与自动化办公流程。', category: '文档处理', free: true, type: 'Remote', color: '#10b981' },
  { name: '钉钉文档', mcpId: 9629, desc: '支持查找、创建文档，助力高效协同与内容管理。', category: '文档处理', free: true, type: 'Remote', color: '#7c3aed' },
  { name: '钉钉通讯录', mcpId: 2400, desc: '支持搜索人员/部门、查询成员详情及部门结构，快速获取组织架构信息。', category: '办公协同', free: true, type: 'Remote', color: '#f59e0b' },
  { name: '钉钉日历', mcpId: 1050, desc: '支持创建日程，查询日程，约空闲会议室等能力。', category: '办公协同', free: true, type: 'Remote', color: '#ef4444' },
  { name: '钉钉待办', mcpId: 2034, desc: '提供高效的任务管理能力，支持创建待办事项、更新任务状态、按条件查询待办列表。', category: '办公协同', free: true, type: 'Remote', color: '#be123c' },
  { name: '钉钉表格', mcpId: 9704, desc: '支持新建、编辑等操作，助力高效协同与内容管理。', category: '文档处理', free: true, type: 'Remote', color: '#14b8a6' },
  { name: '高德地图', mcpId: 1031, desc: '包含搜索周边服务、骑行、公交、驾车、步行路径规划，地理编码查询和天气查询功能。', category: '地图服务', free: false, type: 'Remote', color: '#22c55e' },
  { name: '钉钉群聊', mcpId: 2396, desc: '支持通过自然语言快速创建内部群，高效启动团队协作。', category: '办公协同', free: true, type: 'Remote', color: '#6366f1' },
  { name: '通义万相-文生图', mcpId: 1064, desc: '使用wan2.2-t2i-flash大模型，根据自然语言生成图片，推荐写实场景和摄影风格。', category: 'AI模型', free: false, type: 'Remote', color: '#d946ef' },
  { name: '通义万相-文生视频', mcpId: 1068, desc: '使用wan2.2-t2v-plus模型，根据输入的文本生成视频。', category: 'AI模型', free: false, type: 'Remote', color: '#a855f7' },
  { name: '通义千问-图片理解', mcpId: 1078, desc: '调用qwen3-vl-plus大模型，根据文字描述和图片链接对图片进行解读。', category: 'AI模型', free: false, type: 'Remote', color: '#f43f5e' },
  { name: '通义万相-图生视频', mcpId: 1071, desc: '调用wan2.2-i2v-flash模型，根据图片链接作为首帧生成视频。', category: 'AI模型', free: false, type: 'Remote', color: '#0891b2' },
  { name: '通义万相-图生图', mcpId: 2416, desc: '使用wan2.5-i2i-preview大模型，支持单图编辑和多图融合，单次最多3张图片。', category: 'AI模型', free: false, type: 'Remote', color: '#059669' },
  { name: '通义千问-OCR文字提取', mcpId: 2458, desc: '使用qwen-vl-ocr-latest大模型，按照文字描述要求提取图片中的文字。', category: 'AI模型', free: false, type: 'Remote', color: '#e11d48' },
  { name: '通义千问-大模型翻译', mcpId: 2419, desc: '使用qwen-mt-turbo大模型，按指定语种翻译文本。', category: 'AI模型', free: false, type: 'Remote', color: '#7c3aed' },
  { name: '创意海报生成', mcpId: 9436, desc: '调用qwen-image-max大模型，根据自然语言生成高质量海报。', category: 'AI模型', free: true, type: 'Remote', color: '#eab308' },
  { name: '钉钉Teambition 项目管理', mcpId: 2013, desc: 'Teambition MCP助您高效管理项目与任务，支持创建/更新任务、设置截止日期、分配执行人。', category: '项目管理', free: true, type: 'Remote', color: '#1e293b' },
  { name: '通义千问-视频理解', mcpId: 1079, desc: '调用qwen3-vl-plus大模型，根据文字描述和视频进行解读。', category: 'AI模型', free: false, type: 'Remote', color: '#ea580c' },
  { name: '海螺-文生视频', mcpId: 1074, desc: '海螺文生视频能力，根据文字描述生成视频。', category: 'AI模型', free: false, type: 'Remote', color: '#06b6d4' },
  { name: '百度地图路径规划', mcpId: 2486, desc: '支持百度地图的公交、步行、驾车等路径规划和路程计算。', category: '地图服务', free: false, type: 'Remote', color: '#f59e0b' },
  { name: '专利信息查询', mcpId: 9355, desc: '根据专利号或关键词获取专利信息，提供AI驱动的摘要和战略分析。', category: '办公协同', free: true, type: 'Remote', color: '#3b82f6' },
];

const McpPage: React.FC = () => {
  const [activeCategory, setActiveCategory] = useState('全部');
  const [searchText, setSearchText] = useState('');
  const [selectedItem, setSelectedItem] = useState<McpItem | null>(null);

  const filtered = useMemo(() => {
    return mcpItems.filter(item => {
      const matchCategory = activeCategory === '全部' || item.category === activeCategory;
      const matchSearch = !searchText || item.name.toLowerCase().includes(searchText.toLowerCase()) || item.desc.includes(searchText);
      return matchCategory && matchSearch;
    });
  }, [activeCategory, searchText]);

  return (
    <div style={{ padding: '0 8px' }}>
      {/* 头部介绍 */}
      <div style={{
        background: 'linear-gradient(135deg, #10b981 0%, #0ea5e9 100%)',
        borderRadius: 12,
        padding: '32px 40px',
        marginBottom: 24,
        color: '#fff',
      }}>
        <h2 style={{ color: '#fff', fontSize: 28, margin: 0, fontWeight: 700 }}>
          MCP 能力广场
        </h2>
        <p style={{ color: 'rgba(255,255,255,0.85)', fontSize: 15, margin: '8px 0 20px' }}>
          钉钉官方23个MCP服务，从单点技能到全套场景插件，为悟空自由配置最强装备
        </p>
        <Search
          placeholder="搜索MCP服务名称或描述..."
          allowClear
          size="large"
          prefix={<SearchOutlined />}
          onChange={e => setSearchText(e.target.value)}
          style={{ maxWidth: 480 }}
        />
      </div>

      {/* 分类标签 */}
      <div style={{ marginBottom: 20, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {categories.map(cat => (
          <Tag
            key={cat}
            onClick={() => setActiveCategory(cat)}
            style={{
              cursor: 'pointer',
              padding: '6px 16px',
              fontSize: 14,
              borderRadius: 20,
              border: activeCategory === cat ? '1px solid #10b981' : '1px solid #e2e8f0',
              background: activeCategory === cat ? '#10b981' : '#fff',
              color: activeCategory === cat ? '#fff' : '#64748b',
              transition: 'all 0.2s',
            }}
          >
            {cat}
          </Tag>
        ))}
      </div>

      {/* MCP 卡片网格 */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
        gap: 16,
      }}>
        {filtered.map(item => (
          <Card
            key={item.name}
            hoverable
            onClick={() => setSelectedItem(item)}
            style={{ borderRadius: 10, border: '1px solid #f0f0f0' }}
            styles={{ body: { padding: 20 } }}
          >
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
              <div style={{
                width: 44, height: 44, borderRadius: '50%',
                background: item.color, color: '#fff',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 20, fontWeight: 700, flexShrink: 0,
              }}>
                {item.name[0]}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                  <span style={{ fontWeight: 600, fontSize: 15, color: '#1e293b' }}>{item.name}</span>
                </div>
                <p style={{
                  color: '#64748b', fontSize: 13, margin: '4px 0 12px',
                  lineHeight: 1.5,
                  overflow: 'hidden', textOverflow: 'ellipsis',
                  display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' as const,
                }}>
                  {item.desc}
                </p>
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  <Tag
                    color={item.free ? 'success' : 'warning'}
                    style={{ fontSize: 11, margin: 0, borderRadius: 4 }}
                  >
                    {item.free ? '免费' : '付费'}
                  </Tag>
                  <Tag style={{ fontSize: 11, margin: 0, borderRadius: 4 }} color="processing">
                    {item.type}
                  </Tag>
                </div>
              </div>
            </div>
          </Card>
        ))}
      </div>

      {filtered.length === 0 && (
        <div style={{ textAlign: 'center', padding: 60, color: '#94a3b8' }}>
          没有找到匹配的MCP服务
        </div>
      )}

      {/* 详情弹窗 */}
      <Modal
        open={!!selectedItem}
        onCancel={() => setSelectedItem(null)}
        footer={null}
        width={600}
        centered
        styles={{ body: { padding: '24px 28px', maxHeight: '80vh', overflowY: 'auto' } }}
      >
        {selectedItem && (
          <div>
            {/* 头部：头像 + 名称 */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 14, marginBottom: 20 }}>
              <div style={{
                width: 52, height: 52, borderRadius: '50%',
                background: selectedItem.color, color: '#fff',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 24, fontWeight: 700, flexShrink: 0,
              }}>
                {selectedItem.name[0]}
              </div>
              <div>
                <h3 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: '#1e293b' }}>{selectedItem.name}</h3>
                <div style={{ color: '#94a3b8', fontSize: 13, marginTop: 2 }}>{selectedItem.category}</div>
              </div>
            </div>

            {/* 标签 */}
            <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
              <Tag
                icon={<CheckCircleOutlined />}
                color={selectedItem.free ? 'success' : 'warning'}
                style={{ borderRadius: 12, fontSize: 13 }}
              >
                {selectedItem.free ? '免费' : '付费'}
              </Tag>
              <Tag color="processing" style={{ borderRadius: 12, fontSize: 13 }}>
                {selectedItem.type}
              </Tag>
            </div>

            {/* 描述 */}
            <p style={{ color: '#475569', fontSize: 14, lineHeight: 1.8, marginBottom: 20 }}>
              {selectedItem.desc}
            </p>

            {/* 来源信息 */}
            <div style={{
              background: '#f8fafc', borderRadius: 8, padding: '16px 20px',
              border: '1px solid #e2e8f0', marginBottom: 24,
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <UserOutlined style={{ color: '#10b981' }} />
                <span style={{ fontSize: 14, fontWeight: 500, color: '#1e293b' }}>钉钉官方出品</span>
              </div>
              <div style={{ color: '#64748b', fontSize: 13 }}>
                作者：钉钉（中国）信息技术有限公司
              </div>
            </div>

            {/* 三个数据卡片 */}
            <div style={{ display: 'flex', gap: 12, marginBottom: 24 }}>
              <div style={{
                flex: 1, textAlign: 'center', padding: '16px 12px',
                borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff',
              }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>类型</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>MCP 服务</div>
              </div>
              <div style={{
                flex: 1, textAlign: 'center', padding: '16px 12px',
                borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff',
              }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>分类</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>{selectedItem.category}</div>
              </div>
              <div style={{
                flex: 1, textAlign: 'center', padding: '16px 12px',
                borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff',
              }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>费用</div>
                <div style={{ fontSize: 13, color: selectedItem.free ? '#10b981' : '#f59e0b', marginTop: 4 }}>
                  {selectedItem.free ? '免费使用' : '按量付费'}
                </div>
              </div>
            </div>

            {/* 使用按钮 */}
            <Button
              type="primary"
              size="large"
              block
              href={`https://mcp.dingtalk.com/#/detail?mcpId=${selectedItem.mcpId}&detailType=marketMcpDetail`}
              target="_blank"
              style={{
                height: 48, fontSize: 16, fontWeight: 600,
                background: 'linear-gradient(135deg, #10b981 0%, #0ea5e9 100%)',
                border: 'none', borderRadius: 10,
              }}
            >
              在钉钉中使用
            </Button>

            <p style={{ textAlign: 'center', color: '#94a3b8', fontSize: 13, marginTop: 16, marginBottom: 0 }}>
              此MCP服务由钉钉官方提供，可在钉钉中直接使用。
            </p>
          </div>
        )}
      </Modal>
    </div>
  );
};

export default McpPage;
