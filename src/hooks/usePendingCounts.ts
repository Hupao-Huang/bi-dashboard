import { useCallback, useEffect, useState } from 'react';
import { API_BASE } from '../config';

export type PendingCounts = {
  users: number;
  feedback: number;
};

const ZERO: PendingCounts = { users: 0, feedback: 0 };

const POLL_MS = 5 * 60 * 1000;

export const usePendingCounts = (enabled: boolean): PendingCounts => {
  const [counts, setCounts] = useState<PendingCounts>(ZERO);

  const fetchCounts = useCallback((signal?: AbortSignal) => {
    fetch(`${API_BASE}/api/admin/pending-counts`, { credentials: 'include', signal })
      .then(res => (res.ok ? res.json() : null))
      .then(res => {
        if (res?.data) {
          setCounts({
            users: Number(res.data.users) || 0,
            feedback: Number(res.data.feedback) || 0,
          });
        }
      })
      .catch(err => {
        if (err?.name !== 'AbortError') {
          // 静默失败 — 徽标不显示比报错更体面
        }
      });
  }, []);

  useEffect(() => {
    if (!enabled) {
      setCounts(ZERO);
      return;
    }
    const ctrl = new AbortController();
    fetchCounts(ctrl.signal);
    const timer = setInterval(() => fetchCounts(ctrl.signal), POLL_MS);
    return () => {
      clearInterval(timer);
      ctrl.abort();
    };
  }, [enabled, fetchCounts]);

  return counts;
};
