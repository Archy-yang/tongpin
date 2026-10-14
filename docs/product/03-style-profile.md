# 风格画像设计

## 画像维度

| 维度 | 取值 | 说明 |
|---|---|---|
| 沟通节奏 | 快/中/慢 | 回复间隔感知 |
| 回复长度 | 短/中短/中/长 | — |
| 主动程度 | 0.0-1.0 | 发起话题、追问的频率 |
| 情绪表达 | 直接/含蓄 | — |
| 情绪基调 | 积极/平和/低沉 | — |
| 自我暴露速度 | 快/中/慢 | 多快开始谈个人深层话题 |
| 话题偏好 | 枚举列表 | 生活、兴趣、工作、情感、哲学等 |
| 冲突处理 | 回避/理性/情绪化/妥协 | 从分歧对话中识别 |
| 隐私敏感度 | 高/中/低 | 对个人信息话题的回避程度 |
| 关系意图 | 随便聊聊/找朋友/认真关系 | 用户可显式覆盖 |
| 幽默风格 | 冷幽默/自嘲/玩梗/正经 | — |
| 语言习惯 | emoji 频率、标点风格、网络用语、正式程度 | — |
| 活跃时段 | 时段列表 | 从消息时间统计，非 LLM 推断 |

## 风格卡 JSON Schema

```json
{
  "user_id": "u_123",
  "version": 3,
  "communication_style": {
    "pace": "medium",
    "reply_length": "short_to_medium",
    "initiative": 0.62,
    "emoji_usage": "low",
    "formality": "casual"
  },
  "emotional_style": {
    "expression": "reserved",
    "tone": "calm",
    "vulnerability_speed": "slow"
  },
  "topic_preferences": ["music", "movies", "late_night_thoughts", "career"],
  "conflict_style": "rational_avoidant",
  "boundary": {
    "privacy_sensitivity": "high",
    "physical_contact_speed": "slow"
  },
  "relationship_intent": "serious_but_slow",
  "active_hours": ["22:00-01:00"],
  "confidence": 0.78
}
```

字段约定：

- `version` 递增，每次分析生成新版本，保留历史版本可回溯
- `confidence` 由 LLM 自评 + 数据量修正（消息数 < 50 时上限 0.6）
- `active_hours` 由代码从消息时间戳统计，不经过 LLM
- `relationship_intent` 以用户显式选择为准（opt-in 时填写），LLM 推断仅作参考

## 风格分析 Prompt 模板

```
你是风格分析器。根据以下对话记录，输出 JSON。
不要推断敏感个人信息（真实姓名、地址、身份证、财务、健康）。
只分析沟通风格、情绪表达、话题偏好、边界感、关系意图。

对话记录：
{{messages}}

已有一份历史风格卡（如有），本次为增量更新，输出合并后的完整风格卡：
{{previous_profile}}

输出格式（严格 JSON，不要多余文本）：
{
  "communication_style": {
    "pace": "fast|medium|slow",
    "reply_length": "short|short_to_medium|medium|long",
    "initiative": 0.0-1.0,
    "emoji_usage": "none|low|medium|high",
    "formality": "formal|neutral|casual"
  },
  "emotional_style": {
    "expression": "direct|reserved",
    "tone": "positive|calm|melancholic",
    "vulnerability_speed": "fast|medium|slow"
  },
  "topic_preferences": ["..."],
  "conflict_style": "avoidant|rational|emotional|compromising|null",
  "boundary": {
    "privacy_sensitivity": "high|medium|low",
    "physical_contact_speed": "fast|medium|slow"
  },
  "relationship_intent": "casual|friends|serious|null",
  "confidence": 0.0-1.0
}

注意：证据不足的字段输出 null，不要猜测。
```

## 向量化（用于匹配）

- 风格卡 → 匹配向量的映射由**代码完成，不走 LLM**：枚举字段 one-hot、数值字段归一化、话题偏好用多标签编码
- 向量存入 pgvector，维度固定（如 64 维），风格卡更新时同步重算
- 相似度用余弦相似度；部分维度做"互补奖励"（如主动性一高一低加分），在应用层实现，不改向量定义

## 数据与隐私边界

- 风格卡不含敏感个人信息（姓名、住址、证件、财务、健康）——Prompt 层已禁止，存储层再做一次字段白名单校验
- 用户删除风格卡 = 删除 profile 全部版本 + 向量，且停止分析任务
- 用户编辑仅影响展示层；匹配使用原始分析结果（见 MVP 范围文档的决策记录）
