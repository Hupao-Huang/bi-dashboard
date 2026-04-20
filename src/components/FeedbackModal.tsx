import React, { useState } from 'react';
import { Form, Input, Modal, Upload, message } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { UploadFile } from 'antd';
import { API_BASE } from '../config';

interface FeedbackModalProps {
  open: boolean;
  onClose: () => void;
}

const FeedbackModal: React.FC<FeedbackModalProps> = ({ open, onClose }) => {
  const [form] = Form.useForm();
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewImage, setPreviewImage] = useState('');

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      setSubmitting(true);

      const formData = new FormData();
      formData.append('title', values.title);
      formData.append('content', values.content);
      formData.append('pageUrl', window.location.href);

      fileList.forEach(file => {
        if (file.originFileObj) {
          formData.append('files', file.originFileObj);
        }
      });

      const res = await fetch(`${API_BASE}/api/feedback`, {
        method: 'POST',
        credentials: 'include',
        body: formData,
      });

      if (!res.ok) {
        const body = await res.json().catch(err => { console.warn('FeedbackModal json:', err); return {}; });
        throw new Error(body.msg || '提交失败');
      }

      message.success('反馈提交成功，感谢您的反馈！');
      form.resetFields();
      setFileList([]);
      onClose();
    } catch (err: unknown) {
      if (err instanceof Error && err.message !== 'Validation failed') {
        message.error(err.message);
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handlePreview = async (file: UploadFile) => {
    if (!file.url && !file.preview) {
      file.preview = await new Promise<string>((resolve) => {
        const reader = new FileReader();
        reader.readAsDataURL(file.originFileObj as File);
        reader.onload = () => resolve(reader.result as string);
      });
    }
    setPreviewImage(file.url || (file.preview as string));
    setPreviewOpen(true);
  };

  return (
    <>
      <Modal
        title="问题反馈"
        open={open}
        onCancel={onClose}
        onOk={handleSubmit}
        okText="提交"
        cancelText="取消"
        confirmLoading={submitting}
        destroyOnHidden
        width={520}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="title"
            label="标题"
            rules={[{ required: true, message: '请输入标题' }]}
          >
            <Input placeholder="简要描述问题" maxLength={100} />
          </Form.Item>
          <Form.Item
            name="content"
            label="问题描述"
            rules={[{ required: true, message: '请描述问题' }]}
          >
            <Input.TextArea
              placeholder="详细描述遇到的问题、期望的效果等"
              rows={4}
              maxLength={2000}
              showCount
            />
          </Form.Item>
          <Form.Item label="截图（最多5张）">
            <Upload
              listType="picture-card"
              fileList={fileList}
              onChange={({ fileList }) => setFileList(fileList)}
              onPreview={handlePreview}
              beforeUpload={() => false}
              accept="image/*"
              maxCount={5}
            >
              {fileList.length >= 5 ? null : (
                <div>
                  <PlusOutlined />
                  <div style={{ marginTop: 8, fontSize: 12 }}>上传截图</div>
                </div>
              )}
            </Upload>
          </Form.Item>
          <div style={{ fontSize: 12, color: '#94a3b8' }}>
            当前页面：{window.location.pathname}
          </div>
        </Form>
      </Modal>
      <Modal
        open={previewOpen}
        footer={null}
        onCancel={() => setPreviewOpen(false)}
        width={800}
      >
        <img alt="预览" style={{ width: '100%' }} src={previewImage} />
      </Modal>
    </>
  );
};

export default FeedbackModal;
