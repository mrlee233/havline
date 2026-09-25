import type { NotifyType, WebhookProvider } from '../api/types'

export const NOTIFY_TYPE_OPTIONS: { label: string; value: NotifyType }[] = [
  { label: '关闭', value: '' },
  { label: '邮件（SMTP）', value: 'email' },
  // 企业微信 / 钉钉 / 飞书机器人都是「POST 一个 URL + JSON」，归在 Webhook 预设里，标签要说明白
  { label: 'Webhook / IM 机器人', value: 'webhook' },
  { label: 'Telegram 机器人', value: 'telegram' },
]

export const WEBHOOK_PROVIDER_OPTIONS: { label: string; value: WebhookProvider }[] = [
  { label: 'Bark', value: 'bark' },
  { label: 'ntfy', value: 'ntfy' },
  { label: 'Gotify', value: 'gotify' },
  { label: '企业微信机器人', value: 'wecom' },
  { label: '钉钉机器人', value: 'dingtalk' },
  { label: '飞书机器人', value: 'feishu' },
  { label: '自定义 Webhook', value: 'custom' },
]

export const DEFAULT_BARK_SERVER = 'https://api.day.app'
export const DEFAULT_NTFY_SERVER = 'https://ntfy.sh'
export const DEFAULT_WECOM_SERVER = 'https://qyapi.weixin.qq.com'
export const DEFAULT_DINGTALK_SERVER = 'https://oapi.dingtalk.com'
export const DEFAULT_FEISHU_SERVER = 'https://open.feishu.cn'
export const MASKED_SECRET = '********'
