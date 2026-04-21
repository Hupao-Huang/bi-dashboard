import React, { useState, useMemo } from 'react';
import { Input, Tag, Card, Modal, Button } from 'antd';
import {
  SearchOutlined,
  UserOutlined,
} from '@ant-design/icons';

const { Search } = Input;

interface Skill {
  name: string;
  skillId: string;
  desc: string;
  category: string;
  tags: string[];
  color: string;
}

const categories = ['全部', '办公协同', '文档处理', '信息搜索', '营销推广', 'AI创作', '教育', '效率工具', '会议'];

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

const SkillsPage: React.FC = () => {
  const [activeCategory, setActiveCategory] = useState('全部');
  const [searchText, setSearchText] = useState('');
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);

  const filtered = useMemo(() => {
    return skills.filter(s => {
      const matchCategory = activeCategory === '全部' || s.category === activeCategory;
      const matchSearch = !searchText || s.name.toLowerCase().includes(searchText.toLowerCase()) || s.desc.includes(searchText);
      return matchCategory && matchSearch;
    });
  }, [activeCategory, searchText]);

  return (
    <div style={{ padding: '0 8px' }}>
      {/* 头部介绍 */}
      <div style={{
        background: 'linear-gradient(135deg, #3b82f6 0%, #6366f1 100%)',
        borderRadius: 12,
        padding: '32px 40px',
        marginBottom: 24,
        color: '#fff',
      }}>
        <h2 style={{ color: '#fff', fontSize: 28, margin: 0, fontWeight: 700 }}>
          Skills 技能广场
        </h2>
        <p style={{ color: 'rgba(255,255,255,0.85)', fontSize: 15, margin: '8px 0 20px' }}>
          钉钉官方34个AI技能，提出你的工作痛点，一键为你组装解法
        </p>
        <Search
          placeholder="搜索技能名称或描述..."
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

      {/* 技能卡片网格 */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
        gap: 16,
      }}>
        {filtered.map(skill => (
          <Card
            key={skill.name}
            hoverable
            onClick={() => setSelectedSkill(skill)}
            style={{ borderRadius: 10, border: '1px solid #f0f0f0' }}
            styles={{ body: { padding: 20 } }}
          >
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
              <div style={{
                width: 44, height: 44, borderRadius: '50%',
                background: skill.color, color: '#fff',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 20, fontWeight: 700, flexShrink: 0,
              }}>
                {skill.name[0]}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                  <span style={{ fontWeight: 600, fontSize: 15, color: '#1e293b' }}>{skill.name}</span>
                </div>
                <p style={{
                  color: '#64748b', fontSize: 13, margin: '4px 0 12px',
                  lineHeight: 1.5,
                  overflow: 'hidden', textOverflow: 'ellipsis',
                  display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' as const,
                }}>
                  {skill.desc}
                </p>
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  {skill.tags.map(tag => (
                    <Tag key={tag} style={{ fontSize: 11, margin: 0, borderRadius: 4 }} color="blue">{tag}</Tag>
                  ))}
                </div>
              </div>
            </div>
          </Card>
        ))}
      </div>

      {filtered.length === 0 && (
        <div style={{ textAlign: 'center', padding: 60, color: '#94a3b8' }}>
          没有找到匹配的技能
        </div>
      )}

      {/* 详情弹窗 */}
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
            {/* 头部：头像 + 名称 */}
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

            {/* 标签 */}
            <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
              {selectedSkill.tags.map(tag => (
                <Tag key={tag} color="blue" style={{ borderRadius: 12, fontSize: 13 }}>{tag}</Tag>
              ))}
            </div>

            {/* 描述 */}
            <p style={{ color: '#475569', fontSize: 14, lineHeight: 1.8, marginBottom: 20 }}>
              {selectedSkill.desc}
            </p>

            {/* 来源信息 */}
            <div style={{
              background: '#f8fafc', borderRadius: 8, padding: '16px 20px',
              border: '1px solid #e2e8f0', marginBottom: 24,
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <UserOutlined style={{ color: '#3b82f6' }} />
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
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>AI 技能</div>
              </div>
              <div style={{
                flex: 1, textAlign: 'center', padding: '16px 12px',
                borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff',
              }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>分类</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>{selectedSkill.category}</div>
              </div>
              <div style={{
                flex: 1, textAlign: 'center', padding: '16px 12px',
                borderRadius: 12, border: '1px solid #f0f0f0', background: '#fff',
              }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#1e293b' }}>来源</div>
                <div style={{ fontSize: 13, color: '#94a3b8', marginTop: 4 }}>钉钉官方</div>
              </div>
            </div>

            {/* 使用按钮 */}
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
    </div>
  );
};

export default SkillsPage;
