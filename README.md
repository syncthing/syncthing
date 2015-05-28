# Syncthing Docs

This repo is the source behind http://docs.syncthing.net/.

# Editing

To edit the documentation you need a GitHub account. Once you have created one
and logged in, you can edit any page by navigating to the corresponding file
and clicking the edit (pen) icon. This will create what is called a "fork" and
a "pull request", which will be approved by one of the existing documentation
team members. Once you've made a contribution or two, you can be added to the
documentation team and perform edits without requiring approval.

In the long run, learning to use git and running
[Jekyll](http://jekyllrb.com/) on your computer is beneficial.

# Structure

The documentation is divided into an index page (`index.md`) and various subsections. The sections are:

 - Introductory information in `_intro`.
 - Information for users in `_users`.
 - Information for developers in `_dev`.

Each page has what is called a "front matter", which looks something like:

 ```
 ---
 title: Getting Started
 weight: 0
 ---
 ```

 This sets, at minimum the title of the page. There are various other attributes that can be added, the most common ones being `weight` (to adjust the order of pages in the index; lower number gets sorted higher up) and `nav` (set `nav: false` to have a page not be listed in the index).

The rest of the page is in [Markdown format](https://help.github.com/articles/github-flavored-markdown/).
