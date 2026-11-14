# 个人博客

这是我的个人博客仓库，访问地址：

- https://blog.xiaoshae.cn
- https://xiaoshae.github.io/

为什么写博客？可以参考：

- [Writing Is Magic, 2022](https://brooker.co.za/blog/2022/11/08/writing.html)
  （中文版：[长文写作的魔力](https://arthurchiao.art/blog/writing-is-magic-zh)）

## 1. 主题来源

本博客基于 [ArthurChiao 的博客模板](https://github.com/ArthurChiao/arthurchiao.github.io)，
其主题最初源自 [Long Haul](https://github.com/brianmaierjr/long-haul)，此后经过了大量修改和演进。

## 2. 使用此主题或模板

### 1. 创建自己的 GitHub 仓库

仓库名称必须为 `<your_github_id>.github.io`，其中 `<your_github_id>` 替换为你的 GitHub 用户名。

### 2. 克隆模板并替换个人信息

以下操作适用于创建新博客，请勿在需要保留历史记录和文章的现有仓库中执行清理命令。

```shell
git clone https://github.com/Xiaoshae/xiaoshae.github.io.git <your_github_id>.github.io
cd <your_github_id>.github.io

# 删除模板的 Git 历史、文章和草稿；执行前请确认当前目录并做好备份。
rm -rf .git/ _posts/* _drafts/*

# 查找并替换模板中的个人信息。
grep -riE 'arthurchiao|xiaoshae' --exclude-dir=.git .
```

同时检查 `_config.yml` 中的 `google_analytics` 配置，删除原有统计代码，或替换成自己的代码，避免网站访问数据被发送到他人的统计账户。

### 3. 添加文章

在 `_posts` 目录中创建文章，文件名采用 `年-月-日-文章名称.md` 格式，例如：

```shell
touch _posts/2023-11-05-your-first-post.md
```

在文章开头添加以下元数据：

```yaml
---
layout    : post
title     : "我的第一篇文章"
date      : 2023-11-05
lastupdate: 2023-11-05
categories: tag1 tag2 tag3
---
```

然后使用 Markdown 编写正文，并根据实际内容调整标题、日期和分类。

### 4. 本地预览

安装项目所需的 Ruby 和 Bundler 依赖后，在仓库目录中执行：

```shell
bundle install
bundle exec jekyll serve --drafts --incremental
```

使用浏览器访问 `http://localhost:4000` 即可预览。

### 5. 推送到 GitHub

以下命令适用于已经删除模板 Git 历史、尚未初始化的新仓库：

```shell
git init
git config user.name "YOUR_NAME"
git config user.email "YOUR_EMAIL"
git add .
git commit -m "Initial commit"
git branch -M master
git remote add origin https://github.com/<your_github_id>/<your_github_id>.github.io.git
git push -u origin master
```

完成 GitHub Pages 配置和部署后，即可访问 `https://<your_github_id>.github.io/`。

## 3. 文章截图自动转换

文章仍可引用放在 `assets/img` 中的 PNG、JPEG、TIFF、静态 GIF、BMP、AVIF 或 WebP 原图，无需手动转换或修改 Markdown 链接。

GitHub Actions 在 Jekyll 构建完成后、上传部署产物前，会自动处理文章中的本地截图：

- 使用 `cwebp -lossless -exact` 生成无损 WebP，保留透明像素颜色和 ICC 色彩配置，不缩放图片。JPEG、AVIF 等原图已有的损失无法恢复，但转换不会再做有损压缩。
- 默认按 CPU 数量并发转换，最多 8 个任务；同一图片只处理一次，内容相同的图片共享缓存。修改原图、编码参数或工具版本后自动重新转换。
- 只更新 `_site` 中的 HTML 和 RSS 图片引用，支持普通图片、`srcset` 和指向原图的链接；仓库原图、文章和本地 Jekyll 预览不变。
- 转换和引用更新全部成功后，删除部署产物中对应的原图，仅保留 WebP；仓库原图不受影响。输出名称如 `screenshot.webp`；遇到同名图片或已存在的 WebP 文件时，自动添加短标识，避免覆盖。
- 页头背景、捐赠页图片和 SVG 不主动转换；外链图片与内嵌图片不处理。动画或多帧图片会明确报错，不会静默丢失帧；HEIC/HEIF 请先导出 PNG。

转换代码与锁定的依赖位于 `.github/image-build/`。需要调整并发数时，可在工作流的转换命令中添加 `--jobs 4` 等参数。

## 4. 人类创作声明

文章支持布尔字段 `human_authored`，默认值为 `true`。文章末尾、上一篇／下一篇导航之前，会显示英文和简体中文各两张黑白版 Not By AI 徽章；无需逐篇添加字段。

当内容有 AI 参与或辅助创作时，在文章开头的元数据中设置：

```yaml
human_authored: false
```

设置 `human_authored: false` 会显示英文、简体中文各两张黑白版 AI 共创徽章，而不是隐藏徽章。设置 `human_authored: true` 或不填写此字段时，仍显示原有的人类创作徽章。请使用不带引号的布尔值。

四张徽章仅保存 SVG，文件名分别以 `-en.svg`、`-zh-CN.svg` 结尾，不参与构建时的 WebP 转换。英文版沿用仓库原有图形；简体中文版来自官网的 [白色版](https://notbyai.fyi/img/written-by-human-not-by-ai-white-cn.svg) 和 [黑色版](https://notbyai.fyi/img/written-by-human-not-by-ai-black-cn.svg)，未修改图形。桌面端并排展示，小屏自动换行，展示高度保持 42 像素。

AI 共创徽章基于 [Made by Human](https://github.com/JarlLyng/madebyhuman) 的 MIT 许可图形改编，边框和机器人图标经过重新设计，简体中文文字为新增版本。四张文件以 `Co-Created-With-AI-Badge-` 开头，仅使用 SVG，不参与 WebP 转换；显示高度统一为 42 像素，宽度按原始比例自适应。原始版权和 MIT 许可全文已保存在每张 SVG 的元数据中。

徽章是作者自我声明，并非独立认证。人类创作徽章的使用需符合 [Not By AI 官方许可](https://notbyai.fyi/license) 和人类创作比例规则；商业用途需另行确认授权。

祝你写作愉快！
