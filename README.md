# Xiaoshae Blog（Zola 版）

由原 Jekyll 博客迁移而来，内容与视觉效果与原站保持一致。
访问地址：

- https://blog.xiaoshae.cn
- https://xiaoshae.github.io

## 1. 环境准备

Zola 是单个二进制文件，无需 Ruby、Node 等运行时。以 Linux x86_64 为例：

```shell
curl -sL -o zola.tar.gz \
  https://github.com/getzola/zola/releases/download/v0.23.6/zola-v0.23.6-x86_64-unknown-linux-gnu.tar.gz
tar xzf zola.tar.gz zola
./zola --version
```

macOS 也可以 `brew install zola`。本工程验证版本为 **0.23.6**，
配置文件使用了 0.23 引入的 `[markdown.highlighting]` 结构，低于 0.23 的版本无法使用。

## 2. 常用命令

```shell
zola build                          # 构建到 public/（全站约 0.2~0.3 秒）
zola serve                          # 本地预览，默认 http://127.0.0.1:1111
zola serve --interface 192.168.133.133 --port 8080   # 绑定指定地址
```

`zola serve` 会自动把 `base_url` 替换为监听地址，并监视文件变化实时重建。
生产构建使用 `config.toml` 中的 `base_url = "https://blog.xiaoshae.cn"`。

## 3. 目录结构

```
config.toml            # 站点配置（导航在 extra.navigation）
content/               # 文章，平铺存放，与原 _posts 习惯一致
  _index.md            # 根 section：日期排序、每页 10 篇、feed
  pages/               # 静态页（关于/文章/分类/速查手册/捐赠）
templates/             # Tera 模板
static/                # 原样拷贝的资源（assets/css、js、img）
public/                # 构建产物（勿手动修改）
```

## 4. 写文章

在 `content/` 下新建 `<Slug>.md`，URL 通过 `path` 显式指定，保持原有的
`/blog/<Slug>/` 形式（含大小写）：

```toml
+++
title = "文章标题"
date = 2025-11-19
path = "blog/my-post"
[taxonomies]
tags = ["linux"]              # 含 cheatsheet 则进入"速查手册"，且不在首页展示
[extra]
lastupdate = "2025-11-19"
lang = "zh"
# human_authored = false      # 有 AI 参与时设为 false，显示 AI 共创徽章
# description = "首页摘要"     # 可选；不填则自动截取正文前 270 字
+++
```

- 分类（tags）同时会生成 `/tags/<名称>/` 分类详情页（新增能力）。
- 文章正文中如需展示 `{{ }}`、`{% %}`（例如 Ansible/Jinja 示例），
  用 `{% raw %} ... {% endraw %}` 包裹，与原 Jekyll 习惯相同。
- 代码高亮使用内置 `github-light` 主题，无需任何 CSS 维护。
- 正文独立图片使用 `{{ <article_image src="/assets/img/post/screenshot.png" alt="有意义的图片描述" lazy={true} /> }}`。组件自动填写原图宽高；首屏可能出现的图片不要传 `lazy`。对无法读取尺寸的特殊格式，可显式传 `width={...}` 和 `height={...}`。

## 5. 与原 Jekyll 站的差异

- 分页第二页起地址为 `/blog/page/2/`（原 `/blog/page2/`），不做兼容重定向。
- 新增 `/tags/` 分类聚合页与 `/sitemap.xml`。
- `articles-en`（英文文章列表）未迁移：没有英文文章且导航中早已隐藏。
- Disqus 评论（原配置指向他人站点且从未启用）、Google Analytics、
  未使用的社交链接、`assets/code/` 示例代码均已清理。
- 个别原站因 kramdown 方言产生的字面 `**` 已修复为正常粗体。

## 6. 部署

`.github/workflows/zola.yml` 在推送到 `master` 后自动执行：

1. 下载固定版本的 Zola 二进制（SHA256 校验）并执行 `zola build`；
2. 冒烟测试：关键页面、双语创作徽章、feed 生成器标识；
3. 从 `image-cache` 分支浅拉取固定的 `article-webp` 静态可执行文件和已生成的 WebP（不使用 GitHub Actions 缓存），校验后运行。工具只重新编码缺失的图片，并更新 `public/` 里的文章和订阅源引用；
4. 在 Chromium 验收构建产物；不达标则不部署。
5. 同一份产物部署到 GitHub Pages 与阿里云 ESA Pages。

ESA 部署使用仓库 secrets 中的 `ESA_ACCESS_KEY_ID` / `ESA_ACCESS_KEY_SECRET`，
与原 Jekyll 流程相同。

### 性能验收

使用与部署一致的构建产物运行验收：

```shell
zola build
bash .github/image-build/build-with-git-cache.sh "$(mktemp -d)/delta"
npm --prefix .github/site-audit ci
npm --prefix .github/site-audit exec -- playwright install --only-shell chromium
npm --prefix .github/site-audit run audit -- "$PWD/public"
```

移动端使用 390 × 844 像素、150 毫秒网络延迟、1.6 兆比特每秒下载带宽和四倍处理器降速，移动端及桌面端均运行三轮。首页验收预算为：压缩传输不超过 20 KB、最大内容绘制不超过 2.5 秒、累积布局偏移不超过 0.1；文章与相邻文章点击到目标正文绘制不超过 1.2 秒。另验证往返缓存与滚动位置、同源分页、按意图预取、图片供图和无脚本键盘导航。实验室结果不能代替线上真实访问的第 75 百分位数据。

上线后分别检查两个域名的响应头和移动端关键路径。ESA 的浏览器缓存及缓存键在站点控制台设置：若为带 `?h=` 的 CSS 配置长缓存，须先确认查询参数参与缓存键，避免不同版本互相覆盖；内容指纹文件名的 `/processed_images/` 可独立配置长期缓存。HTML 保持短缓存或重新验证，不对全站统一设置长缓存。

## 7. 分支说明

- `master`：Zola 技术栈（本工程），独立提交历史，生产环境由此部署。
- `jekyll`：原 Jekyll 技术栈的完整历史，仅作存档，不再部署。
- `image-cache`：存放预编译工具、上游许可和 `webp/` 缓存，不部署。生产流程按 `.github/image-build/article-webp.sha256` 选择 `bin/article-webp-linux-amd64-<SHA-256>`，并校验内容。每个工具版本使用独立缓存目录；旧版本可在不再需要回滚后清理。

生成的 WebP 按源图和转换配置缓存，同名图片内容变化也会重新编码。构建任务只有读权限；有新增缓存时才上传增量产物，由独立的 `update-image-cache` 任务读取同一提交固定的工具版本并写回分支。回写与双端部署并行，失败不阻止部署；全命中时不上传增量，也不启动回写任务。只有回写任务需要仓库内容写权限，建议用分支保护限制 `master` 并限制 `image-cache` 的写入者。缓存历史会累积二进制，删除当前文件不会缩小已有历史。

### 更新图片工具

源码和测试在 `.github/image-build/tool/`。工具内置上游 `cwebp` 和特殊格式解码器，生产运行时不需要安装 Go、Python、`uv` 或额外的编码器。

1. 在 Linux x86-64 上运行 `bash .github/image-build/build-tool.sh /tmp/article-webp`。脚本校验固定的 Go 和 WebP 工具包，运行测试并输出静态文件及其 SHA-256。
2. 将二进制以 `bin/article-webp-linux-amd64-<SHA-256>` 存入 `image-cache` 分支；若第三方依赖变化，一并更新许可。先推送缓存分支，再把新的 SHA-256 写入 `master` 的 `.github/image-build/article-webp.sha256` 并推送。
3. 独立测试工作流会重新编译并对比摘要，不读写缓存分支。
