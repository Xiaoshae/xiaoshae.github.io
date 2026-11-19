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
3. 复用原站的图片管线，把 `public/` 中的文章截图无损转为 WebP 并更新引用；
4. 同一份产物部署到 GitHub Pages 与阿里云 ESA Pages。

ESA 部署使用仓库 secrets 中的 `ESA_ACCESS_KEY_ID` / `ESA_ACCESS_KEY_SECRET`，
与原 Jekyll 流程相同。

## 7. 分支说明

- `master`：Zola 技术栈（本工程），独立提交历史，生产环境由此部署。
- `jekyll`：原 Jekyll 技术栈的完整历史，仅作存档，不再部署。
