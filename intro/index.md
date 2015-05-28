---
title: Index
nav: false
---

{% assign pages = site.intro | sort: 'title' | sort: 'weight' %}
{% for node in pages %}
{% if node.title != null and node.nav == true %}
<a class="sidebar-nav-item{% if page.url == node.url %} active{% endif %}" href="{{ site.baseurl }}{{ node.url }}">{{ node.title }}</a>
{% endif %}
{% endfor %}
