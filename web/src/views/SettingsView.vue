<template>
  <PageHeader title="系统设置" description="配置界面、域名策略与系统维护" />

  <LoadError v-if="pageError" :message="pageError" @retry="load" />

  <n-spin v-else :show="loading">
    <n-tabs v-model:value="settingsTab" type="line" animated class="settings-tabs">
      <n-tab-pane name="general" tab="常规">
        <div class="settings-grid">
      <HavlineCard title="外观" subtitle="界面主题与时区">
        <div class="settings-block">
          <div class="settings-block__label">主题模式</div>
          <div class="theme-cards">
            <button
              v-for="item in themeOptions"
              :key="item.value"
              type="button"
              class="theme-card"
              :class="{ 'theme-card--active': form.theme === item.value }"
              @click="selectTheme(item.value)"
            >
              <n-icon :component="item.icon" class="theme-card__icon" />
              <span class="theme-card__text">{{ item.label }}</span>
              <n-icon
                v-if="form.theme === item.value"
                :component="CheckmarkCircle"
                class="theme-card__check"
              />
            </button>
          </div>
          <p class="field-hint">切换后立即预览，保存后写入配置</p>
        </div>

        <div class="settings-block settings-block--spaced">
          <div class="settings-block__label">时区</div>
          <n-select v-model:value="form.timezone" :options="timezoneOptions" />
          <p class="field-hint">影响日志、证书到期时间等显示</p>
        </div>
      </HavlineCard>

      <HavlineCard title="自动任务" subtitle="DDNS 与证书维护策略">
        <div class="settings-fields">
          <div class="settings-field">
            <div class="settings-field__row">
              <span class="settings-field__label">DDNS 检查周期（分钟）</span>
              <n-input-number v-model:value="ddnsInterval" :min="1" :max="1440" class="settings-field__input" />
            </div>
            <p class="field-hint">每隔 N 分钟检查公网 IP 是否变化并更新 DNS</p>
          </div>
          <div class="settings-field">
            <div class="settings-field__row">
              <span class="settings-field__label">证书续签检查（天）</span>
              <n-input-number v-model:value="certThreshold" :min="1" :max="90" class="settings-field__input" />
            </div>
            <p class="field-hint">当证书剩余有效期少于此天数时自动续签</p>
          </div>
          <div class="settings-field">
            <div class="settings-field__row">
              <span class="settings-field__label">日志保留天数</span>
              <n-input-number v-model:value="logRetention" :min="1" :max="365" class="settings-field__input" />
            </div>
            <p class="field-hint">超过保留天数的日志将被自动清理</p>
          </div>
        </div>
      </HavlineCard>

      <HavlineCard title="ACME 证书" subtitle="申请与续签所需账户信息">
        <div class="settings-fields">
          <div class="settings-field">
            <div class="settings-field__label">ACME 邮箱</div>
            <n-input v-model:value="acmeEmail" placeholder="admin@example.com" />
            <p class="field-hint">用于 Let's Encrypt / ZeroSSL / LiteSSL 账户注册</p>
          </div>
          <ConfiguredSecretField
            v-model="zerosslApiKey"
            label="ZeroSSL API Key"
            :configured="zerosslHasKey"
            placeholder="申请 ZeroSSL 证书时填写"
          />
          <p class="field-hint zerossl-key-hint">申请 ZeroSSL 证书时必填，可在 ZeroSSL 控制台获取</p>
          <div class="settings-field">
            <div class="settings-field__label">DNS 解析器（高级）</div>
            <n-input v-model:value="acmeResolvers" placeholder="223.5.5.5:53, 1.1.1.1:53" />
            <p class="field-hint">
              DNS-01 验证时用它查域名的权威 NS 记录。容器里的系统解析器通常指向 Docker 的
              127.0.0.11，不返回 NS，于是报「could not determine authoritative nameservers」；
              所以默认用公共解析器。留空 = 用默认值；填 system = 回到系统解析器；
              多个用逗号分隔，不写端口默认 53。
            </p>
          </div>
          <div class="settings-field">
            <div class="settings-field__label">LiteSSL EAB Kid</div>
            <n-input v-model:value="litesslEabKid" placeholder="FreeSSL → 证书自动化 → EAB 管理 里创建后粘贴" />
            <p class="field-hint">
              只在用 LiteSSL 申请时必填。LiteSSL 的目录要求 EAB（externalAccountRequired），
              凭据在 FreeSSL 平台的「证书自动化 → EAB 管理」创建；EAB Kid 不是密钥，可回显。
            </p>
          </div>
          <ConfiguredSecretField
            v-model="litesslEabHmac"
            label="LiteSSL EAB HMAC"
            :configured="litesslEabHmacConfigured"
            placeholder="用 LiteSSL 申请证书时填写"
          />
        </div>
      </HavlineCard>

      <HavlineCard title="数据" subtitle="备份与恢复">
        <div class="data-actions">
          <button type="button" class="data-action-card" :disabled="exporting" @click="exportBackup">
            <n-icon :component="DownloadOutline" class="data-action-card__icon" />
            <div class="data-action-card__title">导出配置</div>
            <div class="data-action-card__hint">下载数据库、证书与 Nginx 配置</div>
          </button>
          <n-upload
            class="data-upload"
            :show-file-list="false"
            accept=".tar.gz,.tgz"
            @change="onRestoreFile"
          >
            <div
              class="data-action-card"
              :class="{ 'data-action-card--disabled': restoring }"
              role="button"
              tabindex="0"
            >
              <n-icon :component="CloudUploadOutline" class="data-action-card__icon" />
              <div class="data-action-card__title">恢复配置</div>
              <div class="data-action-card__hint">从备份文件还原系统配置</div>
            </div>
          </n-upload>
        </div>
        <div class="backup-auto">
          <div class="form-switch-row">
            <div class="form-switch-row__text">
              <div class="form-switch-row__label">自动备份</div>
              <div class="form-switch-row__hint">
                每天自动备份一份到服务器，超过保留份数后自动清理最旧的（重启不会重复备份）
              </div>
            </div>
            <n-switch v-model:value="backupAutoEnabled" />
          </div>
          <div class="backup-auto__actions">
            <div class="settings-field backup-auto__keep">
              <label class="settings-field__label">保留份数</label>
              <n-input-number v-model:value="backupKeepCount" :min="1" :max="60" class="settings-field__input" />
            </div>
            <n-button
              size="small"
              :loading="backupBusy === 'create'"
              :disabled="backupBusy !== null"
              @click="createBackupArchive"
            >
              立即备份
            </n-button>
          </div>
        </div>

        <div class="backup-list">
          <div v-for="archive in backupArchives" :key="archive.name" class="backup-row">
            <div class="backup-row__main">
              <span class="mono">{{ formatDate(archive.mod_time) }}</span>
              <span class="backup-row__size">{{ formatBytes(archive.size) }}</span>
            </div>
            <div class="backup-row__actions">
              <n-button
                size="tiny"
                quaternary
                :loading="backupBusy === `download:${archive.name}`"
                :disabled="backupBusy !== null"
                @click="downloadBackupArchive(archive)"
              >
                下载
              </n-button>
              <n-popconfirm @positive-click="restoreBackupArchive(archive)">
                <template #trigger>
                  <n-button size="tiny" quaternary type="warning" :disabled="backupBusy !== null">恢复</n-button>
                </template>
                恢复会覆盖当前的数据库、证书与 Nginx 配置，确定继续？
              </n-popconfirm>
              <n-popconfirm @positive-click="deleteBackupArchive(archive)">
                <template #trigger>
                  <n-button size="tiny" quaternary type="error" :disabled="backupBusy !== null">删除</n-button>
                </template>
                删除这份备份？
              </n-popconfirm>
            </div>
          </div>
          <p v-if="backupError" class="field-hint field-hint--error">{{ backupError }}</p>
          <p v-else-if="backupArchives.length === 0" class="field-hint">还没有留在服务器上的备份。</p>
        </div>

        <p class="field-hint data-note">恢复配置将覆盖当前数据，操作前请确保已备份。</p>
      </HavlineCard>
        </div>
      </n-tab-pane>

      <n-tab-pane name="notify" tab="通知">
        <div class="settings-grid settings-grid--single">
      <HavlineCard title="通知" subtitle="告警通道与事件（仅可启用一种方式）">
        <div class="security-form notify-form">
          <div class="settings-field">
            <div class="settings-field__label">通知方式</div>
            <n-radio-group v-model:value="notifyType" class="notify-type-group">
              <n-radio v-for="item in NOTIFY_TYPE_OPTIONS" :key="item.value || 'off'" :value="item.value">
                {{ item.label }}
              </n-radio>
            </n-radio-group>
          </div>

          <div v-if="notifyType === 'email'" class="settings-fields settings-fields--compact">
            <div class="settings-field notify-smtp-endpoint">
              <div class="notify-smtp-endpoint__col">
                <div class="settings-field__label">SMTP 主机</div>
                <n-input v-model:value="notifyEmail.host" placeholder="smtp.example.com" />
              </div>
              <div class="notify-smtp-endpoint__col notify-smtp-endpoint__col--port">
                <div class="settings-field__label">端口</div>
                <n-input-number v-model:value="notifyEmail.port" :min="1" :max="65535" />
              </div>
            </div>
            <div class="settings-field">
              <div class="settings-field__label">用户名</div>
              <n-input v-model:value="notifyEmail.username" placeholder="user@example.com" />
            </div>
            <ConfiguredSecretField
              v-model="notifySMTPPassword"
              label="密码"
              :configured="notifyEmail.has_password"
              placeholder="SMTP 密码"
            />
            <div class="settings-field">
              <div class="settings-field__label">发件人</div>
              <n-input v-model:value="notifyEmail.from" placeholder="havline@example.com" />
            </div>
            <div class="settings-field">
              <div class="settings-field__label">收件人（每行一个）</div>
              <n-input v-model:value="notifyEmailToText" type="textarea" :rows="2" placeholder="admin@example.com" />
            </div>
            <div class="form-switch-row">
              <div class="form-switch-row__text">
                <div class="form-switch-row__label">启用 TLS</div>
              </div>
              <n-switch v-model:value="notifyEmail.tls" />
            </div>
          </div>

          <div v-else-if="notifyType === 'webhook'" class="settings-fields settings-fields--compact">
            <div class="settings-field">
              <div class="settings-field__label">Webhook 预设</div>
              <n-select
                v-model:value="notifyWebhook.provider"
                :options="WEBHOOK_PROVIDER_OPTIONS"
                @update:value="onWebhookProviderChange"
              />
            </div>
            <template v-if="notifyWebhook.provider === 'bark'">
              <div class="settings-field">
                <div class="settings-field__label">Bark 服务地址</div>
                <n-input v-model:value="notifyWebhook.server" :placeholder="DEFAULT_BARK_SERVER" />
              </div>
              <ConfiguredSecretField
                v-model="notifyWebhookKey"
                label="Device Key"
                :configured="notifyWebhook.has_secret"
                placeholder="Bark Device Key"
              />
            </template>
            <template v-else-if="notifyWebhook.provider === 'ntfy'">
              <div class="settings-field">
                <div class="settings-field__label">ntfy 服务地址</div>
                <n-input v-model:value="notifyWebhook.server" :placeholder="DEFAULT_NTFY_SERVER" />
              </div>
              <div class="settings-field">
                <div class="settings-field__label">Topic</div>
                <n-input v-model:value="notifyWebhook.topic" placeholder="havline-alerts" />
              </div>
              <ConfiguredSecretField
                v-model="notifyWebhookKey"
                label="访问令牌（可选）"
                :configured="notifyWebhook.has_secret"
                placeholder="Bearer Token"
              />
            </template>
            <template v-else-if="notifyWebhook.provider === 'gotify'">
              <div class="settings-field">
                <div class="settings-field__label">Gotify 服务地址</div>
                <n-input v-model:value="notifyWebhook.server" placeholder="https://gotify.example.com" />
              </div>
              <ConfiguredSecretField
                v-model="notifyWebhookKey"
                label="App Token"
                :configured="notifyWebhook.has_secret"
                placeholder="Gotify App Token"
              />
            </template>
            <template v-else-if="notifyWebhook.provider === 'wecom'">
              <div class="settings-field">
                <div class="settings-field__label">企业微信服务地址</div>
                <n-input v-model:value="notifyWebhook.server" :placeholder="DEFAULT_WECOM_SERVER" />
              </div>
              <ConfiguredSecretField
                v-model="notifyWebhookKey"
                label="机器人 Key"
                :configured="notifyWebhook.has_secret"
                placeholder="Webhook 地址里 key= 后面的部分"
              />
              <p class="field-hint">群机器人 → 添加机器人 → 复制 Webhook 地址，取其中 key= 之后的部分。</p>
            </template>
            <template v-else-if="notifyWebhook.provider === 'dingtalk'">
              <div class="settings-field">
                <div class="settings-field__label">钉钉服务地址</div>
                <n-input v-model:value="notifyWebhook.server" :placeholder="DEFAULT_DINGTALK_SERVER" />
              </div>
              <ConfiguredSecretField
                v-model="notifyWebhookKey"
                label="Access Token"
                :configured="notifyWebhook.has_secret"
                placeholder="access_token 参数值"
              />
              <p class="field-hint">
                安全设置选「自定义关键词」并填 <span class="mono">Havline</span>：消息正文固定以「Havline · 」开头。加签方式暂不支持。
              </p>
            </template>
            <template v-else-if="notifyWebhook.provider === 'feishu'">
              <div class="settings-field">
                <div class="settings-field__label">飞书服务地址</div>
                <n-input v-model:value="notifyWebhook.server" :placeholder="DEFAULT_FEISHU_SERVER" />
              </div>
              <ConfiguredSecretField
                v-model="notifyWebhookKey"
                label="机器人 Key"
                :configured="notifyWebhook.has_secret"
                placeholder="Webhook 地址 /hook/ 后面的部分"
              />
            </template>
            <template v-else>
              <div class="settings-field">
                <div class="settings-field__label">Webhook URL</div>
                <n-input v-model:value="notifyWebhook.url" placeholder="https://example.com/hook" />
                <p class="field-hint">将以 JSON 发送：event、category、icon、title、message、formatted、time</p>
              </div>
            </template>
          </div>

          <div v-else-if="notifyType === 'telegram'" class="settings-fields settings-fields--compact">
            <ConfiguredSecretField
              v-model="notifyTelegramToken"
              label="Bot Token"
              :configured="notifyTelegram.has_bot_token"
              placeholder="123456:ABC-DEF"
            />
            <div class="settings-field">
              <div class="settings-field__label">Chat ID</div>
              <n-input v-model:value="notifyTelegram.chat_id" placeholder="-1001234567890" />
            </div>
            <div class="settings-field">
              <div class="settings-field__label">代理地址（可选）</div>
              <n-input v-model:value="notifyTelegram.proxy_url" placeholder="http://127.0.0.1:7890" />
              <p class="field-hint">仅在无法直连 Telegram API 时填写 HTTP 代理</p>
            </div>
          </div>

          <div v-if="notifyType" class="notify-events">
            <div class="notify-events__title">告警事件</div>
            <div v-for="group in NOTIFY_EVENT_GROUPS" :key="group.title" class="notify-event-group">
              <div class="notify-event-group__title">{{ group.title }}</div>
              <div class="form-switch-list">
                <template v-for="event in group.events" :key="event.key">
                  <div class="form-switch-row">
                    <div class="form-switch-row__text">
                      <div class="form-switch-row__label">{{ event.label }}</div>
                      <div v-if="event.hint" class="form-switch-row__hint">{{ event.hint }}</div>
                    </div>
                    <n-switch v-model:value="notifyEventFlags[event.key]" />
                  </div>
                  <div
                    v-if="event.hasThreshold && notifyEventFlags[event.key]"
                    class="settings-fields settings-fields--compact notify-threshold-fields"
                  >
                    <template v-if="event.thresholdKey === 'ip_frequent'">
                      <div class="settings-field">
                        <label class="settings-field__label">访问次数阈值</label>
                        <n-input-number v-model:value="notifyIPFrequentThreshold" :min="10" :max="10000" class="settings-field__input" />
                      </div>
                      <div class="settings-field">
                        <label class="settings-field__label">统计窗口（秒）</label>
                        <n-input-number v-model:value="notifyIPFrequentWindowSec" :min="10" :max="3600" class="settings-field__input" />
                      </div>
                    </template>
                    <template v-else-if="event.thresholdKey === 'login_failure'">
                      <div class="settings-field">
                        <label class="settings-field__label">失败次数阈值</label>
                        <n-input-number v-model:value="notifyLoginFailureThreshold" :min="3" :max="100" class="settings-field__input" />
                      </div>
                      <div class="settings-field">
                        <label class="settings-field__label">统计窗口（秒）</label>
                        <n-input-number v-model:value="notifyLoginFailureWindowSec" :min="10" :max="3600" class="settings-field__input" />
                      </div>
                    </template>
                    <template v-else-if="event.thresholdKey === 'resource'">
                      <div class="settings-field">
                        <label class="settings-field__label">CPU 阈值（%）</label>
                        <n-input-number v-model:value="notifyCPUThreshold" :min="1" :max="100" class="settings-field__input" />
                      </div>
                      <div class="settings-field">
                        <label class="settings-field__label">内存阈值（%）</label>
                        <n-input-number v-model:value="notifyMemoryThreshold" :min="1" :max="100" class="settings-field__input" />
                      </div>
                      <div class="settings-field">
                        <label class="settings-field__label">磁盘阈值（%）</label>
                        <n-input-number v-model:value="notifyDiskThreshold" :min="1" :max="100" class="settings-field__input" />
                      </div>
                    </template>
                    <template v-else-if="event.thresholdKey === 'tunnel_fail'">
                      <div class="settings-field">
                        <label class="settings-field__label">连续失败次数</label>
                        <n-input-number v-model:value="notifyTunnelFailThreshold" :min="1" :max="20" class="settings-field__input" />
                      </div>
                    </template>
                    <template v-else-if="event.thresholdKey === 'frps_fail'">
                      <div class="settings-field">
                        <label class="settings-field__label">连续失败次数</label>
                        <n-input-number v-model:value="notifyFrpsFailThreshold" :min="1" :max="20" class="settings-field__input" />
                      </div>
                    </template>
                    <template v-else-if="event.thresholdKey === 'agent_fail'">
                      <div class="settings-field">
                        <label class="settings-field__label">连续失败次数</label>
                        <n-input-number v-model:value="notifyAgentFailThreshold" :min="1" :max="20" class="settings-field__input" />
                      </div>
                    </template>
                    <template v-else-if="event.thresholdKey === 'drift_cooldown'">
                      <div class="settings-field">
                        <label class="settings-field__label">提醒间隔（小时）</label>
                        <n-input-number v-model:value="notifyDriftCooldownHours" :min="1" :max="168" class="settings-field__input" />
                      </div>
                    </template>
                  </div>
                </template>
              </div>
            </div>
          </div>

          <div v-if="notifyType" class="notify-test-row">
            <n-button :loading="notifyTesting" @click="testNotify">测试通知</n-button>
            <p class="field-hint">使用当前表单配置发送测试消息，无需先保存</p>
          </div>
        </div>
      </HavlineCard>
        </div>
      </n-tab-pane>

      <n-tab-pane name="security" tab="安全">
        <div class="settings-grid">
      <HavlineCard title="反代安全" subtitle="全局 IP 策略与真实客户端 IP">
        <div class="security-form proxy-security-form">
          <p class="field-hint proxy-security-form__intro">
            IP 黑白名单、仅中国大陆等策略依赖正确识别客户端 IP。若域名经 CDN/多层反代，请在此配置信任代理。
          </p>
          <div class="form-switch-row">
            <div class="form-switch-row__text">
              <div class="form-switch-row__label">启用信任代理</div>
              <div class="form-switch-row__hint">从请求头解析真实客户端 IP</div>
            </div>
            <n-switch v-model:value="trustedProxyEnabled" />
          </div>
          <div v-if="trustedProxyEnabled" class="settings-fields settings-fields--compact">
            <div class="settings-field">
              <div class="settings-field__label">预设</div>
              <n-select v-model:value="trustedProxyPreset" :options="trustedProxyPresets" />
            </div>
            <template v-if="trustedProxyPreset === 'custom'">
              <div class="settings-field">
                <div class="settings-field__label">信任 CIDR（每行一个）</div>
                <n-input v-model:value="trustedProxyCIDRs" type="textarea" :rows="3" placeholder="203.0.113.0/24" />
              </div>
              <div class="settings-field">
                <div class="settings-field__label">IP 头字段</div>
                <n-select v-model:value="trustedProxyHeader" :options="trustedProxyHeaders" />
              </div>
            </template>
          </div>
          <div class="ip-policy-grid">
            <div class="settings-field ip-policy-field">
              <div class="settings-field__label">全局 IP 黑名单</div>
              <div class="ip-policy-field__input">
                <n-input
                  v-model:value="globalIPBlacklistText"
                  type="textarea"
                  :rows="6"
                  placeholder="1.2.3.4&#10;5.6.7.0/24"
                />
              </div>
              <p class="field-hint">每行一个，优先于所有规则拦截</p>
            </div>
            <div class="settings-field ip-policy-field">
              <div class="settings-field__label">全局 IP 白名单</div>
              <div class="ip-policy-field__input">
                <n-input
                  v-model:value="globalIPWhitelistText"
                  type="textarea"
                  :rows="6"
                  placeholder="203.0.113.10&#10;203.0.113.0/24"
                />
              </div>
              <p class="field-hint">内网段已默认放行（已预填），可在下方追加额外 IP，每行一个</p>
            </div>
          </div>
          <div class="china-cidr-panel">
            <div class="settings-field__row china-cidr-panel__head">
              <span class="settings-field__label">中国 IP 段</span>
              <StatusBadge :kind="chinaCIDRBadgeKind" :text="chinaCIDRBadgeText" />
              <n-button
                size="small"
                :loading="refreshingCIDR || chinaCIDR.updating"
                :disabled="chinaCIDR.updating && !refreshingCIDR"
                @click="refreshChinaCIDR"
              >
                {{ refreshingCIDR || chinaCIDR.updating ? '更新中…' : '立即更新' }}
              </n-button>
            </div>
            <p class="field-hint">
              <template v-if="chinaCIDR.ready">
                上次更新：{{ formatDate(chinaCIDR.updated_at) }}（{{ formatRelativeTime(chinaCIDR.updated_at) }}） · IPv4
                {{ chinaCIDR.entry_count_v4 }} 条 · IPv6 {{ chinaCIDR.entry_count_v6 }} 条
              </template>
              <template v-else-if="chinaCIDR.updated_at">
                上次更新：{{ formatDate(chinaCIDR.updated_at) }} · IPv4 {{ chinaCIDR.entry_count_v4 }} 条 · IPv6
                {{ chinaCIDR.entry_count_v6 }} 条（当前不可用，请重新更新）
              </template>
              <template v-else-if="refreshingCIDR || chinaCIDR.updating">
                正在下载 IPv4 / IPv6 段并校验 Nginx 配置，通常需要 10–60 秒…
              </template>
              <template v-else>尚未下载中国 IP 段，启用「仅中国大陆」前请先更新</template>
            </p>
            <p v-if="chinaCIDR.last_error && !refreshingCIDR && !chinaCIDR.updating" class="field-hint field-hint--error">
              {{ chinaCIDR.last_error }}
            </p>
          </div>
          <div class="settings-field">
            <div class="settings-field__row">
              <span class="settings-field__label">更新间隔（小时）</span>
              <n-input-number v-model:value="chinaCIDRHours" :min="1" :max="168" class="settings-field__input" />
            </div>
            <p class="field-hint">自动检查并更新中国 IP 段库的间隔</p>
          </div>
        </div>
      </HavlineCard>

      <HavlineCard title="管理员账户" subtitle="修改登录密码" class="settings-card settings-card--security">
        <div class="security-form">
          <div class="settings-fields">
            <div class="settings-field">
              <div class="settings-field__label">当前密码</div>
              <n-input v-model:value="passwordForm.old_password" type="password" show-password-on="click" />
            </div>
            <div class="settings-field">
              <div class="settings-field__label">新密码</div>
              <n-input v-model:value="passwordForm.new_password" type="password" show-password-on="click" />
            </div>
            <div class="settings-field">
              <div class="settings-field__label">确认新密码</div>
              <n-input v-model:value="passwordForm.confirm_password" type="password" show-password-on="click" />
            </div>
          </div>
          <n-button type="primary" class="password-btn" :loading="changingPassword" @click="changePassword">
            修改管理员密码
          </n-button>
        </div>
      </HavlineCard>
        </div>
      </n-tab-pane>

      <n-tab-pane name="advanced" tab="高级">
        <div class="settings-grid settings-grid--single">
      <HavlineCard title="Nginx 全局配置" subtitle="http 块内自定义片段">
        <p class="field-hint">
          仅作用于 <code>http {}</code> 内部、各反代 <code>server {}</code> 之前。单条规则的 server 块请在反代详情 → Nginx 中编辑。
        </p>
        <details class="nginx-framework">
          <summary>查看自动生成的框架说明</summary>
          <NginxCodeEditor
            :model-value="globalNginxFramework"
            readonly
            min-height="240px"
            class="nginx-framework__editor"
          />
        </details>
        <details class="nginx-framework">
          <summary>主配置版本与回滚（{{ configVersions.length }} 个历史版本）</summary>
          <p class="field-hint">
            保存规则或设置时会重新生成 <code>nginx.conf</code>，这里保留最近 10 次内容有变化的版本。
            若配置被外部改动导致 Nginx 起不来，可直接回滚（回滚前先校验语法与 nginx -t，不过关不会覆盖当前配置）。
          </p>
          <p v-if="configVersionsError" class="config-versions__error">{{ configVersionsError }}</p>
          <div class="config-versions">
            <div class="config-versions__list">
              <div
                v-for="version in configVersions"
                :key="version.name"
                class="config-versions__item"
                :class="{ 'config-versions__item--active': selectedConfigVersion === version.name }"
                @click="selectConfigVersion(version)"
              >
                <span class="mono">{{ formatDate(version.created_at) }}</span>
                <span class="config-versions__size">{{ formatBytes(version.size) }}</span>
              </div>
              <p v-if="configVersions.length === 0" class="config-versions__empty">暂无历史版本</p>
            </div>
            <div class="config-versions__body">
              <ConfigDiffView
                v-if="selectedConfigVersion"
                :from-text="configCurrentContent"
                :to-text="selectedConfigVersionContent"
                from-label="当前主配置"
                :to-label="`版本 ${formatDate(configVersionCreatedAt(selectedConfigVersion))}`"
              />
              <NginxCodeEditor v-else :model-value="configCurrentContent" readonly min-height="200px" />
              <div v-if="selectedConfigVersion" class="config-versions__actions">
                <span class="config-versions__hint">回滚会先校验该版本，通过后才写回并重载 Nginx。</span>
                <n-popconfirm @positive-click="rollbackConfigVersion">
                  <template #trigger>
                    <n-button size="small" type="warning" :loading="configVersionBusy">
                      回滚到该版本
                    </n-button>
                  </template>
                  确定把主配置回滚到该版本？
                </n-popconfirm>
              </div>
            </div>
          </div>
        </details>
        <div class="log-panel-head global-nginx-head">
          <div class="nginx-mode-toggle">
            <n-button
              size="tiny"
              quaternary
              :type="globalNginxEditMode === 'auto' ? 'primary' : 'default'"
              @click="setGlobalNginxEditMode('auto')"
            >
              自动生成
            </n-button>
            <n-button
              size="tiny"
              quaternary
              :type="globalNginxEditMode === 'custom' ? 'primary' : 'default'"
              @click="setGlobalNginxEditMode('custom')"
            >
              手动编辑
            </n-button>
          </div>
          <div class="log-panel-actions">
            <n-button
              v-if="globalNginxEditMode === 'custom'"
              size="tiny"
              quaternary
              type="primary"
              :loading="globalNginxSaving"
              :disabled="!globalNginxDirty"
              @click="saveGlobalNginx"
            >
              保存
            </n-button>
            <n-button
              v-if="globalNginxEditMode === 'custom' && globalNginxBackups.length > 0"
              size="tiny"
              quaternary
              :loading="globalNginxSaving"
              @click="rollbackGlobalNginx()"
            >
              回滚
            </n-button>
            <n-button
              v-if="globalNginxEditMode === 'custom' && globalNginxServerMode === 'custom'"
              size="tiny"
              quaternary
              :loading="globalNginxSaving"
              @click="resetGlobalNginxAuto"
            >
              恢复自动生成
            </n-button>
          </div>
        </div>
        <NginxCodeEditor
          v-model="globalNginxDraft"
          class="nginx-editor global-nginx-editor"
          :readonly="globalNginxEditMode === 'auto'"
          min-height="280px"
          placeholder="# 在此添加 http 块内的自定义指令&#10;# 例如：gzip on;"
          @update:model-value="onGlobalNginxDraftInput"
        />
        <p class="global-nginx-upload-hint field-hint">
          上传大小默认 50M。如需针对单条规则单独设置，请到对应反代 → 编辑 → Nginx 中修改
          <code>client_max_body_size</code>。
        </p>
        <n-alert type="warning" :bordered="false" class="global-nginx-warn">
          小白勿碰。若保存后 Nginx 启动失败，请点「恢复自动生成」或「回滚」还原配置。
        </n-alert>
      </HavlineCard>
      <HavlineCard title="API Token" subtitle="给脚本、HomeAssistant、手机快捷指令调用">
        <p class="field-hint">
          浏览器之外的调用方拿不到会话 cookie，可在这里签发令牌，用
          <code>Authorization: Bearer &lt;token&gt;</code> 调用接口。明文只在创建时显示一次。
        </p>
        <n-alert v-if="createdToken" type="success" :bordered="false" class="token-created">
          <div class="token-created__head">
            <strong>令牌已创建，请立即复制</strong>
            <span>关闭后无法再次查看明文，只能撤销重建。</span>
          </div>
          <div class="token-created__row">
            <code class="token-created__value">{{ createdToken }}</code>
            <n-button size="small" @click="copyCreatedToken">复制</n-button>
          </div>
        </n-alert>

        <div class="settings-fields settings-fields--compact token-create">
          <div class="settings-field">
            <label class="settings-field__label">名称</label>
            <n-input v-model:value="tokenForm.name" placeholder="例如 homeassistant" />
          </div>
          <div class="settings-field">
            <label class="settings-field__label">权限</label>
            <n-select v-model:value="tokenForm.scope" :options="tokenScopeOptions" />
          </div>
          <div class="settings-field">
            <label class="settings-field__label">有效期</label>
            <n-select v-model:value="tokenForm.expiresInDays" :options="tokenExpiryOptions" />
          </div>
          <div class="settings-field">
            <label class="settings-field__label">新建</label>
            <n-button type="primary" :loading="creatingToken" :disabled="!tokenForm.name.trim()" @click="createToken">
              创建令牌
            </n-button>
          </div>
        </div>

        <LoadError v-if="tokenError" :message="tokenError" />
        <div v-if="apiTokens.length > 0" class="token-list">
          <div v-for="token in apiTokens" :key="token.id" class="token-row">
            <div class="token-row__main">
              <span class="token-row__name">{{ token.name }}</span>
              <StatusBadge
                :text="token.scope === 'write' ? '读写' : '只读'"
                :kind="token.scope === 'write' ? 'warning' : 'disabled'"
              />
            </div>
            <div class="token-row__meta">
              <span>创建 {{ formatDate(token.created_at) }}</span>
              <span>{{ token.last_used_at ? `最近使用 ${formatDate(token.last_used_at)}` : '尚未使用' }}</span>
              <span>{{ token.expires_at ? `有效期至 ${formatDate(token.expires_at)}` : '永不过期' }}</span>
            </div>
            <n-popconfirm @positive-click="deleteToken(token)">
              <template #trigger>
                <n-button size="small" type="warning" quaternary>撤销</n-button>
              </template>
              撤销后该令牌立即失效，确定继续？
            </n-popconfirm>
          </div>
        </div>
        <p v-else-if="!tokenError" class="field-hint">还没有令牌。</p>
      </HavlineCard>

      <HavlineCard title="公开状态页" subtitle="给外部看的只读页面（默认关闭）">
        <p class="field-hint">
          开启后 <code>/status</code> 任何人都能访问，只展示域名与 24 小时可用率——不含内网地址、端口与证书路径。
          建议设一个访问码，它会让页面需要带码才能打开。
        </p>
        <div class="form-switch-row">
          <div class="form-switch-row__text">
            <div class="form-switch-row__label">开启状态页</div>
            <div class="form-switch-row__hint">关闭时接口一律返回 404，且匿名访问按 IP 限流</div>
          </div>
          <n-switch v-model:value="statusPageEnabled" />
        </div>
        <div class="settings-field status-page-field">
          <ConfiguredSecretField
            v-model="statusPageCode"
            label="访问码"
            :configured="statusPageConfigured"
            placeholder="留空表示不需要访问码"
          />
        </div>
        <div class="status-page-actions">
          <n-button size="small" :disabled="!statusPageEnabled" @click="openStatusPage">
            打开状态页
          </n-button>
          <span class="field-hint">访问码只存 SHA-256，忘了只能重设。</span>
        </div>
      </HavlineCard>
        </div>
      </n-tab-pane>
    </n-tabs>

    <div class="save-bar">
      <n-button type="primary" size="large" :loading="saving" @click="save">保存设置</n-button>
    </div>
  </n-spin>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  NButton,
  NIcon,
  NInput,
  NInputNumber,
  NRadio,
  NRadioGroup,
  NSelect,
  NSpin,
  NSwitch,
  NTabPane,
  NTabs,
  NUpload,
  useMessage,
  type UploadFileInfo,
} from 'naive-ui'
import {
  CheckmarkCircle,
  CloudUploadOutline,
  DesktopOutline,
  DownloadOutline,
  MoonOutline,
  SunnyOutline,
} from '@vicons/ionicons5'
import { api } from '../api/client'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'
import type {
  ApiToken,
  BackupArchive,
  ChinaCIDRStatus,
  GlobalNginxView,
  NginxConfigVersionEntry,
  NotifyEmailConfig,
  NotifyTelegramConfig,
  NotifyTestPayload,
  NotifyType,
  NotifyWebhookConfig,
  WebhookProvider,
} from '../api/types'
import ConfiguredSecretField from '../components/ConfiguredSecretField.vue'
import ConfigDiffView from '../components/ConfigDiffView.vue'
import HavlineCard from '../components/HavlineCard.vue'
import NginxCodeEditor from '../components/NginxCodeEditor.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { DEFAULT_PRIVATE_CIDRS, mergeIPList, stripDefaultIPList } from '../constants/ipPolicy'
import {
  DEFAULT_AGENT_FAIL_THRESHOLD,
  DEFAULT_CPU_THRESHOLD,
  DEFAULT_DISK_THRESHOLD,
  DEFAULT_DRIFT_COOLDOWN_HOURS,
  DEFAULT_FRPS_FAIL_THRESHOLD,
  DEFAULT_LOGIN_FAILURE_THRESHOLD,
  DEFAULT_LOGIN_FAILURE_WINDOW_SEC,
  DEFAULT_MEMORY_THRESHOLD,
  DEFAULT_TUNNEL_FAIL_THRESHOLD,
  NOTIFY_EVENT_GROUPS,
} from '../constants/notifyEvents'
import {
  DEFAULT_BARK_SERVER,
  DEFAULT_DINGTALK_SERVER,
  DEFAULT_FEISHU_SERVER,
  DEFAULT_NTFY_SERVER,
  DEFAULT_WECOM_SERVER,
  MASKED_SECRET,
  NOTIFY_TYPE_OPTIONS,
  WEBHOOK_PROVIDER_OPTIONS,
} from '../constants/notifyProviders'
import { useTheme } from '../composables/useTheme'
import { copyText } from '../utils/clipboard'
import { formatBytes, formatDate, formatRelativeTime } from '../utils/format'
import type { StatusKind } from '../utils/status'

const message = useMessage()

// 与后端 backup.DefaultKeepCount 一致：设置项没填时两边都用 7
const DEFAULT_BACKUP_KEEP_COUNT = 7
const { setThemeMode } = useTheme()
const saving = ref(false)
const loading = ref(false)
const pageError = ref('')
const changingPassword = ref(false)
const exporting = ref(false)
const restoring = ref(false)

// 备份：开关与保留份数走设置项（随「保存设置」提交），备份列表单独读
const backupAutoEnabled = ref(true)
const backupKeepCount = ref(DEFAULT_BACKUP_KEEP_COUNT)
const backupArchives = ref<BackupArchive[]>([])
const backupBusy = ref<string | null>(null)
const backupError = ref('')

// 公开状态页：开关与访问码走设置项（访问码按「掩码往返」处理）
const statusPageEnabled = ref(false)
const statusPageCode = ref('')
const statusPageConfigured = ref(false)

const form = reactive({
  theme: 'system',
  timezone: 'Asia/Shanghai',
})

const passwordForm = reactive({ old_password: '', new_password: '', confirm_password: '' })
const settingsTab = ref('general')
const notifyType = ref<NotifyType>('')
const notifyEventFlags = reactive<Record<string, boolean>>({
  on_ddns_ip_change: true,
  on_ddns_failure: true,
  on_cert_expiry: true,
  on_cert_renew_success: true,
  on_cert_renew_failure: true,
  on_ip_frequent_access: false,
  on_login_failure: false,
  on_nginx_reload_failure: true,
  on_resource_threshold: false,
  on_tunnel_down: false,
  on_frps_down: false,
  on_agent_unreachable: false,
  on_route_drift: false,
  on_backup_failure: true,
})
const notifyIPFrequentThreshold = ref(100)
const notifyIPFrequentWindowSec = ref(60)
const notifyLoginFailureThreshold = ref(DEFAULT_LOGIN_FAILURE_THRESHOLD)
const notifyLoginFailureWindowSec = ref(DEFAULT_LOGIN_FAILURE_WINDOW_SEC)
const notifyCPUThreshold = ref(DEFAULT_CPU_THRESHOLD)
const notifyMemoryThreshold = ref(DEFAULT_MEMORY_THRESHOLD)
const notifyDiskThreshold = ref(DEFAULT_DISK_THRESHOLD)
const notifyTunnelFailThreshold = ref(DEFAULT_TUNNEL_FAIL_THRESHOLD)
const notifyFrpsFailThreshold = ref(DEFAULT_FRPS_FAIL_THRESHOLD)
const notifyAgentFailThreshold = ref(DEFAULT_AGENT_FAIL_THRESHOLD)
const notifyDriftCooldownHours = ref(DEFAULT_DRIFT_COOLDOWN_HOURS)

// API Token：明文只在创建响应里回一次，因此单独用一个 ref 暂存展示
const apiTokens = ref<ApiToken[]>([])
const tokenError = ref('')
const creatingToken = ref(false)
const createdToken = ref('')
const tokenForm = reactive({ name: '', scope: 'read' as 'read' | 'write', expiresInDays: 0 })
const tokenScopeOptions = [
  { label: '只读（仅 GET）', value: 'read' },
  { label: '读写（与会话同权）', value: 'write' },
]
const tokenExpiryOptions = [
  { label: '永不过期', value: 0 },
  { label: '30 天', value: 30 },
  { label: '90 天', value: 90 },
  { label: '365 天', value: 365 },
]

async function loadApiTokens() {
  try {
    const result = await api.listApiTokens()
    apiTokens.value = result.tokens ?? []
    tokenError.value = ''
  } catch (error) {
    tokenError.value = `读取令牌失败 —— ${error instanceof Error ? error.message : '未知错误'}`
  }
}

async function createToken() {
  const name = tokenForm.name.trim()
  if (!name || creatingToken.value) return
  creatingToken.value = true
  try {
    const result = await api.createApiToken({
      name,
      scope: tokenForm.scope,
      expires_in_days: tokenForm.expiresInDays,
    })
    createdToken.value = result.token
    tokenForm.name = ''
    await loadApiTokens()
    message.success('令牌已创建，请复制明文')
  } catch (error) {
    message.error(`创建令牌失败：${error instanceof Error ? error.message : '未知错误'}`)
  } finally { creatingToken.value = false }
}

async function copyCreatedToken() {
  if (await copyText(createdToken.value)) message.success('已复制令牌')
  else message.error('复制失败，请手动选择文本复制')
}

async function deleteToken(token: ApiToken) {
  try {
    await api.deleteApiToken(token.id)
    if (createdToken.value) createdToken.value = ''
    await loadApiTokens()
    message.success('令牌已撤销')
  } catch (error) {
    message.error(`撤销失败：${error instanceof Error ? error.message : '未知错误'}`)
  }
}
const notifyTesting = ref(false)
const notifySMTPPassword = ref('')
const notifyWebhookKey = ref('')
const notifyTelegramToken = ref('')
const notifyEmailToText = ref('')
const notifyEmail = reactive({
  host: '',
  port: 587,
  username: '',
  from: '',
  tls: true,
  has_password: false,
})
const notifyWebhook = reactive({
  provider: 'bark' as WebhookProvider,
  server: DEFAULT_BARK_SERVER,
  topic: '',
  url: '',
  has_secret: false,
})

function defaultWebhookServer(provider: WebhookProvider): string {
  switch (provider) {
    case 'ntfy': return DEFAULT_NTFY_SERVER
    case 'wecom': return DEFAULT_WECOM_SERVER
    case 'dingtalk': return DEFAULT_DINGTALK_SERVER
    case 'feishu': return DEFAULT_FEISHU_SERVER
    case 'gotify': return ''
    default: return DEFAULT_BARK_SERVER
  }
}

// 切换预设时把服务地址换成该预设的默认值：否则会把上一个预设的地址（如 api.day.app）带到新预设上
function onWebhookProviderChange(provider: WebhookProvider) {
  notifyWebhook.server = defaultWebhookServer(provider)
}
const notifyTelegram = reactive({
  chat_id: '',
  proxy_url: '',
  has_bot_token: false,
})
const ddnsInterval = ref(5)
const certThreshold = ref(30)
const logRetention = ref(30)
const acmeEmail = ref('')
const acmeResolvers = ref('')
// LiteSSL 的 EAB 凭据：kid 不是秘密（可回显），hmac 是秘密（只回掩码、重输才提交）
const litesslEabKid = ref('')
const litesslEabHmac = ref('')
const litesslEabHmacConfigured = ref(false)
const zerosslApiKey = ref('')
const zerosslHasKey = ref(false)
const trustedProxyEnabled = ref(false)
const trustedProxyPreset = ref('cloudflare')
const trustedProxyCIDRs = ref('')
const trustedProxyHeader = ref('X-Forwarded-For')
const globalIPBlacklistText = ref('')
const globalIPWhitelistText = ref('')
const chinaCIDRHours = ref(24)
const refreshingCIDR = ref(false)
const chinaCIDR = ref<ChinaCIDRStatus>({ entry_count_v4: 0, entry_count_v6: 0 })
const chinaCIDRPolling = ref(false)

const globalNginxFramework = ref('')
const globalNginxGeneratedSnippet = ref('')
const globalNginxDraft = ref('')
const globalNginxEditMode = ref<'auto' | 'custom'>('auto')
const globalNginxServerMode = ref<'auto' | 'custom'>('auto')
const globalNginxDirty = ref(false)
const globalNginxSaving = ref(false)
const globalNginxBackups = ref<{ name: string; created_at: string }[]>([])
const configVersions = ref<NginxConfigVersionEntry[]>([])
const configCurrentContent = ref('')
const selectedConfigVersion = ref('')
const selectedConfigVersionContent = ref('')
const configVersionsError = ref('')
const configVersionBusy = ref(false)

const globalNginxCustomHint = `# 在此添加 http 块内的自定义指令
# 上传大小限制（全局默认 50M，单条规则可在 server 块覆盖）
client_max_body_size 50m;
# 例如：gzip on;
`

const chinaCIDRBadgeKind = computed((): StatusKind => {
  if (refreshingCIDR.value || chinaCIDR.value.updating) return 'warning'
  if (chinaCIDR.value.ready) return 'success'
  if (chinaCIDR.value.last_error) return 'error'
  return 'disabled'
})

const chinaCIDRBadgeText = computed(() => {
  if (refreshingCIDR.value || chinaCIDR.value.updating) return '更新中'
  if (chinaCIDR.value.ready) return '已就绪'
  if (chinaCIDR.value.last_error) return '更新失败'
  return '未下载'
})

const trustedProxyPresets = [
  { label: 'Cloudflare', value: 'cloudflare' },
  { label: '自定义', value: 'custom' },
]
const trustedProxyHeaders = [
  { label: 'X-Forwarded-For', value: 'X-Forwarded-For' },
  { label: 'X-Real-IP', value: 'X-Real-IP' },
  { label: 'CF-Connecting-IP', value: 'CF-Connecting-IP' },
]

const themeOptions = [
  { value: 'system', label: '跟随系统', icon: DesktopOutline },
  { value: 'light', label: '浅色', icon: SunnyOutline },
  { value: 'dark', label: '深色', icon: MoonOutline },
]

const timezoneOptions = [
  { label: 'Asia/Shanghai (UTC+8)', value: 'Asia/Shanghai' },
  { label: 'Asia/Tokyo (UTC+9)', value: 'Asia/Tokyo' },
  { label: 'UTC', value: 'UTC' },
  { label: 'America/New_York (UTC-5/-4)', value: 'America/New_York' },
  { label: 'Europe/London (UTC+0/+1)', value: 'Europe/London' },
]

function selectTheme(mode: string) {
  form.theme = mode
  setThemeMode(mode)
}

function applySettingsToForm(settings: Record<string, string>) {
  Object.assign(form, {
    theme: settings.theme ?? 'system',
    timezone: settings.timezone ?? 'Asia/Shanghai',
  })
  applyNotifySettings(settings)
  ddnsInterval.value = Number(settings.ddns_check_interval_minutes ?? 5)
  certThreshold.value = Number(settings.cert_renew_threshold_days ?? 30)
  logRetention.value = Number(settings.log_retention_days ?? 30)
  // 未配置时按默认值处理：自动备份是默认开的
  backupAutoEnabled.value = settings.backup_auto_enabled !== '0'
  backupKeepCount.value = Number(settings.backup_keep_count || DEFAULT_BACKUP_KEEP_COUNT)
  statusPageEnabled.value = settings.status_page_enabled === '1'
  statusPageCode.value = settings.status_page_code ?? ''
  statusPageConfigured.value = statusPageCode.value === MASKED_SECRET
  acmeEmail.value = settings.acme_email ?? ''
  acmeResolvers.value = settings.acme_dns_resolvers ?? ''
  litesslEabKid.value = settings.litessl_eab_kid ?? ''
  litesslEabHmacConfigured.value = Boolean(settings.litessl_eab_hmac?.trim())
  litesslEabHmac.value = ''
  zerosslHasKey.value = Boolean(settings.zerossl_api_key?.trim())
  zerosslApiKey.value = ''
  chinaCIDRHours.value = Number(settings.china_cidr_update_interval_hours ?? 24)
  try {
    const tp = JSON.parse(settings.trusted_proxy_json || '{}') as {
      enabled?: boolean
      preset?: string
      cidrs?: string[]
      header?: string
    }
    trustedProxyEnabled.value = tp.enabled ?? false
    trustedProxyPreset.value = tp.preset || 'cloudflare'
    trustedProxyCIDRs.value = (tp.cidrs ?? []).join('\n')
    trustedProxyHeader.value = tp.header || 'X-Forwarded-For'
  } catch {
    trustedProxyEnabled.value = false
  }
  try {
    const bl = JSON.parse(settings.global_ip_blacklist || '[]') as string[]
    globalIPBlacklistText.value = bl.join('\n')
  } catch {
    globalIPBlacklistText.value = ''
  }
  try {
    const wl = JSON.parse(settings.global_ip_whitelist || '[]') as string[]
    globalIPWhitelistText.value = mergeIPList(DEFAULT_PRIVATE_CIDRS, wl).join('\n')
  } catch {
    globalIPWhitelistText.value = DEFAULT_PRIVATE_CIDRS.join('\n')
  }
}

function buildTrustedProxyJSON() {
  const cidrs = trustedProxyCIDRs.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  return JSON.stringify({
    enabled: trustedProxyEnabled.value,
    preset: trustedProxyPreset.value,
    cidrs,
    header: trustedProxyHeader.value,
    recursive: trustedProxyHeader.value === 'X-Forwarded-For',
  })
}

function buildGlobalIPListJSON(text: string) {
  const list = text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  return JSON.stringify(list)
}

function buildGlobalWhitelistJSON() {
  const list = globalIPWhitelistText.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  return JSON.stringify(stripDefaultIPList(DEFAULT_PRIVATE_CIDRS, list))
}

function applyGlobalNginxView(view: GlobalNginxView) {
  globalNginxFramework.value = view.generated_framework
  globalNginxGeneratedSnippet.value = view.generated_snippet || globalNginxCustomHint
  globalNginxServerMode.value = view.mode
  globalNginxEditMode.value = view.mode
  globalNginxBackups.value = view.backups ?? []
  globalNginxDraft.value =
    view.mode === 'auto' ? globalNginxGeneratedSnippet.value : view.content || globalNginxCustomHint
  globalNginxDirty.value = false
}

async function loadGlobalNginx() {
  try {
    applyGlobalNginxView(await api.getGlobalNginx())
  } catch {
    // optional on load
  }
  await loadConfigVersions()
}

// 主配置（nginx.conf）的版本历史：与全局片段是两份文件，分开读、分开回滚
async function loadConfigVersions() {
  try {
    const view = await api.getNginxConfigVersions()
    configCurrentContent.value = view.content
    configVersions.value = view.versions ?? []
    configVersionsError.value = ''
  } catch (error) {
    configVersionsError.value = error instanceof Error ? error.message : '读取主配置版本失败'
  }
}

// 点已选中的版本 = 取消对比
async function selectConfigVersion(version: NginxConfigVersionEntry) {
  if (configVersionBusy.value) return
  if (selectedConfigVersion.value === version.name) {
    selectedConfigVersion.value = ''
    selectedConfigVersionContent.value = ''
    return
  }
  configVersionBusy.value = true
  try {
    const result = await api.getNginxConfigVersion(version.name)
    selectedConfigVersion.value = version.name
    selectedConfigVersionContent.value = result.content
  } catch (error) {
    message.error(`读取该版本失败：${error instanceof Error ? error.message : '未知错误'}`)
  } finally { configVersionBusy.value = false }
}

async function rollbackConfigVersion() {
  if (!selectedConfigVersion.value) return
  configVersionBusy.value = true
  try {
    const view = await api.rollbackNginxConfigVersion(selectedConfigVersion.value)
    configCurrentContent.value = view.content
    configVersions.value = view.versions ?? []
    selectedConfigVersion.value = ''
    selectedConfigVersionContent.value = ''
    message.success(view.message || '已回滚到所选版本')
  } catch (error) {
    message.error(`回滚失败：${error instanceof Error ? error.message : '未知错误'}`)
  } finally { configVersionBusy.value = false }
}

function configVersionCreatedAt(name: string): string {
  return configVersions.value.find((item) => item.name === name)?.created_at ?? ''
}

function setGlobalNginxEditMode(mode: 'auto' | 'custom') {
  if (mode === globalNginxEditMode.value) return
  if (mode === 'custom') {
    if (!globalNginxDraft.value.trim() || globalNginxDraft.value === globalNginxGeneratedSnippet.value) {
      globalNginxDraft.value = globalNginxCustomHint
      globalNginxDirty.value = false
    } else {
      globalNginxDirty.value = globalNginxServerMode.value !== 'custom'
    }
  } else {
    globalNginxDraft.value = globalNginxGeneratedSnippet.value
    globalNginxDirty.value = false
  }
  globalNginxEditMode.value = mode
}

function onGlobalNginxDraftInput() {
  if (globalNginxEditMode.value === 'custom') {
    globalNginxDirty.value = true
  }
}

async function saveGlobalNginx() {
  globalNginxSaving.value = true
  try {
    applyGlobalNginxView(await api.saveGlobalNginx({
      mode: 'custom',
      content: globalNginxDraft.value,
    }))
    message.success('全局 Nginx 配置已保存')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    globalNginxSaving.value = false
  }
}

async function rollbackGlobalNginx(backup?: string) {
  globalNginxSaving.value = true
  try {
    applyGlobalNginxView(await api.rollbackGlobalNginx(backup))
    message.success('已回滚到上一版本')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '回滚失败')
  } finally {
    globalNginxSaving.value = false
  }
}

async function resetGlobalNginxAuto() {
  globalNginxSaving.value = true
  try {
    applyGlobalNginxView(await api.saveGlobalNginx({ mode: 'auto' }))
    message.success('已恢复自动生成')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '操作失败')
  } finally {
    globalNginxSaving.value = false
  }
}

async function loadChinaCIDRStatus() {
  try {
    chinaCIDR.value = await api.getChinaCIDRStatus()
    chinaCIDRPolling.value = Boolean(chinaCIDR.value.updating)
  } catch {
    // optional on load
  }
}

async function refreshChinaCIDR() {
  refreshingCIDR.value = true
  chinaCIDRPolling.value = false
  try {
    chinaCIDR.value = await api.refreshChinaCIDR()
    if (chinaCIDR.value.ready) {
      message.success(
        `中国 IP 段已更新：IPv4 ${chinaCIDR.value.entry_count_v4} 条，IPv6 ${chinaCIDR.value.entry_count_v6} 条`,
      )
    } else {
      message.warning('更新完成，但数据尚未就绪，请查看下方错误信息')
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '更新失败')
    await loadChinaCIDRStatus()
  } finally {
    refreshingCIDR.value = false
  }
}

function applyNotifySettings(settings: Record<string, string>) {
  notifyType.value = (settings.notify_type as NotifyType) || ''
  notifySMTPPassword.value = ''
  notifyWebhookKey.value = ''
  notifyTelegramToken.value = ''

  try {
    const email = JSON.parse(settings.notify_email_json || '{}') as NotifyEmailConfig
    notifyEmail.host = email.host ?? ''
    notifyEmail.port = email.port ?? 587
    notifyEmail.username = email.username ?? ''
    notifyEmail.from = email.from ?? ''
    notifyEmail.tls = email.tls ?? true
    notifyEmail.has_password = email.has_password ?? false
    notifyEmailToText.value = (email.to ?? []).join('\n')
  } catch {
    notifyEmailToText.value = ''
  }

  try {
    const webhook = JSON.parse(settings.notify_webhook_json || '{}') as NotifyWebhookConfig
    notifyWebhook.provider = webhook.provider ?? 'bark'
    notifyWebhook.server = webhook.server ?? defaultWebhookServer(webhook.provider)
    notifyWebhook.topic = webhook.topic ?? ''
    notifyWebhook.url = webhook.url ?? ''
    notifyWebhook.has_secret = webhook.has_secret ?? false
  } catch {
    notifyWebhook.provider = 'bark'
    notifyWebhook.server = DEFAULT_BARK_SERVER
  }

  try {
    const telegram = JSON.parse(settings.notify_telegram_json || '{}') as NotifyTelegramConfig
    notifyTelegram.chat_id = telegram.chat_id ?? ''
    notifyTelegram.proxy_url = telegram.proxy_url ?? ''
    notifyTelegram.has_bot_token = telegram.has_bot_token ?? false
  } catch {
    notifyTelegram.chat_id = ''
    notifyTelegram.proxy_url = ''
  }

  if (!notifyType.value && settings.notify_webhook_url) {
    notifyType.value = 'webhook'
    notifyWebhook.provider = 'custom'
    notifyWebhook.url = settings.notify_webhook_url
  }

  const flagOn = (key: string) => settings[key] !== '0' && settings[key] !== 'false'
  const flagOptIn = (key: string) => settings[key] === '1' || settings[key] === 'true'

  const hasNewNotifyEvents =
    settings.notify_on_ddns_ip_change != null ||
    settings.notify_on_ddns_failure != null ||
    settings.notify_on_cert_expiry != null ||
    settings.notify_on_cert_renew_success != null ||
    settings.notify_on_ip_frequent_access != null

  if (hasNewNotifyEvents) {
    notifyEventFlags.on_ddns_ip_change = flagOn('notify_on_ddns_ip_change')
    notifyEventFlags.on_ddns_failure = flagOn('notify_on_ddns_failure')
    notifyEventFlags.on_cert_expiry = flagOn('notify_on_cert_expiry')
    notifyEventFlags.on_cert_renew_success = flagOn('notify_on_cert_renew_success')
    notifyEventFlags.on_cert_renew_failure = flagOn('notify_on_cert_renew_failure')
    notifyEventFlags.on_ip_frequent_access = flagOptIn('notify_on_ip_frequent_access')
    notifyEventFlags.on_login_failure = flagOptIn('notify_on_login_failure')
    notifyEventFlags.on_nginx_reload_failure = flagOn('notify_on_nginx_reload_failure')
    notifyEventFlags.on_resource_threshold = flagOptIn('notify_on_resource_threshold')
    notifyEventFlags.on_tunnel_down = flagOptIn('notify_on_tunnel_down')
    notifyEventFlags.on_frps_down = flagOptIn('notify_on_frps_down')
    notifyEventFlags.on_agent_unreachable = flagOptIn('notify_on_agent_unreachable')
    notifyEventFlags.on_route_drift = flagOptIn('notify_on_route_drift')
    notifyEventFlags.on_backup_failure = flagOn('notify_on_backup_failure')
  } else {
    const legacyDDNS = flagOn('notify_on_ddns_error')
    const legacyCert = flagOn('notify_on_cert_error')
    notifyEventFlags.on_ddns_failure = legacyDDNS
    notifyEventFlags.on_cert_expiry = legacyCert
    notifyEventFlags.on_cert_renew_failure = legacyCert
    notifyEventFlags.on_ddns_ip_change = true
    notifyEventFlags.on_cert_renew_success = true
    notifyEventFlags.on_nginx_reload_failure = true
    notifyEventFlags.on_ip_frequent_access = false
    notifyEventFlags.on_login_failure = false
    notifyEventFlags.on_resource_threshold = false
    notifyEventFlags.on_tunnel_down = false
    notifyEventFlags.on_frps_down = false
    notifyEventFlags.on_agent_unreachable = false
    notifyEventFlags.on_route_drift = false
    notifyEventFlags.on_backup_failure = true
  }
  notifyIPFrequentThreshold.value = Number(settings.notify_ip_frequent_threshold || 100)
  notifyIPFrequentWindowSec.value = Number(settings.notify_ip_frequent_window_sec || 60)
  notifyLoginFailureThreshold.value = Number(settings.notify_login_failure_threshold || DEFAULT_LOGIN_FAILURE_THRESHOLD)
  notifyLoginFailureWindowSec.value = Number(settings.notify_login_failure_window_sec || DEFAULT_LOGIN_FAILURE_WINDOW_SEC)
  notifyCPUThreshold.value = Number(settings.notify_cpu_threshold || DEFAULT_CPU_THRESHOLD)
  notifyMemoryThreshold.value = Number(settings.notify_memory_threshold || DEFAULT_MEMORY_THRESHOLD)
  notifyDiskThreshold.value = Number(settings.notify_disk_threshold || DEFAULT_DISK_THRESHOLD)
  notifyTunnelFailThreshold.value = Number(settings.notify_tunnel_fail_threshold || DEFAULT_TUNNEL_FAIL_THRESHOLD)
  notifyFrpsFailThreshold.value = Number(settings.notify_frps_fail_threshold || DEFAULT_FRPS_FAIL_THRESHOLD)
  notifyAgentFailThreshold.value = Number(settings.notify_agent_fail_threshold || DEFAULT_AGENT_FAIL_THRESHOLD)
  notifyDriftCooldownHours.value = Number(settings.notify_drift_cooldown_hours || DEFAULT_DRIFT_COOLDOWN_HOURS)
}

function buildNotifyEmailJSON(): string {
  const payload: NotifyEmailConfig = {
    host: notifyEmail.host.trim(),
    port: notifyEmail.port || 587,
    username: notifyEmail.username.trim(),
    from: notifyEmail.from.trim(),
    to: notifyEmailToText.value
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean),
    tls: notifyEmail.tls,
    has_password: notifyEmail.has_password || Boolean(notifySMTPPassword.value.trim()),
  }
  return JSON.stringify(payload)
}

function buildNotifyWebhookJSON(): string {
  const payload: NotifyWebhookConfig = {
    provider: notifyWebhook.provider,
    server: notifyWebhook.server.trim(),
    key: '',
    topic: notifyWebhook.topic.trim(),
    url: notifyWebhook.url.trim(),
    has_secret: notifyWebhook.has_secret || Boolean(notifyWebhookKey.value.trim()),
  }
  return JSON.stringify(payload)
}

function buildNotifyTelegramJSON(): string {
  const payload: NotifyTelegramConfig = {
    chat_id: notifyTelegram.chat_id.trim(),
    proxy_url: notifyTelegram.proxy_url.trim(),
    has_bot_token: notifyTelegram.has_bot_token || Boolean(notifyTelegramToken.value.trim()),
  }
  return JSON.stringify(payload)
}

function resolveSecretInput(value: string, hasExisting: boolean) {
  const trimmed = value.trim()
  if (trimmed) return trimmed
  if (hasExisting) return MASKED_SECRET
  return ''
}

function buildNotifyTestPayload(): NotifyTestPayload {
  return {
    type: notifyType.value,
    email: JSON.parse(buildNotifyEmailJSON()) as NotifyEmailConfig,
    webhook: JSON.parse(buildNotifyWebhookJSON()) as NotifyWebhookConfig,
    telegram: JSON.parse(buildNotifyTelegramJSON()) as NotifyTelegramConfig,
    on_ddns_ip_change: notifyEventFlags.on_ddns_ip_change,
    on_ddns_failure: notifyEventFlags.on_ddns_failure,
    on_cert_expiry: notifyEventFlags.on_cert_expiry,
    on_cert_renew_success: notifyEventFlags.on_cert_renew_success,
    on_cert_renew_failure: notifyEventFlags.on_cert_renew_failure,
    on_ip_frequent_access: notifyEventFlags.on_ip_frequent_access,
    on_login_failure: notifyEventFlags.on_login_failure,
    on_nginx_reload_failure: notifyEventFlags.on_nginx_reload_failure,
    ip_frequent_threshold: notifyIPFrequentThreshold.value,
    ip_frequent_window_sec: notifyIPFrequentWindowSec.value,
    login_failure_threshold: notifyLoginFailureThreshold.value,
    login_failure_window_sec: notifyLoginFailureWindowSec.value,
    smtp_password: resolveSecretInput(notifySMTPPassword.value, notifyEmail.has_password),
    webhook_secret: resolveSecretInput(notifyWebhookKey.value, notifyWebhook.has_secret),
    telegram_token: resolveSecretInput(notifyTelegramToken.value, notifyTelegram.has_bot_token),
  }
}

async function testNotify() {
  if (!notifyType.value) {
    message.warning('请先选择通知方式')
    return
  }
  notifyTesting.value = true
  try {
    const result = await api.testNotify(buildNotifyTestPayload())
    message.success(result.message || '测试通知已发送')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '测试通知失败')
  } finally {
    notifyTesting.value = false
  }
}

function buildSavePayload() {
  const payload: Record<string, string> = {
    theme: form.theme,
    timezone: form.timezone,
    notify_type: notifyType.value,
    notify_email_json: buildNotifyEmailJSON(),
    notify_smtp_password: resolveSecretInput(notifySMTPPassword.value, notifyEmail.has_password),
    notify_webhook_json: buildNotifyWebhookJSON(),
    notify_webhook_secret: resolveSecretInput(notifyWebhookKey.value, notifyWebhook.has_secret),
    notify_telegram_json: buildNotifyTelegramJSON(),
    notify_telegram_token: resolveSecretInput(notifyTelegramToken.value, notifyTelegram.has_bot_token),
    notify_on_ddns_ip_change: notifyEventFlags.on_ddns_ip_change ? '1' : '0',
    notify_on_ddns_failure: notifyEventFlags.on_ddns_failure ? '1' : '0',
    notify_on_cert_expiry: notifyEventFlags.on_cert_expiry ? '1' : '0',
    notify_on_cert_renew_success: notifyEventFlags.on_cert_renew_success ? '1' : '0',
    notify_on_cert_renew_failure: notifyEventFlags.on_cert_renew_failure ? '1' : '0',
    notify_on_ip_frequent_access: notifyEventFlags.on_ip_frequent_access ? '1' : '0',
    notify_on_login_failure: notifyEventFlags.on_login_failure ? '1' : '0',
    notify_on_nginx_reload_failure: notifyEventFlags.on_nginx_reload_failure ? '1' : '0',
    notify_on_resource_threshold: notifyEventFlags.on_resource_threshold ? '1' : '0',
    notify_on_tunnel_down: notifyEventFlags.on_tunnel_down ? '1' : '0',
    notify_on_frps_down: notifyEventFlags.on_frps_down ? '1' : '0',
    notify_on_agent_unreachable: notifyEventFlags.on_agent_unreachable ? '1' : '0',
    notify_on_route_drift: notifyEventFlags.on_route_drift ? '1' : '0',
    notify_on_backup_failure: notifyEventFlags.on_backup_failure ? '1' : '0',
    notify_frps_fail_threshold: String(notifyFrpsFailThreshold.value || DEFAULT_FRPS_FAIL_THRESHOLD),
    notify_agent_fail_threshold: String(notifyAgentFailThreshold.value || DEFAULT_AGENT_FAIL_THRESHOLD),
    notify_drift_cooldown_hours: String(notifyDriftCooldownHours.value || DEFAULT_DRIFT_COOLDOWN_HOURS),
    backup_auto_enabled: backupAutoEnabled.value ? '1' : '0',
    backup_keep_count: String(backupKeepCount.value || DEFAULT_BACKUP_KEEP_COUNT),
    status_page_enabled: statusPageEnabled.value ? '1' : '0',
    status_page_code: statusPageCode.value.trim(),
    notify_cpu_threshold: String(notifyCPUThreshold.value || DEFAULT_CPU_THRESHOLD),
    notify_memory_threshold: String(notifyMemoryThreshold.value || DEFAULT_MEMORY_THRESHOLD),
    notify_disk_threshold: String(notifyDiskThreshold.value || DEFAULT_DISK_THRESHOLD),
    notify_tunnel_fail_threshold: String(notifyTunnelFailThreshold.value || DEFAULT_TUNNEL_FAIL_THRESHOLD),
    notify_ip_frequent_threshold: String(notifyIPFrequentThreshold.value || 100),
    notify_ip_frequent_window_sec: String(notifyIPFrequentWindowSec.value || 60),
    notify_login_failure_threshold: String(notifyLoginFailureThreshold.value || DEFAULT_LOGIN_FAILURE_THRESHOLD),
    notify_login_failure_window_sec: String(notifyLoginFailureWindowSec.value || DEFAULT_LOGIN_FAILURE_WINDOW_SEC),
    ddns_check_interval_minutes: String(ddnsInterval.value),
    cert_renew_threshold_days: String(certThreshold.value),
    log_retention_days: String(logRetention.value),
    acme_email: acmeEmail.value.trim(),
    acme_dns_resolvers: acmeResolvers.value.trim(),
    litessl_eab_kid: litesslEabKid.value.trim(),
    trusted_proxy_json: buildTrustedProxyJSON(),
    global_ip_blacklist: buildGlobalIPListJSON(globalIPBlacklistText.value),
    global_ip_whitelist: buildGlobalWhitelistJSON(),
    china_cidr_update_interval_hours: String(chinaCIDRHours.value),
  }
  if (notifyType.value === 'webhook' && notifyWebhook.provider === 'custom') {
    payload.notify_webhook_url = notifyWebhook.url.trim()
  }
  const zerosslKey = zerosslApiKey.value.trim()
  if (zerosslKey) {
    payload.zerossl_api_key = zerosslKey
  }
  const litesslHmac = litesslEabHmac.value.trim()
  if (litesslHmac) {
    payload.litessl_eab_hmac = litesslHmac
  }
  return payload
}

async function load() {
  loading.value = true
  pageError.value = ''
  try {
    applySettingsToForm(await api.getSettings())
    await Promise.all([loadChinaCIDRStatus(), loadGlobalNginx()])
  } catch (error) {
    pageError.value = error instanceof Error ? error.message : '请检查 Havline 服务是否正常运行'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const saved = await api.saveSettings(buildSavePayload())
    applySettingsToForm(saved)
    setThemeMode(saved.theme ?? form.theme)
    message.success('设置已保存')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function changePassword() {
  if (!passwordForm.old_password || !passwordForm.new_password) {
    message.warning('请填写当前密码和新密码')
    return
  }
  if (passwordForm.new_password !== passwordForm.confirm_password) {
    message.warning('两次输入的新密码不一致')
    return
  }
  changingPassword.value = true
  try {
    await api.changePassword(passwordForm.old_password, passwordForm.new_password)
    passwordForm.old_password = ''
    passwordForm.new_password = ''
    passwordForm.confirm_password = ''
    message.success('密码已更新')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '更新密码失败')
  } finally {
    changingPassword.value = false
  }
}

async function createBackupArchive() {
  if (backupBusy.value) return
  backupBusy.value = 'create'
  try {
    await api.createBackupArchive()
    await loadBackupArchives()
    message.success('已创建一份备份')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '备份失败')
  } finally { backupBusy.value = null }
}

async function loadBackupArchives() {
  try {
    const result = await api.listBackupArchives()
    backupArchives.value = result.archives ?? []
    backupError.value = ''
  } catch (error) {
    backupError.value = error instanceof Error ? error.message : '读取备份列表失败'
  }
}

async function downloadBackupArchive(archive: BackupArchive) {
  backupBusy.value = `download:${archive.name}`
  try {
    await api.downloadBackupArchive(archive.name)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '下载失败')
  } finally { backupBusy.value = null }
}

async function restoreBackupArchive(archive: BackupArchive) {
  backupBusy.value = `restore:${archive.name}`
  try {
    const result = await api.restoreBackupArchive(archive.name)
    message.success(result.message || '备份已恢复，建议重启 Havline')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '恢复失败')
  } finally { backupBusy.value = null }
}

async function deleteBackupArchive(archive: BackupArchive) {
  backupBusy.value = `delete:${archive.name}`
  try {
    await api.deleteBackupArchive(archive.name)
    await loadBackupArchives()
    message.success('已删除该备份')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '删除失败')
  } finally { backupBusy.value = null }
}

// 存了访问码时会先要求输入，所以这里只开页面、不带参数
function openStatusPage() {
  window.open('/status', '_blank', 'noopener')
}

async function exportBackup() {
  exporting.value = true
  try {
    await api.exportBackup()
    message.success('备份已开始下载')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '导出失败')
  } finally {
    exporting.value = false
  }
}

async function onRestoreFile({ file }: { file: UploadFileInfo }) {
  if (!file.file) return
  restoring.value = true
  try {
    const result = await api.restoreBackup(file.file)
    message.success(result.message)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '恢复失败')
  } finally {
    restoring.value = false
  }
}

onMounted(() => {
  void load()
  void loadApiTokens()
  void loadBackupArchives()
})

useVisibilityPolling(loadChinaCIDRStatus, 2000, { enabled: chinaCIDRPolling })
</script>

<style scoped>
.settings-tabs {
  margin-bottom: var(--havline-space-4);
}

.settings-tabs :deep(.n-tabs-nav) {
  margin-bottom: var(--havline-space-4);
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--havline-space-5);
  align-items: stretch;
}

.settings-grid--single {
  grid-template-columns: minmax(0, 1fr);
}

.settings-grid :deep(.havline-card) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.settings-grid :deep(.havline-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.settings-block--spaced {
  margin-top: var(--havline-space-5);
}

.settings-block__label,
.settings-field__label {
  font-size: 14px;
  font-weight: 500;
  color: var(--havline-text);
}

.settings-block__label,
.settings-field > .settings-field__label {
  margin-bottom: 8px;
}

.theme-cards {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--havline-space-3);
}

.theme-card {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 80px;
  padding: 12px 8px;
  border: 1px solid var(--havline-border);
  border-radius: 10px;
  background: var(--havline-bg);
  cursor: pointer;
  transition: border-color 0.15s;
}

.theme-card:hover {
  border-color: color-mix(in srgb, #10b981 40%, var(--havline-border));
}

.theme-card--active {
  border-color: #10b981;
  background: color-mix(in srgb, #10b981 6%, var(--havline-bg));
}

.theme-card__icon {
  font-size: 20px;
  color: var(--havline-text-secondary);
}

.theme-card--active .theme-card__icon {
  color: #10b981;
}

.theme-card__text {
  font-size: 13px;
  font-weight: 500;
}

.theme-card__check {
  position: absolute;
  top: 8px;
  right: 8px;
  font-size: 16px;
  color: #10b981;
}

.field-hint--error {
  color: var(--n-error-color, #e11d48);
}

.field-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--havline-text-muted);
  line-height: 1.5;
}

.nginx-framework {
  margin: 12px 0;
}

.nginx-framework__editor {
  margin-top: 8px;
}

/* 主配置版本：左侧版本列表 + 右侧当前内容 / 差异 */
.config-versions { display: grid; grid-template-columns: 208px minmax(0, 1fr); gap: var(--havline-space-4); margin-top: 8px; }
.config-versions__list { border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); overflow: auto; max-height: 320px; }
.config-versions__item {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 6px var(--havline-space-3); border-bottom: 1px solid var(--havline-border);
  font-size: 12px; color: var(--havline-text-secondary); cursor: pointer;
}
.config-versions__item:hover { background: var(--havline-bg-muted); }
.config-versions__item--active { background: var(--havline-info-soft); color: var(--havline-text); }
.config-versions__size { flex: none; color: var(--havline-text-muted); }
.config-versions__empty { padding: var(--havline-space-3); font-size: 12px; color: var(--havline-text-muted); }
.config-versions__body { min-width: 0; display: grid; gap: var(--havline-space-3); align-content: start; }
.config-versions__actions { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); }
.config-versions__hint { font-size: 12px; color: var(--havline-text-muted); line-height: 1.6; }
.config-versions__error { margin: 8px 0 0; font-size: 12px; color: var(--havline-error); line-height: 1.6; }

/* 自动备份：开关 + 保留份数 + 备份列表 */
.backup-auto {
  display: grid; gap: var(--havline-space-3);
  margin-top: var(--havline-space-4); padding-top: var(--havline-space-4);
  border-top: 1px solid var(--havline-border);
}
.backup-auto__actions { display: flex; align-items: end; gap: var(--havline-space-3); }
.backup-auto__keep { max-width: 160px; }
.backup-list { display: grid; gap: 6px; margin-top: var(--havline-space-3); }
.backup-row {
  display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3);
  padding: 6px var(--havline-space-3); border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm);
}
.backup-row__main { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
.backup-row__size { font-size: 12px; color: var(--havline-text-muted); }
.backup-row__actions { display: flex; align-items: center; gap: 4px; flex: none; }

/* 公开状态页：访问码 + 打开链接 */
.status-page-field { margin-top: var(--havline-space-3); }
.status-page-actions { display: flex; align-items: center; gap: var(--havline-space-3); margin-top: var(--havline-space-3); flex-wrap: wrap; }

/* API Token：创建区一行放四项，列表一行一项 */
/* 必须带上 .settings-fields 提高优先级：那条纵向 flex 规则在样式表里位置更靠后，
   否则 display 会被它覆盖回 flex-column（子项被推到右侧、整卡变很高） */
.settings-fields.token-create {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  align-items: end;
}
.token-created { margin: 12px 0; }
.token-created__head { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; font-size: 13px; }
.token-created__row { display: flex; align-items: center; gap: 8px; margin-top: 8px; }
.token-created__value { flex: 1; min-width: 0; font-family: var(--havline-mono); font-size: 12px; overflow-wrap: anywhere; }
.token-list { display: grid; gap: 8px; margin-top: 12px; }
.token-row { display: flex; align-items: center; gap: var(--havline-space-3); padding: 10px var(--havline-space-3); border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); }
.token-row__main { display: flex; align-items: center; gap: 8px; min-width: 0; }
.token-row__name { font-weight: 600; color: var(--havline-text); }
.token-row__meta { flex: 1; display: flex; gap: var(--havline-space-3); flex-wrap: wrap; font-size: 12px; color: var(--havline-text-muted); }

.global-nginx-head {
  margin-top: 12px;
}

.nginx-mode-toggle {
  display: inline-flex;
  gap: 4px;
}

.log-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.log-panel-actions {
  display: inline-flex;
  gap: 4px;
}

.log-panel-actions :deep(.n-button) {
  min-width: 56px;
}

.global-nginx-editor,
.nginx-editor {
  flex: 1;
  min-height: 0;
}

.global-nginx-upload-hint {
  margin: 10px 0 0;
}

.global-nginx-warn {
  margin-top: 12px;
}

.settings-fields {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.settings-field {
  min-width: 0;
}

.settings-field__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.settings-field__row .settings-field__label {
  margin-bottom: 0;
  flex: 1;
  min-width: 0;
}

.settings-field__input {
  width: 120px;
  flex-shrink: 0;
}

.security-form {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
}

.proxy-security-form {
  gap: 14px;
}

.proxy-security-form__intro {
  margin: 0;
}

.settings-fields--compact {
  gap: 12px;
}

.ip-policy-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  align-items: start;
}

.ip-policy-field {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.ip-policy-field__input {
  width: 100%;
}

.ip-policy-field__input :deep(.n-input) {
  width: 100%;
}

.ip-policy-field__input :deep(.n-input-wrapper) {
  width: 100%;
}

.ip-policy-field__input :deep(.n-input__textarea-el) {
  width: 100%;
  height: 152px;
  min-height: 152px;
  max-height: 152px;
  resize: none;
  box-sizing: border-box;
}

.ip-policy-field .field-hint {
  margin-top: 6px;
}

.china-cidr-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 14px;
  border: 1px solid var(--havline-border);
  border-radius: 10px;
  background: var(--havline-bg);
}

.china-cidr-panel__head {
  margin-bottom: 0;
}

.notify-type-group {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 16px;
}

.notify-smtp-endpoint {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 120px;
  gap: 12px;
  align-items: start;
}

.notify-smtp-endpoint__col--port :deep(.n-input-number) {
  width: 100%;
}

.notify-input-wide {
  flex: 1;
  min-width: 180px;
}

.notify-form {
  gap: 16px;
}

.notify-events {
  margin-top: var(--havline-space-4);
}

.notify-events__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--havline-text);
  margin-bottom: 12px;
}

.notify-event-group {
  margin-bottom: 16px;
}

.notify-event-group__title {
  font-size: 12px;
  font-weight: 600;
  color: var(--havline-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  margin-bottom: 8px;
}

.notify-threshold-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin: -4px 0 8px;
  padding: 0 14px;
}

.zerossl-key-hint {
  margin-top: -8px;
}

.notify-test-row {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}

.security-form .settings-fields {
  width: 100%;
}

.security-form .settings-field {
  width: 100%;
}

.security-form :deep(.n-input) {
  width: 100%;
}

.password-btn {
  margin-top: var(--havline-space-4);
  align-self: flex-start;
  flex-shrink: 0;
}

.form-switch-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: var(--havline-space-4);
}

.form-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--havline-space-4);
  padding: 12px 14px;
  border: 1px solid var(--havline-border);
  border-radius: 10px;
  background: var(--havline-bg);
}

.form-switch-row__label {
  font-size: 14px;
  font-weight: 500;
  color: var(--havline-text);
}

.form-switch-row__hint {
  margin-top: 2px;
  font-size: 12px;
  color: var(--havline-text-muted);
  line-height: 1.5;
}

.data-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--havline-space-3);
  align-items: stretch;
}

.data-upload {
  display: flex;
  min-width: 0;
}

.data-upload :deep(.n-upload) {
  width: 100%;
  display: flex;
}

.data-upload :deep(.n-upload-trigger) {
  width: 100%;
  display: flex;
}

.data-action-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: flex-start;
  gap: 4px;
  width: 100%;
  min-height: 96px;
  height: 100%;
  padding: 14px;
  border: 1px solid var(--havline-border);
  border-radius: 10px;
  background: var(--havline-bg);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s;
  box-sizing: border-box;
  font: inherit;
  color: inherit;
}

.data-action-card:hover:not(:disabled):not(.data-action-card--disabled) {
  border-color: color-mix(in srgb, #10b981 35%, var(--havline-border));
}

.data-action-card:disabled,
.data-action-card--disabled {
  opacity: 0.6;
  cursor: not-allowed;
  pointer-events: none;
}

.data-action-card__icon {
  font-size: 18px;
  color: #10b981;
}

.data-action-card__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--havline-text);
}

.data-action-card__hint {
  font-size: 12px;
  color: var(--havline-text-muted);
  line-height: 1.45;
}

.data-note {
  margin-top: auto;
  padding-top: var(--havline-space-3);
}

.save-bar {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--havline-space-5);
  padding-bottom: var(--havline-space-2);
}

@media (max-width: 960px) {
  .settings-grid {
    grid-template-columns: 1fr;
  }

  .theme-cards {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .data-actions {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .theme-cards {
    grid-template-columns: 1fr;
  }

  .ip-policy-grid {
    grid-template-columns: 1fr;
  }

  .settings-field__row {
    flex-direction: column;
    align-items: stretch;
  }

  .settings-field__input {
    width: 100%;
  }
}
</style>
