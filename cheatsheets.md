---
layout: default
title: 速查手册
---

<div id="cheatsheets">
  <h1>速查手册</h1>
  
  <ul class="posts noList">
    {% for post in site.categories.cheatsheet %}
      <li>
        <span class="date">{{ post.date | date: "%Y-%m-%d" }}</span>
        <a href="{{ post.url }}">{{ post.title }}</a>
      </li>
    {% endfor %}
  </ul>
</div>
