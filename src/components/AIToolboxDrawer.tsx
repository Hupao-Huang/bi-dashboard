import React, { useState, useMemo } from 'react';
import { Drawer, Tabs, Input, Tag, Card, Modal, Button } from 'antd';
import {
  SearchOutlined,
  UserOutlined,
  CheckCircleOutlined,
  RocketOutlined,
} from '@ant-design/icons';

const { Search } = Input;

/* ---------- Skills data ---------- */

interface Skill {
  name: string;
  skillId: string;
  desc: string;
  category: string;
  tags: string[];
  color: string;
}

const skillCategories = ['全部', '办公协同', '文档处理', '信息搜索', '营销推广', 'AI创作', '教育', '效率工具', '会议'];

const skills: Skill[] = [
  { name: '会议自动驾驶', skillId: 'wXBeEZcwzToe', desc: '基于钉钉日历和会议口令，自动完成会议前后流程，包括议程提醒、纪要生成和待办创建。', category: '办公协同', tags: ['办公协同', 'A1', '待办', '日程管理'], color: '#3b82f6' },
  { name: '抖音短视频爆款文案', skillId: '9O3lDFGPbFzU', desc: '本技能运行时会访问先进数据库和创意海报大模型的一站式数据驱动MCP服务，帮你自动完成爆款内容创作。', category: '营销推广', tags: ['短视频', '内容创作', '营销推广'], color: '#ef4444' },
  { name: 'AI视觉工坊', skillId: 'Z8cMjQPIhyi7', desc: '基于通义万相多模型+创意海报大模型的一站式图文创作技能。支持文生图、图生图、创意海报生成、参考图搜索。', category: 'AI创作', tags: ['AI创作', '设计'], color: '#7c3aed' },
  { name: '技术方案文档', skillId: 'Tn9604jQigkd', desc: '帮用户撰写完整的技术方案文档，输出到钉钉文档。涵盖需求分析、现有系统调研、技术选型、架构设计、模块设计。', category: '文档处理', tags: ['开发者工具', '文档处理'], color: '#0ea5e9' },
  { name: 'AI 听记-灵感捕手', skillId: 'DdTJuLomevxi', desc: '从你的所有对话中自动识别、组装、培育灵感碎片，发现你自己都没注意到的好想法。支持听记URL触发。', category: '效率工具', tags: ['效率工具'], color: '#f59e0b' },
  { name: '教案助手', skillId: 'enFNXG4WglPh', desc: '生成符合《义务教育课程标准（2022年版）》的专业教案，基于教学评一致性原则设计。', category: '教育', tags: ['教育'], color: '#10b981' },
  { name: '链接速读', skillId: 'CUEVpnSo0LTh', desc: '为智能体提供链接速读能力，当用户传入URL后分析其中内容给出简洁摘要。', category: '效率工具', tags: ['信息搜索', '效率工具'], color: '#6366f1' },
  { name: '企业信用智能尽调', skillId: 'cI1OvpK6R0h6', desc: '本技能运行时会访问先进的数据服务和企业信用数据库，帮你自动完成企业信用调查和风险评估。', category: '信息搜索', tags: ['财务审计', '财务', '信息搜索'], color: '#14b8a6' },
  { name: '智能全网搜索', skillId: 'NuhzMxvGP903', desc: '本技能需要联网搜索，为智能体提供搜索先进数据的能力，帮你搜索最新、最全面的数据信息。', category: '信息搜索', tags: ['信息搜索', '效率工具'], color: '#be123c' },
  { name: '群聊内容摘要', skillId: 'hPBr83ckN0oi', desc: '将钉钉群聊消息提取、分析并生成结构化钉钉云文档摘要，涵盖议题、决策、行动项与风险。', category: '办公协同', tags: ['办公协同', '效率工具'], color: '#ea580c' },
  { name: '审批催办提醒', skillId: 'nlAYVAyw1ye6', desc: '识别用户已发起但仍待审批的流程，定位当前审批人，发送催办提醒。', category: '办公协同', tags: ['办公协同', '效率工具'], color: '#d946ef' },
  { name: 'AI 听记-面试评估助手', skillId: 'ZW8G5OR4NEHn', desc: '自动从钉钉AI听记中深度评估候选人表现，支持简历预分析、面试指南生成、多轮面试串联和候选人对比。', category: '效率工具', tags: ['效率工具'], color: '#0891b2' },
  { name: '互动学习卡片', skillId: 'hXGX16grvUfg', desc: '为教师创建课堂专用的交互式学习应用，包含闪卡、测验和浏览模式。基于记忆科学原理设计。', category: '教育', tags: ['教育'], color: '#059669' },
  { name: '英语学习乐园', skillId: 'V8UJthJr1lpZ', desc: '生成互动式英语学习内容，包括词汇学习卡片、语法知识讲解、情景对话练习、英语知识问答游戏。', category: '教育', tags: ['教育'], color: '#7c3aed' },
  { name: '语文趣学堂', skillId: '6bO54iTINTXs', desc: '通过情境化教学、文化浸润和创意表达创造沉浸式的语文学习体验。支持古诗词、文言文、阅读写作。', category: '教育', tags: ['教育'], color: '#e11d48' },
  { name: '去AI味', skillId: 'rS1RRpVe9baH', desc: '识别并去除AI生成文本中的机械痕迹，让文字更自然、更有人味。支持多种文体风格。', category: '文档处理', tags: ['文案', '博主'], color: '#06b6d4' },
  { name: 'Word文档生成器', skillId: 'GmsB1B1sozk9', desc: '基于模板的智能Word文档生成、编辑格式化工具，基于OpenXML SDK(NET)，支持从零创建或模板化定制文档。', category: '文档处理', tags: ['文档处理', '办公协同'], color: '#22c55e' },
  { name: 'Excel处理工具', skillId: 'PBkJvyk89gpK', desc: 'Excel电子表格全能处理工具。支持含盖、读取、分析、创建、修改xlsx/xlsm/csv文件。', category: '文档处理', tags: ['文档处理', '效率工具'], color: '#1e293b' },
  { name: 'AI行业日报', skillId: 'JI2XEtHiTCPz', desc: 'AI行业日报技能。可自动帮用户产出当天某主题AI行业日报，适用于AI资讯整理和AI行业分析。', category: '信息搜索', tags: ['信息搜索', 'AI创作'], color: '#a855f7' },
  { name: '股票个股分析', skillId: 'tHTBBJ3e6TF7', desc: '本技能运行时会访问先进数据服务和股票数据库，帮你自动完成股票个股分析和投资建议。', category: '信息搜索', tags: ['投资达人', '财务', '信息搜索'], color: '#f43f5e' },
  { name: '行业研究报告', skillId: 'OgAqeTRgxzRk', desc: '行业研究、赛道分析、市场规模、竞争格局、技术趋势、政策环境或投融资动态报告，输出为钉钉文档。', category: '信息搜索', tags: ['信息搜索', '文档处理'], color: '#3b82f6' },
  { name: '产品发布会材料', skillId: 'NFXo2NbOdg12', desc: '围绕产品发布会或产品上市节点，提炼核心卖点与叙事主线，设计发布会流程与环节节奏。', category: '营销推广', tags: ['营销推广', '文档处理'], color: '#eab308' },
  { name: '竞品分析', skillId: 'vhzrPXVHGJJC', desc: '针对目标产品与竞争对手进行系统性竞品研究，输出结构化钉钉在线文档。', category: '信息搜索', tags: ['信息搜索', '营销推广'], color: '#ef4444' },
  { name: '公司调研', skillId: 'S3zhKB006SuL', desc: '基于公开信息和企业工商数据进行全维度公司研究，覆盖公司概况、业务产品、竞争格局、经营财务。', category: '信息搜索', tags: ['信息搜索', '文档处理'], color: '#7c3aed' },
  { name: '商业计划书', skillId: 'Vp0ulf4jfnhF', desc: '覆盖市场机会验证、竞品对比、商业模式画布、财务预测与融资需求，输出结构化商业计划书。', category: '文档处理', tags: ['文档处理', '营销推广'], color: '#0ea5e9' },
  { name: '社媒内容生成', skillId: 'TkWLzrVHiuEp', desc: '社交媒体内容创作，支持单条文案、多平台改写、内容日历排期、互动优化。', category: '营销推广', tags: ['内容创作', '营销推广'], color: '#10b981' },
  { name: '一句话发邮件', skillId: 'BcdzEzN8LpJF', desc: '用一句自然语言发送邮件：自动解析收件人和邮件内容，匹配钉钉通讯录获取邮箱，生成正式邮件。', category: '办公协同', tags: ['办公协同', '效率工具'], color: '#f59e0b' },
  { name: '群聊转待办', skillId: 'qqlkjeRj4GEO', desc: '从钉钉群聊消息中识别与我相关的任务类内容，自动创建钉钉待办。', category: '办公协同', tags: ['办公协同', '效率工具'], color: '#be123c' },
  { name: '审批进度查询', skillId: 'LTfWOdJar7ZK', desc: '审批流程链路回溯：查询审批进度、获取完整审批链路、分析当前节点状态、提供催办建议。', category: '办公协同', tags: ['办公协同', '效率工具'], color: '#14b8a6' },
  { name: '会前材料准备', skillId: 'WLni3fJPr3WB', desc: '围绕会议议题、决策点与材料清单，结合历史会议纪要、群聊讨论，生成结构化会前材料文档。', category: '会议', tags: ['会议', '办公协同', '文档处理'], color: '#6366f1' },
  { name: '项目启动报告', skillId: 'K45xzo2C7dkL', desc: '明确目标与成功标准、范围与不在范围、里程碑与交付节奏、角色分工与RACI、风险与依赖。', category: '文档处理', tags: ['项目管理', '文档处理'], color: '#d946ef' },
  { name: '知识库审计', skillId: '85tLKDSjGUwV', desc: '对钉钉知识库进行系统性诊断审计，支持轻量级审计和深度审计两种模式。', category: '文档处理', tags: ['文档处理', '办公协同'], color: '#ea580c' },
  { name: '论文深度解读', skillId: 'LeisNyOsgAXN', desc: '对学术论文或研究话题进行深度解读，覆盖核心论文方法与定理细节、研究脉络梳理。', category: '教育', tags: ['教育', '信息搜索'], color: '#0891b2' },
  { name: '新闻动态总结', skillId: 'yd5gDteQ5Kig', desc: '根据用户指定的主题和时间范围，多源检索新闻动态，生成结构化钉钉云文档动态报告。', category: '信息搜索', tags: ['信息搜索', '内容创作'], color: '#059669' },
];

/* ---------- MCP data ---------- */

interface McpItem {
  name: string;
  mcpId: number;
  desc: string;
  category: string;
  free: boolean;
  type: string;
  color: string;
}

const mcpCategories = ['全部', '办公协同', '文档处理', 'AI模型', '地图服务', '项目管理'];

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

/* ---------- Sub-components ---------- */

interface SkillsTabProps {
  onSelectSkill: (skill: Skill) => void;
}

const SkillsTab: React.FC<SkillsTabProps> = ({ onSelectSkill }) => {
  const [activeCategory, setActiveCategory] = useState('全部');
  const [searchText, setSearchText] = useState('');

  const filtered = useMemo(() => {
    return skills.filter(s => {
      const matchCategory = activeCategory === '全部' || s.category === activeCategory;
      const matchSearch = !searchText || s.name.toLowerCase().includes(searchText.toLowerCase()) || s.desc.includes(searchText);
      return matchCategory && matchSearch;
    });
  }, [activeCategory, searchText]);

  return (
    <div>
      <Search
        placeholder="搜索技能名称或描述..."
        allowClear
        prefix={<SearchOutlined />}
        onChange={e => setSearchText(e.target.value)}
        style={{ marginBottom: 12 }}
      />
      <div style={{ marginBottom: 12, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {skillCategories.map(cat => (
          <Tag
            key={cat}
            onClick={() => setActiveCategory(cat)}
            style={{
              cursor: 'pointer',
              padding: '4px 12px',
              fontSize: 13,
              borderRadius: 16,
              border: activeCategory === cat ? '1px solid #3b82f6' : '1px solid #e2e8f0',
              background: activeCategory === cat ? '#3b82f6' : '#fff',
              color: activeCategory === cat ? '#fff' : '#64748b',
              transition: 'all 0.2s',
            }}
          >
            {cat}
          </Tag>
        ))}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
        {filtered.map(skill => (
          <Card
            key={skill.name}
            hoverable
            onClick={() => onSelectSkill(skill)}
            size="small"
            style={{ borderRadius: 8, border: '1px solid #f0f0f0' }}
            styles={{ body: { padding: 14 } }}
          >
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10 }}>
              <div style={{
                width: 36, height: 36, borderRadius: '50%',
                background: skill.color, color: '#fff',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 16, fontWeight: 700, flexShrink: 0,
              }}>
                {skill.name[0]}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 600, fontSize: 14, color: '#1e293b', marginBottom: 2 }}>{skill.name}</div>
                <p style={{
                  color: '#64748b', fontSize: 12, margin: '2px 0 8px',
                  lineHeight: 1.5,
                  overflow: 'hidden', textOverflow: 'ellipsis',
                  display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' as const,
                }}>
                  {skill.desc}
                </p>
                <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                  {skill.tags.slice(0, 2).map(tag => (
                    <Tag key={tag} style={{ fontSize: 10, margin: 0, borderRadius: 4 }} color="blue">{tag}</Tag>
                  ))}
                  {skill.tags.length > 2 && (
                    <Tag style={{ fontSize: 10, margin: 0, borderRadius: 4 }} color="default">+{skill.tags.length - 2}</Tag>
                  )}
                </div>
              </div>
            </div>
          </Card>
        ))}
      </div>
      {filtered.length === 0 && (
        <div style={{ textAlign: 'center', padding: 40, color: '#94a3b8' }}>没有找到匹配的技能</div>
      )}
    </div>
  );
};

interface McpTabProps {
  onSelectMcp: (item: McpItem) => void;
}

const McpTab: React.FC<McpTabProps> = ({ onSelectMcp }) => {
  const [activeCategory, setActiveCategory] = useState('全部');
  const [searchText, setSearchText] = useState('');

  const filtered = useMemo(() => {
    return mcpItems.filter(item => {
      const matchCategory = activeCategory === '全部' || item.category === activeCategory;
      const matchSearch = !searchText || item.name.toLowerCase().includes(searchText.toLowerCase()) || item.desc.includes(searchText);
      return matchCategory && matchSearch;
    });
  }, [activeCategory, searchText]);

  return (
    <div>
      <Search
        placeholder="搜索MCP服务名称或描述..."
        allowClear
        prefix={<SearchOutlined />}
        onChange={e => setSearchText(e.target.value)}
        style={{ marginBottom: 12 }}
      />
      <div style={{ marginBottom: 12, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {mcpCategories.map(cat => (
          <Tag
            key={cat}
            onClick={() => setActiveCategory(cat)}
            style={{
              cursor: 'pointer',
              padding: '4px 12px',
              fontSize: 13,
              borderRadius: 16,
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
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
        {filtered.map(item => (
          <Card
            key={item.name}
            hoverable
            onClick={() => onSelectMcp(item)}
            size="small"
            style={{ borderRadius: 8, border: '1px solid #f0f0f0' }}
            styles={{ body: { padding: 14 } }}
          >
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10 }}>
              <div style={{
                width: 36, height: 36, borderRadius: '50%',
                background: item.color, color: '#fff',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 16, fontWeight: 700, flexShrink: 0,
              }}>
                {item.name[0]}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 600, fontSize: 14, color: '#1e293b', marginBottom: 2 }}>{item.name}</div>
                <p style={{
                  color: '#64748b', fontSize: 12, margin: '2px 0 8px',
                  lineHeight: 1.5,
                  overflow: 'hidden', textOverflow: 'ellipsis',
                  display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' as const,
                }}>
                  {item.desc}
                </p>
                <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                  <Tag color={item.free ? 'success' : 'warning'} style={{ fontSize: 10, margin: 0, borderRadius: 4 }}>
                    {item.free ? '免费' : '付费'}
                  </Tag>
                  <Tag style={{ fontSize: 10, margin: 0, borderRadius: 4 }} color="processing">{item.type}</Tag>
                </div>
              </div>
            </div>
          </Card>
        ))}
      </div>
      {filtered.length === 0 && (
        <div style={{ textAlign: 'center', padding: 40, color: '#94a3b8' }}>没有找到匹配的MCP服务</div>
      )}
    </div>
  );
};

/* ---------- Main drawer component ---------- */

interface AIToolboxDrawerProps {
  open: boolean;
  onClose: () => void;
}

const AIToolboxDrawer: React.FC<AIToolboxDrawerProps> = ({ open, onClose }) => {
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
  const [selectedMcp, setSelectedMcp] = useState<McpItem | null>(null);

  return (
    <>
      <Drawer
        title={
          <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <RocketOutlined style={{ color: '#1e40af' }} />
            <span>AI 工具箱</span>
          </span>
        }
        placement="right"
        size={720}
        open={open}
        onClose={onClose}
        styles={{ body: { padding: '12px 20px' } }}
      >
        <Tabs
          defaultActiveKey="skills"
          items={[
            {
              key: 'skills',
              label: `Skills (${skills.length})`,
              children: <SkillsTab onSelectSkill={setSelectedSkill} />,
            },
            {
              key: 'mcp',
              label: `MCP (${mcpItems.length})`,
              children: <McpTab onSelectMcp={setSelectedMcp} />,
            },
          ]}
        />
      </Drawer>

      {/* Skill detail modal */}
      <Modal
        open={!!selectedSkill}
        onCancel={() => setSelectedSkill(null)}
        footer={null}
        width={600}
        centered
        styles={{ body: { padding: '24px 28px', maxHeight: '80vh', overflowY: 'auto' } }}
      >
        {selectedSkill && (
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 14, marginBottom: 20 }}>
              <div style={{
                width: 52, height: 52, borderRadius: '50%',
                background: selectedSkill.color, color: '#fff',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 24, fontWeight: 700, flexShrink: 0,
              }}>
                {selectedSkill.name[0]}
              </div>
              <div>
                <h3 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: '#1e293b' }}>{selectedSkill.name}</h3>
                <div style={{ color: '#94a3b8', fontSize: 13, marginTop: 2 }}>{selectedSkill.category}</div>
              </div>
            </div>
            <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
              {selectedSkill.tags.map(tag => (
                <Tag key={tag} color="blue" style={{ borderRadius: 12, fontSize: 13 }}>{tag}</Tag>
              ))}
            </div>
            <p style={{ color: '#475569', fontSize: 14, lineHeight: 1.8, marginBottom: 20 }}>
              {selectedSkill.desc}
            </p>
            <div style={{
              background: '#f8fafc', borderRadius: 8, padding: '16px 20px',
              border: '1px solid #e2e8f0', marginBottom: 24,
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <UserOutlined style={{ color: '#3b82f6' }} />
                <span style={{ fontSize: 14, fontWeight: 500, color: '#1e293b' }}>钉钉官方出品</span>
              </div>
              <div style={{ color: '#64748b', fontSize: 13 }}>作者：钉钉（中国）信息技术有限公司</div>
            </div>
            <div style={{ display: 'flex', gap: 12, marginBottom: 24 }}>
              <div style={{ flex: 1, textAlign: 'center', padding: '16px 12px', borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff' }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>类型</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>AI 技能</div>
              </div>
              <div style={{ flex: 1, textAlign: 'center', padding: '16px 12px', borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff' }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>分类</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>{selectedSkill.category}</div>
              </div>
              <div style={{ flex: 1, textAlign: 'center', padding: '16px 12px', borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff' }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>来源</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>钉钉官方</div>
              </div>
            </div>
            <Button
              type="primary"
              size="large"
              block
              href={`https://mcp.dingtalk.com/#/detail/skill?skillId=${selectedSkill.skillId}`}
              target="_blank"
              style={{
                height: 48, fontSize: 16, fontWeight: 600,
                background: 'linear-gradient(135deg, #3b82f6 0%, #6366f1 100%)',
                border: 'none', borderRadius: 10,
              }}
            >
              在钉钉中使用
            </Button>
            <p style={{ textAlign: 'center', color: '#94a3b8', fontSize: 13, marginTop: 16, marginBottom: 0 }}>
              此技能由钉钉官方提供，可在钉钉中直接使用。
            </p>
          </div>
        )}
      </Modal>

      {/* MCP detail modal */}
      <Modal
        open={!!selectedMcp}
        onCancel={() => setSelectedMcp(null)}
        footer={null}
        width={600}
        centered
        styles={{ body: { padding: '24px 28px', maxHeight: '80vh', overflowY: 'auto' } }}
      >
        {selectedMcp && (
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 14, marginBottom: 20 }}>
              <div style={{
                width: 52, height: 52, borderRadius: '50%',
                background: selectedMcp.color, color: '#fff',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 24, fontWeight: 700, flexShrink: 0,
              }}>
                {selectedMcp.name[0]}
              </div>
              <div>
                <h3 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: '#1e293b' }}>{selectedMcp.name}</h3>
                <div style={{ color: '#94a3b8', fontSize: 13, marginTop: 2 }}>{selectedMcp.category}</div>
              </div>
            </div>
            <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
              <Tag
                icon={<CheckCircleOutlined />}
                color={selectedMcp.free ? 'success' : 'warning'}
                style={{ borderRadius: 12, fontSize: 13 }}
              >
                {selectedMcp.free ? '免费' : '付费'}
              </Tag>
              <Tag color="processing" style={{ borderRadius: 12, fontSize: 13 }}>{selectedMcp.type}</Tag>
            </div>
            <p style={{ color: '#475569', fontSize: 14, lineHeight: 1.8, marginBottom: 20 }}>
              {selectedMcp.desc}
            </p>
            <div style={{
              background: '#f8fafc', borderRadius: 8, padding: '16px 20px',
              border: '1px solid #e2e8f0', marginBottom: 24,
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <UserOutlined style={{ color: '#10b981' }} />
                <span style={{ fontSize: 14, fontWeight: 500, color: '#1e293b' }}>钉钉官方出品</span>
              </div>
              <div style={{ color: '#64748b', fontSize: 13 }}>作者：钉钉（中国）信息技术有限公司</div>
            </div>
            <div style={{ display: 'flex', gap: 12, marginBottom: 24 }}>
              <div style={{ flex: 1, textAlign: 'center', padding: '16px 12px', borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff' }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>类型</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>MCP 服务</div>
              </div>
              <div style={{ flex: 1, textAlign: 'center', padding: '16px 12px', borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff' }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>分类</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>{selectedMcp.category}</div>
              </div>
              <div style={{ flex: 1, textAlign: 'center', padding: '16px 12px', borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff' }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>费用</div>
                <div style={{ fontSize: 13, color: selectedMcp.free ? '#10b981' : '#f59e0b', marginTop: 4 }}>
                  {selectedMcp.free ? '免费使用' : '按量付费'}
                </div>
              </div>
            </div>
            <Button
              type="primary"
              size="large"
              block
              href={`https://mcp.dingtalk.com/#/detail?mcpId=${selectedMcp.mcpId}&detailType=marketMcpDetail`}
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
    </>
  );
};

export default AIToolboxDrawer;
