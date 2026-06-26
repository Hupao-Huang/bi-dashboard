// 人民币金额转中文大写（会计）—— 纯函数，便于单测
// 拿用友凭证「肆万贰仟元整」「陆拾贰万叁仟柒佰玖拾元陆角玖分」对照

const CN_DIGITS = ['零', '壹', '贰', '叁', '肆', '伍', '陆', '柒', '捌', '玖'];
const CN_POS = ['', '拾', '佰', '仟']; // 组内位（个十百千）
const CN_GROUP = ['', '万', '亿', '兆']; // 组单位

// 4 位数（0-9999）转中文：组内中间零合并、尾零去除；返回不含组单位（n=0 返回空串）
function groupToCN(n: number): string {
  let s = '';
  let zeroPending = false;
  let started = false;
  for (let pos = 3; pos >= 0; pos--) {
    const d = Math.floor(n / Math.pow(10, pos)) % 10;
    if (d === 0) {
      if (started) zeroPending = true;
    } else {
      if (zeroPending) {
        s += '零';
        zeroPending = false;
      }
      s += CN_DIGITS[d] + CN_POS[pos];
      started = true;
    }
  }
  return s;
}

export function amountToChinese(amount: number): string {
  if (!Number.isFinite(amount)) return '';
  const neg = amount < 0;
  const cents = Math.round(Math.abs(amount) * 100);
  if (cents === 0) return '零元整';

  const yuan = Math.floor(cents / 100);
  const jiao = Math.floor((cents % 100) / 10);
  const fen = cents % 10;

  // 整数（元）部分：按 4 位分组
  let intCN = '';
  if (yuan === 0) {
    intCN = '零';
  } else {
    const groups: number[] = [];
    let y = yuan;
    while (y > 0) {
      groups.push(y % 10000);
      y = Math.floor(y / 10000);
    }
    for (let gi = groups.length - 1; gi >= 0; gi--) {
      const g = groups[gi];
      if (g === 0) {
        // 空组：后面还有非零低组时补一个零（统一在末尾去重）
        if (intCN && !intCN.endsWith('零')) intCN += '零';
      } else {
        // 高位组存在且本组千位为 0（不足千），组间补零，如 一万零五 / 一百万零五百
        if (gi < groups.length - 1 && g < 1000 && intCN && !intCN.endsWith('零')) {
          intCN += '零';
        }
        intCN += groupToCN(g) + CN_GROUP[gi];
      }
    }
    intCN = intCN.replace(/零+$/, '');
  }

  let result = intCN + '元';
  if (jiao === 0 && fen === 0) {
    result += '整';
  } else {
    if (jiao > 0) {
      result += CN_DIGITS[jiao] + '角';
    } else {
      result += '零'; // 有分无角，元后补零
    }
    if (fen > 0) {
      result += CN_DIGITS[fen] + '分';
    }
  }
  return neg ? '负' + result : result;
}
