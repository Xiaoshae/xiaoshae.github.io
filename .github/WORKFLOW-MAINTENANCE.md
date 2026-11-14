# 工作流版本维护规则

## 运行策略

- 版本升级只由维护者手动修改、测试和提交，不在部署任务中发现或选择新版本。
- 所有直接使用的动作固定完整提交哈希；动作自身打包的依赖随该提交固定。
- Ruby 固定为 `4.0.9`，Bundler 固定为 `4.0.24`。使用该 Ruby 自带的 RubyGems `4.0.20`，不执行 `gem update --system`。版本不符合预期时直接失败。
- Ruby 依赖以 `Gemfile.lock` 为准，启用冻结模式并关闭 Bundler 更新提示。安装锁定版本不是升级依赖；禁止在部署任务中执行 `bundle update`。
- Node.js 固定为 `26.13.1`，使用随附的 npm `11.20.0`，关闭 `check-latest`，并在运行时校验版本。
- EdgeOne 命令行工具及全部间接依赖由 `.github/edgeone/package-lock.json` 锁定，包下载按锁文件的完整性摘要验证。
- EdgeOne 缓存精确匹配系统、架构、Node/npm 版本及锁文件配置，不使用前缀回退。命中后跳过安装；未命中时只执行 `npm ci`，禁止 `npm update` 和按需解析的 `npx`。
- 关闭 npm 更新通知、自动审计与安装脚本。直接调用已安装的本地 EdgeOne 部署命令，不调用会查询新版本的 EdgeOne `version` 命令。

## 手动升级流程

1. 查阅发布说明，手动更新工作流中的动作提交哈希或完整工具版本。
2. Ruby 依赖需要变更时，在与工作流相同的工具版本下有选择地更新 `Gemfile.lock`；不要在部署工作流中执行更新。
3. EdgeOne 需要变更时，修改 `.github/edgeone/package.json`，使用指定 Node/npm 生成并审查新的锁文件：

   ```sh
   cd .github/edgeone
   npm install --package-lock-only --ignore-scripts --no-audit --no-fund
   npm ci --ignore-scripts --no-audit --no-fund
   ```

4. 如果升级 Node/npm，同步修改工作流版本校验、缓存键以及 `package.json` 的 `engines`。
5. 固定构建时间，对比升级前后的文件列表和全部文件字节；运行完整工作流及缓存命中测试后再确认升级。

## 托管运行器边界

`ubuntu-26.04` 固定的是系统发行版，并非不可变的运行器镜像；GitHub 仍会维护其系统补丁、运行器和动作宿主 Node。工具版本校验会阻止显式工具链悄然变化，但不能冻结整个托管基础设施。若需要连系统镜像也完全固定，需要另行维护固定摘要的构建镜像或自托管运行器，而不是声称发行版标签等同于不可变环境。
