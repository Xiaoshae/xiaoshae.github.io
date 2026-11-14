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

祝你写作愉快！
