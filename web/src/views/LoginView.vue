<template>

  <div class="auth-page">

    <div class="auth-card">

      <div class="auth-brand">

        <img class="auth-brand__logo" :src="logoImg" alt="Havline" />

        <div>

          <h1>Havline</h1>

          <p>让 NAS 访问更简单</p>

        </div>

      </div>

      <h2 class="auth-title">登录管理后台</h2>

      <n-form @submit.prevent="submit">

        <n-form-item label="用户名">

          <n-input v-model:value="username" size="large" />

        </n-form-item>

        <n-form-item label="密码">

          <n-input v-model:value="password" type="password" show-password-on="click" size="large" />

        </n-form-item>

        <n-button type="primary" block size="large" :loading="loading" attr-type="submit">登录</n-button>

      </n-form>

      <p class="auth-hint">首次使用请查看系统日志中的初始密码，登录后请在「设置」中修改。</p>

    </div>

  </div>

</template>



<script setup lang="ts">

import { ref } from 'vue'

import { useRouter } from 'vue-router'

import { NButton, NForm, NFormItem, NInput, useMessage } from 'naive-ui'

import { api } from '../api/client'
import logoImg from '../assets/brand/havline-logo.svg'



const router = useRouter()

const message = useMessage()

const username = ref('admin')

const password = ref('')

const loading = ref(false)



async function submit() {

  loading.value = true

  try {

    await api.login(username.value, password.value)

    message.success('登录成功')

    router.push({ name: 'dashboard' })

  } catch (error) {

    message.error(error instanceof Error ? error.message : '登录失败')

  } finally {

    loading.value = false

  }

}

</script>



<style scoped>

.auth-page {

  min-height: 100vh;

  display: grid;

  place-items: center;

  padding: var(--havline-space-5);

  background: var(--havline-bg);

}



.auth-card {

  width: 100%;

  max-width: 420px;

  background: var(--havline-surface);

  border: 1px solid var(--havline-border);

  border-radius: var(--havline-radius);

  box-shadow: var(--havline-shadow-md);

  padding: var(--havline-space-6) var(--havline-space-5);

}



.auth-brand {

  display: flex;

  align-items: center;

  gap: var(--havline-space-4);

  margin-bottom: var(--havline-space-5);

}



.auth-brand__logo {

  width: 48px;

  height: 48px;

  border-radius: 14px;

  object-fit: cover;

  box-shadow: 0 6px 16px rgba(16, 185, 129, 0.22);

}



.auth-brand h1 {

  margin: 0;

  font-size: 22px;

  font-weight: 700;

}



.auth-brand p {

  margin: 4px 0 0;

  font-size: 13px;

  color: var(--havline-text-muted);

}



.auth-title {

  margin: 0 0 var(--havline-space-5);

  font-size: 16px;

  font-weight: 600;

  color: var(--havline-text);

}

.auth-hint {
  margin: var(--havline-space-4) 0 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--havline-text-muted);
}

</style>
