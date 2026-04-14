import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Modal, message } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { API_BASE } from '../config';

interface SliderCaptchaProps {
  open: boolean;
  onSuccess: (captchaId: string, captchaAnswer: number) => void;
  onCancel: () => void;
}

const SLIDER_W = 320;
const PIECE_VISUAL_W = 50;

const SliderCaptcha: React.FC<SliderCaptchaProps> = ({ open, onSuccess, onCancel }) => {
  const [captchaId, setCaptchaId] = useState('');
  const [bgImage, setBgImage] = useState('');
  const [pieceImage, setPieceImage] = useState('');
  const [pieceY, setPieceY] = useState(0);
  const [loading, setLoading] = useState(false);
  const [sliderX, setSliderX] = useState(0);
  const [dragging, setDragging] = useState(false);
  const [status, setStatus] = useState<'' | 'success' | 'fail'>('');
  const trackRef = useRef<HTMLDivElement>(null);
  const startXRef = useRef(0);

  const loadCaptcha = useCallback(async () => {
    setLoading(true);
    setSliderX(0);
    setStatus('');
    try {
      const res = await fetch(`${API_BASE}/api/auth/captcha`);
      const body = await res.json();
      const data = body.data || body;
      setCaptchaId(data.id);
      setBgImage(data.bg);
      setPieceImage(data.piece);
      setPieceY(data.y);
    } catch {
      message.error('验证码加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (open) loadCaptcha();
  }, [open, loadCaptcha]);

  const handleMouseDown = (e: React.MouseEvent) => {
    if (status) return;
    e.preventDefault();
    setDragging(true);
    startXRef.current = e.clientX - sliderX;
  };

  const handleTouchStart = (e: React.TouchEvent) => {
    if (status) return;
    setDragging(true);
    startXRef.current = e.touches[0].clientX - sliderX;
  };

  useEffect(() => {
    if (!dragging) return;

    const handleMove = (clientX: number) => {
      const maxX = SLIDER_W - 44;
      let newX = clientX - startXRef.current;
      if (newX < 0) newX = 0;
      if (newX > maxX) newX = maxX;
      setSliderX(newX);
    };

    const handleMouseMove = (e: MouseEvent) => handleMove(e.clientX);
    const handleTouchMove = (e: TouchEvent) => handleMove(e.touches[0].clientX);

    const handleEnd = async () => {
      setDragging(false);
      if (sliderX <= 10) return;
      try {
        const res = await fetch(`${API_BASE}/api/auth/captcha/verify`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ captchaId, captchaAnswer: Math.round(sliderX) }),
        });
        const body = await res.json();
        if (body.code === 200) {
          setStatus('success');
          setTimeout(() => onSuccess(captchaId, Math.round(sliderX)), 400);
        } else {
          setStatus('fail');
          setTimeout(() => { loadCaptcha(); }, 800);
        }
      } catch {
        setStatus('fail');
        setTimeout(() => { loadCaptcha(); }, 800);
      }
    };

    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleEnd);
    window.addEventListener('touchmove', handleTouchMove);
    window.addEventListener('touchend', handleEnd);

    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleEnd);
      window.removeEventListener('touchmove', handleTouchMove);
      window.removeEventListener('touchend', handleEnd);
    };
  }, [dragging, sliderX, captchaId, onSuccess]);

  const trackBg = status === 'success'
    ? 'linear-gradient(90deg, #52c41a33, #52c41a22)'
    : status === 'fail'
    ? 'linear-gradient(90deg, #ff4d4f33, #ff4d4f22)'
    : '#f5f7fa';

  const thumbColor = status === 'success' ? '#52c41a' : status === 'fail' ? '#ff4d4f' : '#4f6bff';

  return (
    <Modal
      open={open}
      onCancel={onCancel}
      footer={null}
      width={380}
      centered
      destroyOnClose
      title="安全验证"
    >
      {loading ? (
        <div style={{ height: 220, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#999' }}>
          加载中...
        </div>
      ) : (
        <div style={{ userSelect: 'none' }}>
          {/* 拼图区域 */}
          <div style={{ position: 'relative', width: SLIDER_W, height: 160, margin: '0 auto', borderRadius: 8, overflow: 'hidden' }}>
            {bgImage && <img src={bgImage} alt="" style={{ width: SLIDER_W, height: 160, display: 'block' }} draggable={false} />}
            <ReloadOutlined
              onClick={loadCaptcha}
              style={{ position: 'absolute', top: 8, right: 8, fontSize: 16, color: '#fff', cursor: 'pointer', textShadow: '0 1px 3px rgba(0,0,0,0.5)' }}
            />
            {pieceImage && (
              <img
                src={pieceImage}
                alt=""
                style={{
                  position: 'absolute',
                  top: pieceY,
                  left: sliderX,
                  height: PIECE_VISUAL_W,
                  filter: 'drop-shadow(2px 2px 4px rgba(0,0,0,0.3))',
                  pointerEvents: 'none',
                }}
                draggable={false}
              />
            )}
          </div>

          {/* 滑动条 */}
          <div style={{ width: SLIDER_W, margin: '16px auto 8px', position: 'relative' }}>
            <div
              ref={trackRef}
              style={{
                height: 44,
                borderRadius: 22,
                background: trackBg,
                border: '1px solid #e5e7eb',
                position: 'relative',
                overflow: 'hidden',
                transition: status ? 'background 0.3s' : undefined,
              }}
            >
              {/* 已滑过的区域 */}
              <div style={{
                position: 'absolute',
                left: 0,
                top: 0,
                width: sliderX + 22,
                height: '100%',
                borderRadius: 22,
                background: status === 'success' ? '#52c41a22' : '#4f6bff11',
                transition: status ? 'background 0.3s' : undefined,
              }} />

              {/* 提示文字 */}
              <div style={{
                position: 'absolute',
                width: '100%',
                textAlign: 'center',
                lineHeight: '44px',
                fontSize: 13,
                color: '#b0b8c8',
                letterSpacing: 4,
                pointerEvents: 'none',
              }}>
                {status === 'success' ? '验证成功' : status === 'fail' ? '验证失败，请重试' : '向右拖动滑块完成拼图'}
              </div>

              {/* 滑块 */}
              <div
                onMouseDown={handleMouseDown}
                onTouchStart={handleTouchStart}
                style={{
                  position: 'absolute',
                  left: sliderX,
                  top: 2,
                  width: 40,
                  height: 40,
                  borderRadius: 20,
                  background: thumbColor,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: status ? 'default' : 'grab',
                  boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
                  transition: status ? 'background 0.3s' : undefined,
                  zIndex: 2,
                }}
              >
                <span style={{ color: '#fff', fontSize: 16, fontWeight: 700 }}>
                  {status === 'success' ? '\u2713' : '\u276F'}
                </span>
              </div>
            </div>
          </div>
        </div>
      )}
    </Modal>
  );
};

export default SliderCaptcha;
