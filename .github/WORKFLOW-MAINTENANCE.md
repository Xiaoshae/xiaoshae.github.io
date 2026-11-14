# 工作流版本维护规则

## 运行策略

- 版本升级只由维护者手动修改、测试和提交，不在部署任务中发现或选择新版本。
- 外部动作固定完整提交哈希；本地动作随检出的源码提交固定；动作自身打包的依赖随对应提交固定。
- Ruby 环境由本地动作 `.github/actions/setup-ruby-environment` 管理，固定版本及官方运行时压缩包的 SHA-256 集中在 `versions.env`：Ruby `4.0.9`、RubyGems `4.0.20`、Bundler `4.0.24`。不执行 `gem update --system`，版本不符就失败。
- 完整缓存包含 Ruby 安装目录、独立的 Bundler GEM_HOME 和 `vendor/bundle`。每次恢复前将运行器原有 Ruby 目录移到同盘备用位置，避免旧文件混入；运行过程不依赖预装 Ruby。
- 缓存键精确包含平台、工具链版本、`Gemfile`、`Gemfile.lock`、本地动作文件和手动维护的 `cache-version`；无前缀回退。命中后只激活路径，并在一个 Ruby 进程中校验版本、安装前缀及全部锁定依赖，不再安装。
- 未命中时下载固定 URL 的 Ruby 官方运行时压缩包，先校验已提交的 SHA-256，再解压和安装固定版本 Bundler、锁定依赖。验证成功才保存缓存；不查询发行版 API，也不探测最新版本。缓存损坏时校验失败，不静默修补；维护者可递增工作流中的 `cache-version` 强制重建。
- Ruby 依赖以 `Gemfile.lock` 为准，启用冻结模式并关闭 Bundler 更新提示。安装锁定版本不是升级依赖；禁止在部署任务中执行 `bundle update`。
- ESA Pages 部署由本地零依赖 JavaScript Action `.github/actions/deploy-esa-pages` 执行，运行器通过 `using: node24` 提供 Node，不安装 npm 包，也不执行 `npm ci`。
- 该 Action 只调用公开的 ESA OpenAPI（版本 `2024-09-10`）：创建资产版本、向返回的 OSS 表单地址上传 zip、轮询版本状态、发布到目标环境；不读取或修改 DNS、证书和域名绑定。
- ESA 凭据只通过 `ESA_ACCESS_KEY_ID` 和 `ESA_ACCESS_KEY_SECRET` 环境变量注入部署步骤，不使用命令行参数或配置文件。
- Action 只依赖 Node 内置模块；打包结果按文件名排序并使用固定时间戳，保证同一产物生成相同的 zip。

## 手动升级流程

1. 查阅发布说明，手动更新工作流中的动作提交哈希或完整工具版本。
2. Ruby 工具链需要升级时，手动修改 `.github/actions/setup-ruby-environment/versions.env` 中的完整版本、Ruby API 目录和对应压缩包 SHA-256，校验新压缩包并测试。Ruby 依赖需要变更时，在相同工具版本下有选择地更新 `Gemfile.lock`；不要在部署工作流中执行更新。升级 Ruby 时也要同步工作流中的 Jekyll 转换缓存版本标签。
3. ESA Action 的 API 版本和字段是硬编码的。升级前在非生产项目验证四个调用仍兼容，并对比部署前后的文件列表和文件字节；该 Action 没有第三方依赖，不需要锁文件。
4. 如果升级本地 Action 使用的 Node 运行时（`action.yml` 中的 `using`），确认托管运行器已支持该版本。
5. 固定构建时间，对比升级前后的文件列表和全部文件字节；运行完整工作流及缓存命中测试后再确认升级。

## 托管运行器边界

`ubuntu-26.04` 固定的是系统发行版，并非不可变的运行器镜像；GitHub 仍会维护其系统补丁、运行器和动作宿主 Node。工具版本校验会阻止显式工具链悄然变化，但不能冻结整个托管基础设施。若需要连系统镜像也完全固定，需要另行维护固定摘要的构建镜像或自托管运行器，而不是声称发行版标签等同于不可变环境。
