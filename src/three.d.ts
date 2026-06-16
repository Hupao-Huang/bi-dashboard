// three 没有自带 TS 类型, 这里仅在数据关联三维图用到 FogExp2 等少量 API,
// 不引入重型 @types/three(版本敏感), 用模块声明放行即可。
declare module 'three';
