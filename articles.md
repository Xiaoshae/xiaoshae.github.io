---
layout: default
title: 文章
---

<div id="articles-zh">
  <h1>文章</h1>
  <ul class="posts noList">
    {% for post in site.posts %}
      {% unless post.categories contains 'cheatsheet' %}
        {% if post.lang != 'en' %}
          <li>
            <span class="date">{{ post.date | date: "%Y-%m-%d" }}</span>
            <a href="{{ post.url }}">{{ post.title }}</a>
          </li>
        {% endif %}
      {% endunless %}
    {% endfor %}
  </ul>
</div>
