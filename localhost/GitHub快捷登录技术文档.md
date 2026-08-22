# GitHub 快捷登录技术文档

> 本文档记录 sub2api 项目 GitHub OAuth 快捷登录的关键技术架构、实现流程、关键代码位置和数据库表结构。

---

## 一、关键技术架构

### 1.1 整体方案

采用 **OAuth 2.0 授权码模式（Authorization Code）** + **后端中转** + **前端回调收 Token** 的三层架构。

**核心设计要点**：

| 要点 | 说明 |
|------|------|
| OAuth 协议 | Authorization Code 模式（最安全的 OAuth 流程） |
| client_secret 保护 | 仅在后端使用，不暴露给前端 |
| state 防 CSRF | 发起时生成随机 state 存 Cookie，回调时校验 |
| Token 传递方式 | 通过 URL Hash（`#access_token=xxx`），避免出现在服务器日志和 Referer 头 |
| 邮箱验证 | 强制调用 GitHub `/user/emails` 接口取 `primary+verified` 邮箱 |
| 配置管理 | 存数据库 settings 表，admin 可动态配置，无需重启 |
| 身份绑定 | GitHub 账号通过 `auth_identities` 表关联本地 user |

### 1.2 GitHub 与 Google 共用 Email OAuth 流程

GitHub 和 Google 登录共用同一套 `emailOAuth*` 方法，通过 `provider` 参数（`"github"` / `"google"`）区分。差异仅在：
- GitHub 需额外调用 `/user/emails` 接口获取验证邮箱
- Google 的 email 直接在 `/userinfo` 返回中带 `email_verified` 字段

### 1.3 配置来源

GitHub OAuth 的端点 URL 硬编码在代码中作为默认值，但 `client_id` / `client_secret` / `redirect_url` 等敏感配置存数据库 `settings` 表，可由管理员在后台动态修改。

---

## 二、完整实现流程

### 2.1 时序图

```
┌──────────┐                    ┌──────────┐                    ┌──────────┐
│  前端    │                    │  后端    │                    │  GitHub  │
│ LoginView│                    │AuthHandler│                   │   OAuth  │
└────┬─────┘                    └────┬─────┘                    └────┬─────┘
     │                               │                               │
     │ 1. 点击 GitHub 按钮           │                               │
     │   startLogin('github')        │                               │
     │   存 sessionStorage            │                               │
     │   emit('start', request)      │                               │
     │                               │                               │
     │ 2. handleOAuthStart           │                               │
     │   ├ 无验证码: GET /start      │                               │
     │   └ 有验证码: POST /start     │                               │
     │   (携带 turnstile/captcha)    │                               │
     ├──────────────────────────────→│                               │
     │                               │                               │
     │                               │ 3. 校验验证码                 │
     │                               │   生成 state 存 Cookie        │
     │                               │   构造 authorize_url          │
     │   ← 返回 authorize_url ──────┤                               │
     │                               │                               │
     │ 4. window.location.href       │                               │
     │   = authorize_url              │                               │
     ├───────────────────────────────────────────────────────────────→│
     │                               │                               │
     │                               │                               │ 5. 用户授权
     │                               │                               │
     │                               │ 6. 回调后端                   │
     │                               │   GET /callback?code=xxx      │
     │                               │       &state=yyy             │
     │                               ←───────────────────────────────┤
     │                               │                               │
     │                               │ 7. 校验 state                 │
     │                               │   用 code 换 access_token      │
     │                               │   POST /login/oauth/access_token
     │                               ├──────────────────────────────→│
     │                               │   ← access_token ─────────────┤
     │                               │                               │
     │                               │ 8. 获取用户信息               │
     │                               │   GET /user                   │
     │                               │   GET /user/emails            │
     │                               ├──────────────────────────────→│
     │                               │   ← user info + emails ──────┤
     │                               │                               │
     │                               │ 9. 登录/注册/绑定            │
     │                               │   查 auth_identities 表       │
     │                               │   ├ 已绑定: 生成 JWT          │
     │                               │   ├ 邮箱存在: 创建 pending    │
     │                               │   └ 全新: 创建用户+identity   │
     │                               │                               │
     │ 10. 302 重定向前端            │                               │
     │   /auth/oauth/callback        │                               │
     │   #access_token=xxx           │                               │
     │   &refresh_token=yyy          │
     ←───────────────────────────────┤                               │
     │                               │                               │
     │ 11. onMounted 读 hash          │                               │
     │   存 localStorage              │                               │
     │   跳转 /dashboard              │                               │
     │                               │                               │
```

### 2.2 流程步骤详解

#### 步骤 1-2：前端发起

1. 用户在登录页点击 GitHub 按钮
2. `EmailOAuthButtons.vue` 的 `startLogin('github')` 将 provider 存入 sessionStorage
3. emit `start` 事件给 `LoginView.vue`
4. `handleOAuthStart` 根据是否启用验证码：
   - 无验证码：直接 GET 跳转 `/api/v1/auth/oauth/github/start`
   - 有验证码：POST 调用 `/auth/oauth/github/start`，携带 Turnstile/Tencent 验证码

#### 步骤 3-4：后端发起 OAuth

1. `GitHubOAuthStart` → `emailOAuthStart(c, "github")`
2. 校验验证码（如果启用）
3. 读取 GitHub OAuth 配置（client_id, client_secret, redirect_url）
4. 生成随机 `state`，存入 Cookie（10 分钟有效）
5. 构造 GitHub 授权 URL：`https://github.com/login/oauth/authorize?response_type=code&client_id=xxx&redirect_uri=xxx&state=xxx&scope=read:user+user:email`
6. 返回 `authorize_url` 给前端
7. 前端 `window.location.href = authorize_url` 跳转到 GitHub

#### 步骤 5-6：GitHub 授权回调

1. 用户在 GitHub 页面登录并授权
2. GitHub 回调后端：`GET /api/v1/auth/oauth/github/callback?code=xxx&state=yyy`
3. `GitHubOAuthCallback` → `emailOAuthCallback(c, "github")`

#### 步骤 7-8：后端处理回调

1. 校验 `state` 与 Cookie 中的值一致（防 CSRF）
2. 校验 provider 与 Cookie 中的值一致
3. 用 `code` 换 `access_token`（POST 到 `https://github.com/login/oauth/access_token`）
4. 用 `access_token` 调用 `/user` 获取用户基本信息
5. 用 `access_token` 调用 `/user/emails` 获取已验证的主邮箱
6. 解析用户信息：GitHub ID（subject）、email、login、name、avatar_url

#### 步骤 9：登录/注册/绑定

调用 `LoginOrRegisterVerifiedEmailOAuthWithSignupCodes`，三种结果：

| 场景 | 条件 | 处理 |
|------|------|------|
| 直接登录 | auth_identities 表中已有该 GitHub 用户 | 生成 JWT TokenPair |
| 需确认绑定 | email 已注册但未绑定该 GitHub | 创建 pending session，重定向前端要求确认 |
| 全新注册 | email 和 GitHub ID 都不存在 | 创建 user + auth_identity，生成 JWT TokenPair |

#### 步骤 10-11：返回前端

1. 后端 302 重定向到前端回调页：`/auth/oauth/callback#access_token=xxx&refresh_token=yyy&expires_in=xxx`
2. 前端 `OAuthCallbackView.vue` 的 `onMounted` 读取 URL hash 中的 token
3. 存入 localStorage
4. 跳转到 `/dashboard`

---

## 三、关键代码位置

### 3.1 后端

#### 配置层

| 文件 | 行号 | 说明 |
|------|------|------|
| [config.go](file:///d:/working/AI/sub2api/backend/internal/config/config.go#L81) | L81 | `GitHubOAuth EmailOAuthProviderConfig` 字段定义 |
| [config.go](file:///d:/working/AI/sub2api/backend/internal/config/config.go#L279-L286) | L279-286 | `EmailOAuthProviderConfig` 结构体字段 |
| [setting_service.go](file:///d:/working/AI/sub2api/backend/internal/service/setting_service.go#L194-L199) | L194-199 | GitHub OAuth 默认 URL 常量 |
| [domain_constants.go](file:///d:/working/AI/sub2api/backend/internal/service/domain_constants.go#L271-L275) | L271-275 | settings 表存储键 |
| [setting_oauth.go](file:///d:/working/AI/sub2api/backend/internal/service/setting_oauth.go#L278-L287) | L278-287 | 默认配置生成 |
| [setting_oauth.go](file:///d:/working/AI/sub2api/backend/internal/service/setting_oauth.go#L347-L368) | L347-368 | 数据库配置覆盖默认值 |

**GitHub OAuth 默认 URL**：

```go
defaultGitHubOAuthAuthorize  = "https://github.com/login/oauth/authorize"
defaultGitHubOAuthToken      = "https://github.com/login/oauth/access_token"
defaultGitHubOAuthUserInfo   = "https://api.github.com/user"
defaultGitHubOAuthEmails     = "https://api.github.com/user/emails"
defaultGitHubOAuthScopes     = "read:user user:email"
defaultGitHubOAuthFrontend   = "/auth/oauth/callback"
```

#### 路由层

| 文件 | 行号 | 路由 | Handler |
|------|------|------|---------|
| [routes/auth.go](file:///d:/working/AI/sub2api/backend/internal/server/routes/auth.go#L79) | L79 | `GET /api/v1/auth/oauth/github/start` | `GitHubOAuthStart` |
| [routes/auth.go](file:///d:/working/AI/sub2api/backend/internal/server/routes/auth.go#L80-L82) | L80-82 | `POST /api/v1/auth/oauth/github/start` | `GitHubOAuthStart`（带限流） |
| [routes/auth.go](file:///d:/working/AI/sub2api/backend/internal/server/routes/auth.go#L83) | L83 | `GET /api/v1/auth/oauth/github/callback` | `GitHubOAuthCallback` |
| [routes/auth.go](file:///d:/working/AI/sub2api/backend/internal/server/routes/auth.go#L84-L89) | L84-89 | `POST /api/v1/auth/oauth/github/complete-registration` | `CompleteGitHubOAuthRegistration` |

#### Handler 层（核心实现）

**文件**：[auth_email_oauth.go](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go)

| 方法 | 行号 | 说明 |
|------|------|------|
| `GitHubOAuthStart` | [L49](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L49) | 入口，委托给 `emailOAuthStart` |
| `GitHubOAuthCallback` | [L52](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L52) | 回调入口，委托给 `emailOAuthCallback` |
| `CompleteGitHubOAuthRegistration` | [L54-56](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L54-L56) | 完成注册入口 |
| `emailOAuthStart` | [L61-97](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L61-L97) | 发起 OAuth：校验验证码、生成 state、构造 URL |
| `emailOAuthCallback` | [L99-155](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L99-L155) | 回调处理：校验 state、换 token、取 profile |
| `emailOAuthCallbackWithProfile` | [L157-200](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L157-L200) | 登录/注册/绑定决策 |
| `buildEmailOAuthAuthorizeURL` | [L454-469](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L454-L469) | 构造 GitHub 授权 URL |
| `exchangeEmailOAuthCode` | [L471-498](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L471-L498) | 用 code 换 access_token |
| `fetchEmailOAuthProfile` | [L500-521](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L500-L521) | 获取用户信息（区分 provider） |
| `parseGitHubOAuthProfile` | [L523-554](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L523-L554) | 解析 GitHub /user 响应 |
| `fetchGitHubPrimaryVerifiedEmail` | [L556-585](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L556-L585) | 获取 GitHub 已验证主邮箱 |

#### Service 层

| 文件 | 行号 | 说明 |
|------|------|------|
| [auth_email_oauth_auto.go](file:///d:/working/AI/sub2api/backend/internal/service/auth_email_oauth_auto.go#L41-L49) | L41-49 | `LoginOrRegisterVerifiedEmailOAuthWithSignupCodes` 入口 |
| [auth_email_oauth_auto.go](file:///d:/working/AI/sub2api/backend/internal/service/auth_email_oauth_auto.go#L51-L80) | L51-80 | 核心登录/注册逻辑 |

### 3.2 前端

#### 组件层

| 文件 | 行号 | 说明 |
|------|------|------|
| [EmailOAuthButtons.vue](file:///d:/working/AI/sub2api/frontend/src/components/auth/EmailOAuthButtons.vue#L77-L87) | L77-87 | GitHub 按钮点击 `startLogin('github')` |
| [GitHubMark.vue](file:///d:/working/AI/sub2api/frontend/src/components/auth/GitHubMark.vue) | - | GitHub 图标组件 |
| [OAuthCallbackView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/OAuthCallbackView.vue#L366-L397) | L366-397 | 回调页 `onMounted` 处理 |
| [OAuthCallbackView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/OAuthCallbackView.vue#L258-L272) | L258-272 | `redirectProviderCallbackToBackend` 转发后端 |
| [OAuthCallbackView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/OAuthCallbackView.vue#L274-L283) | L274-283 | `finalizeTokenResponse` 存储 token |
| [LoginView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/LoginView.vue#L162-L168) | L162-168 | 登录页 GitHub 按钮渲染 |
| [LoginView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/LoginView.vue#L661-L696) | L661-696 | `handleOAuthStart` 处理点击 |
| [RegisterView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/RegisterView.vue#L287) | L287 | 注册页 GitHub 按钮 |
| [SettingsView.vue](file:///d:/working/AI/sub2api/frontend/src/views/admin/SettingsView.vue#L2526) | L2526 | 管理员配置 GitHub OAuth |

#### API 层

| 文件 | 行号 | 说明 |
|------|------|------|
| [api/auth.ts](file:///d:/working/AI/sub2api/frontend/src/api/auth.ts#L44-L50) | L44-50 | `buildOAuthLoginStartURL` 构造发起 URL |
| [api/auth.ts](file:///d:/working/AI/sub2api/frontend/src/api/auth.ts#L52-L62) | L52-62 | `startOAuthLogin` POST 发起 |
| [api/auth.ts](file:///d:/working/AI/sub2api/frontend/src/api/auth.ts#L620-L631) | L620-631 | `createPendingOAuthAccount` 完成注册 |
| [api/admin/settings.ts](file:///d:/working/AI/sub2api/frontend/src/api/admin/settings.ts#L542-L546) | L542-546 | OAuth 设置类型定义 |

---

## 四、数据库表结构

### 4.1 auth_identities 表（核心）

**Schema 定义**：[ent/schema/auth_identity.go](file:///d:/working/AI/sub2api/backend/ent/schema/auth_identity.go)

**用途**：存储用户的第三方登录身份，一个 user 可关联多个 provider。

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | bigint | 主键 |
| `user_id` | bigint | 关联 `users.id` |
| `provider_type` | varchar(20) | 提供方类型：`github` |
| `provider_key` | text | 提供方标识，通常等于 provider_type |
| `provider_subject` | text | **GitHub 用户 ID**（唯一标识，不会变） |
| `verified_at` | timestamptz | 身份验证时间 |
| `issuer` | text | 可选，签发者 |
| `metadata` | jsonb | 附加信息（如 GitHub login） |
| `created_at` | timestamptz | 创建时间 |
| `updated_at` | timestamptz | 更新时间 |

**索引**：

| 索引 | 字段 | 类型 | 用途 |
|------|------|------|------|
| `auth_identities_provider_type_provider_key_provider_subject_key` | `(provider_type, provider_key, provider_subject)` | UNIQUE | 防止重复绑定 |
| `auth_identities_user_id_key` | `(user_id)` | INDEX | 查询用户绑定的所有身份 |
| `auth_identities_user_id_provider_type_key` | `(user_id, provider_type)` | INDEX | 查询用户绑定的特定 provider |

**支持的 provider_type**（[L17-25](file:///d:/working/AI/sub2api/backend/ent/schema/auth_identity.go#L17-L25)）：
- `email` - 邮箱密码
- `github` - GitHub
- `google` - Google
- `linuxdo` - LinuxDo
- `oidc` - 通用 OIDC
- `wechat` - 微信
- `dingtalk` - 钉钉

### 4.2 pending_auth_sessions 表

**Schema 定义**：[ent/schema/pending_auth_session.go](file:///d:/working/AI/sub2api/backend/ent/schema/pending_auth_session.go)

**用途**：存储待完成的 OAuth 注册/绑定会话（当邮箱已存在但未绑定该 GitHub 时创建）。

### 4.3 settings 表

**用途**：存储 GitHub OAuth 配置项。

| key | 说明 |
|-----|------|
| `github_oauth_enabled` | 是否启用 GitHub 登录（`"true"` / `"false"`） |
| `github_oauth_client_id` | GitHub OAuth App Client ID |
| `github_oauth_client_secret` | GitHub OAuth App Client Secret |
| `github_oauth_redirect_url` | 后端回调地址（需在 GitHub 后台登记） |
| `github_oauth_frontend_redirect_url` | 前端回调页路由（默认 `/auth/oauth/callback`） |

### 4.4 users 表（关联）

GitHub 登录成功后，会创建或更新 `users` 表记录，主要字段：

| 字段 | 说明 |
|------|------|
| `email` | GitHub 已验证的主邮箱 |
| `username` | GitHub login 或 name |
| `avatar_url` | GitHub 头像 URL |
| `status` | 用户状态（active/disabled） |
| `role` | 角色（user/admin） |

---

## 五、安全设计要点

### 5.1 CSRF 防护

- 发起 OAuth 时生成随机 `state`，存入 HttpOnly Cookie
- 回调时校验 URL 中的 `state` 与 Cookie 中的值一致
- Cookie 有效期 10 分钟（[L29](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go#L29)）

### 5.2 邮箱验证

- GitHub `/user` 接口返回的 email 可能是公开邮箱（未验证）
- 必须调用 `/user/emails` 接口，取 `primary=true && verified=true` 的邮箱
- 如果没有已验证邮箱，登录失败

### 5.3 Token 传递安全

- JWT token 通过 URL Hash（`#access_token=xxx`）传递
- Hash 不会发送到服务器日志和 Referer 头
- 前端读取后立即从 URL 中清除

### 5.4 client_secret 保护

- `client_secret` 仅在后端使用
- 前端只接触 `client_id` 和 `authorize_url`
- 换 token 的请求由后端发起

### 5.5 速率限制

| 端点 | 限制 |
|------|------|
| `POST /oauth/github/start` | 20 次/分钟 |
| `POST /oauth/github/complete-registration` | 10 次/分钟 |
| Redis 故障时 | fail-close（拒绝请求） |

### 5.6 前端重定向安全

- `sanitizeFrontendRedirectPath` 校验重定向路径
- 必须以 `/` 开头
- 禁止 `//` 开头（协议相对 URL）
- 禁止包含 `://`（绝对 URL）
- 默认重定向到 `/dashboard`

---

## 六、配置指南

### 6.1 创建 GitHub OAuth App

1. 访问 https://github.com/settings/developers
2. New OAuth App
3. 填写：
   - Application name: `Sub2API`
   - Homepage URL: `http://localhost:3000`
   - Authorization callback URL: `http://localhost:8080/api/v1/auth/oauth/github/callback`
4. 获取 Client ID 和 Client Secret

### 6.2 在后台配置

访问 管理后台 → 系统设置 → OAuth 配置（[SettingsView.vue:2526](file:///d:/working/AI/sub2api/frontend/src/views/admin/SettingsView.vue#L2526)）：

| 配置项 | 值 |
|--------|-----|
| 启用 GitHub 登录 | ✅ |
| Client ID | GitHub OAuth App 的 Client ID |
| Client Secret | GitHub OAuth App 的 Client Secret |
| 后端回调地址 | `http://localhost:8080/api/v1/auth/oauth/github/callback` |
| 前端回调地址 | `/auth/oauth/callback`（默认值） |

### 6.3 测试流程

1. 访问 http://localhost:3000
2. 点击 GitHub 登录按钮
3. 跳转到 GitHub 授权页
4. 授权后自动跳回，登录成功

---

## 七、相关文件索引

### 后端

| 文件 | 说明 |
|------|------|
| [internal/config/config.go](file:///d:/working/AI/sub2api/backend/internal/config/config.go) | 配置结构定义 |
| [internal/handler/auth_email_oauth.go](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth.go) | OAuth 核心实现 |
| [internal/handler/auth_email_oauth_test.go](file:///d:/working/AI/sub2api/backend/internal/handler/auth_email_oauth_test.go) | 单元测试 |
| [internal/server/routes/auth.go](file:///d:/working/AI/sub2api/backend/internal/server/routes/auth.go) | 路由注册 |
| [internal/service/auth_email_oauth_auto.go](file:///d:/working/AI/sub2api/backend/internal/service/auth_email_oauth_auto.go) | 登录/注册逻辑 |
| [internal/service/setting_oauth.go](file:///d:/working/AI/sub2api/backend/internal/service/setting_oauth.go) | OAuth 配置读取 |
| [internal/service/setting_service.go](file:///d:/working/AI/sub2api/backend/internal/service/setting_service.go) | 默认 URL 常量 |
| [internal/service/domain_constants.go](file:///d:/working/AI/sub2api/backend/internal/service/domain_constants.go) | settings 表键名 |
| [internal/handler/dto/settings.go](file:///d:/working/AI/sub2api/backend/internal/handler/dto/settings.go) | 设置 DTO |
| [ent/schema/auth_identity.go](file:///d:/working/AI/sub2api/backend/ent/schema/auth_identity.go) | 身份表 Schema |
| [ent/schema/pending_auth_session.go](file:///d:/working/AI/sub2api/backend/ent/schema/pending_auth_session.go) | 待注册会话 Schema |

### 前端

| 文件 | 说明 |
|------|------|
| [src/components/auth/EmailOAuthButtons.vue](file:///d:/working/AI/sub2api/frontend/src/components/auth/EmailOAuthButtons.vue) | GitHub/Google 按钮组件 |
| [src/components/auth/GitHubMark.vue](file:///d:/working/AI/sub2api/frontend/src/components/auth/GitHubMark.vue) | GitHub 图标 |
| [src/views/auth/LoginView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/LoginView.vue) | 登录页 |
| [src/views/auth/RegisterView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/RegisterView.vue) | 注册页 |
| [src/views/auth/OAuthCallbackView.vue](file:///d:/working/AI/sub2api/frontend/src/views/auth/OAuthCallbackView.vue) | OAuth 回调页 |
| [src/views/admin/SettingsView.vue](file:///d:/working/AI/sub2api/frontend/src/views/admin/SettingsView.vue) | 管理员设置页 |
| [src/api/auth.ts](file:///d:/working/AI/sub2api/frontend/src/api/auth.ts) | 认证 API |
| [src/api/admin/settings.ts](file:///d:/working/AI/sub2api/frontend/src/api/admin/settings.ts) | 设置 API 类型 |
