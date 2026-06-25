// 账簿选择器纯逻辑 (2026-06-25) — 不依赖 React / antd, 便于单测
// (与 AccbookPicker.tsx 视图分离: 这里只放可独立验证的纯函数)
export interface AccbookOption {
  code: string;
  name: string;
}

// 按 编码 或 名称 模糊过滤 (大小写不敏感, 去首尾空格)
export const filterBooks = (books: AccbookOption[], keyword: string): AccbookOption[] => {
  const kw = keyword.trim().toLowerCase();
  if (!kw) return books;
  return books.filter(
    (b) => b.code.toLowerCase().includes(kw) || b.name.toLowerCase().includes(kw)
  );
};

// 勾/取消一个账簿编码
export const toggleCode = (codes: string[], code: string): string[] =>
  codes.includes(code) ? codes.filter((c) => c !== code) : [...codes, code];

// 把一批账簿并入已选 (去重, 用于「全选当前过滤结果」)
export const mergeAll = (codes: string[], books: AccbookOption[]): string[] => {
  const next = new Set(codes);
  books.forEach((b) => next.add(b.code));
  return Array.from(next);
};

// 外部触发器文案: 空→placeholder; 1-2个→名称; 多个→"已选N个账簿"
export const triggerLabelOf = (
  value: string[],
  nameOf: Map<string, string>,
  placeholder: string
): string => {
  if (value.length === 0) return placeholder;
  if (value.length <= 2) return value.map((c) => nameOf.get(c) || c).join('、');
  return `已选 ${value.length} 个账簿`;
};
