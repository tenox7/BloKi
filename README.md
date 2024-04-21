# BloKi - A minimalistic Blog and Wiki Platform

If you want an ultra small, simple, easy to use, high performance, micro blog/wiki platform, you're tired of, or afraid to even touch WordPress, MediaWiki, LAMP, this might be for you.

## Main features

- No PHP, Apache, Nginx, JavaScript and SQL.

- The UI, blog/wiki viewer and the admin interface will resemble WordPress and MediaWiki, but without the bloat. There are user editable themes based on simple .html/css template files. 

- Different HTML templates for modern, legacy and vintage web browsers. I see no reason why you shouldn't be able to read rendered markdown in Netscape, Mosaic or Lynx.

- There will be a web based admin interface with markdown editor. However the articles and media are stored on disk as files. Editable in any editor of your choice. Versioned by Git. In future it will support S3 buckets.

- The server software is a single file, statically linked, self contained binary. Runs on most modern OSes without any external dependencies, scripting runtimes, libraries, etc. Designed for Docker, Lambda, Cloud Run, etc.


## Current status

- Blog engine works if articles and media are created on a disk by hand
- Admin interface is coming up next

Development progress can be tracked on [dogfood blog](https://blog.tenox.net/)

## Legal
Copyright (c) 2024 by Antoni Sawicki
Licensed under Apache-2.0 license