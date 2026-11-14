# ESA Pages 部署 Action

零依赖的本地 JavaScript Action，把静态站点目录发布到阿里云边缘安全加速（ESA）的 Functions & Pages 项目。它不使用官方 `esa-cli`，也不安装任何 npm 包。

## 使用方式

```yaml
- name: Deploy to Alibaba Cloud ESA Pages
  uses: ./.github/actions/deploy-esa-pages
  with:
    project: xiaoshae-blog
    path: ./site
    environment: production
    description: GitHub ${{ github.sha }}
  env:
    ESA_ACCESS_KEY_ID: ${{ secrets.ESA_ACCESS_KEY_ID }}
    ESA_ACCESS_KEY_SECRET: ${{ secrets.ESA_ACCESS_KEY_SECRET }}
```

使用本地 Action 前，需要先检出该目录：

```yaml
- uses: actions/checkout@<sha>
  with:
    sparse-checkout: .github/actions/deploy-esa-pages
    persist-credentials: false
```

## 输入与输出

| 输入 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `project` | 是 | — | ESA Functions & Pages 项目名 |
| `path` | 是 | — | 待部署的静态目录，路径相对 `GITHUB_WORKSPACE` |
| `environment` | 否 | `production` | 目标环境，`staging` 或 `production` |
| `description` | 否 | 空 | 写入代码版本的描述 |
| `not_found_strategy` | 否 | 空 | 未匹配路径时的回退策略。设为 `404Page` 时返回站点根目录的 `404.html`，留空则使用 Pages 默认行为 |

| 输出 | 说明 |
|---|---|
| `code-version` | 新建的代码版本 ID |
| `deployment-id` | 发布返回的部署 ID |

凭据只从环境变量读取：

- `ESA_ACCESS_KEY_ID`、`ESA_ACCESS_KEY_SECRET`（必填）
- `ESA_SECURITY_TOKEN`（使用 STS 时可选）
- `ESA_ENDPOINT`、`ESA_REGION`（默认国内站 `esa.cn-hangzhou.aliyuncs.com`、`cn-hangzhou`）

## 内部流程

与官方 CLI 的 Pages 部署顺序一致：

1. `CreateRoutineWithAssetsCodeVersion` 创建版本，返回 `OssPostConfig`；设置 `not_found_strategy` 时通过 `ConfOptions.NotFoundStrategy` 一并提交。
2. 把目录打包为 `assets/` 前缀的 zip，向 `OssPostConfig.Url` 发送 multipart POST，失败重试 3 次。
3. 每秒轮询 `GetRoutineCodeVersionInfo`，直到 `Status` 为 `available`，最多等待 5 分钟。
4. `CreateRoutineCodeDeployment` 以 `percentage` 策略把 100% 流量发布到目标环境。

## 与官方 CLI 的差异

- 不调用 `GetSubscriptionPlan` 做预检，权限和套餐状态由实际部署调用返回。
- 不获取预览域名和访问令牌，避免额外的 API 调用与日志暴露。
- 未传入 `not_found_strategy` 时不设置 `NotFoundStrategy`，使用 Pages 默认的 404 行为；本站传入 `404Page` 以显示自定义中文 404 页面。
- 不修改 DNS、证书或域名绑定，这些属于一次性控制台配置。

## 维护

- 打包结果是确定的：文件按名称排序，zip 内使用固定的 1980-01-01 时间戳。
- 仅依赖 Node 内置模块；运行器通过 `using: node24` 提供运行时。
- API 版本固定为 `2024-09-10`。若 ESA 调整请求字段或返回结构，需要同步修改 `index.js`，并在非生产项目验证后再发布。
- 不要把 AccessKey、签名或预签名 OSS 地址写入日志；失败信息只保留错误码与消息。
